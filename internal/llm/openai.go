package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	openai "github.com/sashabaranov/go-openai"
)

// OpenAIProvider implements LLMProvider using the OpenAI Chat Completions API.
type OpenAIProvider struct {
	client *openai.Client
}

// NewOpenAIProvider creates a new OpenAI LLM provider.
// If baseURL is non-empty, the client points to that endpoint (e.g. LiteLLM).
func NewOpenAIProvider(apiKey, baseURL string) *OpenAIProvider {
	cfg := openai.DefaultConfig(apiKey)
	if baseURL != "" {
		cfg.BaseURL = baseURL
	}
	client := openai.NewClientWithConfig(cfg)
	return &OpenAIProvider{client: client}
}

func (p *OpenAIProvider) Models() []ModelInfo {
	return []ModelInfo{
		{ID: "gpt-4o", Name: "GPT-4o", Provider: "openai"},
		{ID: "gpt-4o-mini", Name: "GPT-4o Mini", Provider: "openai"},
		{ID: "gpt-4-turbo", Name: "GPT-4 Turbo", Provider: "openai"},
	}
}

func (p *OpenAIProvider) ChatStream(ctx context.Context, req ChatRequest) (<-chan ChatEvent, error) {
	// Build messages
	msgs := make([]openai.ChatCompletionMessage, 0, len(req.Messages)+1)

	if req.SystemPrompt != "" {
		msgs = append(msgs, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: req.SystemPrompt,
		})
	}

	for _, m := range req.Messages {
		switch m.Role {
		case "user":
			if m.ToolCallID != "" {
				msgs = append(msgs, openai.ChatCompletionMessage{
					Role:       openai.ChatMessageRoleTool,
					Content:    m.Content,
					ToolCallID: m.ToolCallID,
				})
			} else {
				msgs = append(msgs, openai.ChatCompletionMessage{
					Role:    openai.ChatMessageRoleUser,
					Content: m.Content,
				})
			}
		case "assistant":
			msg := openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleAssistant,
				Content: m.Content,
			}
			for _, tc := range m.ToolCalls {
				msg.ToolCalls = append(msg.ToolCalls, openai.ToolCall{
					ID:   tc.ID,
					Type: openai.ToolTypeFunction,
					Function: openai.FunctionCall{
						Name:      tc.Name,
						Arguments: string(tc.Input),
					},
				})
			}
			msgs = append(msgs, msg)
		}
	}

	// Build tools
	var tools []openai.Tool
	for _, t := range req.Tools {
		var params any
		if err := json.Unmarshal(t.Parameters, &params); err != nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}

		tools = append(tools, openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  params,
			},
		})
	}

	model := req.Model
	if model == "" {
		model = "gpt-4o"
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	openaiReq := openai.ChatCompletionRequest{
		Model:     model,
		Messages:  msgs,
		MaxTokens: maxTokens,
		Stream:    true,
	}

	if len(tools) > 0 {
		openaiReq.Tools = tools
	}

	if req.Temperature > 0 {
		openaiReq.Temperature = float32(req.Temperature)
	}

	stream, err := p.client.CreateChatCompletionStream(ctx, openaiReq)
	if err != nil {
		return nil, err
	}

	events := make(chan ChatEvent, 100)

	go func() {
		defer close(events)
		defer stream.Close()

		// Track tool calls being built up across deltas
		type pendingTC struct {
			id       string
			name     string
			argsJSON string
		}
		toolCalls := make(map[int]*pendingTC)

		for {
			resp, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				events <- ChatEvent{Type: EventError, Error: err}
				return
			}

			for _, choice := range resp.Choices {
				delta := choice.Delta

				// Text content
				if delta.Content != "" {
					events <- ChatEvent{
						Type: EventTextDelta,
						Text: delta.Content,
					}
				}

				// Tool calls
				for _, tc := range delta.ToolCalls {
					idx := 0
					if tc.Index != nil {
						idx = *tc.Index
					}
					pending, exists := toolCalls[idx]
					if !exists {
						pending = &pendingTC{}
						toolCalls[idx] = pending
					}

					if tc.ID != "" {
						pending.id = tc.ID
					}
					if tc.Function.Name != "" {
						pending.name = tc.Function.Name
						events <- ChatEvent{
							Type: EventToolCallStart,
							ToolCall: &ToolCall{
								ID:   pending.id,
								Name: pending.name,
							},
						}
					}
					if tc.Function.Arguments != "" {
						pending.argsJSON += tc.Function.Arguments
					}
				}

				// Finish reason
				if choice.FinishReason == openai.FinishReasonToolCalls || choice.FinishReason == openai.FinishReasonStop {
					// Emit completed tool calls
					for _, tc := range toolCalls {
						if tc.name != "" {
							events <- ChatEvent{
								Type: EventToolCallDone,
								ToolCall: &ToolCall{
									ID:    tc.id,
									Name:  tc.name,
									Input: json.RawMessage(tc.argsJSON),
								},
							}
						}
					}
				}
			}
		}

		events <- ChatEvent{Type: EventDone}
	}()

	return events, nil
}
