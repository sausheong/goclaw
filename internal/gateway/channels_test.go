package gateway

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sausheong/goclaw/internal/channel"
	"github.com/sausheong/goclaw/internal/config"
	"github.com/sausheong/goclaw/internal/llm"
	"github.com/sausheong/goclaw/internal/router"
	"github.com/sausheong/goclaw/internal/session"
	"github.com/sausheong/goclaw/internal/tools"
)

// mockChannel is a test double for the Channel interface.
type mockChannel struct {
	name     string
	inbound  chan channel.InboundMessage
	sent     []channel.OutboundMessage
	mu       sync.Mutex
	status   channel.ChannelStatus
	onSend   func(channel.OutboundMessage) // optional callback
}

func newMockChannel(name string) *mockChannel {
	return &mockChannel{
		name:    name,
		inbound: make(chan channel.InboundMessage, 10),
		status:  channel.StatusDisconnected,
	}
}

func (m *mockChannel) Name() string { return m.name }

func (m *mockChannel) Connect(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status = channel.StatusConnected
	return nil
}

func (m *mockChannel) Disconnect() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status = channel.StatusDisconnected
	close(m.inbound)
	return nil
}

func (m *mockChannel) Send(_ context.Context, msg channel.OutboundMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, msg)
	if m.onSend != nil {
		m.onSend(msg)
	}
	return nil
}

func (m *mockChannel) Receive() <-chan channel.InboundMessage {
	return m.inbound
}

func (m *mockChannel) Status() channel.ChannelStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

func (m *mockChannel) Sent() []channel.OutboundMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]channel.OutboundMessage{}, m.sent...)
}

// mockProvider is a minimal LLM provider that returns a fixed response.
type mockProvider struct {
	response string
}

func (p *mockProvider) ChatStream(_ context.Context, req llm.ChatRequest) (<-chan llm.ChatEvent, error) {
	ch := make(chan llm.ChatEvent, 10)
	go func() {
		defer close(ch)
		ch <- llm.ChatEvent{Type: llm.EventTextDelta, Text: p.response}
		ch <- llm.ChatEvent{Type: llm.EventDone}
	}()
	return ch, nil
}

func (p *mockProvider) Models() []llm.ModelInfo {
	return []llm.ModelInfo{{ID: "test-model", Name: "Test Model", Provider: "test"}}
}

func TestChannelManagerRouting(t *testing.T) {
	tmpDir := t.TempDir()
	sessionStore := session.NewStore(tmpDir)

	cfg := config.DefaultConfig()
	cfg.Agents.List = []config.AgentConfig{
		{
			ID:        "agent1",
			Name:      "Agent One",
			Model:     "test/test-model",
			Workspace: t.TempDir(),
			Sandbox:   "none",
		},
		{
			ID:        "agent2",
			Name:      "Agent Two",
			Model:     "test/test-model",
			Workspace: t.TempDir(),
			Sandbox:   "none",
		},
	}

	bindings := []config.Binding{
		{AgentID: "agent1", Match: config.BindingMatch{Channel: "mock1"}},
		{AgentID: "agent2", Match: config.BindingMatch{Channel: "mock2"}},
	}

	r := router.NewRouter(bindings, "agent1")

	providers := map[string]llm.LLMProvider{
		"test": &mockProvider{response: "Hello from agent!"},
	}

	toolReg := tools.NewRegistry()

	cm := NewChannelManager(r, providers, toolReg, sessionStore, cfg)

	// Create and register two mock channels
	mock1 := newMockChannel("mock1")
	mock2 := newMockChannel("mock2")
	cm.Register(mock1)
	cm.Register(mock2)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := cm.Start(ctx)
	require.NoError(t, err)

	// Send a message on mock1
	done := make(chan struct{})
	mock1.mu.Lock()
	mock1.onSend = func(msg channel.OutboundMessage) {
		close(done)
	}
	mock1.mu.Unlock()

	mock1.inbound <- channel.InboundMessage{
		Channel:    "mock1",
		AccountID:  "chat1",
		SenderID:   "user1",
		SenderName: "Test User",
		Text:       "hello",
		Timestamp:  time.Now(),
	}

	// Wait for response
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for response")
	}

	sent := mock1.Sent()
	require.Len(t, sent, 1)
	assert.Equal(t, "chat1", sent[0].ChatID) // Should reply to the chat ID
	assert.Equal(t, "Hello from agent!", sent[0].Text)

	cm.Stop()
}

func TestChannelManagerFallbackChatID(t *testing.T) {
	// When AccountID is empty, should fall back to SenderID
	tmpDir := t.TempDir()
	sessionStore := session.NewStore(tmpDir)

	cfg := config.DefaultConfig()
	cfg.Agents.List[0].Workspace = t.TempDir()

	r := router.NewRouter(nil, "default")

	providers := map[string]llm.LLMProvider{
		"anthropic": &mockProvider{response: "response text"},
	}

	toolReg := tools.NewRegistry()

	cm := NewChannelManager(r, providers, toolReg, sessionStore, cfg)

	mock := newMockChannel("test")
	cm.Register(mock)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := cm.Start(ctx)
	require.NoError(t, err)

	done := make(chan struct{})
	mock.mu.Lock()
	mock.onSend = func(msg channel.OutboundMessage) {
		close(done)
	}
	mock.mu.Unlock()

	mock.inbound <- channel.InboundMessage{
		Channel:    "test",
		SenderID:   "sender123",
		SenderName: "Test",
		Text:       "hi",
		Timestamp:  time.Now(),
	}

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for response")
	}

	sent := mock.Sent()
	require.Len(t, sent, 1)
	assert.Equal(t, "sender123", sent[0].ChatID) // Falls back to SenderID
	assert.Equal(t, "response text", sent[0].Text)

	cm.Stop()
}
