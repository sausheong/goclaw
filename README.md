# GoClaw

A self-hosted AI agent gateway written in Go. Single binary, low memory, fast startup.

GoClaw connects messaging channels (Telegram, WhatsApp, CLI) to LLMs (Claude, GPT, Gemini, DeepSeek, Ollama), enabling autonomous task execution on your own hardware. Inspired by [OpenClaw](https://github.com/openclaw/openclaw), rewritten in Go for single-binary deployment, sub-50MB memory, and <100ms startup.

---

## Features

- **Single binary** — no runtime dependencies, no Node.js, no npm. Download and run.
- **Three interfaces** — Telegram (mobile/remote), WhatsApp (personal messaging), CLI (local terminal)
- **Model-agnostic** — Claude, GPT, Gemini, DeepSeek, Ollama, LM Studio, or any OpenAI-compatible API
- **Multi-agent** — run multiple agents with different models, tools, and personas
- **Persistent memory** — BM25 search over Markdown files, recalled automatically each turn
- **Skill system** — Markdown files with YAML frontmatter, selectively injected per-turn based on relevance
- **Heartbeat daemon** — proactive agent actions on a schedule via HEARTBEAT.md checklists
- **Cron jobs** — recurring prompts on configurable intervals
- **Tool policies** — per-agent allow/deny lists for read_file, write_file, edit_file, bash, web_fetch, web_search
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
make build
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

## Architecture

```
        Telegram / WhatsApp / CLI
                    |
                    v
      +---------------------------+
      |         Gateway           |
      |  (HTTP + WebSocket on     |
      |   127.0.0.1:18789)        |
      |                           |
      |  +---------------------+  |
      |  |  Message Router     |  |
      |  |  (Bindings Engine)  |  |
      |  +----------+----------+  |
      |             |              |
      |  +----------v----------+  |
      |  |  Session Manager    |  |
      |  |  (DAG-based JSONL)  |  |
      |  +----------+----------+  |
      |             |              |
      |  +----------v----------+  |
      |  |   Agent Runtime     |  |
      |  |                     |  |
      |  |  Context Assembler  |  |
      |  |  LLM Client         |  |
      |  |  Tool Executor      |  |
      |  |  Skill Loader       |  |
      |  +---------------------+  |
      |                           |
      |  +---------------------+  |
      |  |  Memory Manager     |  |
      |  |  (BM25 search)      |  |
      |  +---------------------+  |
      |                           |
      |  +---------------------+  |
      |  |  Heartbeat + Cron   |  |
      |  +---------------------+  |
      |                           |
      |  +---------------------+  |
      |  |  Config Manager     |  |
      |  |  (fsnotify watch)   |  |
      |  +---------------------+  |
      +---------------------------+
                    |
        +-----------+-----------+
        v           v           v
   ~/.goclaw/   LLM APIs     Docker
   (local fs)  (Anthropic,   (sandbox)
               OpenAI,
               Ollama...)
```

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
- **Cron Scheduler** — Recurring prompts on configurable intervals (e.g., "24h", "1h", "30m").
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

Uses the Telegram Bot API. Create a bot via [@BotFather](https://t.me/BotFather), add the token to your config, and start the gateway. Supports polling and webhook modes, group chats with mention-only mode, and media attachments.

### WhatsApp

Uses the WhatsApp Web multidevice protocol via [whatsmeow](https://github.com/tulir/whatsmeow). No Meta Business account, no webhooks, no public URL needed. QR code authentication on first connect, credentials persisted in SQLite for automatic reconnection.

### CLI

Interactive terminal chat with Markdown rendering. Available via `goclaw chat` without starting the full gateway.

---

## Configuration

All configuration lives in `~/.goclaw/goclaw.json5` (JSON5 format for comments and trailing commas).

### Example

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
        "tools": {
          "allow": ["read_file", "write_file", "edit_file", "bash", "web_fetch", "web_search"]
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

API keys can also be set via environment variables (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, etc.), which take precedence over config values.

---

## Tools

Nine built-in tools that agents can use:

| Tool | Description |
|------|-------------|
| `read_file` | Read file contents |
| `write_file` | Create or overwrite files |
| `edit_file` | Make targeted edits to existing files |
| `bash` | Execute shell commands |
| `web_fetch` | Fetch a URL and return its content |
| `web_search` | Search the web |
| `browser` | Headless Chrome automation (navigate, click, type, screenshot, evaluate JS) |
| `send_message` | Send a message to a user/group on any connected channel |
| `cron` | Dynamically schedule or list recurring tasks |

Tool access is controlled per-agent via allow/deny policies.

---

## Data Directory

All state lives in `~/.goclaw/` — no external database required.

```
~/.goclaw/
  goclaw.json5             # Configuration file
  sessions/                # Conversation history (JSONL)
  memory/entries/          # Memory entries (Markdown)
  skills/                  # Shared skills (SKILL.md files)
  workspace-default/       # Default agent workspace
    IDENTITY.md            # Agent identity/persona
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
| `agent.status` | List all configured agents |
| `session.list` | List sessions |

HTTP endpoints: `GET /health` (health check), `GET /ws` (WebSocket), `GET /metrics` (Prometheus metrics), `GET /ui` (control panel).

---

## Security

- **Gateway auth** — bearer token authentication on WebSocket and HTTP
- **Localhost binding** — binds to `127.0.0.1` by default, no public exposure
- **Tool policies** — cascading allow/deny lists per agent
- **Execution approvals** — `deny`, `allowlist`, or `full` modes for shell commands
- **DM policy** — control how agents respond to unknown senders (`ignore`, `respond`, `notify`)
- **Group policy** — require @mention in group chats

---

## Development

```bash
make build          # Build the binary
make test           # Run all tests
make test-race      # Run tests with race detector
make lint           # Run golangci-lint
make fmt            # Format source files
make tidy           # Tidy module dependencies
make snapshot       # Cross-platform build via goreleaser
make help           # Show all targets
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
| Gemini client | `github.com/google/generative-ai-go` |
| Vector DB | `github.com/philippgille/chromem-go` |
| File watching | `github.com/fsnotify/fsnotify` |
| Browser automation | `github.com/chromedp/chromedp` |
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
| `internal/cron` | 87.9% |
| `internal/skill` | 86.6% |
| `internal/session` | 84.6% |
| `internal/agent` | 79.2% |
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
