package session

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionAppendAndHistory(t *testing.T) {
	sess := NewSession("default", "test")

	sess.Append(UserMessageEntry("hello"))
	sess.Append(AssistantMessageEntry("hi there"))
	sess.Append(UserMessageEntry("how are you?"))

	history := sess.History()
	assert.Len(t, history, 3)

	assert.Equal(t, EntryTypeMessage, history[0].Type)
	assert.Equal(t, "user", history[0].Role)
	assert.Equal(t, "assistant", history[1].Role)
	assert.Equal(t, "user", history[2].Role)
}

func TestSessionDAGTraversal(t *testing.T) {
	sess := NewSession("default", "test")

	sess.Append(UserMessageEntry("first"))
	sess.Append(AssistantMessageEntry("second"))

	history := sess.History()
	assert.Len(t, history, 2)

	// Parent chain should be connected
	assert.Empty(t, history[0].ParentID)
	assert.Equal(t, history[0].ID, history[1].ParentID)
}

func TestStorePersistence(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	// Create and populate a session
	sess, err := store.Load("agent1", "test_peer")
	require.NoError(t, err)

	sess.Append(UserMessageEntry("hello"))
	sess.Append(AssistantMessageEntry("world"))

	// Reload from disk
	sess2, err := store.Load("agent1", "test_peer")
	require.NoError(t, err)

	history := sess2.History()
	assert.Len(t, history, 2)

	// Check file exists
	path := filepath.Join(dir, "agent1", "test_peer.jsonl")
	assert.FileExists(t, path)
}

func TestToolCallEntries(t *testing.T) {
	sess := NewSession("default", "test")

	sess.Append(UserMessageEntry("run ls"))
	sess.Append(ToolCallEntry("tc_1", "bash", []byte(`{"command":"ls"}`)))
	sess.Append(ToolResultEntry("tc_1", "file1\nfile2", ""))
	sess.Append(AssistantMessageEntry("Here are the files."))

	history := sess.History()
	assert.Len(t, history, 4)
	assert.Equal(t, EntryTypeToolCall, history[1].Type)
	assert.Equal(t, EntryTypeToolResult, history[2].Type)
}

func TestSessionBranch(t *testing.T) {
	sess := NewSession("default", "test")

	sess.Append(UserMessageEntry("first"))
	firstID := sess.LeafID()
	sess.Append(AssistantMessageEntry("response 1"))
	sess.Append(UserMessageEntry("second"))

	// Branch back to first entry
	err := sess.Branch(firstID)
	require.NoError(t, err)

	assert.Equal(t, firstID, sess.LeafID())

	// Append on the branch
	sess.Append(AssistantMessageEntry("alternate response"))

	// History should follow the branch
	history := sess.History()
	assert.Len(t, history, 2) // first + alternate response
	assert.Equal(t, "user", history[0].Role)
	assert.Equal(t, "assistant", history[1].Role)
}

func TestSessionBranchInvalidID(t *testing.T) {
	sess := NewSession("default", "test")
	sess.Append(UserMessageEntry("hello"))

	err := sess.Branch("nonexistent")
	assert.Error(t, err)
}

func TestSessionCompact(t *testing.T) {
	sess := NewSession("default", "test")

	// Add 10 exchanges
	for i := 0; i < 10; i++ {
		sess.Append(UserMessageEntry("question " + string(rune('0'+i))))
		sess.Append(AssistantMessageEntry("answer " + string(rune('0'+i))))
	}

	history := sess.History()
	assert.Len(t, history, 20)

	// Compact, keeping last 4 entries
	sess.Compact("Summary of conversation: discussed topics 0-7", 4)

	history = sess.History()
	// Should have: 1 summary + 4 kept entries = 5
	assert.Len(t, history, 5)

	// First entry should be the summary meta entry
	assert.Equal(t, EntryTypeMeta, history[0].Type)
	assert.Equal(t, "system", history[0].Role)
}

func TestSessionCompactNoOp(t *testing.T) {
	sess := NewSession("default", "test")
	sess.Append(UserMessageEntry("hello"))
	sess.Append(AssistantMessageEntry("world"))

	// Compacting with keepEntries >= history length should be a no-op
	sess.Compact("summary", 10)

	history := sess.History()
	assert.Len(t, history, 2)
}

func TestSessionCompactWithStore(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	sess, err := store.Load("agent1", "compact_test")
	require.NoError(t, err)

	for i := 0; i < 10; i++ {
		sess.Append(UserMessageEntry("msg " + string(rune('0'+i))))
		sess.Append(AssistantMessageEntry("reply " + string(rune('0'+i))))
	}

	sess.Compact("Summary of conversation", 4)

	// Reload and verify
	sess2, err := store.Load("agent1", "compact_test")
	require.NoError(t, err)

	history := sess2.History()
	assert.Len(t, history, 5) // 1 summary + 4 kept
	assert.Equal(t, EntryTypeMeta, history[0].Type)
}

func TestEstimateTokens(t *testing.T) {
	sess := NewSession("default", "test")
	sess.Append(UserMessageEntry("Hello, how are you doing today?"))
	sess.Append(AssistantMessageEntry("I'm doing well, thank you for asking!"))

	tokens := sess.EstimateTokens()
	assert.Greater(t, tokens, 0)
}
