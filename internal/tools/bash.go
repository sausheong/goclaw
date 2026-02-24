package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

const defaultBashTimeout = 120 * time.Second

// BashTool executes shell commands.
type BashTool struct {
	WorkDir string
}

type bashInput struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout"` // seconds, optional
}

func (t *BashTool) Name() string { return "bash" }

func (t *BashTool) Description() string {
	return "Execute a bash command and return its output. The command runs in a shell with a configurable timeout (default 120 seconds)."
}

func (t *BashTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {
				"type": "string",
				"description": "The bash command to execute"
			},
			"timeout": {
				"type": "integer",
				"description": "Timeout in seconds (default: 120)"
			}
		},
		"required": ["command"]
	}`)
}

func (t *BashTool) Execute(ctx context.Context, input json.RawMessage) (ToolResult, error) {
	var in bashInput
	if err := json.Unmarshal(input, &in); err != nil {
		return ToolResult{Error: fmt.Sprintf("invalid input: %v", err)}, nil
	}

	if in.Command == "" {
		return ToolResult{Error: "command is required"}, nil
	}

	timeout := defaultBashTimeout
	if in.Timeout > 0 {
		timeout = time.Duration(in.Timeout) * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", in.Command)
	if t.WorkDir != "" {
		cmd.Dir = t.WorkDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	output := stdout.String()
	errOutput := stderr.String()

	if err != nil {
		msg := err.Error()
		if ctx.Err() == context.DeadlineExceeded {
			msg = "command timed out"
		}
		if errOutput != "" {
			msg = errOutput
		}
		return ToolResult{
			Output: output,
			Error:  msg,
		}, nil
	}

	if errOutput != "" {
		output += "\nSTDERR:\n" + errOutput
	}

	return ToolResult{Output: output}, nil
}
