package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockMessageSender records calls for verification.
type mockMessageSender struct {
	lastChannel string
	lastChatID  string
	lastText    string
	err         error
}

func (m *mockMessageSender) SendToChannel(ctx context.Context, channel, chatID, text string) error {
	m.lastChannel = channel
	m.lastChatID = chatID
	m.lastText = text
	return m.err
}

func (m *mockMessageSender) AvailableChannels() []string {
	return []string{"telegram", "whatsapp"}
}

func TestSendMessageToolName(t *testing.T) {
	tool := &SendMessageTool{}
	assert.Equal(t, "send_message", tool.Name())
}

func TestSendMessageToolParameters(t *testing.T) {
	tool := &SendMessageTool{}
	params := tool.Parameters()
	assert.True(t, json.Valid(params), "Parameters() should return valid JSON")
}

func TestSendMessageToolSuccess(t *testing.T) {
	sender := &mockMessageSender{}
	tool := &SendMessageTool{Sender: sender}
	input, _ := json.Marshal(sendMessageInput{
		Channel: "telegram",
		ChatID:  "123456",
		Text:    "hello world",
	})

	result, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Empty(t, result.Error)
	assert.Contains(t, result.Output, "Message sent")
	assert.Contains(t, result.Output, "telegram")

	assert.Equal(t, "telegram", sender.lastChannel)
	assert.Equal(t, "123456", sender.lastChatID)
	assert.Equal(t, "hello world", sender.lastText)
}

func TestSendMessageToolMissingChannel(t *testing.T) {
	sender := &mockMessageSender{}
	tool := &SendMessageTool{Sender: sender}
	input, _ := json.Marshal(sendMessageInput{
		ChatID: "123456",
		Text:   "hello",
	})

	result, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, result.Error, "channel is required")
}

func TestSendMessageToolMissingChatID(t *testing.T) {
	sender := &mockMessageSender{}
	tool := &SendMessageTool{Sender: sender}
	input, _ := json.Marshal(sendMessageInput{
		Channel: "telegram",
		Text:    "hello",
	})

	result, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, result.Error, "chat_id is required")
}

func TestSendMessageToolMissingText(t *testing.T) {
	sender := &mockMessageSender{}
	tool := &SendMessageTool{Sender: sender}
	input, _ := json.Marshal(sendMessageInput{
		Channel: "telegram",
		ChatID:  "123456",
	})

	result, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, result.Error, "text is required")
}

func TestSendMessageToolNilSender(t *testing.T) {
	tool := &SendMessageTool{Sender: nil}
	input, _ := json.Marshal(sendMessageInput{
		Channel: "telegram",
		ChatID:  "123456",
		Text:    "hello",
	})

	result, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, result.Error, "not available")
}

func TestSendMessageToolSendError(t *testing.T) {
	sender := &mockMessageSender{err: errors.New("connection failed")}
	tool := &SendMessageTool{Sender: sender}
	input, _ := json.Marshal(sendMessageInput{
		Channel: "telegram",
		ChatID:  "123456",
		Text:    "hello",
	})

	result, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, result.Error, "send failed")
	assert.Contains(t, result.Error, "connection failed")
}
