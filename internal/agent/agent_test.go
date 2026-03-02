package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sausheong/goclaw/internal/llm"
	"github.com/sausheong/goclaw/internal/session"
	"github.com/sausheong/goclaw/internal/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockLLMProvider returns canned ChatEvent streams for testing.
type mockLLMProvider struct {
	events []llm.ChatEvent
}

func (m *mockLLMProvider) ChatStream(ctx context.Context, req llm.ChatRequest) (<-chan llm.ChatEvent, error) {
	ch := make(chan llm.ChatEvent, len(m.events))
	for _, e := range m.events {
		ch <- e
	}
	close(ch)
	return ch, nil
}

func (m *mockLLMProvider) Models() []llm.ModelInfo {
	return []llm.ModelInfo{{ID: "mock", Name: "Mock", Provider: "mock"}}
}

// --- assembleSystemPrompt tests ---

func TestAssembleSystemPromptWithIdentity(t *testing.T) {
	dir := t.TempDir()
	identityContent := "You are a test assistant."
	err := os.WriteFile(filepath.Join(dir, "IDENTITY.md"), []byte(identityContent), 0o644)
	require.NoError(t, err)

	result := assembleSystemPrompt(dir, "")
	assert.Contains(t, result, identityContent)
	assert.Contains(t, result, "configuration file")
}

func TestAssembleSystemPromptDefault(t *testing.T) {
	dir := t.TempDir() // no IDENTITY.md
	result := assembleSystemPrompt(dir, "")
	assert.Contains(t, result, defaultIdentity)
	assert.Contains(t, result, "data directory")
}

func TestAssembleSystemPromptConfigOverride(t *testing.T) {
	dir := t.TempDir()
	// Even with IDENTITY.md present, config system_prompt takes priority
	err := os.WriteFile(filepath.Join(dir, "IDENTITY.md"), []byte("identity file content"), 0o644)
	require.NoError(t, err)

	configPrompt := "You are a custom agent from config."
	result := assembleSystemPrompt(dir, configPrompt)
	assert.Contains(t, result, configPrompt)
	assert.NotContains(t, result, "identity file content")
}

// --- assembleMessages tests ---

func TestAssembleMessagesUserAndAssistant(t *testing.T) {
	history := []session.SessionEntry{
		session.UserMessageEntry("hello"),
		session.AssistantMessageEntry("hi there"),
	}

	msgs := assembleMessages(history)
	require.Len(t, msgs, 2)
	assert.Equal(t, "user", msgs[0].Role)
	assert.Equal(t, "hello", msgs[0].Content)
	assert.Equal(t, "assistant", msgs[1].Role)
	assert.Equal(t, "hi there", msgs[1].Content)
}

func TestAssembleMessagesToolCallAndResult(t *testing.T) {
	tc := session.ToolCallEntry("tc_1", "bash", json.RawMessage(`{"command":"echo hi"}`))
	tr := session.ToolResultEntry("tc_1", "hi\n", "", nil)

	history := []session.SessionEntry{
		session.UserMessageEntry("run echo hi"),
		tc,
		tr,
	}

	msgs := assembleMessages(history)
	require.Len(t, msgs, 3)

	// User message
	assert.Equal(t, "user", msgs[0].Role)

	// Tool call should be an assistant message with tool calls
	assert.Equal(t, "assistant", msgs[1].Role)
	require.Len(t, msgs[1].ToolCalls, 1)
	assert.Equal(t, "tc_1", msgs[1].ToolCalls[0].ID)
	assert.Equal(t, "bash", msgs[1].ToolCalls[0].Name)

	// Tool result should be a user message with ToolCallID
	assert.Equal(t, "user", msgs[2].Role)
	assert.Equal(t, "tc_1", msgs[2].ToolCallID)
	assert.Equal(t, "hi\n", msgs[2].Content)
}

func TestAssembleMessagesMeta(t *testing.T) {
	summaryData, _ := json.Marshal(session.MessageData{Text: "previous conversation summary"})
	meta := session.SessionEntry{
		Type: session.EntryTypeMeta,
		Role: "system",
		Data: summaryData,
	}

	msgs := assembleMessages([]session.SessionEntry{meta})
	require.Len(t, msgs, 1)
	assert.Equal(t, "user", msgs[0].Role)
	assert.Contains(t, msgs[0].Content, "[Session Summary]")
	assert.Contains(t, msgs[0].Content, "previous conversation summary")
}

func TestAssembleMessagesEmpty(t *testing.T) {
	msgs := assembleMessages(nil)
	assert.Nil(t, msgs)

	msgs = assembleMessages([]session.SessionEntry{})
	assert.Nil(t, msgs)
}

func TestAssembleMessagesOrphanedToolCall(t *testing.T) {
	// Simulate an interrupted session: tool_call without tool_result, followed by a new user message
	tc := session.ToolCallEntry("tc_orphan", "bash", json.RawMessage(`{"command":"pwd"}`))

	history := []session.SessionEntry{
		session.UserMessageEntry("run pwd"),
		tc,
		session.UserMessageEntry("hello again"),
	}

	msgs := assembleMessages(history)

	// Should have: user, assistant(tool_call), synthetic tool_result, user
	require.Len(t, msgs, 4)
	assert.Equal(t, "user", msgs[0].Role)
	assert.Equal(t, "assistant", msgs[1].Role)
	require.Len(t, msgs[1].ToolCalls, 1)
	// Synthetic result injected
	assert.Equal(t, "user", msgs[2].Role)
	assert.Equal(t, "tc_orphan", msgs[2].ToolCallID)
	assert.True(t, msgs[2].IsError)
	assert.Contains(t, msgs[2].Content, "interrupted")
	// New user message
	assert.Equal(t, "user", msgs[3].Role)
	assert.Equal(t, "hello again", msgs[3].Content)
}

func TestAssembleMessagesOrphanedToolCallAtEnd(t *testing.T) {
	// Tool call at end of history with no result and no following message
	tc := session.ToolCallEntry("tc_end", "bash", json.RawMessage(`{"command":"ls"}`))

	history := []session.SessionEntry{
		session.UserMessageEntry("list files"),
		tc,
	}

	msgs := assembleMessages(history)

	// Should have: user, assistant(tool_call), synthetic tool_result
	require.Len(t, msgs, 3)
	assert.Equal(t, "tc_end", msgs[2].ToolCallID)
	assert.True(t, msgs[2].IsError)
}

// --- pruneToolResults tests ---

func TestPruneToolResults(t *testing.T) {
	longContent := strings.Repeat("a", 20000)
	msgs := []llm.Message{
		{Role: "user", Content: "hello"},
		{Role: "user", Content: longContent, ToolCallID: "tc_1"},
	}

	pruneToolResults(msgs, 10000)

	// User message should be unchanged
	assert.Equal(t, "hello", msgs[0].Content)

	// Tool result should be truncated
	assert.Less(t, len(msgs[1].Content), 20000)
	assert.Contains(t, msgs[1].Content, "[output truncated")
}

func TestPruneToolResultsShort(t *testing.T) {
	msgs := []llm.Message{
		{Role: "user", Content: "short output", ToolCallID: "tc_1"},
	}

	pruneToolResults(msgs, 10000)

	assert.Equal(t, "short output", msgs[0].Content)
}

func TestPruneToolResultsNewlineBoundary(t *testing.T) {
	// Build content with newlines so truncation prefers a newline boundary
	var b strings.Builder
	for i := 0; i < 200; i++ {
		b.WriteString(strings.Repeat("x", 80))
		b.WriteString("\n")
	}
	content := b.String() // ~16200 chars

	msgs := []llm.Message{
		{Role: "user", Content: content, ToolCallID: "tc_1"},
	}

	pruneToolResults(msgs, 10000)

	// Should be truncated and contain the truncation marker
	truncated := msgs[0].Content
	assert.Contains(t, truncated, "[output truncated")
	assert.Less(t, len(truncated), len(content))

	// The truncated content (before the suffix) should end at a newline boundary
	suffixIdx := strings.Index(truncated, "\n\n[output truncated")
	assert.Greater(t, suffixIdx, 0, "should contain truncation suffix")
}

// --- Runtime tests ---

func TestRuntimeRun(t *testing.T) {
	mock := &mockLLMProvider{
		events: []llm.ChatEvent{
			{Type: llm.EventTextDelta, Text: "Hello "},
			{Type: llm.EventTextDelta, Text: "world!"},
			{Type: llm.EventDone},
		},
	}

	sess := session.NewSession("test-agent", "test-key")
	reg := tools.NewRegistry()

	rt := &Runtime{
		LLM:       mock,
		Tools:     reg,
		Session:   sess,
		Model:     "mock-model",
		Workspace: t.TempDir(),
		MaxTurns:  5,
	}

	events, err := rt.Run(context.Background(), "hi", nil)
	require.NoError(t, err)

	var textParts []string
	var gotDone bool
	for e := range events {
		switch e.Type {
		case EventTextDelta:
			textParts = append(textParts, e.Text)
		case EventDone:
			gotDone = true
		case EventError:
			t.Fatalf("unexpected error: %v", e.Error)
		}
	}

	assert.Equal(t, []string{"Hello ", "world!"}, textParts)
	assert.True(t, gotDone)
}

func TestRuntimeRunWithToolCalls(t *testing.T) {
	callCount := 0

	// Use a stateful mock that returns different responses
	statefulMock := &statefulMockLLMProvider{
		responses: [][]llm.ChatEvent{
			// First response: tool call
			{
				{Type: llm.EventToolCallStart, ToolCall: &llm.ToolCall{ID: "tc_1", Name: "read_file"}},
				{Type: llm.EventToolCallDone, ToolCall: &llm.ToolCall{
					ID:    "tc_1",
					Name:  "read_file",
					Input: json.RawMessage(`{"path":"/tmp/test.txt"}`),
				}},
				{Type: llm.EventDone},
			},
			// Second response: text
			{
				{Type: llm.EventTextDelta, Text: "File contents: hello"},
				{Type: llm.EventDone},
			},
		},
		callCount: &callCount,
	}

	sess := session.NewSession("test-agent", "test-key")

	// Create a registry with a mock tool
	reg := tools.NewRegistry()
	reg.Register(&mockTool{name: "read_file", output: "hello"})

	rt := &Runtime{
		LLM:       statefulMock,
		Tools:     reg,
		Session:   sess,
		Model:     "mock-model",
		Workspace: t.TempDir(),
		MaxTurns:  5,
	}

	events, err := rt.Run(context.Background(), "read test.txt", nil)
	require.NoError(t, err)

	var gotToolResult bool
	var gotDone bool
	for e := range events {
		switch e.Type {
		case EventToolResult:
			gotToolResult = true
			assert.Equal(t, "read_file", e.ToolCall.Name)
		case EventDone:
			gotDone = true
		case EventError:
			t.Fatalf("unexpected error: %v", e.Error)
		}
	}

	assert.True(t, gotToolResult, "should have received tool result")
	assert.True(t, gotDone, "should have received done event")
}

func TestRuntimeRunSync(t *testing.T) {
	mock := &mockLLMProvider{
		events: []llm.ChatEvent{
			{Type: llm.EventTextDelta, Text: "Hello "},
			{Type: llm.EventTextDelta, Text: "world!"},
			{Type: llm.EventDone},
		},
	}

	sess := session.NewSession("test-agent", "test-key")
	reg := tools.NewRegistry()

	rt := &Runtime{
		LLM:       mock,
		Tools:     reg,
		Session:   sess,
		Model:     "mock-model",
		Workspace: t.TempDir(),
		MaxTurns:  5,
	}

	text, err := rt.RunSync(context.Background(), "hi", nil)
	require.NoError(t, err)
	assert.Equal(t, "Hello world!", text)
}

// --- Helpers ---

// statefulMockLLMProvider returns different responses on successive calls.
type statefulMockLLMProvider struct {
	responses [][]llm.ChatEvent
	callCount *int
}

func (m *statefulMockLLMProvider) ChatStream(ctx context.Context, req llm.ChatRequest) (<-chan llm.ChatEvent, error) {
	idx := *m.callCount
	*m.callCount++
	if idx >= len(m.responses) {
		idx = len(m.responses) - 1
	}
	events := m.responses[idx]
	ch := make(chan llm.ChatEvent, len(events))
	for _, e := range events {
		ch <- e
	}
	close(ch)
	return ch, nil
}

func (m *statefulMockLLMProvider) Models() []llm.ModelInfo {
	return []llm.ModelInfo{{ID: "mock", Name: "Mock", Provider: "mock"}}
}

// mockTool is a simple tool that returns a canned output.
type mockTool struct {
	name   string
	output string
}

func (t *mockTool) Name() string                    { return t.name }
func (t *mockTool) Description() string             { return "mock tool" }
func (t *mockTool) Parameters() json.RawMessage      { return json.RawMessage(`{"type":"object","properties":{}}`) }
func (t *mockTool) Execute(ctx context.Context, input json.RawMessage) (tools.ToolResult, error) {
	return tools.ToolResult{Output: t.output}, nil
}
