package router

import (
	"github.com/sausheong/goclaw/internal/channel"
	"github.com/sausheong/goclaw/internal/config"
)

// Router matches inbound messages to agent IDs using binding rules.
type Router struct {
	bindings []config.Binding
	fallback string
}

// NewRouter creates a new message router.
func NewRouter(bindings []config.Binding, fallbackAgentID string) *Router {
	return &Router{
		bindings: bindings,
		fallback: fallbackAgentID,
	}
}

// Route returns the agent ID that should handle the given message.
// Matching priority: peer.id > peer.kind > accountId > channel > default.
func (r *Router) Route(msg channel.InboundMessage) string {
	var channelMatch string

	for _, b := range r.bindings {
		m := b.Match

		// Most specific: peer.id match
		if m.Peer != nil && m.Peer.ID != "" && m.Peer.ID == msg.SenderID {
			if m.Channel == "" || m.Channel == msg.Channel {
				return b.AgentID
			}
		}

		// Peer kind match
		if m.Peer != nil && m.Peer.Kind != "" && m.Peer.Kind == string(msg.ChatType) {
			if m.Channel == "" || m.Channel == msg.Channel {
				return b.AgentID
			}
		}

		// Account ID match
		if m.AccountID != "" && m.AccountID == msg.AccountID {
			if m.Channel == "" || m.Channel == msg.Channel {
				return b.AgentID
			}
		}

		// Channel match (least specific of the explicit matches)
		if m.Channel == msg.Channel && m.Peer == nil && m.AccountID == "" {
			channelMatch = b.AgentID
		}
	}

	if channelMatch != "" {
		return channelMatch
	}

	return r.fallback
}
