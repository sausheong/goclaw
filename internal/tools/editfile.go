package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// EditFileTool performs a string-replace edit on a file.
type EditFileTool struct{}

type editFileInput struct {
	Path      string `json:"path"`
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
}

func (t *EditFileTool) Name() string { return "edit_file" }

func (t *EditFileTool) Description() string {
	return "Edit a file by replacing an exact string match. The old_string must match exactly one occurrence in the file. Use this for targeted edits rather than rewriting entire files."
}

func (t *EditFileTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "The path to the file to edit"
			},
			"old_string": {
				"type": "string",
				"description": "The exact text to find and replace"
			},
			"new_string": {
				"type": "string",
				"description": "The replacement text"
			}
		},
		"required": ["path", "old_string", "new_string"]
	}`)
}

func (t *EditFileTool) Execute(_ context.Context, input json.RawMessage) (ToolResult, error) {
	var in editFileInput
	if err := json.Unmarshal(input, &in); err != nil {
		return ToolResult{Error: fmt.Sprintf("invalid input: %v", err)}, nil
	}

	if in.Path == "" {
		return ToolResult{Error: "path is required"}, nil
	}

	data, err := os.ReadFile(in.Path)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("failed to read file: %v", err)}, nil
	}

	content := string(data)
	count := strings.Count(content, in.OldString)

	if count == 0 {
		return ToolResult{Error: "old_string not found in file"}, nil
	}
	if count > 1 {
		return ToolResult{Error: fmt.Sprintf("old_string found %d times in file, must be unique", count)}, nil
	}

	newContent := strings.Replace(content, in.OldString, in.NewString, 1)

	if err := os.WriteFile(in.Path, []byte(newContent), 0o644); err != nil {
		return ToolResult{Error: fmt.Sprintf("failed to write file: %v", err)}, nil
	}

	return ToolResult{Output: "Successfully edited file"}, nil
}
