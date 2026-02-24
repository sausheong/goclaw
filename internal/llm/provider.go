package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// EventType identifies the kind of streaming event from the LLM.
type EventType int

const (
	EventTextDelta EventType = iota
	EventToolCallStart
	EventToolCallDelta
	EventToolCallDone
	EventDone
	EventError
)

// Message represents a conversation message.
type Message struct {
	Role       string     `json:"role"` // "user", "assistant", "system"
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"` // for tool results
	IsError    bool       `json:"is_error,omitempty"`     // for tool results
}

// ToolCall represents a tool invocation requested by the LLM.
type ToolCall struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// ToolDef defines a tool for the LLM.
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"` // JSON Schema
}

// ChatRequest is the input to a streaming chat call.
type ChatRequest struct {
	Model        string
	Messages     []Message
	Tools        []ToolDef
	MaxTokens    int
	Temperature  float64
	SystemPrompt string
}

// Usage tracks token usage.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// ChatEvent is a single streaming event from the LLM.
type ChatEvent struct {
	Type     EventType
	Text     string
	ToolCall *ToolCall
	Usage    *Usage
	Error    error
}

// ModelInfo describes an available model.
type ModelInfo struct {
	ID       string
	Name     string
	Provider string
}

// LLMProvider is the interface for all LLM backends.
type LLMProvider interface {
	ChatStream(ctx context.Context, req ChatRequest) (<-chan ChatEvent, error)
	Models() []ModelInfo
}

// ParseProviderModel splits "provider/model" into (provider, model).
// If no slash is present, returns ("", name) and the caller should use a default.
func ParseProviderModel(name string) (provider, model string) {
	parts := strings.SplitN(name, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", name
}

// ProviderOptions holds connection details for creating an LLM provider.
type ProviderOptions struct {
	APIKey  string
	BaseURL string
	Kind    string // override: "openai-compatible" uses OpenAI client with custom URL
}

// NewProvider creates an LLMProvider for the given provider name.
func NewProvider(providerName string, opts ProviderOptions) (LLMProvider, error) {
	// If Kind is set, use it to override the provider type.
	// This lets e.g. "anthropic" route through an OpenAI-compatible proxy like LiteLLM.
	kind := opts.Kind
	if kind == "" {
		// If a custom base URL is set, default to openai-compatible
		// since most proxies (LiteLLM, etc.) expose an OpenAI-compatible API.
		if opts.BaseURL != "" {
			kind = "openai-compatible"
		} else {
			kind = providerName
		}
	}

	switch kind {
	case "anthropic":
		return NewAnthropicProvider(opts.APIKey, opts.BaseURL), nil
	case "openai", "openai-compatible":
		return NewOpenAIProvider(opts.APIKey, opts.BaseURL), nil
	default:
		return nil, fmt.Errorf("unknown LLM provider kind: %q", kind)
	}
}
