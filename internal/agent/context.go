package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sausheong/goclaw/internal/llm"
	"github.com/sausheong/goclaw/internal/session"
)

const maxToolResultLen = 10000 // truncate tool results longer than this

const defaultIdentity = `You are a helpful AI assistant called GoClaw. You can read files, write files, edit files, execute bash commands on the user's machine, fetch web pages, and search the web. Be concise and helpful. When executing tasks, think step by step and use your tools to accomplish the user's goals. When asked to access websites, use the web_fetch tool. When asked to search for information, use the web_search tool.`

// assembleSystemPrompt builds the system prompt from the workspace identity file.
func assembleSystemPrompt(workspace string) string {
	identityPath := filepath.Join(workspace, "IDENTITY.md")
	data, err := os.ReadFile(identityPath)
	if err != nil {
		return defaultIdentity
	}
	return string(data)
}

// assembleMessages converts session history into LLM messages.
func assembleMessages(history []session.SessionEntry) []llm.Message {
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
			msgs = append(msgs, llm.Message{
				Role:    entry.Role,
				Content: md.Text,
			})

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
			msgs = append(msgs, llm.Message{
				Role:       "user",
				Content:    content,
				ToolCallID: tr.ToolCallID,
				IsError:    tr.IsError,
			})
		}
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
