package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseProviderModel(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantProvider string
		wantModel    string
	}{
		{
			name:         "provider and model",
			input:        "anthropic/claude-sonnet",
			wantProvider: "anthropic",
			wantModel:    "claude-sonnet",
		},
		{
			name:         "model only",
			input:        "model-only",
			wantProvider: "",
			wantModel:    "model-only",
		},
		{
			name:         "empty string",
			input:        "",
			wantProvider: "",
			wantModel:    "",
		},
		{
			name:         "multiple slashes",
			input:        "openai/gpt-4/turbo",
			wantProvider: "openai",
			wantModel:    "gpt-4/turbo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, model := ParseProviderModel(tt.input)
			assert.Equal(t, tt.wantProvider, provider)
			assert.Equal(t, tt.wantModel, model)
		})
	}
}

func TestNewProviderAnthropic(t *testing.T) {
	p, err := NewProvider("anthropic", ProviderOptions{APIKey: "test-key"})
	require.NoError(t, err)
	assert.NotNil(t, p)
	_, ok := p.(*AnthropicProvider)
	assert.True(t, ok, "expected *AnthropicProvider")
}

func TestNewProviderOpenAI(t *testing.T) {
	p, err := NewProvider("openai", ProviderOptions{APIKey: "test-key"})
	require.NoError(t, err)
	assert.NotNil(t, p)
	_, ok := p.(*OpenAIProvider)
	assert.True(t, ok, "expected *OpenAIProvider")
}

func TestNewProviderOpenAICompatible(t *testing.T) {
	p, err := NewProvider("openai-compatible", ProviderOptions{
		APIKey:  "test-key",
		BaseURL: "http://localhost:8080/v1",
		Kind:    "openai-compatible",
	})
	require.NoError(t, err)
	assert.NotNil(t, p)
	_, ok := p.(*OpenAIProvider)
	assert.True(t, ok, "expected *OpenAIProvider for openai-compatible")
}

func TestNewProviderUnknown(t *testing.T) {
	p, err := NewProvider("unknown-provider", ProviderOptions{})
	assert.Nil(t, p)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown LLM provider kind")
}

func TestNewProviderBaseURLDefault(t *testing.T) {
	// When kind is empty but base URL is set, should default to openai-compatible
	p, err := NewProvider("anything", ProviderOptions{
		APIKey:  "test-key",
		BaseURL: "http://localhost:11434/v1",
	})
	require.NoError(t, err)
	assert.NotNil(t, p)
	_, ok := p.(*OpenAIProvider)
	assert.True(t, ok, "expected *OpenAIProvider when base URL is set")
}

func TestAnthropicProviderModels(t *testing.T) {
	p := NewAnthropicProvider("test-key", "")
	models := p.Models()
	assert.NotEmpty(t, models)
	for _, m := range models {
		assert.NotEmpty(t, m.ID)
		assert.NotEmpty(t, m.Name)
		assert.Equal(t, "anthropic", m.Provider)
	}
}

func TestOpenAIProviderModels(t *testing.T) {
	p := NewOpenAIProvider("test-key", "")
	models := p.Models()
	assert.NotEmpty(t, models)
	for _, m := range models {
		assert.NotEmpty(t, m.ID)
		assert.NotEmpty(t, m.Name)
		assert.Equal(t, "openai", m.Provider)
	}
}
