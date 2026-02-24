package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// MessageSender is the interface for sending messages to channels.
// This avoids importing the gateway package (circular dependency).
// The gateway's ChannelManager implements this interface.
type MessageSender interface {
	SendToChannel(ctx context.Context, channel, chatID, text string) error
	AvailableChannels() []string
}

// SendMessageTool sends a message to a specified channel and chat.
type SendMessageTool struct {
	Sender MessageSender
}

type sendMessageInput struct {
	Channel string `json:"channel"`  // "telegram", "whatsapp", etc.
	ChatID  string `json:"chat_id"`  // recipient chat/user ID
	Text    string `json:"text"`     // message content
}

func (t *SendMessageTool) Name() string { return "send_message" }

func (t *SendMessageTool) Description() string {
	return `Send a message to a user or group on a messaging channel. Use this to proactively notify the user, send results to a different channel, or communicate with specific contacts. Requires the channel name (e.g. "telegram", "whatsapp"), the chat/user ID, and the message text.`
}

func (t *SendMessageTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"channel": {
				"type": "string",
				"description": "The messaging channel to send to (e.g. \"telegram\", \"whatsapp\")"
			},
			"chat_id": {
				"type": "string",
				"description": "The recipient chat or user ID on the target channel"
			},
			"text": {
				"type": "string",
				"description": "The message text to send"
			}
		},
		"required": ["channel", "chat_id", "text"]
	}`)
}

func (t *SendMessageTool) Execute(ctx context.Context, input json.RawMessage) (ToolResult, error) {
	var in sendMessageInput
	if err := json.Unmarshal(input, &in); err != nil {
		return ToolResult{Error: fmt.Sprintf("invalid input: %v", err)}, nil
	}

	if in.Channel == "" {
		return ToolResult{Error: "channel is required"}, nil
	}
	if in.ChatID == "" {
		return ToolResult{Error: "chat_id is required"}, nil
	}
	if in.Text == "" {
		return ToolResult{Error: "text is required"}, nil
	}

	if t.Sender == nil {
		return ToolResult{Error: "message sending is not available (no channels connected)"}, nil
	}

	if err := t.Sender.SendToChannel(ctx, in.Channel, in.ChatID, in.Text); err != nil {
		return ToolResult{Error: fmt.Sprintf("send failed: %v", err)}, nil
	}

	return ToolResult{
		Output: fmt.Sprintf("Message sent to %s (chat: %s)", in.Channel, in.ChatID),
		Metadata: map[string]any{
			"channel": in.Channel,
			"chat_id": in.ChatID,
		},
	}, nil
}
