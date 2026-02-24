package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	require.NotNil(t, cfg)

	assert.Equal(t, "127.0.0.1", cfg.Gateway.Host)
	assert.Equal(t, 18789, cfg.Gateway.Port)
	assert.Len(t, cfg.Agents.List, 1)
	assert.Equal(t, "default", cfg.Agents.List[0].ID)
	assert.Equal(t, "anthropic/claude-sonnet-4-5-20250514", cfg.Agents.List[0].Model)
}

func TestLoadMissingFile(t *testing.T) {
	cfg, err := Load("/nonexistent/path/goclaw.json5")
	require.NoError(t, err)
	assert.Equal(t, "default", cfg.Agents.List[0].ID)
}

func TestLoadJSON5(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "goclaw.json5")

	content := `{
  // This is a comment
  "gateway": {
    "host": "0.0.0.0",
    "port": 9999,
  },
  "agents": {
    "list": [
      {
        "id": "test",
        "name": "Test Agent",
        "model": "openai/gpt-4o",
      },
    ],
  },
}`

	err := os.WriteFile(path, []byte(content), 0o644)
	require.NoError(t, err)

	cfg, err := Load(path)
	require.NoError(t, err)

	assert.Equal(t, "0.0.0.0", cfg.Gateway.Host)
	assert.Equal(t, 9999, cfg.Gateway.Port)
	assert.Equal(t, "test", cfg.Agents.List[0].ID)
	assert.Equal(t, "openai/gpt-4o", cfg.Agents.List[0].Model)
}

func TestValidateNoAgents(t *testing.T) {
	cfg := &Config{}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least one agent")
}

func TestValidateNoModel(t *testing.T) {
	cfg := &Config{
		Agents: AgentsConfig{
			List: []AgentConfig{{ID: "x"}},
		},
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no model")
}

func TestGetAgent(t *testing.T) {
	cfg := DefaultConfig()

	a, ok := cfg.GetAgent("default")
	assert.True(t, ok)
	assert.Equal(t, "Assistant", a.Name)

	_, ok = cfg.GetAgent("nonexistent")
	assert.False(t, ok)
}

func TestStripJSON5(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "strip single-line comment",
			input: "// comment\n{\"key\": \"value\"}",
			want:  "{\"key\": \"value\"}\n",
		},
		{
			name:  "strip trailing comma before }",
			input: `{"key": "value",}`,
			want:  "{\"key\": \"value\"}\n",
		},
		{
			name:  "strip trailing comma before ]",
			input: `["a", "b",]`,
			want:  "[\"a\", \"b\"]\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripJSON5(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}
