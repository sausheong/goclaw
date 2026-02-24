package channel

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"

	_ "modernc.org/sqlite"
)

const whatsappMaxMessageLength = 65536

// WhatsAppChannel implements the Channel interface using the WhatsApp Web
// multidevice protocol via whatsmeow.
type WhatsAppChannel struct {
	dbPath string

	client  *whatsmeow.Client
	inbound chan InboundMessage
	status  ChannelStatus
	cancel  context.CancelFunc
	mu      sync.Mutex
}

// NewWhatsAppChannel creates a new WhatsApp channel adapter.
// dbPath is the SQLite database path for storing device credentials.
func NewWhatsAppChannel(dbPath string) *WhatsAppChannel {
	return &WhatsAppChannel{
		dbPath:  dbPath,
		inbound: make(chan InboundMessage, 100),
		status:  StatusDisconnected,
	}
}

func (w *WhatsAppChannel) Name() string { return "whatsapp" }

func (w *WhatsAppChannel) Connect(ctx context.Context) error {
	w.mu.Lock()
	w.status = StatusConnecting
	w.mu.Unlock()

	ctx, cancel := context.WithCancel(ctx)

	// Ensure parent directory exists so SQLite can create the database file
	if err := os.MkdirAll(filepath.Dir(w.dbPath), 0o755); err != nil {
		cancel()
		w.mu.Lock()
		w.status = StatusError
		w.mu.Unlock()
		return fmt.Errorf("create whatsapp database directory: %w", err)
	}

	// Open SQLite database for device state (using pure-Go "sqlite" driver).
	// - foreign_keys: required by whatsmeow schema
	// - journal_mode=WAL: allows concurrent reads/writes (avoids SQLITE_BUSY during history sync)
	// - busy_timeout=5000: wait up to 5s for locks instead of failing immediately
	logger := waLog.Stdout("whatsmeow", "WARN", true)
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", w.dbPath)
	container, err := sqlstore.New(ctx, "sqlite", dsn, logger)
	if err != nil {
		cancel()
		w.mu.Lock()
		w.status = StatusError
		w.mu.Unlock()
		return fmt.Errorf("open whatsapp database: %w", err)
	}

	// Get first device or create a new one
	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		cancel()
		w.mu.Lock()
		w.status = StatusError
		w.mu.Unlock()
		return fmt.Errorf("get whatsapp device: %w", err)
	}

	client := whatsmeow.NewClient(deviceStore, logger)
	client.AddEventHandler(w.eventHandler)

	// Connect: if not logged in, show QR code; otherwise reconnect
	if client.Store.ID == nil {
		// First-time login: QR code flow
		qrChan, _ := client.GetQRChannel(ctx)
		if err := client.Connect(); err != nil {
			cancel()
			w.mu.Lock()
			w.status = StatusError
			w.mu.Unlock()
			return fmt.Errorf("whatsapp connect: %w", err)
		}

		fmt.Println()
		fmt.Println("WhatsApp QR Code Authentication")
		fmt.Println("================================")
		fmt.Println("Scan the QR code below with WhatsApp on your phone:")
		fmt.Println("  1. Open WhatsApp on your phone")
		fmt.Println("  2. Go to Settings > Linked Devices")
		fmt.Println("  3. Tap 'Link a Device'")
		fmt.Println("  4. Scan the QR code displayed below")
		fmt.Println()

		for evt := range qrChan {
			switch evt.Event {
			case "code":
				printQRCode(evt.Code)
			case "login":
				slog.Info("whatsapp login successful")
			case "timeout":
				cancel()
				w.mu.Lock()
				w.status = StatusError
				w.mu.Unlock()
				return fmt.Errorf("whatsapp QR code scan timed out — restart to try again")
			}
		}
	} else {
		// Already logged in: reconnect with stored credentials
		if err := client.Connect(); err != nil {
			cancel()
			w.mu.Lock()
			w.status = StatusError
			w.mu.Unlock()
			return fmt.Errorf("whatsapp reconnect: %w", err)
		}
	}

	w.mu.Lock()
	w.client = client
	w.cancel = cancel
	w.status = StatusConnected
	w.mu.Unlock()

	slog.Info("whatsapp channel connected")

	// Keep alive until context is cancelled
	go func() {
		<-ctx.Done()
		client.Disconnect()
	}()

	return nil
}

func (w *WhatsAppChannel) Disconnect() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.status = StatusDisconnected
	if w.cancel != nil {
		w.cancel()
	}
	return nil
}

func (w *WhatsAppChannel) Send(ctx context.Context, msg OutboundMessage) error {
	w.mu.Lock()
	client := w.client
	w.mu.Unlock()

	if client == nil {
		return fmt.Errorf("whatsapp client not connected")
	}

	jid, err := parseWhatsAppJID(msg.ChatID)
	if err != nil {
		return fmt.Errorf("invalid whatsapp JID %q: %w", msg.ChatID, err)
	}

	// Split long messages
	chunks := splitMessage(msg.Text, whatsappMaxMessageLength)
	for _, chunk := range chunks {
		waMsg := &waE2E.Message{
			Conversation: proto.String(chunk),
		}
		_, err := client.SendMessage(ctx, jid, waMsg)
		if err != nil {
			return fmt.Errorf("whatsapp send: %w", err)
		}
	}

	return nil
}

func (w *WhatsAppChannel) Receive() <-chan InboundMessage {
	return w.inbound
}

func (w *WhatsAppChannel) Status() ChannelStatus {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.status
}

// eventHandler processes incoming WhatsApp events.
func (w *WhatsAppChannel) eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		w.handleMessage(v)
	case *events.Connected:
		slog.Info("whatsapp connected event received")
	case *events.Disconnected:
		slog.Warn("whatsapp disconnected event received")
		w.mu.Lock()
		w.status = StatusDisconnected
		w.mu.Unlock()
	case *events.LoggedOut:
		slog.Warn("whatsapp logged out — re-run 'goclaw start' to scan QR code again")
		w.mu.Lock()
		w.status = StatusError
		w.mu.Unlock()
	}
}

// handleMessage converts a whatsmeow message event to an InboundMessage.
func (w *WhatsAppChannel) handleMessage(evt *events.Message) {
	// Skip messages sent by us
	if evt.Info.IsFromMe {
		return
	}

	// Extract text content
	text := extractWhatsAppText(evt.Message)
	if text == "" {
		return
	}

	// Determine chat type and IDs
	chatType := ChatTypeDirect
	senderID := evt.Info.Sender.ToNonAD().String()
	senderName := evt.Info.PushName
	accountID := senderID // for direct messages, reply to sender

	if evt.Info.IsGroup {
		chatType = ChatTypeGroup
		accountID = evt.Info.Chat.ToNonAD().String() // reply to group
	}

	// Extract media attachments
	var media []MediaAttachment
	if img := evt.Message.GetImageMessage(); img != nil {
		media = append(media, MediaAttachment{
			Type:     "photo",
			MimeType: img.GetMimetype(),
			Caption:  img.GetCaption(),
		})
	}
	if doc := evt.Message.GetDocumentMessage(); doc != nil {
		media = append(media, MediaAttachment{
			Type:     "document",
			FileName: doc.GetFileName(),
			MimeType: doc.GetMimetype(),
			Caption:  doc.GetCaption(),
		})
	}
	if audio := evt.Message.GetAudioMessage(); audio != nil {
		media = append(media, MediaAttachment{
			Type:     "audio",
			MimeType: audio.GetMimetype(),
		})
	}
	if video := evt.Message.GetVideoMessage(); video != nil {
		media = append(media, MediaAttachment{
			Type:     "video",
			MimeType: video.GetMimetype(),
			Caption:  video.GetCaption(),
		})
	}

	w.inbound <- InboundMessage{
		Channel:    "whatsapp",
		AccountID:  accountID,
		ChatType:   chatType,
		SenderID:   senderID,
		SenderName: senderName,
		Text:       text,
		Media:      media,
		Timestamp:  evt.Info.Timestamp,
	}
}

// extractWhatsAppText extracts the text content from a WhatsApp message proto.
func extractWhatsAppText(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}

	// Regular text message
	if text := msg.GetConversation(); text != "" {
		return text
	}

	// Extended text message (replies, links, etc.)
	if ext := msg.GetExtendedTextMessage(); ext != nil {
		return ext.GetText()
	}

	// Image/video/document with caption only (no separate text)
	if img := msg.GetImageMessage(); img != nil && img.GetCaption() != "" {
		return img.GetCaption()
	}
	if vid := msg.GetVideoMessage(); vid != nil && vid.GetCaption() != "" {
		return vid.GetCaption()
	}
	if doc := msg.GetDocumentMessage(); doc != nil && doc.GetCaption() != "" {
		return doc.GetCaption()
	}

	return ""
}

// parseWhatsAppJID parses a JID string into a types.JID.
// Accepts formats like:
//   - "1234567890@s.whatsapp.net" (direct message)
//   - "1234567890-1234567890@g.us" (group)
//   - "1234567890" (bare number, assumes @s.whatsapp.net)
func parseWhatsAppJID(s string) (types.JID, error) {
	if s == "" {
		return types.JID{}, fmt.Errorf("empty JID")
	}

	// If it contains @, parse as-is
	if strings.Contains(s, "@") {
		jid, err := types.ParseJID(s)
		if err != nil {
			return types.JID{}, fmt.Errorf("parse JID: %w", err)
		}
		return jid, nil
	}

	// Bare number: assume direct message
	return types.NewJID(s, types.DefaultUserServer), nil
}

// printQRCode renders a QR code string to the terminal.
// The QR code from whatsmeow is a string that can be displayed with a
// QR code terminal renderer. For simplicity, we print the raw code
// and instruct the user to use a QR reader, or use a terminal QR library.
func printQRCode(code string) {
	qrterminal.GenerateHalfBlock(code, qrterminal.L, os.Stdout)
	fmt.Println()
}

