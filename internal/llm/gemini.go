package llm

import (
	"context"
	"encoding/json"
	"fmt"

	"google.golang.org/genai"
)

// GeminiProvider implements LLMProvider using the Google Gemini API.
type GeminiProvider struct {
	client *genai.Client
}

// NewGeminiProvider creates a new Gemini LLM provider.
func NewGeminiProvider(ctx context.Context, apiKey string) (*GeminiProvider, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("create gemini client: %w", err)
	}
	return &GeminiProvider{client: client}, nil
}

func (p *GeminiProvider) Models() []ModelInfo {
	return []ModelInfo{
		{ID: "gemini-2.5-flash", Name: "Gemini 2.5 Flash", Provider: "gemini"},
		{ID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro", Provider: "gemini"},
		{ID: "gemini-2.0-flash", Name: "Gemini 2.0 Flash", Provider: "gemini"},
	}
}

func (p *GeminiProvider) ChatStream(ctx context.Context, req ChatRequest) (<-chan ChatEvent, error) {
	// Build a map from tool call ID to function name for resolving tool results.
	toolIDToName := make(map[string]string)
	for _, m := range req.Messages {
		for _, tc := range m.ToolCalls {
			toolIDToName[tc.ID] = tc.Name
		}
	}

	// Build contents
	var contents []*genai.Content
	for _, m := range req.Messages {
		switch m.Role {
		case "user":
			if m.ToolCallID != "" {
				// Tool result — send as function response
				funcName := m.ToolCallID
				if name, ok := toolIDToName[m.ToolCallID]; ok {
					funcName = name
				}
				var response map[string]any
				if m.IsError {
					response = map[string]any{"error": m.Content}
				} else {
					response = map[string]any{"output": m.Content}
				}
				contents = append(contents, &genai.Content{
					Role: "user",
					Parts: []*genai.Part{
						{FunctionResponse: &genai.FunctionResponse{
							Name:     funcName,
							ID:       m.ToolCallID,
							Response: response,
						}},
					},
				})
			} else {
				var parts []*genai.Part
				for _, img := range m.Images {
					parts = append(parts, &genai.Part{
						InlineData: &genai.Blob{
							Data:     img.Data,
							MIMEType: img.MimeType,
						},
					})
				}
				if m.Content != "" {
					parts = append(parts, genai.NewPartFromText(m.Content))
				}
				contents = append(contents, &genai.Content{
					Role:  "user",
					Parts: parts,
				})
			}
		case "assistant":
			var parts []*genai.Part
			if m.Content != "" {
				parts = append(parts, genai.NewPartFromText(m.Content))
			}
			for _, tc := range m.ToolCalls {
				var args map[string]any
				if err := json.Unmarshal(tc.Input, &args); err != nil {
					args = map[string]any{}
				}
				parts = append(parts, &genai.Part{
					FunctionCall: &genai.FunctionCall{
						ID:   tc.ID,
						Name: tc.Name,
						Args: args,
					},
				})
			}
			if len(parts) > 0 {
				contents = append(contents, &genai.Content{
					Role:  "model",
					Parts: parts,
				})
			}
		}
	}

	// Build tool declarations
	var tools []*genai.Tool
	if len(req.Tools) > 0 {
		var decls []*genai.FunctionDeclaration
		for _, t := range req.Tools {
			var schema any
			if err := json.Unmarshal(t.Parameters, &schema); err != nil {
				schema = map[string]any{"type": "object", "properties": map[string]any{}}
			}
			decls = append(decls, &genai.FunctionDeclaration{
				Name:                 t.Name,
				Description:          t.Description,
				ParametersJsonSchema: schema,
			})
		}
		tools = append(tools, &genai.Tool{FunctionDeclarations: decls})
	}

	// Build config
	model := req.Model
	if model == "" {
		model = "gemini-2.5-flash"
	}

	config := &genai.GenerateContentConfig{}

	if req.SystemPrompt != "" {
		config.SystemInstruction = genai.NewContentFromText(req.SystemPrompt, "user")
	}

	if req.MaxTokens > 0 {
		config.MaxOutputTokens = int32(req.MaxTokens)
	}

	if req.Temperature > 0 {
		temp := float32(req.Temperature)
		config.Temperature = &temp
	}

	if len(tools) > 0 {
		config.Tools = tools
	}

	// Stream responses
	events := make(chan ChatEvent, 100)

	go func() {
		defer close(events)

		for resp, err := range p.client.Models.GenerateContentStream(ctx, model, contents, config) {
			if err != nil {
				events <- ChatEvent{Type: EventError, Error: err}
				return
			}

			if resp.UsageMetadata != nil {
				events <- ChatEvent{
					Type: EventDone,
					Usage: &Usage{
						InputTokens:  int(resp.UsageMetadata.PromptTokenCount),
						OutputTokens: int(resp.UsageMetadata.CandidatesTokenCount),
					},
				}
			}

			for _, cand := range resp.Candidates {
				if cand.Content == nil {
					continue
				}
				for _, part := range cand.Content.Parts {
					if part.Text != "" {
						events <- ChatEvent{
							Type: EventTextDelta,
							Text: part.Text,
						}
					}
					if part.FunctionCall != nil {
						fc := part.FunctionCall
						argsJSON, err := json.Marshal(fc.Args)
						if err != nil {
							argsJSON = []byte("{}")
						}
						id := fc.ID
						if id == "" {
							id = fc.Name
						}
						events <- ChatEvent{
							Type: EventToolCallStart,
							ToolCall: &ToolCall{
								ID:   id,
								Name: fc.Name,
							},
						}
						events <- ChatEvent{
							Type: EventToolCallDone,
							ToolCall: &ToolCall{
								ID:    id,
								Name:  fc.Name,
								Input: json.RawMessage(argsJSON),
							},
						}
					}
				}
			}
		}

		// Ensure a Done event is always sent
		events <- ChatEvent{Type: EventDone}
	}()

	return events, nil
}

