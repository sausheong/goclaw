package llm

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// AnthropicProvider implements LLMProvider using the Anthropic Messages API.
type AnthropicProvider struct {
	client anthropic.Client
}

// NewAnthropicProvider creates a new Anthropic LLM provider.
// If baseURL is non-empty, the client points to that endpoint.
func NewAnthropicProvider(apiKey, baseURL string) *AnthropicProvider {
	opts := []option.RequestOption{option.WithAPIKey(apiKey)}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	client := anthropic.NewClient(opts...)
	return &AnthropicProvider{client: client}
}

func (p *AnthropicProvider) Models() []ModelInfo {
	return []ModelInfo{
		{ID: "claude-sonnet-4-5-20250514", Name: "Claude Sonnet 4.5", Provider: "anthropic"},
		{ID: "claude-opus-4-0-20250514", Name: "Claude Opus 4", Provider: "anthropic"},
		{ID: "claude-haiku-3-5-20241022", Name: "Claude Haiku 3.5", Provider: "anthropic"},
	}
}

func (p *AnthropicProvider) ChatStream(ctx context.Context, req ChatRequest) (<-chan ChatEvent, error) {
	// Build messages
	msgs := make([]anthropic.MessageParam, 0, len(req.Messages))
	for _, m := range req.Messages {
		switch m.Role {
		case "user":
			if m.ToolCallID != "" {
				// This is a tool result message
				msgs = append(msgs, anthropic.NewUserMessage(
					anthropic.NewToolResultBlock(m.ToolCallID, m.Content, m.IsError),
				))
			} else if len(m.Images) > 0 {
				var blocks []anthropic.ContentBlockParamUnion
				for _, img := range m.Images {
					encoded := base64.StdEncoding.EncodeToString(img.Data)
					blocks = append(blocks, anthropic.NewImageBlockBase64(img.MimeType, encoded))
				}
				if m.Content != "" {
					blocks = append(blocks, anthropic.NewTextBlock(m.Content))
				}
				msgs = append(msgs, anthropic.NewUserMessage(blocks...))
			} else {
				msgs = append(msgs, anthropic.NewUserMessage(anthropic.NewTextBlock(m.Content)))
			}
		case "assistant":
			var blocks []anthropic.ContentBlockParamUnion
			if m.Content != "" {
				blocks = append(blocks, anthropic.NewTextBlock(m.Content))
			}
			for _, tc := range m.ToolCalls {
				var input any
				if err := json.Unmarshal(tc.Input, &input); err != nil {
					input = map[string]any{}
				}
				blocks = append(blocks, anthropic.NewToolUseBlock(tc.ID, input, tc.Name))
			}
			if len(blocks) > 0 {
				msgs = append(msgs, anthropic.NewAssistantMessage(blocks...))
			}
		}
	}

	// Build tools
	tools := make([]anthropic.ToolUnionParam, 0, len(req.Tools))
	for _, t := range req.Tools {
		var props any
		var required []string
		// Parse the JSON Schema to extract properties and required fields
		var schema struct {
			Properties any      `json:"properties"`
			Required   []string `json:"required"`
		}
		if err := json.Unmarshal(t.Parameters, &schema); err == nil {
			props = schema.Properties
			required = schema.Required
		}

		tools = append(tools, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        t.Name,
				Description: anthropic.String(t.Description),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: props,
					Required:   required,
				},
			},
		})
	}

	// Build params
	maxTokens := int64(req.MaxTokens)
	if maxTokens == 0 {
		maxTokens = 4096
	}

	model := req.Model
	if model == "" {
		model = "claude-sonnet-4-5-20250514"
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: maxTokens,
		Messages:  msgs,
	}

	if len(tools) > 0 {
		params.Tools = tools
	}

	if req.SystemPrompt != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: req.SystemPrompt},
		}
	}

	if req.Temperature > 0 {
		params.Temperature = anthropic.Float(req.Temperature)
	}

	// Create stream
	stream := p.client.Messages.NewStreaming(ctx, params)

	events := make(chan ChatEvent, 100)

	go func() {
		defer close(events)
		defer stream.Close()

		// Track tool calls being built up across events
		type pendingTC struct {
			id        string
			name      string
			inputJSON string
		}
		var pendingTools []pendingTC
		var currentBlockType string

		for stream.Next() {
			event := stream.Current()

			switch event.Type {
			case "content_block_start":
				cb := event.ContentBlock
				switch cb.Type {
				case "text":
					currentBlockType = "text"
				case "tool_use":
					currentBlockType = "tool_use"
					pendingTools = append(pendingTools, pendingTC{
						id:   cb.ID,
						name: cb.Name,
					})
					events <- ChatEvent{
						Type: EventToolCallStart,
						ToolCall: &ToolCall{
							ID:   cb.ID,
							Name: cb.Name,
						},
					}
				}

			case "content_block_delta":
				delta := event.Delta
				switch delta.Type {
				case "text_delta":
					events <- ChatEvent{
						Type: EventTextDelta,
						Text: delta.Text,
					}
				case "input_json_delta":
					if len(pendingTools) > 0 {
						pendingTools[len(pendingTools)-1].inputJSON += delta.PartialJSON
					}
				}

			case "content_block_stop":
				if currentBlockType == "tool_use" && len(pendingTools) > 0 {
					tc := pendingTools[len(pendingTools)-1]
					events <- ChatEvent{
						Type: EventToolCallDone,
						ToolCall: &ToolCall{
							ID:    tc.id,
							Name:  tc.name,
							Input: json.RawMessage(tc.inputJSON),
						},
					}
				}
				currentBlockType = ""

			case "message_delta":
				if event.Usage.OutputTokens > 0 {
					events <- ChatEvent{
						Type: EventDone,
						Usage: &Usage{
							OutputTokens: int(event.Usage.OutputTokens),
						},
					}
				}

			case "message_stop":
				// Final event — nothing extra to emit

			case "error":
				slog.Error("anthropic stream error", "event", event.Type)
			}
		}

		if err := stream.Err(); err != nil {
			events <- ChatEvent{
				Type:  EventError,
				Error: err,
			}
		}
	}()

	return events, nil
}
