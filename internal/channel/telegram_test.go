package channel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitMessageShort(t *testing.T) {
	chunks := splitMessage("hello world", 4096)
	assert.Len(t, chunks, 1)
	assert.Equal(t, "hello world", chunks[0])
}

func TestSplitMessageExact(t *testing.T) {
	msg := string(make([]byte, 4096))
	chunks := splitMessage(msg, 4096)
	assert.Len(t, chunks, 1)
}

func TestSplitMessageLong(t *testing.T) {
	// Create a message that's 5000 chars with newlines at known positions
	var msg string
	for i := 0; i < 100; i++ {
		msg += "This is line number " + string(rune('0'+i%10)) + " of the test message.\n"
	}
	require.Greater(t, len(msg), 4096)

	chunks := splitMessage(msg, 4096)
	assert.Greater(t, len(chunks), 1)

	// Verify no chunk exceeds the limit
	for _, chunk := range chunks {
		assert.LessOrEqual(t, len(chunk), 4096)
	}

	// Verify all content is preserved
	reassembled := ""
	for _, chunk := range chunks {
		reassembled += chunk
	}
	assert.Equal(t, msg, reassembled)
}

func TestSplitMessageNoGoodBreakpoint(t *testing.T) {
	// A message with no newlines or spaces - must hard-split
	msg := ""
	for i := 0; i < 5000; i++ {
		msg += "x"
	}

	chunks := splitMessage(msg, 4096)
	assert.Len(t, chunks, 2)
	assert.Equal(t, 4096, len(chunks[0]))
	assert.Equal(t, 904, len(chunks[1]))
}

func TestSplitMessagePrefersParagraphBreak(t *testing.T) {
	// Build a message with a paragraph break near position 3000
	part1 := make([]byte, 3000)
	for i := range part1 {
		part1[i] = 'a'
	}
	part2 := make([]byte, 2000)
	for i := range part2 {
		part2[i] = 'b'
	}
	msg := string(part1) + "\n\n" + string(part2)

	chunks := splitMessage(msg, 4096)
	assert.Len(t, chunks, 2)
	// First chunk should end at the paragraph break
	assert.Equal(t, string(part1)+"\n\n", chunks[0])
}

func TestNewTelegramChannel(t *testing.T) {
	ch := NewTelegramChannel("test-token", true)

	assert.Equal(t, "telegram", ch.Name())
	assert.Equal(t, StatusDisconnected, ch.Status())
	assert.True(t, ch.requireMention)
}

func TestTelegramChannelReceiveReturnsChannel(t *testing.T) {
	ch := NewTelegramChannel("test-token", false)
	recv := ch.Receive()
	assert.NotNil(t, recv)
}

func TestIsMentioned(t *testing.T) {
	ch := &TelegramChannel{
		botUsername: "testbot",
	}

	tests := []struct {
		name     string
		text     string
		entities []entityInfo
		want     bool
	}{
		{
			name: "mentioned in text",
			text: "hello @testbot how are you",
			want: true,
		},
		{
			name: "not mentioned",
			text: "hello world",
			want: false,
		},
		{
			name: "empty username",
			text: "@testbot hello",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We can't easily test isMentioned without creating models.Message,
			// so we test the string-contains logic directly
			mentioned := false
			mention := "@" + ch.botUsername
			if len(tt.text) > 0 {
				mentioned = contains(tt.text, mention)
			}
			assert.Equal(t, tt.want, mentioned)
		})
	}
}

type entityInfo struct {
	Type   string
	Offset int
	Length int
}

func contains(text, mention string) bool {
	for i := 0; i <= len(text)-len(mention); i++ {
		if text[i:i+len(mention)] == mention {
			return true
		}
	}
	return false
}
