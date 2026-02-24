package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/sausheong/goclaw/internal/llm"
	"github.com/sausheong/goclaw/internal/memory"
	"github.com/sausheong/goclaw/internal/session"
	"github.com/sausheong/goclaw/internal/skill"
	"github.com/sausheong/goclaw/internal/tools"
)

// EventType identifies the kind of agent event.
type EventType int

const (
	EventTextDelta EventType = iota
	EventToolCallStart
	EventToolResult
	EventDone
	EventError
)

// AgentEvent is a single streaming event from the agent.
type AgentEvent struct {
	Type     EventType
	Text     string
	ToolCall *llm.ToolCall
	Result   *tools.ToolResult
	Error    error
}

// Runtime is the agent think-act loop.
type Runtime struct {
	LLM       llm.LLMProvider
	Tools     tools.Executor
	Session   *session.Session
	Model     string
	Workspace string
	MaxTurns  int // safety limit for tool-use loops
	Skills    *skill.Loader   // optional: skill loader for selective injection
	Memory    *memory.Manager // optional: memory manager for context retrieval
}

// Run executes the agent loop for a user message, returning a channel of events.
func (r *Runtime) Run(ctx context.Context, userMsg string) (<-chan AgentEvent, error) {
	events := make(chan AgentEvent, 100)

	go func() {
		defer close(events)

		// Append user message to session
		r.Session.Append(session.UserMessageEntry(userMsg))

		maxTurns := r.MaxTurns
		if maxTurns == 0 {
			maxTurns = 25
		}

		for turn := 0; turn < maxTurns; turn++ {
			// Assemble context with skills and memory
			systemPrompt := assembleSystemPrompt(r.Workspace)

			// Inject relevant skills
			if r.Skills != nil {
				matched := r.Skills.MatchSkills(userMsg, 3)
				if extra := skill.FormatForPrompt(matched); extra != "" {
					systemPrompt += extra
				}
			}

			// Inject relevant memory
			if r.Memory != nil {
				memEntries := r.Memory.Search(userMsg, 3)
				if extra := memory.FormatForPrompt(memEntries); extra != "" {
					systemPrompt += extra
				}
			}

			history := r.Session.History()
			msgs := assembleMessages(history)

			// Prune oversized tool results
			pruneToolResults(msgs, maxToolResultLen)

			toolDefs := r.Tools.ToolDefs()

			req := llm.ChatRequest{
				Model:        r.Model,
				Messages:     msgs,
				Tools:        toolDefs,
				MaxTokens:    8192,
				SystemPrompt: systemPrompt,
			}

			// Call LLM
			stream, err := r.LLM.ChatStream(ctx, req)
			if err != nil {
				events <- AgentEvent{Type: EventError, Error: fmt.Errorf("llm error: %w", err)}
				return
			}

			// Collect the response
			var textContent strings.Builder
			var toolCalls []llm.ToolCall

			for event := range stream {
				switch event.Type {
				case llm.EventTextDelta:
					textContent.WriteString(event.Text)
					events <- AgentEvent{Type: EventTextDelta, Text: event.Text}

				case llm.EventToolCallStart:
					events <- AgentEvent{Type: EventToolCallStart, ToolCall: event.ToolCall}

				case llm.EventToolCallDone:
					if event.ToolCall != nil {
						toolCalls = append(toolCalls, *event.ToolCall)
					}

				case llm.EventError:
					events <- AgentEvent{Type: EventError, Error: event.Error}
					return
				}
			}

			// Save assistant response to session
			if textContent.Len() > 0 {
				r.Session.Append(session.AssistantMessageEntry(textContent.String()))
			}

			// If no tool calls, we're done
			if len(toolCalls) == 0 {
				events <- AgentEvent{Type: EventDone}
				return
			}

			// Save tool calls to session
			for _, tc := range toolCalls {
				r.Session.Append(session.ToolCallEntry(tc.ID, tc.Name, tc.Input))
			}

			// Execute tools
			for _, tc := range toolCalls {
				slog.Info("executing tool", "tool", tc.Name, "id", tc.ID)

				result, err := r.Tools.Execute(ctx, tc.Name, tc.Input)
				if err != nil {
					result = tools.ToolResult{Error: err.Error()}
				}

				// Save tool result to session
				r.Session.Append(session.ToolResultEntry(tc.ID, result.Output, result.Error))

				events <- AgentEvent{
					Type:     EventToolResult,
					ToolCall: &tc,
					Result:   &result,
				}
			}

			// Loop back for next LLM turn with tool results
		}

		events <- AgentEvent{
			Type:  EventError,
			Error: fmt.Errorf("agent exceeded maximum turns (%d)", r.MaxTurns),
		}
	}()

	return events, nil
}

// RunSync is a convenience method that runs the agent and collects the full text response.
func (r *Runtime) RunSync(ctx context.Context, userMsg string) (string, error) {
	events, err := r.Run(ctx, userMsg)
	if err != nil {
		return "", err
	}

	var response strings.Builder
	for event := range events {
		switch event.Type {
		case EventTextDelta:
			response.WriteString(event.Text)
		case EventError:
			return response.String(), event.Error
		}
	}

	return response.String(), nil
}
