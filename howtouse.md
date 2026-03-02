# How to Use GoClaw

GoClaw is a self-hosted AI agent gateway. It runs as a single binary on your machine and connects messaging channels (CLI, Telegram, WhatsApp) to LLM providers (Claude, GPT, Ollama, and more). You talk to it, and it talks back — with the ability to read files, write code, run commands, and browse the web on your behalf.

## Table of Contents

- [Quick Start](#quick-start)
- [CLI Commands](#cli-commands)
- [macOS Menu Bar App](#macos-menu-bar-app)
- [Configuration](#configuration)
- [LLM Providers](#llm-providers)
- [Messaging Channels](#messaging-channels)
- [Image / Vision Support](#image--vision-support)
- [Multiple Agents](#multiple-agents)
- [Message Routing](#message-routing)
- [Skills](#skills)
- [Memory](#memory)
- [Heartbeat](#heartbeat)
- [Cron Jobs](#cron-jobs)
- [Browser Tool](#browser-tool)
- [Send Message Tool](#send-message-tool)
- [Dynamic Cron Tool](#dynamic-cron-tool)
- [Ask Agent Tool](#ask-agent-tool)
- [Tool Policies](#tool-policies)
- [WebSocket API](#websocket-api)
- [Security](#security)
- [Example Configurations](#example-configurations)

---

## Quick Start

### 1. Build

```bash
make build        # CLI binary
make build-app    # macOS menu bar app (GoClaw.app)
```

### 2. Run the setup wizard

```bash
./goclaw onboard
```

The wizard walks you through choosing an LLM provider, entering your API key, and optionally setting up Telegram or WhatsApp.

### 3. Start chatting

**Interactive CLI session (no gateway needed):**
```bash
./goclaw chat
```

**Or start the full gateway (enables Telegram, WhatsApp, WebSocket API):**
```bash
./goclaw start
```

**Or launch the macOS menu bar app:**
```bash
open GoClaw.app
```

### 4. Verify your setup

```bash
./goclaw doctor
```

This checks your config file, data directories, API keys, agent workspaces, and channel configurations.

---

## CLI Commands

| Command | Description |
|---------|-------------|
| `goclaw onboard` | Interactive setup wizard |
| `goclaw start` | Start the gateway server (HTTP + WebSocket + all channels) |
| `goclaw chat` | Start an interactive CLI chat session |
| `goclaw chat myagent` | Chat with a specific agent |
| `goclaw chat -m openai/gpt-4o` | Chat with a model override |
| `goclaw status` | Query the running gateway for agent status |
| `goclaw doctor` | Run diagnostic checks |
| `goclaw doctor -c /path/to/config.json5` | Doctor with a custom config path |
| `goclaw version` | Print version and commit info |

### Chat session commands

Inside a `goclaw chat` session:

```
> Hello, what files are in this directory?
> Describe this image ~/Downloads/photo.png
> /screenshot What's in this window?
> /quit
> /exit
```

The agent can read files, write files, edit files, run shell commands, fetch web pages, and search the web — all on your local machine. You can also send images for vision analysis (see [Image / Vision Support](#image--vision-support)).

---

## macOS Menu Bar App

GoClaw includes a native macOS menu bar app that runs the gateway in the background with a system tray icon. No terminal window needed — just double-click `GoClaw.app` or drag it to `/Applications`.

### Build

```bash
make build-app
```

This produces a `GoClaw.app` bundle in the project directory.

### Usage

Launch the app by double-clicking `GoClaw.app` or from the terminal:

```bash
open GoClaw.app
```

A claw machine icon appears in the menu bar. The gateway starts automatically in the background.

### Menu items

| Item | Action |
|------|--------|
| **Open GoClaw Chat** | Opens `http://localhost:18789/chat` in your default browser |
| **Settings** | Opens `~/.goclaw/goclaw.json5` in your default text editor |
| **Quit GoClaw** | Gracefully shuts down the gateway and exits the app |

### Web chat interface

Clicking **Open GoClaw Chat** (or visiting `http://localhost:18789/chat` directly) opens a web-based chat interface with:

- **Agent selector** — a dropdown in the header lists all configured agents; switch between them without leaving the page. Each agent maintains its own session history, which is loaded when you select it.
- **Streaming responses** — text appears as the LLM generates it
- **Abort** — click the Stop button to cancel a response in progress
- **Light/dark mode** — toggle via the moon/sun button in the header; preference is saved across sessions
- **Session management** — Clear button wipes the current agent's session; switching agents loads that agent's history
- **Tool call display** — tool invocations appear inline with collapsible output
- **Markdown rendering** — code blocks, bold, italic, and links are rendered
- **Auto-reconnect** — if the WebSocket connection drops, it reconnects automatically

The root URL `http://localhost:18789` redirects to `/chat` for convenience.

### Environment variables and API keys

macOS `.app` bundles do not inherit environment variables from your shell profile (`.zshrc`, `.bashrc`, etc.). GoClaw.app handles this by automatically sourcing your shell environment at startup, so API keys like `ANTHROPIC_API_KEY` work as expected.

If you prefer not to rely on environment variables, you can set API keys directly in the config file:

```json5
{
  "providers": {
    "anthropic": {
      "kind": "anthropic",
      "api_key": "sk-ant-..."
    }
  }
}
```

### How it differs from `goclaw start`

Both `goclaw start` and `GoClaw.app` run the same gateway with the same config file. The differences:

| | `goclaw start` | `GoClaw.app` |
|---|---|---|
| Runs in | Terminal (foreground) | Menu bar (background) |
| Chat interface | WebSocket API / web chat | Web chat in browser |
| Quit | Ctrl+C | Menu bar > Quit GoClaw |
| Environment vars | Inherited from shell | Loaded from shell profile |
| Logs | Printed to terminal | System log (`Console.app`) |

---

## Configuration

GoClaw uses a JSON5 config file at `~/.goclaw/goclaw.json5`. JSON5 supports comments and trailing commas, making it easier to maintain.

### Minimal config

```json5
{
  "providers": {
    "anthropic": {
      "kind": "anthropic",
      "api_key": "sk-ant-..."
    }
  },
  "agents": {
    "list": [
      {
        "id": "default",
        "name": "Assistant",
        "model": "anthropic/claude-sonnet-4-5-20250514"
      }
    ]
  },
  "bindings": [
    { "agentId": "default", "match": { "channel": "cli" } }
  ],
  "channels": {
    "cli": { "enabled": true, "interactive": true }
  }
}
```

### Environment variables

API keys can be set via environment variables instead of (or in addition to) the config file. Environment variables take precedence.

```bash
export ANTHROPIC_API_KEY="sk-ant-..."
export OPENAI_API_KEY="sk-..."
export OLLAMA_BASE_URL="http://localhost:11434/v1"
```

The naming convention is `{PROVIDER}_API_KEY` or `{PROVIDER}_AUTH_TOKEN`, and `{PROVIDER}_BASE_URL` for custom endpoints. The `{PROVIDER}` part is the uppercased version of the provider name from your config (e.g., a provider named `"deepseek"` uses `DEEPSEEK_API_KEY`).

### Config hot-reload

GoClaw watches the config file for changes. When you edit `goclaw.json5` while the gateway is running, it hot-reloads automatically — no restart needed.

---

## LLM Providers

GoClaw supports multiple LLM providers simultaneously. Each provider is defined in the `providers` section of the config with a unique name and a `kind` that determines how GoClaw communicates with it.

### Provider kinds

| Kind | Description | Use for |
|------|-------------|---------|
| `anthropic` | Anthropic's native API (custom SSE streaming) | Claude models |
| `openai` | OpenAI's native API | GPT models |
| `openai-compatible` | Any API that implements the OpenAI chat completions spec | Ollama, LM Studio, DeepSeek, Google Gemini, LiteLLM, vLLM, and more |

### Provider config fields

| Field | Type | Description |
|-------|------|-------------|
| `kind` | string | Required. One of `"anthropic"`, `"openai"`, or `"openai-compatible"` |
| `api_key` | string | API key or auth token. Not needed for local servers like Ollama |
| `base_url` | string | Custom API endpoint. Required for `"openai-compatible"`, optional for others |

### Setting up standard providers

#### Anthropic (Claude)

1. Get an API key from [console.anthropic.com](https://console.anthropic.com/)
2. Add to your config:

```json5
{
  "providers": {
    "anthropic": {
      "kind": "anthropic",
      "api_key": "sk-ant-api03-..."
    }
  }
}
```

Or set via environment variable:

```bash
export ANTHROPIC_API_KEY="sk-ant-api03-..."
```

Available models (use with `"model": "anthropic/<model-name>"`):

| Model | Description |
|-------|-------------|
| `claude-sonnet-4-5-20250514` | Best balance of speed and capability |
| `claude-opus-4-0-20250514` | Most capable, best for complex tasks |
| `claude-haiku-3-5-20241022` | Fastest, best for simple tasks |

Example agent config:

```json5
{
  "id": "default",
  "model": "anthropic/claude-sonnet-4-5-20250514"
}
```

#### OpenAI (GPT)

1. Get an API key from [platform.openai.com](https://platform.openai.com/api-keys)
2. Add to your config:

```json5
{
  "providers": {
    "openai": {
      "kind": "openai",
      "api_key": "sk-proj-..."
    }
  }
}
```

Or set via environment variable:

```bash
export OPENAI_API_KEY="sk-proj-..."
```

Available models (use with `"model": "openai/<model-name>"`):

| Model | Description |
|-------|-------------|
| `gpt-4o` | Most capable GPT model |
| `gpt-4o-mini` | Faster, cheaper alternative |
| `gpt-4-turbo` | Previous generation |
| `o3-mini` | Reasoning model |

Example agent config:

```json5
{
  "id": "researcher",
  "model": "openai/gpt-4o"
}
```

### Setting up custom / OpenAI-compatible providers

Any service that exposes an OpenAI-compatible API (i.e., a `/v1/chat/completions` endpoint) can be used with kind `"openai-compatible"`. Set `base_url` to the API root (the part before `/chat/completions`).

#### Ollama (local models)

[Ollama](https://ollama.com/) runs open-source models locally. No API key needed.

1. Install and start Ollama: `ollama serve`
2. Pull a model: `ollama pull llama3`
3. Add to your config:

```json5
{
  "providers": {
    "ollama": {
      "kind": "openai-compatible",
      "base_url": "http://localhost:11434/v1"
    }
  }
}
```

Example agent config:

```json5
{
  "id": "local",
  "model": "ollama/llama3"
}
```

Popular Ollama models: `llama3`, `llama3.1`, `mistral`, `codellama`, `qwen2.5`, `deepseek-coder`, `phi3`, `gemma2`.

For vision-capable models, use `llava` or `bakllava`.

#### LM Studio (local models)

[LM Studio](https://lmstudio.ai/) provides a GUI for running local models with a built-in OpenAI-compatible server.

1. Download and install LM Studio
2. Load a model and start the local server (default port: 1234)
3. Add to your config:

```json5
{
  "providers": {
    "lmstudio": {
      "kind": "openai-compatible",
      "base_url": "http://localhost:1234/v1"
    }
  }
}
```

Example agent config:

```json5
{
  "id": "local",
  "model": "lmstudio/qwen2.5-coder-14b"
}
```

The model name after the slash should match what LM Studio reports for the loaded model.

#### DeepSeek

[DeepSeek](https://platform.deepseek.com/) offers cloud-hosted models with an OpenAI-compatible API.

1. Get an API key from [platform.deepseek.com](https://platform.deepseek.com/)
2. Add to your config:

```json5
{
  "providers": {
    "deepseek": {
      "kind": "openai-compatible",
      "api_key": "sk-...",
      "base_url": "https://api.deepseek.com/v1"
    }
  }
}
```

Or set via environment variables:

```bash
export DEEPSEEK_API_KEY="sk-..."
export DEEPSEEK_BASE_URL="https://api.deepseek.com/v1"
```

Example agent config:

```json5
{
  "id": "coder",
  "model": "deepseek/deepseek-chat"
}
```

Available models: `deepseek-chat`, `deepseek-coder`, `deepseek-reasoner`.

#### Google Gemini

Google Gemini can be used via its [OpenAI-compatible endpoint](https://ai.google.dev/gemini-api/docs/openai).

1. Get an API key from [aistudio.google.com](https://aistudio.google.com/apikey)
2. Add to your config:

```json5
{
  "providers": {
    "gemini": {
      "kind": "openai-compatible",
      "api_key": "AIza...",
      "base_url": "https://generativelanguage.googleapis.com/v1beta/openai"
    }
  }
}
```

Or set via environment variables:

```bash
export GEMINI_API_KEY="AIza..."
export GEMINI_BASE_URL="https://generativelanguage.googleapis.com/v1beta/openai"
```

Example agent config:

```json5
{
  "id": "default",
  "model": "gemini/gemini-2.0-flash"
}
```

Available models: `gemini-2.0-flash`, `gemini-2.0-flash-lite`, `gemini-1.5-pro`, `gemini-1.5-flash`.

#### LiteLLM / Other proxies

[LiteLLM](https://github.com/BerriAI/litellm) is a proxy that exposes 100+ LLMs through a single OpenAI-compatible API. The same pattern works for vLLM, Anyscale, Together AI, or any OpenAI-compatible proxy.

```json5
{
  "providers": {
    "litellm": {
      "kind": "openai-compatible",
      "base_url": "http://localhost:4000/v1"
    }
  }
}
```

### Model reference format

Agents reference models as `provider/model-name`, where the provider name matches a key in your `providers` section:

```
anthropic/claude-sonnet-4-5-20250514    → uses the "anthropic" provider
openai/gpt-4o                          → uses the "openai" provider
ollama/llama3                           → uses the "ollama" provider
deepseek/deepseek-chat                  → uses the "deepseek" provider
gemini/gemini-2.0-flash                 → uses the "gemini" provider
lmstudio/qwen2.5-coder-14b             → uses the "lmstudio" provider
```

You can override the model for a CLI chat session without changing the config:

```bash
goclaw chat -m openai/gpt-4o
goclaw chat -m ollama/codellama
```

### Using multiple providers

You can configure multiple providers and assign different agents to different models:

```json5
{
  "providers": {
    "anthropic": { "kind": "anthropic", "api_key": "sk-ant-..." },
    "openai":    { "kind": "openai",    "api_key": "sk-..." },
    "ollama":    { "kind": "openai-compatible", "base_url": "http://localhost:11434/v1" },
    "deepseek":  { "kind": "openai-compatible", "api_key": "sk-...", "base_url": "https://api.deepseek.com/v1" }
  },
  "agents": {
    "list": [
      { "id": "default",  "model": "anthropic/claude-sonnet-4-5-20250514" },
      { "id": "reviewer", "model": "openai/gpt-4o" },
      { "id": "quick",    "model": "ollama/llama3" },
      { "id": "coder",    "model": "deepseek/deepseek-chat" }
    ]
  }
}
```

Only providers that are actually referenced by agents are initialized at startup.

---

## Messaging Channels

### CLI

The CLI channel is always available via `goclaw chat`. It renders Markdown responses in your terminal.

```bash
# Default agent
goclaw chat

# Specific agent
goclaw chat coder

# Override the model for this session
goclaw chat -m anthropic/claude-opus-4-0-20250514
```

### Telegram

Connect a Telegram bot so you can chat with your agent from your phone.

**Setup:**

1. Open Telegram and message [@BotFather](https://t.me/BotFather)
2. Send `/newbot`, follow the prompts, and copy the bot token
3. Add to your config:

```json5
{
  "channels": {
    "telegram": {
      "token": "123456:ABC-DEF...",
      "mode": "polling"
    }
  },
  "bindings": [
    { "agentId": "default", "match": { "channel": "telegram" } }
  ]
}
```

4. Start the gateway: `goclaw start`
5. Message your bot on Telegram

**Group chats:** By default, the bot only responds in groups when mentioned (`@yourbotname`). This is controlled by the `security.groupPolicy.requireMention` setting.

**Image support:** Send a photo to the bot (with or without a caption) and the LLM will analyze it. Photos under 10MB are automatically downloaded and passed to the model for vision analysis.

### WhatsApp

Connect your personal WhatsApp account via the Web multidevice protocol. No Meta Business account or public URL needed — everything runs locally.

**Setup:**

1. Add to your config:

```json5
{
  "channels": {
    "whatsapp": {
      "db_path": "~/.goclaw/whatsapp.db",
      "phone_number": "+1234567890"  // optional, for display only
    }
  },
  "bindings": [
    { "agentId": "default", "match": { "channel": "whatsapp" } }
  ]
}
```

2. Start the gateway: `goclaw start`
3. On first start, a QR code appears in the terminal
4. Open WhatsApp on your phone > Settings > Linked Devices > Link a Device
5. Scan the QR code

After the initial pairing, credentials are stored in the SQLite database and reconnection is automatic on subsequent starts.

**Group chats:** In WhatsApp groups, the sender's JID identifies who sent the message, while the group JID is used for replying.

**Image support:** Send a photo to the bot and the LLM will describe and analyze it. Image bytes are downloaded automatically via the WhatsApp protocol.

---

## Image / Vision Support

GoClaw supports sending images to vision-capable LLMs (Claude, GPT-4o, etc.) across all three channels. The LLM sees the actual image pixels — not just metadata.

### How it works

1. **You send an image** via Telegram, WhatsApp, or CLI
2. **GoClaw downloads the image bytes** (from Telegram's API, WhatsApp's protocol, or the local filesystem)
3. **The image is passed to the LLM** as a multipart content block alongside your text
4. **The LLM responds** with a description, analysis, or answer about the image
5. **Images are persisted** in the session history (base64-encoded in JSONL) so the LLM can reference them in follow-up messages

### CLI

In `goclaw chat`, include an image file path in your message. GoClaw detects image paths, reads the file, and sends the bytes to the LLM.

**Supported input formats:**

```bash
# Bare path
> What's in this image? /Users/me/photo.png

# Tilde expansion
> Describe ~/Downloads/screenshot.jpg

# Drag-and-drop from Finder (macOS pastes a quoted path)
> Tell me about this '/Users/me/My Photos/vacation.png'

# Drag-and-drop with escaped spaces
> Analyze /Users/me/My\ Photos/vacation.png

# Image path only (defaults to "What's in this image?")
> ~/Downloads/photo.jpg
```

**Supported image formats:** `.jpg`, `.jpeg`, `.png`, `.gif`, `.webp`, `.bmp` (max 10MB)

### Screenshots

Use the `/screenshot` command in `goclaw chat` to interactively capture a window and send it to the LLM.

```bash
# Capture a window (click to select) and ask the LLM about it
> /screenshot

# Capture with a specific prompt
> /screenshot What's wrong with this UI?
> /screenshot Summarize the text in this window
> /screenshot Convert this table to CSV
```

**How it works:**

1. Type `/screenshot` (optionally followed by a prompt)
2. Your cursor changes to a crosshair — click on the window you want to capture
3. The screenshot is captured, sent to the LLM, and the LLM responds

**Platform support:**

| Platform | Tool used | Selection mode |
|----------|-----------|----------------|
| macOS | `screencapture` (built-in) | Click a window |
| Linux | `maim`, `gnome-screenshot`, or `scrot` | Click a window or drag to select |

### Telegram

Send a photo to your bot — with or without a caption. The bot downloads the photo from Telegram's servers and passes it to the LLM.

```
[Send photo with caption: "What breed is this dog?"]
Agent: That looks like a Golden Retriever! It has the characteristic...
```

### WhatsApp

Send an image message to the bot. The image is downloaded via the WhatsApp protocol and sent to the LLM.

```
[Send image with caption: "Can you read the text in this screenshot?"]
Agent: The screenshot shows a terminal with the following output...
```

### Supported LLM providers

Image/vision support works with providers that support multimodal input:

| Provider | Vision support |
|----------|---------------|
| Anthropic (Claude) | Yes — uses `image` content blocks |
| OpenAI (GPT-4o, etc.) | Yes — uses `image_url` with data URIs |
| Ollama | Depends on the model (e.g., `llava`, `bakllava`) |

If the model doesn't support vision, it will typically ignore the image or return an error.

---

## Multiple Agents

You can run multiple agents, each with its own model, workspace, and tool permissions.

```json5
{
  "agents": {
    "list": [
      {
        "id": "coder",
        "name": "Coder",
        "model": "anthropic/claude-sonnet-4-5-20250514",
        "workspace": "~/.goclaw/workspace-coder",
        "tools": {
          "allow": ["read_file", "write_file", "edit_file", "bash"]
        }
      },
      {
        "id": "researcher",
        "name": "Researcher",
        "model": "openai/gpt-4o",
        "workspace": "~/.goclaw/workspace-researcher",
        "tools": {
          "allow": ["read_file", "web_fetch", "web_search"]
        }
      },
      {
        "id": "local",
        "name": "Local Assistant",
        "model": "ollama/llama3",
        "workspace": "~/.goclaw/workspace-local",
        "tools": {
          "allow": ["read_file"]
        }
      }
    ]
  },
  "providers": {
    "anthropic": { "kind": "anthropic", "api_key": "sk-ant-..." },
    "openai":    { "kind": "openai",    "api_key": "sk-..." },
    "ollama":    { "kind": "openai-compatible", "base_url": "http://localhost:11434/v1" }
  }
}
```

Chat with a specific agent:

```bash
goclaw chat coder
goclaw chat researcher
goclaw chat local
```

### Inter-agent delegation

Agents can delegate subtasks to other agents using the `ask_agent` tool. This enables supervisor/worker patterns where a powerful model orchestrates cheaper or specialized models. See [Ask Agent Tool](#ask-agent-tool) for details.

### Agent identity

Each agent's system prompt is resolved in this priority order:

1. **`system_prompt` in config** — if the agent config has a non-empty `system_prompt` field, it is used directly
2. **`IDENTITY.md` in workspace** — if the file exists in the agent's workspace directory
3. **Built-in default** — a generic helpful-assistant prompt

Inline config example:

```json5
{
  "id": "coder",
  "model": "anthropic/claude-sonnet-4-5-20250514",
  "system_prompt": "You are a senior Go developer. You write clean, idiomatic Go code. Always write tests for new code."
}
```

Or use an `IDENTITY.md` file in the workspace:

```bash
cat > ~/.goclaw/workspace-coder/IDENTITY.md << 'EOF'
You are a senior Go developer. You write clean, idiomatic Go code.
You prefer the standard library over external dependencies.
Always write tests for new code.
EOF
```

### Model fallbacks

If the primary model's provider is unavailable, the agent can fall back to alternatives:

```json5
{
  "id": "resilient",
  "model": "anthropic/claude-sonnet-4-5-20250514",
  "fallbacks": [
    "openai/gpt-4o",
    "ollama/llama3"
  ]
}
```

---

## Message Routing

Bindings control which agent handles messages from which channel/sender. They are evaluated in order, with more specific matches taking priority.

**Priority order:** `peer.id` > `peer.kind` > `accountId` > `channel` > default

```json5
{
  "bindings": [
    // Alice always talks to the coder agent, on any channel
    {
      "agentId": "coder",
      "match": { "peer": { "id": "alice_telegram_id" } }
    },

    // All group chats on Telegram go to the researcher
    {
      "agentId": "researcher",
      "match": { "channel": "telegram", "peer": { "kind": "group" } }
    },

    // WhatsApp messages go to a specific agent
    {
      "agentId": "personal",
      "match": { "channel": "whatsapp" }
    },

    // Everything else (CLI, unmatched Telegram DMs) goes to default
    {
      "agentId": "default",
      "match": { "channel": "cli" }
    }
  ]
}
```

---

## Skills

Skills are Markdown files with YAML frontmatter that get injected into the agent's context when relevant. They teach agents domain-specific knowledge without modifying code.

### Skill file format

Create a `SKILL.md` file in `~/.goclaw/skills/<skill-name>/SKILL.md` or in the agent's workspace at `<workspace>/skills/<skill-name>/SKILL.md`:

```markdown
---
name: git-workflow
description: Guidelines for using git in this project
tags:
  - git
  - version-control
  - commit
---

## Git Workflow

- Always create feature branches from `main`
- Use conventional commit messages: `feat:`, `fix:`, `docs:`, `refactor:`
- Run tests before committing
- Squash merge into main
```

### How skills are matched

When a user sends a message, GoClaw matches it against skill names, descriptions, and tags using keyword scoring. The top 3 matching skills are injected into the agent's system prompt for that turn.

For example, if the user says "commit my changes", skills tagged with `git` or `commit` will be included.

### Skill directories

Skills are loaded from:
1. `~/.goclaw/skills/` — shared across all agents
2. `<agent-workspace>/skills/` — agent-specific skills

---

## Memory

Memory gives agents persistent knowledge across conversations. Entries are stored as Markdown files and retrieved via BM25 text search.

### Enable memory

```json5
{
  "memory": {
    "enabled": true
  }
}
```

### How it works

- Memory entries are stored as `.md` files in `~/.goclaw/memory/entries/`
- When enabled, relevant memories are automatically retrieved and injected into the agent's context each turn
- The agent can create, update, and delete memory entries during conversations
- Retrieval uses BM25 keyword search over entry content

### Manually add a memory entry

Create a Markdown file directly:

```bash
cat > ~/.goclaw/memory/entries/project-conventions.md << 'EOF'
# Project Conventions

- We use Go 1.22+ with generics where appropriate
- All HTTP handlers use chi router
- Tests use testify/assert
- Errors are wrapped with fmt.Errorf("context: %w", err)
EOF
```

The agent will recall this information when it's relevant to the conversation.

---

## Heartbeat

The heartbeat daemon periodically sends a checklist to the agent for proactive actions — like checking if a server is up, if disk space is low, or if there are new emails.

### Enable heartbeat

```json5
{
  "heartbeat": {
    "enabled": true,
    "interval": "30m"
  }
}
```

### Create a heartbeat checklist

Place a `HEARTBEAT.md` file in the agent's workspace:

```bash
cat > ~/.goclaw/workspace-default/HEARTBEAT.md << 'EOF'
- Check if the web server at localhost:8080 is responding
- Check disk usage and warn if any partition is above 90%
- Check if there are any failed systemd services
EOF
```

Every 30 minutes, the agent reads this file, evaluates each item, and takes action if needed. If everything is fine, it responds silently with `HEARTBEAT_OK`.

---

## Cron Jobs

Schedule recurring prompts for an agent. Unlike heartbeat (which reads a checklist file), cron jobs send a fixed prompt on a schedule.

```json5
{
  "agents": {
    "list": [
      {
        "id": "ops",
        "name": "Ops Agent",
        "model": "anthropic/claude-sonnet-4-5-20250514",
        "cron": [
          {
            "name": "daily-summary",
            "schedule": "24h",
            "prompt": "Generate a summary of all log files in /var/log/ from the past 24 hours. Highlight any errors or warnings."
          },
          {
            "name": "hourly-check",
            "schedule": "1h",
            "prompt": "Check if the API at https://api.example.com/health returns 200. If not, write the error to /tmp/api-alert.txt."
          }
        ]
      }
    ]
  }
}
```

Schedule values are Go duration strings: `30m`, `1h`, `6h`, `24h`, etc.

---

## Browser Tool

The `browser` tool gives agents headless Chrome automation capabilities — navigate to pages, click elements, type text, extract content, take screenshots, and run JavaScript.

### Enable for an agent

```json5
{
  "tools": {
    "allow": ["browser"]
  }
}
```

### Available actions

| Action | Description |
|--------|-------------|
| `navigate` | Navigate to a URL |
| `click` | Click an element by CSS selector |
| `type` | Type text into an input by CSS selector |
| `get_text` | Extract text content from an element by CSS selector |
| `screenshot` | Take a screenshot (returns base64 PNG) |
| `evaluate` | Run arbitrary JavaScript and return the result |

### Example conversation

```
You: Go to https://news.ycombinator.com and get the title of the top story.
Agent: [uses browser tool: navigate to URL, then get_text on the first story element]
       The top story is: "Show HN: GoClaw — self-hosted AI agent gateway in Go"

You: Take a screenshot of that page.
Agent: [uses browser tool: screenshot action]
       Here's the screenshot of the Hacker News front page.

You: Click on the "new" link at the top.
Agent: [uses browser tool: click on a.new]
       Done. I've navigated to the newest submissions page.
```

### How it works

Each invocation creates a fresh headless Chrome context with these flags:
- `--headless` — no visible browser window
- `--disable-gpu` — for compatibility on servers
- `--no-sandbox` — required in some container environments

The browser context is destroyed after the action completes, so state (cookies, sessions) does not persist between calls. For multi-step workflows, chain actions in a single conversation turn.

**Requirement:** Chrome or Chromium must be installed on the host machine.

---

## Send Message Tool

The `send_message` tool lets agents proactively send messages to users or groups on any connected channel (Telegram, WhatsApp, CLI).

### Enable for an agent

```json5
{
  "tools": {
    "allow": ["send_message"]
  }
}
```

### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `channel` | string | Channel to send on: `"telegram"`, `"whatsapp"`, or `"cli"` |
| `chat_id` | string | Recipient identifier (Telegram chat ID, WhatsApp JID, etc.) |
| `text` | string | Message content |

### Example conversation

```
You: If the API health check fails, alert me on Telegram.
Agent: [checks API, finds it's down]
       [uses send_message tool: channel="telegram", chat_id="123456789", text="Alert: API health check failed at 14:32 UTC"]
       I've sent you an alert on Telegram about the API being down.
```

### Use cases

- **Heartbeat alerts** — Agent detects an issue during a heartbeat check and notifies you on Telegram or WhatsApp
- **Cross-channel relay** — Agent receives a message on CLI and forwards a summary to a Telegram group
- **Scheduled notifications** — Combined with cron, agent sends periodic reports to a chat

### Finding chat IDs

- **Telegram:** Your numeric user ID (visible when the bot receives a message from you, logged in gateway output)
- **WhatsApp:** The JID, e.g., `1234567890@s.whatsapp.net` for direct messages or `groupid@g.us` for groups

---

## Dynamic Cron Tool

The `cron` tool lets agents dynamically create and list recurring scheduled tasks at runtime — without editing the config file.

### Enable for an agent

```json5
{
  "tools": {
    "allow": ["cron"]
  }
}
```

### Actions

**Add a job:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `action` | string | `"add"` |
| `name` | string | Unique job name |
| `schedule` | string | Go duration string (`"30m"`, `"1h"`, `"24h"`) |
| `prompt` | string | The prompt to send to the agent on each tick |

**List jobs:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `action` | string | `"list"` |

### Example conversation

```
You: Check the disk usage every hour and warn me if it's above 80%.
Agent: [uses cron tool: action="add", name="disk-check", schedule="1h",
        prompt="Check disk usage with 'df -h'. If any partition is above 80%, alert me."]
       Done. I've scheduled a recurring disk check every hour.

You: What cron jobs are currently running?
Agent: [uses cron tool: action="list"]
       Currently scheduled jobs:
       1. disk-check — every 1h — "Check disk usage with 'df -h'..."
       2. daily-summary — every 24h — "Generate a summary of..."
```

### Static vs dynamic cron

GoClaw supports cron jobs in two ways:

1. **Static (config file)** — Define cron jobs in `goclaw.json5` under the agent's `cron` array. These persist across restarts.
2. **Dynamic (cron tool)** — Agents create jobs at runtime via the `cron` tool. These are created on-the-fly during conversations.

Both use the same underlying scheduler. Static jobs are ideal for always-on tasks, while dynamic jobs let agents self-organize based on user requests.

---

## Ask Agent Tool

The `ask_agent` tool lets an agent delegate a task to another configured agent and get back the result. This enables multi-agent workflows where a supervisor agent can orchestrate specialized worker agents — for example, a powerful model delegating subtasks to cheaper or faster models.

### Enable for an agent

Add `ask_agent` to the supervisor agent's tool allow list:

```json5
{
  "tools": {
    "allow": ["read_file", "bash", "ask_agent"]
  }
}
```

### Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `agent_id` | string | The ID of the agent to delegate to (must match an agent in config) |
| `prompt` | string | The instruction or task for the target agent to perform |

The tool's description dynamically lists all available agents so the LLM knows which agents it can delegate to.

### Example conversation

```
You: Research the top 3 trending Go libraries this week, then have the coder agent
     write a summary file.

Supervisor: [uses ask_agent: agent_id="researcher", prompt="Find the top 3 trending
             Go libraries this week with brief descriptions"]
            [researcher agent uses web_search, returns results]
            [uses ask_agent: agent_id="coder", prompt="Write a file at /tmp/trending-go.md
             summarizing these libraries: 1. ..."]
            [coder agent uses write_file, returns confirmation]
            Done! I delegated the research to the researcher agent and had the coder
            agent write the summary to /tmp/trending-go.md.
```

### Example config

```json5
{
  "agents": {
    "list": [
      {
        "id": "supervisor",
        "name": "Supervisor",
        "model": "anthropic/claude-sonnet-4-5-20250514",
        "tools": {
          "allow": ["read_file", "ask_agent"]
        }
      },
      {
        "id": "researcher",
        "name": "Researcher",
        "model": "openai/gpt-4o",
        "tools": {
          "allow": ["read_file", "web_fetch", "web_search"]
        }
      },
      {
        "id": "coder",
        "name": "Coder",
        "model": "anthropic/claude-haiku-3-5-20241022",
        "tools": {
          "allow": ["read_file", "write_file", "edit_file", "bash"]
        }
      }
    ]
  }
}
```

### How it works

- The delegated agent runs with its own **independent session** (keyed as `delegate_{agentID}`) so it doesn't pollute any channel session history
- The delegated agent uses its own **tool policy** — it only gets the tools allowed by its own config
- The delegated agent uses its own **model** — so you can have a powerful supervisor delegating to cheaper/faster workers

### Recursion prevention

To prevent infinite delegation loops, the delegated agent does **not** get the `ask_agent` tool registered in its tool set. Delegation is limited to one level deep: a supervisor can ask a worker, but the worker cannot further delegate.

### Use cases

- **Supervisor/worker** — A powerful model (Claude Opus) orchestrates cheaper models (Haiku) for subtasks like file I/O or simple lookups
- **Specialized agents** — A general-purpose agent delegates coding tasks to a coder agent and research tasks to a researcher agent
- **Cost optimization** — Use an expensive model only for orchestration while delegating the bulk of work to cheaper models
- **Model mixing** — Combine strengths of different providers (e.g., Claude for reasoning, GPT for web search, Ollama for local-only tasks)

---

## Tool Policies

Each agent can have its own tool allow/deny list, controlling what actions it can perform.

### Available tools

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
| `ask_agent` | Delegate a task to another agent and get back the result |

### Policy examples

**Full access (default):**
```json5
{
  "tools": {
    "allow": ["read_file", "write_file", "edit_file", "bash", "web_fetch", "web_search"]
  }
}
```

**Read-only agent (safe for untrusted users):**
```json5
{
  "tools": {
    "allow": ["read_file", "web_fetch", "web_search"]
  }
}
```

**Everything except shell commands:**
```json5
{
  "tools": {
    "allow": ["*"],
    "deny": ["bash"]
  }
}
```

### Execution approvals

For additional safety, you can require approval for specific commands:

```json5
{
  "security": {
    "execApprovals": {
      "level": "allowlist",
      "allowlist": ["ls", "cat", "find", "grep", "head", "tail", "wc", "pwd", "date"]
    }
  }
}
```

Approval levels:
- `"full"` — all commands allowed
- `"allowlist"` — only listed commands allowed
- `"deny"` — no command execution

---

## WebSocket API

When the gateway is running (`goclaw start`), it exposes a JSON-RPC 2.0 API over WebSocket at `ws://127.0.0.1:18789/ws`.

### HTTP endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /` | Redirects to `/chat` |
| `GET /health` | Health check (returns `{"status":"ok"}`) |
| `GET /ws` | WebSocket endpoint |
| `GET /metrics` | Prometheus-style metrics (if enabled) |
| `GET /ui` | Control panel UI |
| `GET /chat` | Web chat interface (light/dark mode, streaming) |

### Send a chat message

```javascript
const ws = new WebSocket("ws://127.0.0.1:18789/ws");

ws.onopen = () => {
  ws.send(JSON.stringify({
    jsonrpc: "2.0",
    method: "chat.send",
    params: { agentId: "default", text: "What files are in the current directory?" },
    id: 1
  }));
};

ws.onmessage = (event) => {
  const response = JSON.parse(event.data);
  // response.result.type is one of:
  //   "text_delta"      — streaming text chunk
  //   "tool_call_start" — agent is calling a tool
  //   "tool_result"     — tool execution result
  //   "done"            — response complete
  //   "error"           — error occurred
  console.log(response.result);
};
```

### Query agent status

```javascript
ws.send(JSON.stringify({
  jsonrpc: "2.0",
  method: "agent.status",
  id: 2
}));
// Returns: { agents: [{ id, name, model, workspace }, ...] }
```

### Available methods

| Method | Description |
|--------|-------------|
| `chat.send` | Send a message to an agent (streams response events) |
| `chat.abort` | Cancel the active response for this connection |
| `agent.status` | List all configured agents |
| `session.list` | List sessions |
| `session.history` | Load conversation history for an agent |
| `session.clear` | Clear an agent's session history |

### Using curl to check health

```bash
curl http://127.0.0.1:18789/health
```

---

## Security

### Gateway auth

Protect the WebSocket API with a bearer token:

```json5
{
  "gateway": {
    "auth": {
      "token": "my-secret-token"
    }
  }
}
```

WebSocket clients must include the token in the connection header.

### DM policy

Control how the agent responds to unknown senders on messaging channels:

```json5
{
  "security": {
    "dmPolicy": {
      "unknownSenders": "ignore"  // "ignore", "respond", or "notify"
    }
  }
}
```

### Group policy

```json5
{
  "security": {
    "groupPolicy": {
      "requireMention": true  // bot only responds in groups when @mentioned
    }
  }
}
```

---

## Example Configurations

### Personal assistant (Claude + Telegram + WhatsApp)

```json5
{
  "providers": {
    "anthropic": {
      "kind": "anthropic",
      "api_key": "sk-ant-..."
    }
  },
  "agents": {
    "list": [
      {
        "id": "assistant",
        "name": "Personal Assistant",
        "model": "anthropic/claude-sonnet-4-5-20250514",
        "workspace": "~/.goclaw/workspace-assistant",
        "tools": {
          "allow": ["read_file", "write_file", "edit_file", "bash", "web_fetch", "web_search"]
        }
      }
    ]
  },
  "bindings": [
    { "agentId": "assistant", "match": { "channel": "cli" } },
    { "agentId": "assistant", "match": { "channel": "telegram" } },
    { "agentId": "assistant", "match": { "channel": "whatsapp" } }
  ],
  "channels": {
    "telegram": { "token": "123456:ABC...", "mode": "polling" },
    "whatsapp": { "db_path": "~/.goclaw/whatsapp.db" },
    "cli": { "enabled": true, "interactive": true }
  },
  "memory": { "enabled": true },
  "heartbeat": { "enabled": false }
}
```

### Multi-agent dev team (Claude + GPT + Ollama)

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
        "id": "lead",
        "name": "Tech Lead",
        "model": "anthropic/claude-sonnet-4-5-20250514",
        // The lead can delegate to the coder or reviewer
        "tools": { "allow": ["read_file", "bash", "ask_agent"] }
      },
      {
        "id": "coder",
        "name": "Senior Developer",
        "model": "anthropic/claude-sonnet-4-5-20250514",
        "tools": { "allow": ["read_file", "write_file", "edit_file", "bash"] }
      },
      {
        "id": "reviewer",
        "name": "Code Reviewer",
        "model": "openai/gpt-4o",
        "tools": { "allow": ["read_file"] }
      },
      {
        "id": "quick",
        "name": "Quick Helper",
        "model": "ollama/llama3",
        "tools": { "allow": ["read_file", "web_search"] }
      }
    ]
  },
  "bindings": [
    { "agentId": "lead",     "match": { "channel": "cli" } },
    { "agentId": "reviewer", "match": { "channel": "telegram", "peer": { "kind": "group" } } },
    { "agentId": "quick",    "match": { "channel": "telegram" } }
  ],
  "channels": {
    "telegram": { "token": "123456:ABC...", "mode": "polling" },
    "cli": { "enabled": true }
  }
}
```

The lead agent can use `ask_agent` to delegate coding tasks to the coder and review tasks to the reviewer, while keeping orchestration control.

### Locked-down read-only agent (safe for shared use)

```json5
{
  "providers": {
    "anthropic": { "kind": "anthropic", "api_key": "sk-ant-..." }
  },
  "agents": {
    "list": [
      {
        "id": "safe",
        "name": "Read-Only Helper",
        "model": "anthropic/claude-sonnet-4-5-20250514",
        "tools": { "allow": ["read_file"] }
      }
    ]
  },
  "bindings": [
    { "agentId": "safe", "match": { "channel": "telegram" } }
  ],
  "channels": {
    "telegram": { "token": "123456:ABC...", "mode": "polling" }
  },
  "security": {
    "execApprovals": { "level": "deny" },
    "dmPolicy": { "unknownSenders": "ignore" },
    "groupPolicy": { "requireMention": true }
  },
  "gateway": {
    "auth": { "token": "my-secret-token" }
  }
}
```

### Ollama-only (fully offline, no API keys)

```json5
{
  "providers": {
    "ollama": {
      "kind": "openai-compatible",
      "base_url": "http://localhost:11434/v1"
    }
  },
  "agents": {
    "list": [
      {
        "id": "local",
        "name": "Local Assistant",
        "model": "ollama/llama3",
        "tools": { "allow": ["read_file", "write_file", "bash"] }
      }
    ]
  },
  "bindings": [
    { "agentId": "local", "match": { "channel": "cli" } }
  ],
  "channels": {
    "cli": { "enabled": true }
  }
}
```

---

## Data Directory

All GoClaw state lives in `~/.goclaw/`:

```
~/.goclaw/
  goclaw.json5           # Configuration file
  sessions/              # Conversation history (JSONL files)
  memory/entries/        # Memory entries (Markdown files)
  skills/                # Shared skills (SKILL.md files)
  workspace-default/     # Default agent workspace
    IDENTITY.md          # Agent identity/personality
    HEARTBEAT.md         # Heartbeat checklist
    skills/              # Agent-specific skills
  whatsapp.db            # WhatsApp device credentials (SQLite)
```

No external database is required. Everything is files on disk.
