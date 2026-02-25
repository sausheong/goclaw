package channel

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const telegramMaxMessageLength = 4096

// TelegramChannel implements the Channel interface using the Telegram Bot API.
type TelegramChannel struct {
	token          string
	requireMention bool
	sendOnly       bool // when true, Connect skips polling (for chat mode)
	botUsername     string

	bot     *bot.Bot
	inbound chan InboundMessage
	status  ChannelStatus
	cancel  context.CancelFunc
	mu      sync.Mutex
}

// NewTelegramChannel creates a new Telegram channel adapter.
func NewTelegramChannel(token string, requireMention bool) *TelegramChannel {
	return &TelegramChannel{
		token:          token,
		requireMention: requireMention,
		inbound:        make(chan InboundMessage, 100),
		status:         StatusDisconnected,
	}
}

func (t *TelegramChannel) Name() string { return "telegram" }

// SetSendOnly enables send-only mode. When true, Connect will establish the
// bot client but skip polling for updates, avoiding conflicts with another
// running instance (e.g. goclaw start).
func (t *TelegramChannel) SetSendOnly(v bool) { t.sendOnly = v }

// BotUsername returns the bot's username (available after Connect).
func (t *TelegramChannel) BotUsername() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.botUsername
}

func (t *TelegramChannel) Connect(ctx context.Context) error {
	t.mu.Lock()
	t.status = StatusConnecting
	t.mu.Unlock()

	ctx, cancel := context.WithCancel(ctx)

	opts := []bot.Option{
		bot.WithDefaultHandler(t.defaultHandler),
		bot.WithErrorsHandler(func(err error) {
			slog.Error("telegram bot error", "error", err)
		}),
	}

	b, err := bot.New(t.token, opts...)
	if err != nil {
		cancel()
		t.mu.Lock()
		t.status = StatusError
		t.mu.Unlock()
		return fmt.Errorf("create telegram bot: %w", err)
	}

	// Get bot info for username (used for mention detection in groups)
	me, err := b.GetMe(ctx)
	if err != nil {
		cancel()
		t.mu.Lock()
		t.status = StatusError
		t.mu.Unlock()
		return fmt.Errorf("telegram getMe: %w", err)
	}

	t.mu.Lock()
	t.bot = b
	t.cancel = cancel
	t.botUsername = me.Username
	t.status = StatusConnected
	t.mu.Unlock()

	slog.Info("telegram bot connected", "username", me.Username)

	if !t.sendOnly {
		go b.Start(ctx)
	}

	return nil
}

func (t *TelegramChannel) Disconnect() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.status = StatusDisconnected
	if t.cancel != nil {
		t.cancel()
	}
	return nil
}

func (t *TelegramChannel) Send(ctx context.Context, msg OutboundMessage) error {
	t.mu.Lock()
	b := t.bot
	t.mu.Unlock()

	if b == nil {
		return fmt.Errorf("telegram bot not connected")
	}

	chatID, err := strconv.ParseInt(msg.ChatID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid chat ID %q: %w", msg.ChatID, err)
	}

	// Split long messages
	chunks := splitMessage(msg.Text, telegramMaxMessageLength)
	for _, chunk := range chunks {
		params := &bot.SendMessageParams{
			ChatID: chatID,
			Text:   chunk,
		}
		if msg.ParseMode != "" {
			params.ParseMode = models.ParseMode(msg.ParseMode)
		}
		if _, err := b.SendMessage(ctx, params); err != nil {
			return fmt.Errorf("telegram send: %w", err)
		}
	}

	return nil
}

func (t *TelegramChannel) Receive() <-chan InboundMessage {
	return t.inbound
}

func (t *TelegramChannel) Status() ChannelStatus {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.status
}

// defaultHandler processes all incoming Telegram updates.
func (t *TelegramChannel) defaultHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	msg := update.Message

	// Determine chat type
	chatType := ChatTypeDirect
	if msg.Chat.Type == models.ChatTypeGroup || msg.Chat.Type == models.ChatTypeSupergroup {
		chatType = ChatTypeGroup
	}

	// In group chats with requireMention, only respond when the bot is mentioned
	if chatType == ChatTypeGroup && t.requireMention {
		if !t.isMentioned(msg) {
			return
		}
	}

	// Build text content
	text := msg.Text
	if text == "" {
		text = msg.Caption
	}

	// Strip bot mention from text in groups
	if chatType == ChatTypeGroup && t.botUsername != "" {
		text = strings.ReplaceAll(text, "@"+t.botUsername, "")
		text = strings.TrimSpace(text)
	}

	// Handle /start command
	if text == "/start" {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: msg.Chat.ID,
			Text:   "Hello! I'm GoClaw, your AI assistant. Send me a message to get started.",
		})
		if err != nil {
			slog.Error("telegram send /start response", "error", err)
		}
		return
	}

	if text == "" {
		return
	}

	// Extract sender info
	senderID := ""
	senderName := ""
	if msg.From != nil {
		senderID = strconv.FormatInt(msg.From.ID, 10)
		senderName = msg.From.FirstName
		if msg.From.LastName != "" {
			senderName += " " + msg.From.LastName
		}
	}

	// Extract media attachments
	var media []MediaAttachment

	if len(msg.Photo) > 0 {
		// Use the largest photo (last in the array)
		largest := msg.Photo[len(msg.Photo)-1]
		media = append(media, MediaAttachment{
			Type:    "photo",
			FileID:  largest.FileID,
			Caption: msg.Caption,
		})
	}

	if msg.Document != nil {
		media = append(media, MediaAttachment{
			Type:     "document",
			FileID:   msg.Document.FileID,
			FileName: msg.Document.FileName,
			MimeType: msg.Document.MimeType,
			Caption:  msg.Caption,
		})
	}

	if msg.Audio != nil {
		media = append(media, MediaAttachment{
			Type:     "audio",
			FileID:   msg.Audio.FileID,
			FileName: msg.Audio.FileName,
			MimeType: msg.Audio.MimeType,
			Caption:  msg.Caption,
		})
	}

	if msg.Video != nil {
		media = append(media, MediaAttachment{
			Type:     "video",
			FileID:   msg.Video.FileID,
			FileName: msg.Video.FileName,
			MimeType: msg.Video.MimeType,
			Caption:  msg.Caption,
		})
	}

	// Download photo data so it can be sent to the LLM
	for i := range media {
		if media[i].Type == "photo" && media[i].FileID != "" {
			data, mimeType, err := t.downloadFile(ctx, b, media[i].FileID)
			if err != nil {
				slog.Warn("telegram photo download failed", "error", err, "file_id", media[i].FileID)
				continue
			}
			media[i].Data = data
			if media[i].MimeType == "" {
				media[i].MimeType = mimeType
			}
		}
	}

	t.inbound <- InboundMessage{
		Channel:    "telegram",
		AccountID:  strconv.FormatInt(msg.Chat.ID, 10), // chat ID for replying
		ChatType:   chatType,
		SenderID:   senderID,
		SenderName: senderName,
		Text:       text,
		Media:      media,
		Timestamp:  time.Unix(int64(msg.Date), 0),
	}
}

const maxImageSize = 10 * 1024 * 1024 // 10MB

// downloadFile downloads a file from Telegram by file ID.
// Returns the file bytes and detected MIME type.
func (t *TelegramChannel) downloadFile(ctx context.Context, b *bot.Bot, fileID string) ([]byte, string, error) {
	file, err := b.GetFile(ctx, &bot.GetFileParams{FileID: fileID})
	if err != nil {
		return nil, "", fmt.Errorf("get file: %w", err)
	}

	if file.FileSize > maxImageSize {
		return nil, "", fmt.Errorf("file too large: %d bytes", file.FileSize)
	}

	url := b.FileDownloadLink(file)
	resp, err := http.Get(url)
	if err != nil {
		return nil, "", fmt.Errorf("download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("download file: status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageSize+1))
	if err != nil {
		return nil, "", fmt.Errorf("read file: %w", err)
	}
	if int64(len(data)) > maxImageSize {
		return nil, "", fmt.Errorf("file too large: exceeded %d bytes", maxImageSize)
	}

	// Detect MIME type from Content-Type header or file path
	mimeType := resp.Header.Get("Content-Type")
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = detectImageMIME(file.FilePath)
	}

	return data, mimeType, nil
}

// detectImageMIME guesses MIME type from a file path extension.
func detectImageMIME(path string) string {
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(lower, ".png"):
		return "image/png"
	case strings.HasSuffix(lower, ".gif"):
		return "image/gif"
	case strings.HasSuffix(lower, ".webp"):
		return "image/webp"
	default:
		return "image/jpeg" // Telegram photos are typically JPEG
	}
}

// isMentioned checks if the bot is mentioned in the message.
func (t *TelegramChannel) isMentioned(msg *models.Message) bool {
	if t.botUsername == "" {
		return false
	}

	mention := "@" + t.botUsername

	// Check text for mention
	if strings.Contains(msg.Text, mention) {
		return true
	}

	// Check entities for bot_command or mention type
	for _, entity := range msg.Entities {
		if entity.Type == "mention" {
			text := msg.Text[entity.Offset : entity.Offset+entity.Length]
			if strings.EqualFold(text, mention) {
				return true
			}
		}
	}

	return false
}

// splitMessage splits a long message into chunks that fit within Telegram's limit.
// It tries to split at newlines or spaces to avoid breaking mid-word.
func splitMessage(text string, maxLen int) []string {
	if len(text) <= maxLen {
		return []string{text}
	}

	var chunks []string
	for len(text) > 0 {
		if len(text) <= maxLen {
			chunks = append(chunks, text)
			break
		}

		// Try to find a good split point
		splitAt := maxLen
		// Prefer splitting at double newline
		if idx := strings.LastIndex(text[:maxLen], "\n\n"); idx > maxLen/2 {
			splitAt = idx + 2
		} else if idx := strings.LastIndex(text[:maxLen], "\n"); idx > maxLen/2 {
			splitAt = idx + 1
		} else if idx := strings.LastIndex(text[:maxLen], " "); idx > maxLen/2 {
			splitAt = idx + 1
		}

		chunks = append(chunks, text[:splitAt])
		text = text[splitAt:]
	}

	return chunks
}
