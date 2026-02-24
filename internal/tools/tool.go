package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/sausheong/goclaw/internal/llm"
)

// Tool is the interface that all GoClaw tools must implement.
type Tool interface {
	Name() string
	Description() string
	Parameters() json.RawMessage // JSON Schema
	Execute(ctx context.Context, input json.RawMessage) (ToolResult, error)
}

// ToolResult holds the output of a tool execution.
type ToolResult struct {
	Output   string         `json:"output"`
	Error    string         `json:"error,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Executor is the interface used by agent runtime for tool operations.
// Both Registry and FilteredRegistry implement this.
type Executor interface {
	Execute(ctx context.Context, name string, input json.RawMessage) (ToolResult, error)
	ToolDefs() []llm.ToolDef
	Names() []string
	Get(name string) (Tool, bool)
}

// Registry manages a collection of available tools.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry creates a new tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry.
func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name()] = t
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// Execute runs a tool by name with the given input.
func (r *Registry) Execute(ctx context.Context, name string, input json.RawMessage) (ToolResult, error) {
	t, ok := r.Get(name)
	if !ok {
		return ToolResult{}, fmt.Errorf("unknown tool: %q", name)
	}
	return t.Execute(ctx, input)
}

// ToolDefs returns the tool definitions for the LLM API.
func (r *Registry) ToolDefs() []llm.ToolDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	defs := make([]llm.ToolDef, 0, len(r.tools))
	for _, t := range r.tools {
		defs = append(defs, llm.ToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Parameters(),
		})
	}
	return defs
}

// Names returns the names of all registered tools.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// RegisterCoreTools registers the core tools.
func RegisterCoreTools(reg *Registry, workDir string) {
	reg.Register(&ReadFileTool{})
	reg.Register(&WriteFileTool{})
	reg.Register(&EditFileTool{})
	reg.Register(&BashTool{WorkDir: workDir})
	reg.Register(&WebFetchTool{})
	reg.Register(&WebSearchTool{})
	reg.Register(&BrowserTool{})
}

// RegisterSendMessage registers the send_message tool with the given sender.
func RegisterSendMessage(reg *Registry, sender MessageSender) {
	reg.Register(&SendMessageTool{Sender: sender})
}

// RegisterCron registers the cron tool with the given scheduler.
func RegisterCron(reg *Registry, scheduler JobScheduler) {
	reg.Register(&CronTool{Scheduler: scheduler})
}
