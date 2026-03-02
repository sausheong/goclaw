package agent

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sausheong/goclaw/internal/llm"
	"github.com/sausheong/goclaw/internal/session"
)

const maxToolResultLen = 10000 // truncate tool results longer than this

// detectImageMIME returns the actual MIME type based on magic bytes.
// Falls back to the provided hint if the format is unrecognized.
func detectImageMIME(data []byte, hint string) string {
	if len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return "image/jpeg"
	}
	if len(data) >= 4 && data[0] == 0x89 && data[1] == 'P' && data[2] == 'N' && data[3] == 'G' {
		return "image/png"
	}
	if len(data) >= 4 && data[0] == 'G' && data[1] == 'I' && data[2] == 'F' && data[3] == '8' {
		return "image/gif"
	}
	if len(data) >= 4 && data[0] == 'R' && data[1] == 'I' && data[2] == 'F' && data[3] == 'F' {
		return "image/webp"
	}
	return hint
}

const defaultIdentity = `You are a helpful AI assistant called GoClaw. You can read files, write files, edit files, execute bash commands on the user's machine, fetch web pages, search the web, automate a headless browser, send messages to other channels, and schedule recurring tasks. You have vision capabilities — you can see and analyze images. Be concise and helpful. When executing tasks, think step by step and use your tools to accomplish the user's goals. When asked to access websites, use the web_fetch tool or the browser tool for interactive pages. When asked to search for information, use the web_search tool. When asked to schedule recurring tasks, use the cron tool. When asked to send messages to other users or channels, use the send_message tool. When you need to visually inspect images (screenshots, photos, camera feeds, etc.), use read_file on the image file or use the browser tool's screenshot action — both return the image for you to see and describe. Do not say you cannot see or analyze images.`

// assembleSystemPrompt builds the system prompt. Priority:
//  1. systemPrompt from config (if non-empty)
//  2. IDENTITY.md in workspace (if file exists)
//  3. Built-in defaultIdentity
func assembleSystemPrompt(workspace, systemPrompt string) string {
	if systemPrompt != "" {
		return systemPrompt
	}
	identityPath := filepath.Join(workspace, "IDENTITY.md")
	data, err := os.ReadFile(identityPath)
	if err != nil {
		return defaultIdentity
	}
	return string(data)
}

// assembleMessages converts session history into LLM messages.
// It ensures that every tool_use block in an assistant message has a
// corresponding tool_result in the next user message. Orphaned tool calls
// (e.g. from interrupted sessions) get synthetic error results injected.
func assembleMessages(history []session.SessionEntry) []llm.Message {
	// First pass: collect tool result IDs so we can detect orphaned tool calls.
	resultIDs := make(map[string]bool)
	for _, entry := range history {
		if entry.Type == session.EntryTypeToolResult {
			var tr session.ToolResultData
			if err := json.Unmarshal(entry.Data, &tr); err == nil {
				resultIDs[tr.ToolCallID] = true
			}
		}
	}

	var msgs []llm.Message

	for _, entry := range history {
		switch entry.Type {
		case session.EntryTypeMeta:
			// Meta entries (e.g. compaction summaries) are treated as system context
			var md session.MessageData
			if err := json.Unmarshal(entry.Data, &md); err != nil {
				continue
			}
			msgs = append(msgs, llm.Message{
				Role:    "user",
				Content: "[Session Summary]\n" + md.Text,
			})

		case session.EntryTypeMessage:
			var md session.MessageData
			if err := json.Unmarshal(entry.Data, &md); err != nil {
				continue
			}
			// Before appending a new message, check if the last assistant
			// message has orphaned tool calls that need synthetic results.
			msgs = injectMissingToolResults(msgs)
			msg := llm.Message{
				Role:    entry.Role,
				Content: md.Text,
			}
			// Convert session images to LLM image content
			if entry.Role == "user" {
				for _, img := range md.Images {
					data, err := base64.StdEncoding.DecodeString(img.Data)
					if err != nil {
						continue
					}
					msg.Images = append(msg.Images, llm.ImageContent{
						MimeType: detectImageMIME(data, img.MimeType),
						Data:     data,
					})
				}
			}
			msgs = append(msgs, msg)

		case session.EntryTypeToolCall:
			var td session.ToolCallData
			if err := json.Unmarshal(entry.Data, &td); err != nil {
				continue
			}
			// Tool calls are part of the assistant turn — merge into the last assistant message
			// or create one if needed
			if len(msgs) == 0 || msgs[len(msgs)-1].Role != "assistant" {
				msgs = append(msgs, llm.Message{Role: "assistant"})
			}
			msgs[len(msgs)-1].ToolCalls = append(msgs[len(msgs)-1].ToolCalls, llm.ToolCall{
				ID:    td.ID,
				Name:  td.Tool,
				Input: td.Input,
			})

		case session.EntryTypeToolResult:
			var tr session.ToolResultData
			if err := json.Unmarshal(entry.Data, &tr); err != nil {
				continue
			}
			content := tr.Output
			if tr.Error != "" {
				content = tr.Error
			}
			if content == "" {
				content = "(no output)"
			}
			msg := llm.Message{
				Role:       "user",
				Content:    content,
				ToolCallID: tr.ToolCallID,
				IsError:    tr.IsError,
			}
			// Convert session images to LLM image content
			for _, img := range tr.Images {
				data, err := base64.StdEncoding.DecodeString(img.Data)
				if err != nil {
					continue
				}
				msg.Images = append(msg.Images, llm.ImageContent{
					MimeType: detectImageMIME(data, img.MimeType),
					Data:     data,
				})
			}
			msgs = append(msgs, msg)
		}
	}

	// Final check: handle orphaned tool calls at the end of history.
	msgs = injectMissingToolResults(msgs)

	return msgs
}

// injectMissingToolResults checks if the last assistant message has tool calls
// without corresponding tool results following it. If so, it injects synthetic
// error results so the message sequence is valid for the LLM API.
func injectMissingToolResults(msgs []llm.Message) []llm.Message {
	if len(msgs) == 0 {
		return msgs
	}
	last := msgs[len(msgs)-1]
	if last.Role != "assistant" || len(last.ToolCalls) == 0 {
		return msgs
	}

	// Collect tool call IDs that already have results after this assistant message.
	// Since this is called before appending a non-tool-result message, any results
	// would already be in msgs. We only need to check if results exist at all.
	// The assistant message is the last one, so there are no results yet.
	for _, tc := range last.ToolCalls {
		msgs = append(msgs, llm.Message{
			Role:       "user",
			Content:    "(tool execution was interrupted)",
			ToolCallID: tc.ID,
			IsError:    true,
		})
	}
	return msgs
}

// pruneToolResults truncates oversized tool results in the message history
// to prevent context window overflow. Only affects tool result messages
// (identified by having a ToolCallID).
func pruneToolResults(msgs []llm.Message, maxLen int) {
	for i := range msgs {
		if msgs[i].ToolCallID != "" && len(msgs[i].Content) > maxLen {
			originalLen := len(msgs[i].Content)
			truncated := msgs[i].Content[:maxLen]
			// Try to cut at a newline boundary
			if idx := strings.LastIndex(truncated, "\n"); idx > maxLen/2 {
				truncated = truncated[:idx]
			}
			msgs[i].Content = fmt.Sprintf("%s\n\n[output truncated — %d chars total]", truncated, originalLen)
		}
	}
}
