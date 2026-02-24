package router

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sausheong/goclaw/internal/channel"
	"github.com/sausheong/goclaw/internal/config"
)

func TestRouterChannelMatch(t *testing.T) {
	r := NewRouter([]config.Binding{
		{AgentID: "agent-tg", Match: config.BindingMatch{Channel: "telegram"}},
		{AgentID: "agent-cli", Match: config.BindingMatch{Channel: "cli"}},
	}, "fallback")

	msg := channel.InboundMessage{Channel: "cli", SenderID: "user1"}
	assert.Equal(t, "agent-cli", r.Route(msg))

	msg.Channel = "telegram"
	assert.Equal(t, "agent-tg", r.Route(msg))
}

func TestRouterPeerMatch(t *testing.T) {
	r := NewRouter([]config.Binding{
		{AgentID: "vip-agent", Match: config.BindingMatch{
			Channel: "telegram",
			Peer:    &config.PeerMatch{ID: "user123"},
		}},
		{AgentID: "default-tg", Match: config.BindingMatch{Channel: "telegram"}},
	}, "fallback")

	// Specific peer should match
	msg := channel.InboundMessage{Channel: "telegram", SenderID: "user123"}
	assert.Equal(t, "vip-agent", r.Route(msg))

	// Other peer falls through to channel match
	msg.SenderID = "other"
	assert.Equal(t, "default-tg", r.Route(msg))
}

func TestRouterFallback(t *testing.T) {
	r := NewRouter([]config.Binding{
		{AgentID: "agent-tg", Match: config.BindingMatch{Channel: "telegram"}},
	}, "default")

	msg := channel.InboundMessage{Channel: "unknown", SenderID: "user1"}
	assert.Equal(t, "default", r.Route(msg))
}
