package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

// ReadFileTool reads the contents of a file.
type ReadFileTool struct{}

type readFileInput struct {
	Path string `json:"path"`
}

func (t *ReadFileTool) Name() string { return "read_file" }

func (t *ReadFileTool) Description() string {
	return "Read the contents of a file at the given path. Returns the file contents as text."
}

func (t *ReadFileTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "The absolute or relative path to the file to read"
			}
		},
		"required": ["path"]
	}`)
}

func (t *ReadFileTool) Execute(_ context.Context, input json.RawMessage) (ToolResult, error) {
	var in readFileInput
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

	return ToolResult{Output: string(data)}, nil
}
