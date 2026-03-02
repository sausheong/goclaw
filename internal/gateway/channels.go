package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/sausheong/goclaw/internal/agent"
	"github.com/sausheong/goclaw/internal/channel"
	"github.com/sausheong/goclaw/internal/config"
	"github.com/sausheong/goclaw/internal/llm"
	"github.com/sausheong/goclaw/internal/memory"
	"github.com/sausheong/goclaw/internal/router"
	"github.com/sausheong/goclaw/internal/session"
	"github.com/sausheong/goclaw/internal/skill"
	"github.com/sausheong/goclaw/internal/tools"
)

// ChannelManager bridges channel adapters to agent runtimes.
// It listens on each registered channel, routes inbound messages to the
// appropriate agent, runs the agent loop, and sends the response back.
type ChannelManager struct {
	channels     map[string]channel.Channel
	router       *router.Router
	providers    map[string]llm.LLMProvider
	tools        *tools.Registry
	sessionStore *session.Store
	config       *config.Config
	skills       *skill.Loader
	memory       *memory.Manager
	dmPolicy     string // "ignore", "respond", "notify"

	connectTimeout time.Duration // 0 means no timeout (blocks until connected)
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	mu             sync.RWMutex
}

// NewChannelManager creates a new ChannelManager.
// dmPolicy controls how unknown senders (those without a peer.id binding) are
// handled: "ignore" (default) drops their messages, "respond" processes all
// messages, and "notify" logs but drops.
func NewChannelManager(
	r *router.Router,
	providers map[string]llm.LLMProvider,
	toolReg *tools.Registry,
	sessionStore *session.Store,
	cfg *config.Config,
	dmPolicy string,
) *ChannelManager {
	if dmPolicy == "" {
		dmPolicy = "ignore"
	}
	return &ChannelManager{
		channels:     make(map[string]channel.Channel),
		router:       r,
		providers:    providers,
		tools:        toolReg,
		sessionStore: sessionStore,
		config:       cfg,
		dmPolicy:     dmPolicy,
	}
}

// SetSkills sets the skill loader for the channel manager.
func (cm *ChannelManager) SetSkills(s *skill.Loader) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.skills = s
}

// SetMemory sets the memory manager for the channel manager.
func (cm *ChannelManager) SetMemory(m *memory.Manager) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.memory = m
}

// SetConnectTimeout sets a per-channel connect timeout. When non-zero,
// channels that don't connect within this duration are skipped. Use this
// for headless environments (e.g. menubar app) where interactive setup
// like WhatsApp QR scanning is not possible.
func (cm *ChannelManager) SetConnectTimeout(d time.Duration) {
	cm.connectTimeout = d
}

// Register adds a channel adapter.
func (cm *ChannelManager) Register(ch channel.Channel) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.channels[ch.Name()] = ch
}

// Start connects all channels and launches message processing goroutines.
// Channel connection failures are logged and skipped so that the gateway
// continues to operate with the channels that succeed.
func (cm *ChannelManager) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	cm.cancel = cancel

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	for name, ch := range cm.channels {
		var err error
		if cm.connectTimeout > 0 {
			// Run Connect in a goroutine with a timeout so channels
			// that block (e.g. WhatsApp QR) don't hang the gateway.
			done := make(chan error, 1)
			go func(c channel.Channel) {
				done <- c.Connect(ctx)
			}(ch)
			select {
			case err = <-done:
			case <-time.After(cm.connectTimeout):
				slog.Warn("channel connect timed out, skipping", "channel", name)
				continue
			}
		} else {
			err = ch.Connect(ctx)
		}
		if err != nil {
			slog.Warn("failed to connect channel, skipping", "channel", name, "error", err)
			continue
		}

		cm.wg.Add(1)
		go cm.processChannel(ctx, ch)
	}

	return nil
}

// Stop disconnects all channels and waits for goroutines to finish.
func (cm *ChannelManager) Stop() {
	if cm.cancel != nil {
		cm.cancel()
	}

	cm.mu.RLock()
	for _, ch := range cm.channels {
		if err := ch.Disconnect(); err != nil {
			slog.Error("disconnect channel", "channel", ch.Name(), "error", err)
		}
	}
	cm.mu.RUnlock()

	cm.wg.Wait()
}

// SendToChannel sends a message to a specific channel and chat ID.
// Implements the tools.MessageSender interface.
func (cm *ChannelManager) SendToChannel(ctx context.Context, channelName, chatID, text string) error {
	cm.mu.RLock()
	ch, ok := cm.channels[channelName]
	cm.mu.RUnlock()

	if !ok {
		return fmt.Errorf("channel %q not connected", channelName)
	}

	return ch.Send(ctx, channel.OutboundMessage{
		ChatID: chatID,
		Text:   text,
	})
}

// AvailableChannels returns the names of all connected channels.
// Implements the tools.MessageSender interface.
func (cm *ChannelManager) AvailableChannels() []string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	names := make([]string, 0, len(cm.channels))
	for name := range cm.channels {
		names = append(names, name)
	}
	return names
}

// processChannel reads messages from a channel and dispatches them to agents.
// Messages from different senders are processed concurrently.
// Messages from the same sender are processed sequentially to avoid
// concurrent session writes.
func (cm *ChannelManager) processChannel(ctx context.Context, ch channel.Channel) {
	defer cm.wg.Done()

	// Track per-sender locks to serialize messages from the same user
	senderLocks := make(map[string]*sync.Mutex)
	var locksMu sync.Mutex

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch.Receive():
			if !ok {
				return
			}

			// Get or create per-sender lock
			locksMu.Lock()
			lock, exists := senderLocks[msg.SenderID]
			if !exists {
				lock = &sync.Mutex{}
				senderLocks[msg.SenderID] = lock
			}
			locksMu.Unlock()

			go func(m channel.InboundMessage, senderLock *sync.Mutex) {
				senderLock.Lock()
				defer senderLock.Unlock()
				cm.handleMessage(ctx, ch, m)
			}(msg, lock)
		}
	}
}

// handleMessage routes a message to an agent and sends the response back.
func (cm *ChannelManager) handleMessage(ctx context.Context, ch channel.Channel, msg channel.InboundMessage) {
	// Enforce DM policy for direct messages from unknown senders
	if msg.ChatType == channel.ChatTypeDirect && cm.dmPolicy != "respond" {
		if !cm.router.IsKnownPeer(msg.SenderID) {
			if cm.dmPolicy == "notify" {
				slog.Info("message from unknown sender (dm policy: notify)",
					"channel", ch.Name(), "sender", msg.SenderID)
			} else {
				slog.Debug("message from unknown sender ignored (dm policy: ignore)",
					"channel", ch.Name(), "sender", msg.SenderID)
			}
			return
		}
	}

	// Route to agent
	agentID := cm.router.Route(msg)

	cm.mu.RLock()
	agentCfg, ok := cm.config.GetAgent(agentID)
	cm.mu.RUnlock()

	if !ok {
		slog.Error("routed to unknown agent", "agentId", agentID, "channel", ch.Name(), "sender", msg.SenderID)
		return
	}

	// Resolve LLM provider
	providerName, modelName := llm.ParseProviderModel(agentCfg.Model)
	provider, ok := cm.providers[providerName]
	if !ok {
		slog.Error("LLM provider not configured", "provider", providerName, "agent", agentID)
		return
	}

	// Session key: {channel}_{senderID}
	sessionKey := fmt.Sprintf("%s_%s", ch.Name(), msg.SenderID)

	sess, err := cm.sessionStore.Load(agentID, sessionKey)
	if err != nil {
		slog.Error("load session", "error", err, "agent", agentID, "key", sessionKey)
		return
	}

	// Ensure workspace exists
	os.MkdirAll(agentCfg.Workspace, 0o755)

	// Create per-agent tool registry with workspace-specific tools
	toolReg := tools.NewRegistry()
	tools.RegisterCoreTools(toolReg, agentCfg.Workspace)
	tools.RegisterSendMessage(toolReg, cm)

	// Apply agent tool policy
	var executor tools.Executor = toolReg
	if len(agentCfg.Tools.Allow) > 0 || len(agentCfg.Tools.Deny) > 0 {
		executor = tools.NewFilteredRegistry(toolReg, tools.Policy{
			Allow: agentCfg.Tools.Allow,
			Deny:  agentCfg.Tools.Deny,
		})
	}

	rt := &agent.Runtime{
		LLM:          provider,
		Tools:        executor,
		Session:      sess,
		Model:        modelName,
		Workspace:    agentCfg.Workspace,
		MaxTurns:     agentCfg.MaxTurns,
		SystemPrompt: agentCfg.SystemPrompt,
		Skills:       cm.skills,
		Memory:       cm.memory,
	}

	slog.Info("processing message",
		"channel", ch.Name(),
		"sender", msg.SenderID,
		"agent", agentID,
		"model", agentCfg.Model,
	)

	// Convert downloaded media attachments to LLM image content
	var images []llm.ImageContent
	for _, m := range msg.Media {
		if len(m.Data) > 0 && m.MimeType != "" {
			images = append(images, llm.ImageContent{
				MimeType: m.MimeType,
				Data:     m.Data,
			})
		}
	}

	events, err := rt.Run(ctx, msg.Text, images)
	if err != nil {
		slog.Error("agent run error", "error", err, "agent", agentID)
		sendErr := ch.Send(ctx, channel.OutboundMessage{
			ChatID: msg.SenderID,
			Text:   "Sorry, I encountered an error. Please try again.",
		})
		if sendErr != nil {
			slog.Error("send error response", "error", sendErr)
		}
		return
	}

	// Collect the full text response (only send final text, not tool events)
	var response strings.Builder
	for event := range events {
		switch event.Type {
		case agent.EventTextDelta:
			response.WriteString(event.Text)
		case agent.EventError:
			slog.Error("agent event error", "error", event.Error, "agent", agentID)
			if response.Len() == 0 {
				response.WriteString("Sorry, I encountered an error processing your request.")
			}
		}
	}

	if response.Len() == 0 {
		return
	}

	// Use AccountID (set by adapter to the chat ID) for replying.
	// For Telegram: AccountID = chat.ID (works for both DMs and groups).
	// Falls back to SenderID for adapters that don't set AccountID (e.g. CLI).
	chatID := msg.AccountID
	if chatID == "" {
		chatID = msg.SenderID
	}

	if err := ch.Send(ctx, channel.OutboundMessage{
		ChatID: chatID,
		Text:   response.String(),
	}); err != nil {
		slog.Error("send response", "error", err, "channel", ch.Name())
	}
}
