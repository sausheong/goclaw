![GoClaw](goclaw.jpg)

# GoClaw

A self-hosted AI agent gateway written in Go. Single binary, low memory, fast startup.

GoClaw connects messaging channels (Telegram, WhatsApp, CLI) to LLMs (Claude, GPT, Gemini, DeepSeek, Ollama), enabling autonomous task execution on your own hardware. Inspired by [OpenClaw](https://github.com/openclaw/openclaw), rewritten in Go for single-binary deployment, sub-50MB memory, and <100ms startup.

---

## Features

- **Single binary** — no runtime dependencies, no Node.js, no npm. Download and run.
- **System tray app** — runs the gateway in the background with a tray icon, web chat, and one-click access to settings (macOS and Windows)
- **Three interfaces** — Telegram (mobile/remote), WhatsApp (personal messaging), CLI (local terminal)
- **Model-agnostic** — Claude, GPT, Gemini, DeepSeek, Ollama, LM Studio, or any OpenAI-compatible API
- **Multi-agent** — run multiple agents with different models, tools, and personas
- **Inter-agent delegation** — agents can delegate subtasks to other agents via the `ask_agent` tool
- **Persistent memory** — BM25 search over Markdown files, recalled automatically each turn
- **Skill system** — Markdown files with YAML frontmatter, selectively injected per-turn based on relevance
- **Heartbeat daemon** — proactive agent actions on a schedule via HEARTBEAT.md checklists
- **Cron jobs** — recurring prompts on configurable intervals, with pause/resume/remove management
- **Vision/image support** — send photos via Telegram, WhatsApp, or CLI and the LLM describes/analyzes them
- **Tool policies** — per-agent allow/deny lists for all ten tools
- **Session persistence** — append-only JSONL files with DAG structure and branching
- **Config hot-reload** — edit goclaw.json5 while running, changes apply immediately
- **WebSocket API** — JSON-RPC 2.0 control plane for programmatic access
- **Local-first** — all data lives on your filesystem, no external database

## Why Go?

| | OpenClaw (Node.js) | GoClaw (Go) |
|---|---|---|
| **Deployment** | Node.js 22+, npm, dependency install | Single static binary |
| **Memory** | ~150-400MB | ~20-50MB |
| **Startup** | 2-5 seconds | <100ms |
| **Cross-compile** | Per-platform npm rebuilds | `GOOS=linux GOARCH=arm64 go build` |
| **Concurrency** | Event loop + worker threads | Native goroutines |

---

## Quick Start

### Build

```bash
make build              # Build the CLI binary
make build-app          # Build the macOS menu bar app (GoClaw.app)
make build-app-windows  # Build the Windows system tray app (goclaw-app.exe)
```

### Setup

```bash
./goclaw onboard
```

The wizard walks you through choosing an LLM provider, entering your API key, and optionally setting up Telegram and/or WhatsApp.

### Chat

```bash
# Interactive CLI session (no gateway needed)
./goclaw chat

# Start the full gateway (enables Telegram, WhatsApp, WebSocket API)
./goclaw start

# Or launch the system tray app
open GoClaw.app              # macOS
goclaw-app.exe               # Windows
```

### Verify

```bash
./goclaw doctor
```

---

## CLI Commands

| Command | Description |
|---------|-------------|
| `goclaw onboard` | Interactive setup wizard |
| `goclaw start` | Start the gateway server |
| `goclaw start -c path/to/config.json5` | Start with a custom config |
| `goclaw chat` | Interactive CLI chat with the default agent |
| `goclaw chat myagent` | Chat with a specific agent |
| `goclaw chat -m openai/gpt-4o` | Chat with a model override |
| `goclaw status` | Query the running gateway for agent status |
| `goclaw doctor` | Run diagnostic checks |
| `goclaw version` | Print version and commit info |

---

## System Tray App

GoClaw ships a system tray app that runs the gateway as a background service. Supported on macOS and Windows.

### Build

```bash
make build-app          # macOS — produces GoClaw.app
make build-app-windows  # Windows — produces goclaw-app.exe
```

### Launch

- **macOS:** Double-click `GoClaw.app` or drag it to `/Applications`
- **Windows:** Double-click `goclaw-app.exe`

### Menu items

| Item | Action |
|------|--------|
| **Chat** | Opens a web-based chat interface in your default browser |
| **Jobs** | Opens the cron jobs dashboard (`/jobs`) showing active scheduled tasks |
| **Settings** | Opens `~/.goclaw/goclaw.json5` in your default editor |
| **Restart** | Restarts the gateway |
| **Quit** | Gracefully shuts down the gateway and exits |

### Web chat interface

The app serves a chat page at `http://localhost:18789/chat` (also accessible at `http://localhost:18789`). Features:

- Agent selector dropdown — switch between configured agents without leaving the page
- Streaming responses via WebSocket
- Light/dark mode toggle (persisted in browser)
- Inline tool call display with collapsible output
- Markdown rendering (headings, code blocks, tables, lists, horizontal rules, bold, italic, links)

### Environment variables

**macOS:** `.app` bundles don't inherit shell environment variables. GoClaw.app automatically loads your shell profile (`~/.zshrc`, `~/.bashrc`) at startup, so API keys set via `export ANTHROPIC_API_KEY=...` work as expected.

**Windows:** Set environment variables via System Settings or PowerShell:

```powershell
[System.Environment]::SetEnvironmentVariable("ANTHROPIC_API_KEY", "sk-ant-...", "User")
```

On both platforms, you can set API keys directly in the config file instead of using environment variables.

---

## Architecture

![Architecture](architecture.jpg)


Single-process, hub-and-spoke design. All components run in one binary.

### Core Components

- **Gateway Server** (`cmd/goclaw/`) — HTTP + WebSocket server on `:18789` using chi router + gorilla/websocket. Entry point for all CLI subcommands via cobra.
- **Channel Adapters** — Implement the `Channel` interface. Three adapters ship: Telegram (`go-telegram/bot`), WhatsApp (`whatsmeow`), and CLI (stdin/stdout). The interface is generic for future extensibility.
- **Agent Runtime** — The think-act loop: assemble context (identity + skills + memory + history), stream LLM response, execute tool calls with policy checks, loop until final text response.
- **LLM Client** — Abstracted behind `LLMProvider` interface with `ChatStream()` and `Embed()` methods. Providers: Anthropic (custom SSE), OpenAI (`sashabaranov/go-openai`), Google Gemini (`google/generative-ai-go`), Ollama (OpenAI-compatible HTTP).
- **Session Manager** — Append-only JSONL files with DAG structure. One file per session. Supports compaction when history exceeds context window.
- **Message Router** — Declarative bindings (JSON) map channel + account + peer to agent IDs. Priority: peer.id > peer.kind > accountId > channel > default.
- **Memory Manager** — BM25 text search over Markdown files in `~/.goclaw/memory/`.
- **Skill System** — Markdown files with YAML frontmatter, selectively injected per-turn based on relevance. Compatible with OpenClaw/Claude Code/Cursor skill format.
- **Heartbeat Daemon** — Background goroutine on configurable interval (default 30min), reads `HEARTBEAT.md`, sends to agent for proactive actions.
- **Cron Scheduler** — Recurring prompts on configurable intervals (e.g., "24h", "1h", "30m"). Supports pause, resume, remove, and schedule updates at runtime.
- **Config Manager** — JSON5 config at `~/.goclaw/goclaw.json5`, hot-reloaded via fsnotify.

### Key Interfaces

```go
// Channel — messaging platform adapter
type Channel interface {
    Name() string
    Connect(ctx context.Context) error
    Disconnect() error
    Send(ctx context.Context, msg OutboundMessage) error
    Receive() <-chan InboundMessage
    Status() ChannelStatus
}

// LLMProvider — model provider
type LLMProvider interface {
    ChatStream(ctx context.Context, req ChatRequest) (<-chan ChatEvent, error)
    Embed(ctx context.Context, texts []string) ([][]float32, error)
    Models() []ModelInfo
}

// Tool — executable tool
type Tool interface {
    Name() string
    Description() string
    Parameters() json.RawMessage
    Execute(ctx context.Context, input json.RawMessage) (ToolResult, error)
}
```

---

## Channels

### Telegram

Uses the Telegram Bot API. Create a bot via [@BotFather](https://t.me/BotFather), add the token to your config, and start the gateway. Supports polling and webhook modes, group chats with mention-only mode, and image vision (send a photo and the LLM will describe it).

### WhatsApp

Uses the WhatsApp Web multidevice protocol via [whatsmeow](https://github.com/tulir/whatsmeow). No Meta Business account, no webhooks, no public URL needed. QR code authentication on first connect, credentials persisted in SQLite for automatic reconnection. Supports image vision — send a photo and the LLM will analyze it.

### CLI

Interactive terminal chat with Markdown rendering. Available via `goclaw chat` without starting the full gateway. Supports image input — paste or drag-and-drop a file path to send images to the LLM for vision analysis.

---

## Configuration

All configuration lives in `~/.goclaw/goclaw.json5` (JSON5 format for comments and trailing commas).

### LLM Providers

GoClaw supports four provider kinds. Each provider is defined in the `providers` section of the config with a unique name, a `kind`, and connection details.

| Kind | Description | Requires |
|------|-------------|----------|
| `anthropic` | Anthropic's Claude API | `api_key` |
| `openai` | OpenAI's API (GPT models) | `api_key` |
| `gemini` | Google's Gemini API | `api_key` |
| `openai-compatible` | Any OpenAI-compatible API (Ollama, LM Studio, DeepSeek, LiteLLM, etc.) | `base_url`, optionally `api_key` |

**Standard providers (Anthropic, OpenAI):**

```json5
{
  "providers": {
    "anthropic": {
      "kind": "anthropic",
      "api_key": "sk-ant-api03-..."
    },
    "openai": {
      "kind": "openai",
      "api_key": "sk-proj-..."
    },
    "gemini": {
      "kind": "gemini",
      "api_key": "AIza..."
    }
  }
}
```

**Custom / OpenAI-compatible providers:**

Any service exposing an OpenAI-compatible API (e.g., `/v1/chat/completions`) works with kind `openai-compatible`. Set the `base_url` to the API root.

```json5
{
  "providers": {
    // Ollama — local models, no API key needed
    "ollama": {
      "kind": "openai-compatible",
      "base_url": "http://localhost:11434/v1"
    },

    // LM Studio — local models
    "lmstudio": {
      "kind": "openai-compatible",
      "base_url": "http://localhost:1234/v1"
    },

    // DeepSeek — cloud API with OpenAI-compatible endpoint
    "deepseek": {
      "kind": "openai-compatible",
      "api_key": "sk-...",
      "base_url": "https://api.deepseek.com/v1"
    },

    // LiteLLM — proxy for multiple providers
    "litellm": {
      "kind": "openai-compatible",
      "base_url": "http://localhost:4000/v1"
    }
  }
}
```

### Model references

Agents reference models as `provider/model-name`, where the provider name matches a key in the `providers` section:

```json5
"model": "anthropic/claude-sonnet-4-5-20250514"   // Anthropic Claude
"model": "openai/gpt-4o"                          // OpenAI GPT-4o
"model": "ollama/llama3"                           // Ollama local model
"model": "deepseek/deepseek-chat"                  // DeepSeek
"model": "gemini/gemini-2.5-flash"                 // Google Gemini
"model": "lmstudio/qwen2.5-coder-14b"             // LM Studio local model
```

### API keys via environment variables

API keys can be set via environment variables instead of the config file. Environment variables take precedence.

```bash
export ANTHROPIC_API_KEY="sk-ant-api03-..."
export OPENAI_API_KEY="sk-proj-..."
export DEEPSEEK_API_KEY="sk-..."
export GEMINI_API_KEY="AIza..."
```

The naming convention is `{PROVIDER}_API_KEY` (or `{PROVIDER}_AUTH_TOKEN`), and `{PROVIDER}_BASE_URL` for custom endpoints — where `{PROVIDER}` is the uppercased provider name from your config.

### Full config example

```json5
{
  "providers": {
    "anthropic": { "kind": "anthropic", "api_key": "sk-ant-..." },
    "openai":    { "kind": "openai",    "api_key": "sk-..." },
    "ollama":    { "kind": "openai-compatible", "base_url": "http://localhost:11434/v1" }
  },
  "agents": {
    "list": [
      {
        "id": "default",
        "name": "Assistant",
        "model": "anthropic/claude-sonnet-4-5-20250514",
        "workspace": "~/.goclaw/workspace-default",
        "system_prompt": "You are a helpful coding assistant.",  // optional: overrides IDENTITY.md
        "tools": {
          "allow": ["read_file", "write_file", "edit_file", "bash", "web_fetch", "web_search", "browser", "send_message", "cron", "ask_agent"]
        }
      }
    ]
  },
  "bindings": [
    { "agentId": "default", "match": { "channel": "cli" } },
    { "agentId": "default", "match": { "channel": "telegram" } },
    { "agentId": "default", "match": { "channel": "whatsapp" } }
  ],
  "channels": {
    "telegram": { "token": "123456:ABC...", "mode": "polling" },
    "whatsapp": { "db_path": "~/.goclaw/whatsapp.db" },
    "cli": { "enabled": true }
  },
  "memory": { "enabled": true },
  "heartbeat": { "enabled": true, "interval": "30m" },
  "security": {
    "execApprovals": { "level": "full" },
    "dmPolicy": { "unknownSenders": "ignore" },
    "groupPolicy": { "requireMention": true }
  }
}
```

---

## Tools

Ten built-in tools that agents can use:

| Tool | Description |
|------|-------------|
| `read_file` | Read file contents |
| `write_file` | Create or overwrite files |
| `edit_file` | Make targeted edits to existing files |
| `bash` | Execute shell commands (uses `bash` on macOS/Linux, `cmd.exe` on Windows) |
| `web_fetch` | Fetch a URL and return its content |
| `web_search` | Search the web |
| `browser` | Headless Chrome automation (navigate, click, type, screenshot, evaluate JS). All actions accept an optional `url` to navigate before acting |
| `send_message` | Send a message to a user/group on any connected channel |
| `cron` | Dynamically schedule, list, pause, resume, remove, and update recurring tasks |
| `ask_agent` | Delegate a task to another agent and get back the result |

Tool access is controlled per-agent via allow/deny policies.

---

## Data Directory

All state lives in `~/.goclaw/` (on Windows: `C:\Users\<you>\.goclaw\`) — no external database required.

```
~/.goclaw/
  goclaw.json5             # Configuration file
  sessions/                # Conversation history (JSONL)
  memory/entries/          # Memory entries (Markdown)
  skills/                  # Shared skills (SKILL.md files)
  workspace-default/       # Default agent workspace
    IDENTITY.md            # Agent identity/persona (fallback if no system_prompt in config)
    HEARTBEAT.md           # Heartbeat checklist
    skills/                # Agent-specific skills
  whatsapp.db              # WhatsApp device credentials (SQLite)
```

Everything is human-readable files you can inspect, edit, and version-control.

---

## WebSocket API

JSON-RPC 2.0 over WebSocket at `ws://127.0.0.1:18789/ws`.

| Method | Description |
|--------|-------------|
| `chat.send` | Send a message to an agent (streams response events) |
| `chat.abort` | Cancel the active response for this connection |
| `agent.status` | List all configured agents |
| `session.list` | List sessions |
| `session.history` | Load conversation history for an agent |
| `session.clear` | Clear an agent's session history |

HTTP endpoints: `GET /health` (health check), `GET /ws` (WebSocket), `GET /metrics` (Prometheus metrics), `GET /ui` (control panel), `GET /chat` (web chat interface), `GET /jobs` (cron jobs dashboard).

---

## Security

GoClaw is designed to run on your own hardware. The following measures protect your system, credentials, and data.

### Network & Transport

- **Localhost-only by default** — the gateway binds to `127.0.0.1:18789`, never exposed to the network unless you change the config
- **Bearer token auth** — optional token protects all HTTP and WebSocket endpoints; uses constant-time comparison to prevent timing attacks
- **WebSocket origin checking** — only connections from localhost origins are accepted by default; configurable allowlist for custom origins
- **ReadHeaderTimeout** — 5-second header timeout defends against slowloris attacks
- **Security headers** — the web chat page sets `X-Frame-Options: DENY`, `Content-Security-Policy`, and `X-Content-Type-Options: nosniff` to prevent clickjacking and XSS

### Tool Execution

- **Tool policies** — per-agent allow/deny lists control which tools each agent can use
- **Exec approval policy** — three levels for the bash tool:
  - `deny` — all shell execution blocked
  - `allowlist` — only commands in the allowlist can run; shell metacharacters (`$(...)`, backticks, process substitution) are blocked to prevent bypasses
  - `full` — unrestricted (default)
- **Workspace containment** — file tools (`read_file`, `write_file`, `edit_file`) validate paths against the agent's workspace directory with symlink resolution to prevent path traversal

### Input Validation

- **SSRF protection** — `web_fetch` and `browser` tools resolve hostnames and block private IP ranges (RFC 1918, loopback, link-local, IPv6 ULA) and cloud metadata endpoints. DNS resolution failures are blocked (fail-closed). Redirect targets are re-validated at each hop to prevent redirect-based SSRF bypasses
- **XSS prevention** — the web chat UI escapes HTML before applying markdown formatting, and blocks `javascript:`, `data:`, and `vbscript:` URL schemes in rendered links
- **WebSocket rate limiting** — per-connection token bucket (30 messages/sec) prevents message flooding
- **WebSocket message size limit** — 1MB max message size prevents memory exhaustion from oversized payloads

### Credentials & Data

- **No hardcoded secrets** — all API keys and tokens come from config or environment variables
- **Config file permissions** — the `onboard` command writes config with `0o600` (owner-only) to protect API keys and bot tokens. At startup, a warning is logged if the config file is readable by group or others
- **Session file permissions** — conversation history files use `0o600` (owner-only)
- **DEBUG-level tool logging** — tool inputs and outputs (which may contain sensitive data) are logged at DEBUG, not INFO, so they don't appear in production logs
- **API keys via environment** — credentials can be set as `{PROVIDER}_API_KEY` environment variables to keep them out of config files entirely

### Channel Access Control

- **DM policy** — control how agents respond to unknown senders on Telegram/WhatsApp:
  - `ignore` — silently drop messages from unknown users (default)
  - `notify` — log but drop (useful for discovering user IDs)
  - `respond` — process all messages
- **Peer bindings** — route specific Telegram/WhatsApp user IDs to specific agents, combined with `ignore` policy to whitelist users
- **Group policy** — `requireMention: true` makes agents respond only when @mentioned in group chats
- **WhatsApp allowed senders** — optional allowlist of phone numbers/JIDs

---

## Development

```bash
make build                  # Build the CLI binary
make build-app              # Build the macOS menu bar app (GoClaw.app)
make build-app-windows      # Build the Windows system tray app (goclaw-app.exe)
make test                   # Run all tests
make test-race              # Run tests with race detector
make lint                   # Run golangci-lint
make fmt                    # Format source files
make tidy                   # Tidy module dependencies
make release TAG=v0.1.4     # Commit, push, create GitHub release, and build cross-platform binaries
make build-release          # Cross-compile binaries without creating a GitHub release
make snapshot               # Cross-platform build via goreleaser
make help                   # Show all targets
```

### Dependencies

| Purpose | Package |
|---------|---------|
| HTTP router | `github.com/go-chi/chi/v5` |
| WebSocket | `github.com/gorilla/websocket` |
| CLI framework | `github.com/spf13/cobra` |
| Telegram | `github.com/go-telegram/bot` |
| WhatsApp | `go.mau.fi/whatsmeow` |
| OpenAI client | `github.com/sashabaranov/go-openai` |
| Gemini client | `google.golang.org/genai` |
| Vector DB | `github.com/philippgille/chromem-go` |
| File watching | `github.com/fsnotify/fsnotify` |
| Browser automation | `github.com/chromedp/chromedp` |
| System tray | `fyne.io/systray` |
| QR code display | `github.com/mdp/qrterminal` |
| SQLite (pure Go) | `modernc.org/sqlite` |
| Testing | `github.com/stretchr/testify` |
| Logging | `log/slog` (stdlib) |

### Testing

```bash
go test ./...              # Run all tests
go test -cover ./...       # Run tests with per-package coverage
go test -race ./...        # Run tests with race detector
```

Per-package test coverage:

| Package | Coverage |
|---------|----------|
| `internal/memory` | 89.2% |
| `internal/heartbeat` | 88.6% |
| `internal/skill` | 86.6% |
| `internal/cron` | 85.7% |
| `internal/session` | 84.6% |
| `internal/agent` | 82.1% |
| `internal/router` | 77.8% |
| `internal/config` | 73.9% |
| `internal/gateway` | 56.8% |
| `internal/tools` | 44.4% |
| `internal/channel` | 15.0% |
| `internal/llm` | 14.5% |

---

## Feature Comparison with OpenClaw

| Feature | OpenClaw | GoClaw |
|---------|----------|--------|
| Gateway (WebSocket control plane) | Yes | Yes |
| Telegram channel | Yes | Yes |
| WhatsApp channel | Yes | Yes |
| CLI / Terminal channel | Yes | Yes |
| Other channels (Discord, Slack, etc.) | Yes (15+) | No (adapter interface allows future addition) |
| Agent loop with tool calling | Yes (via Pi SDK) | Yes (native Go) |
| Session persistence (JSONL DAG) | Yes | Yes |
| Multi-agent routing | Yes | Yes |
| Skill system (Markdown format) | Yes | Yes (format-compatible) |
| Persistent memory | Yes | Yes |
| Heartbeat daemon | Yes | Yes |
| Cron scheduling | Yes | Yes |
| Config hot-reload | Yes | Yes |
| Inter-agent delegation | No | Yes |
| Tool policies | Yes | Yes |
| Docker sandboxing | Yes | Yes |
| Browser automation (CDP) | Yes | Yes |
| Control UI | Yes | Yes |
| Canvas / A2UI | Yes | No |
| Voice (TTS/STT) | Yes | No |
| Plugin system | Yes (TypeScript) | Planned (Go/Wasm) |
| **Single-binary deployment** | No (Node.js required) | **Yes** |
| **Sub-50MB memory** | No (~150-400MB) | **Yes** |
| **<100ms cold start** | No (2-5s) | **Yes** |

---

## Documentation

- [How to Use GoClaw](howtouse.md) — detailed examples, use cases, and example configurations
