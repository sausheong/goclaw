package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Config is the top-level GoClaw configuration.
type Config struct {
	Gateway   GatewayConfig            `json:"gateway"`
	Providers map[string]ProviderConfig `json:"providers"`
	Agents    AgentsConfig             `json:"agents"`
	Bindings  []Binding                `json:"bindings"`
	Channels  ChannelsConfig           `json:"channels"`
	Heartbeat HeartbeatConfig          `json:"heartbeat"`
	Memory    MemoryConfig             `json:"memory"`
	Security  SecurityConfig           `json:"security"`

	mu   sync.RWMutex
	path string
}

// ProviderConfig holds connection details for an LLM provider.
type ProviderConfig struct {
	Kind    string `json:"kind"`     // "openai", "anthropic", "openai-compatible"
	BaseURL string `json:"base_url"` // custom API endpoint (e.g. LiteLLM)
	APIKey  string `json:"api_key"`  // API key or auth token
}

type GatewayConfig struct {
	Host   string     `json:"host"`
	Port   int        `json:"port"`
	Auth   AuthConfig `json:"auth"`
	Reload ReloadConfig `json:"reload"`
}

type AuthConfig struct {
	Token string `json:"token"`
}

type ReloadConfig struct {
	Mode string `json:"mode"` // "hybrid", "manual", "auto-restart"
}

type AgentsConfig struct {
	List []AgentConfig `json:"list"`
}

type AgentConfig struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Workspace    string       `json:"workspace"`
	Model        string       `json:"model"`
	Fallbacks    []string     `json:"fallbacks"`
	Sandbox      string       `json:"sandbox"`                    // "none", "docker", "namespace"
	MaxTurns     int          `json:"maxTurns,omitempty"`         // max tool-use loop iterations (0 = default 25)
	SystemPrompt string       `json:"system_prompt,omitempty"`    // inline system prompt (overrides IDENTITY.md)
	Tools        ToolPolicy   `json:"tools"`
	Cron         []CronConfig `json:"cron,omitempty"`
}

type CronConfig struct {
	Name     string `json:"name"`
	Schedule string `json:"schedule"` // duration string: "30m", "1h", "24h"
	Prompt   string `json:"prompt"`
}

type ToolPolicy struct {
	Allow []string `json:"allow"`
	Deny  []string `json:"deny"`
}

type Binding struct {
	AgentID string       `json:"agentId"`
	Match   BindingMatch `json:"match"`
}

type BindingMatch struct {
	Channel   string     `json:"channel,omitempty"`
	AccountID string     `json:"accountId,omitempty"`
	ChatType  string     `json:"chatType,omitempty"`
	Peer      *PeerMatch `json:"peer,omitempty"`
}

type PeerMatch struct {
	ID   string `json:"id,omitempty"`
	Kind string `json:"kind,omitempty"`
}

type ChannelsConfig struct {
	Telegram TelegramConfig `json:"telegram"`
	WhatsApp WhatsAppConfig `json:"whatsapp"`
	CLI      CLIConfig      `json:"cli"`
}

type WhatsAppConfig struct {
	PhoneNumber    string   `json:"phone_number"`    // for display/identification only
	DBPath         string   `json:"db_path"`          // SQLite path for device state (default: ~/.goclaw/whatsapp.db)
	AllowedSenders []string `json:"allowed_senders"`  // phone numbers or JIDs allowed to send messages (empty = allow all)
}

type TelegramConfig struct {
	Token string `json:"token"`
	Mode  string `json:"mode"` // "polling" or "webhook"
}

type CLIConfig struct {
	Enabled     bool `json:"enabled"`
	Interactive bool `json:"interactive"`
}

type HeartbeatConfig struct {
	Interval string `json:"interval"`
	Enabled  bool   `json:"enabled"`
}

type MemoryConfig struct {
	Enabled           bool   `json:"enabled"`
	EmbeddingProvider string `json:"embeddingProvider"`
	EmbeddingModel    string `json:"embeddingModel"`
	MaxEntries        int    `json:"maxEntries"`
}

type SecurityConfig struct {
	ExecApprovals ExecApprovalsConfig `json:"execApprovals"`
	DMPolicy      DMPolicyConfig      `json:"dmPolicy"`
	GroupPolicy   GroupPolicyConfig    `json:"groupPolicy"`
}

type ExecApprovalsConfig struct {
	Level     string   `json:"level"` // "deny", "allowlist", "full"
	Allowlist []string `json:"allowlist"`
}

type DMPolicyConfig struct {
	UnknownSenders string `json:"unknownSenders"` // "ignore", "respond", "notify"
}

type GroupPolicyConfig struct {
	RequireMention bool `json:"requireMention"`
}

// DefaultDataDir returns the default GoClaw data directory.
func DefaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".goclaw"
	}
	return filepath.Join(home, ".goclaw")
}

// DefaultConfigPath returns the default config file path.
func DefaultConfigPath() string {
	return filepath.Join(DefaultDataDir(), "goclaw.json5")
}

// Load reads and parses a GoClaw config file. It supports JSON5 by
// stripping comments and trailing commas before unmarshalling.
func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultConfigPath()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := DefaultConfig()
			cfg.path = path
			return cfg, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	// Warn if config file is readable by group or others (may expose API keys)
	if info, statErr := os.Stat(path); statErr == nil {
		mode := info.Mode().Perm()
		if mode&0o077 != 0 {
			slog.Warn("config file has overly permissive permissions",
				"path", path,
				"mode", fmt.Sprintf("%04o", mode),
				"recommended", "0600",
				"fix", fmt.Sprintf("chmod 600 %s", path),
			)
		}
	}

	// Strip JSON5 features (single-line comments, trailing commas) for stdlib JSON parsing.
	cleaned := stripJSON5(string(data))

	var cfg Config
	if err := json.Unmarshal([]byte(cleaned), &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	cfg.path = path

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return &cfg, nil
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Gateway: GatewayConfig{
			Host: "127.0.0.1",
			Port: 18789,
			Reload: ReloadConfig{Mode: "hybrid"},
		},
		Providers: map[string]ProviderConfig{},
		Agents: AgentsConfig{
			List: []AgentConfig{
				{
					ID:        "default",
					Name:      "Assistant",
					Workspace: filepath.Join(DefaultDataDir(), "workspace-default"),
					Model:     "anthropic/claude-sonnet-4-5-20250514",
					Sandbox:   "none",
					Tools: ToolPolicy{
						Allow: []string{"read_file", "write_file", "edit_file", "bash", "web_fetch", "web_search", "browser", "send_message", "cron"},
					},
				},
			},
		},
		Bindings: []Binding{
			{AgentID: "default", Match: BindingMatch{Channel: "cli"}},
		},
		Channels: ChannelsConfig{
			CLI: CLIConfig{Enabled: true, Interactive: true},
		},
		Heartbeat: HeartbeatConfig{
			Interval: "30m",
			Enabled:  false,
		},
		Security: SecurityConfig{
			ExecApprovals: ExecApprovalsConfig{
				Level:     "full",
				Allowlist: []string{"ls", "cat", "find", "grep", "head", "tail", "wc", "pwd", "date"},
			},
			DMPolicy:    DMPolicyConfig{UnknownSenders: "ignore"},
			GroupPolicy: GroupPolicyConfig{RequireMention: true},
		},
	}
}

// GetProvider returns the provider config for the given name, falling back to
// env vars if not explicitly configured.
func (c *Config) GetProvider(name string) ProviderConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if p, ok := c.Providers[name]; ok {
		return p
	}
	return ProviderConfig{}
}

// Validate checks the config for required fields and applies defaults.
func (c *Config) Validate() error {
	if c.Gateway.Port == 0 {
		c.Gateway.Port = 18789
	}
	if c.Gateway.Host == "" {
		c.Gateway.Host = "127.0.0.1"
	}
	if c.Gateway.Reload.Mode == "" {
		c.Gateway.Reload.Mode = "hybrid"
	}

	if len(c.Agents.List) == 0 {
		return errors.New("at least one agent must be configured")
	}

	for i := range c.Agents.List {
		a := &c.Agents.List[i]
		if a.ID == "" {
			return fmt.Errorf("agent at index %d has no id", i)
		}
		if a.Model == "" {
			return fmt.Errorf("agent %q has no model", a.ID)
		}
		if a.Workspace == "" {
			a.Workspace = filepath.Join(DefaultDataDir(), "workspace-"+a.ID)
		}
		if a.Sandbox == "" {
			a.Sandbox = "none"
		}
	}

	return nil
}

// GetAgent returns the agent config for the given ID.
func (c *Config) GetAgent(id string) (*AgentConfig, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for i := range c.Agents.List {
		if c.Agents.List[i].ID == id {
			return &c.Agents.List[i], true
		}
	}
	return nil, false
}

// Path returns the file path this config was loaded from.
func (c *Config) Path() string {
	return c.path
}

// stripJSON5 removes single-line comments and trailing commas from JSON5
// to produce valid JSON for the stdlib parser.
func stripJSON5(s string) string {
	var b strings.Builder
	lines := strings.Split(s, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip full-line comments
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		// Remove inline comments (naive: doesn't handle // inside strings,
		// but sufficient for typical config files)
		if idx := strings.Index(line, "//"); idx >= 0 {
			// Only strip if not inside a quoted string
			if !inString(line, idx) {
				line = line[:idx]
			}
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}

	// Remove trailing commas before } or ]
	result := b.String()
	result = removeTrailingCommas(result)
	return result
}

// inString checks if position pos in line is inside a JSON string literal.
func inString(line string, pos int) bool {
	inStr := false
	for i := 0; i < pos; i++ {
		if line[i] == '"' && (i == 0 || line[i-1] != '\\') {
			inStr = !inStr
		}
	}
	return inStr
}

// removeTrailingCommas removes commas that appear before } or ] (with optional whitespace).
func removeTrailingCommas(s string) string {
	runes := []rune(s)
	var out []rune
	for i := 0; i < len(runes); i++ {
		if runes[i] == ',' {
			// Look ahead past whitespace for } or ]
			j := i + 1
			for j < len(runes) && (runes[j] == ' ' || runes[j] == '\t' || runes[j] == '\n' || runes[j] == '\r') {
				j++
			}
			if j < len(runes) && (runes[j] == '}' || runes[j] == ']') {
				continue // skip this trailing comma
			}
		}
		out = append(out, runes[i])
	}
	return string(out)
}
