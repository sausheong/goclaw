package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sausheong/goclaw/internal/llm"
)

// Policy defines allow/deny rules for tool execution.
type Policy struct {
	Allow []string // tool names to allow (empty = allow all)
	Deny  []string // tool names to deny (checked after allow)
}

// IsAllowed checks whether a tool name is permitted by this policy.
// Logic: if Allow is non-empty, the tool must be in Allow.
// If the tool is in Deny, it is blocked regardless.
func (p Policy) IsAllowed(toolName string) bool {
	// Check deny list first
	for _, d := range p.Deny {
		if d == toolName || d == "*" {
			return false
		}
	}

	// If allow list is non-empty, tool must be in it
	if len(p.Allow) > 0 {
		for _, a := range p.Allow {
			if a == toolName || a == "*" {
				return true
			}
		}
		return false
	}

	return true
}

// FilteredRegistry wraps a Registry and enforces a tool policy.
type FilteredRegistry struct {
	inner  *Registry
	policy Policy
}

// NewFilteredRegistry creates a registry that filters tools by policy.
func NewFilteredRegistry(inner *Registry, policy Policy) *FilteredRegistry {
	return &FilteredRegistry{inner: inner, policy: policy}
}

// Execute runs a tool if permitted by the policy.
func (f *FilteredRegistry) Execute(ctx context.Context, name string, input json.RawMessage) (ToolResult, error) {
	if !f.policy.IsAllowed(name) {
		return ToolResult{Error: fmt.Sprintf("tool %q is not allowed by policy", name)}, nil
	}
	return f.inner.Execute(ctx, name, input)
}

// ToolDefs returns only the tool definitions that are allowed by policy.
func (f *FilteredRegistry) ToolDefs() []llm.ToolDef {
	all := f.inner.ToolDefs()
	var filtered []llm.ToolDef
	for _, d := range all {
		if f.policy.IsAllowed(d.Name) {
			filtered = append(filtered, d)
		}
	}
	return filtered
}

// Names returns names of allowed tools only.
func (f *FilteredRegistry) Names() []string {
	all := f.inner.Names()
	var filtered []string
	for _, n := range all {
		if f.policy.IsAllowed(n) {
			filtered = append(filtered, n)
		}
	}
	return filtered
}

// Get returns a tool by name if allowed.
func (f *FilteredRegistry) Get(name string) (Tool, bool) {
	if !f.policy.IsAllowed(name) {
		return nil, false
	}
	return f.inner.Get(name)
}

// Register delegates to the inner registry.
func (f *FilteredRegistry) Register(t Tool) {
	f.inner.Register(t)
}
