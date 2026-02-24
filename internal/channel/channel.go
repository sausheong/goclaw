package channel

import (
	"context"
	"time"
)

// ChannelStatus describes the state of a channel.
type ChannelStatus string

const (
	StatusDisconnected ChannelStatus = "disconnected"
	StatusConnecting   ChannelStatus = "connecting"
	StatusConnected    ChannelStatus = "connected"
	StatusError        ChannelStatus = "error"
)

// ChatType indicates whether a message is from a direct or group chat.
type ChatType string

const (
	ChatTypeDirect ChatType = "direct"
	ChatTypeGroup  ChatType = "group"
)

// MediaAttachment represents a media file attached to a message.
type MediaAttachment struct {
	Type     string // "photo", "document", "audio", "video"
	FileID   string // platform-specific file ID (e.g. Telegram file_id)
	FileName string
	MimeType string
	Caption  string
	Data     []byte // raw file bytes (populated after download)
}

// InboundMessage is a normalised message from any channel.
type InboundMessage struct {
	Channel    string
	AccountID  string
	ChatType   ChatType
	SenderID   string
	SenderName string
	Text       string
	ReplyTo    string
	Media      []MediaAttachment
	Timestamp  time.Time
}

// OutboundMessage is a message to send via a channel.
type OutboundMessage struct {
	ChatID      string
	Text        string
	ParseMode   string // "Markdown", "HTML", ""
	ReplyMarkup any    // for inline keyboards, etc.
}

// Channel is the interface that messaging platform adapters must implement.
type Channel interface {
	Name() string
	Connect(ctx context.Context) error
	Disconnect() error
	Send(ctx context.Context, msg OutboundMessage) error
	Receive() <-chan InboundMessage
	Status() ChannelStatus
}
