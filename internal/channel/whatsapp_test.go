package channel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

func TestNewWhatsAppChannel(t *testing.T) {
	ch := NewWhatsAppChannel("/tmp/test.db")

	assert.Equal(t, "whatsapp", ch.Name())
	assert.Equal(t, StatusDisconnected, ch.Status())
	assert.Equal(t, "/tmp/test.db", ch.dbPath)
}

func TestWhatsAppChannelReceiveReturnsChannel(t *testing.T) {
	ch := NewWhatsAppChannel("/tmp/test.db")
	recv := ch.Receive()
	assert.NotNil(t, recv)
}

func TestWhatsAppChannelStatus(t *testing.T) {
	ch := NewWhatsAppChannel("/tmp/test.db")
	assert.Equal(t, StatusDisconnected, ch.Status())
}

func TestParseWhatsAppJID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    types.JID
		wantErr bool
	}{
		{
			name:  "direct message JID",
			input: "1234567890@s.whatsapp.net",
			want:  types.NewJID("1234567890", types.DefaultUserServer),
		},
		{
			name:  "group JID",
			input: "1234567890-1234567890@g.us",
			want:  types.NewJID("1234567890-1234567890", types.GroupServer),
		},
		{
			name:  "bare number defaults to direct",
			input: "1234567890",
			want:  types.NewJID("1234567890", types.DefaultUserServer),
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseWhatsAppJID(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want.User, got.User)
			assert.Equal(t, tt.want.Server, got.Server)
		})
	}
}

func TestExtractWhatsAppText(t *testing.T) {
	tests := []struct {
		name string
		msg  *waE2E.Message
		want string
	}{
		{
			name: "nil message",
			msg:  nil,
			want: "",
		},
		{
			name: "conversation text",
			msg: &waE2E.Message{
				Conversation: proto.String("hello world"),
			},
			want: "hello world",
		},
		{
			name: "extended text message",
			msg: &waE2E.Message{
				ExtendedTextMessage: &waE2E.ExtendedTextMessage{
					Text: proto.String("extended hello"),
				},
			},
			want: "extended hello",
		},
		{
			name: "image with caption",
			msg: &waE2E.Message{
				ImageMessage: &waE2E.ImageMessage{
					Caption: proto.String("photo caption"),
				},
			},
			want: "photo caption",
		},
		{
			name: "video with caption",
			msg: &waE2E.Message{
				VideoMessage: &waE2E.VideoMessage{
					Caption: proto.String("video caption"),
				},
			},
			want: "video caption",
		},
		{
			name: "document with caption",
			msg: &waE2E.Message{
				DocumentMessage: &waE2E.DocumentMessage{
					Caption: proto.String("doc caption"),
				},
			},
			want: "doc caption",
		},
		{
			name: "empty message",
			msg:  &waE2E.Message{},
			want: "",
		},
		{
			name: "conversation takes priority over caption",
			msg: &waE2E.Message{
				Conversation: proto.String("main text"),
				ImageMessage: &waE2E.ImageMessage{
					Caption: proto.String("image caption"),
				},
			},
			want: "main text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractWhatsAppText(tt.msg)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestWhatsAppSplitMessage(t *testing.T) {
	// WhatsApp reuses the same splitMessage function as Telegram
	// but with a higher limit
	short := "hello"
	chunks := splitMessage(short, whatsappMaxMessageLength)
	assert.Len(t, chunks, 1)
	assert.Equal(t, "hello", chunks[0])

	// Test with the WhatsApp limit
	long := make([]byte, whatsappMaxMessageLength+100)
	for i := range long {
		long[i] = 'x'
	}
	chunks = splitMessage(string(long), whatsappMaxMessageLength)
	assert.Len(t, chunks, 2)
	assert.Equal(t, whatsappMaxMessageLength, len(chunks[0]))
}
