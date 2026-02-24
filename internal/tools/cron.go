package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// JobScheduler is the interface for scheduling recurring jobs.
// This avoids importing the cron package directly (decoupling).
// The cron.Scheduler implements this via an adapter in main.go.
type JobScheduler interface {
	AddJob(name, schedule, prompt string) error
	ListJobs() []JobInfo
}

// JobInfo is a summary of a scheduled job, returned by ListJobs.
type JobInfo struct {
	Name     string `json:"name"`
	Schedule string `json:"schedule"`
	Prompt   string `json:"prompt"`
}

// CronTool allows the agent to dynamically schedule recurring tasks.
type CronTool struct {
	Scheduler JobScheduler
}

type cronInput struct {
	Action   string `json:"action"`             // "add" or "list"
	Name     string `json:"name,omitempty"`      // job name (for add)
	Schedule string `json:"schedule,omitempty"`  // interval e.g. "30m", "1h", "24h" (for add)
	Prompt   string `json:"prompt,omitempty"`    // prompt to send to the agent (for add)
}

func (t *CronTool) Name() string { return "cron" }

func (t *CronTool) Description() string {
	return `Schedule or list recurring tasks. Supports two actions:
- "add": Schedule a new recurring job. Requires "name" (unique identifier), "schedule" (Go duration string like "30m", "1h", "24h"), and "prompt" (the instruction to execute each interval).
- "list": List all currently scheduled jobs.
Use this to set up automated checks, reminders, or periodic tasks.`
}

func (t *CronTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {
				"type": "string",
				"enum": ["add", "list"],
				"description": "The action to perform: add a new job or list existing jobs"
			},
			"name": {
				"type": "string",
				"description": "Unique name for the job (required for add)"
			},
			"schedule": {
				"type": "string",
				"description": "How often to run, as a Go duration string (e.g. \"30m\", \"1h\", \"24h\") (required for add)"
			},
			"prompt": {
				"type": "string",
				"description": "The prompt/instruction to execute each interval (required for add)"
			}
		},
		"required": ["action"]
	}`)
}

func (t *CronTool) Execute(ctx context.Context, input json.RawMessage) (ToolResult, error) {
	var in cronInput
	if err := json.Unmarshal(input, &in); err != nil {
		return ToolResult{Error: fmt.Sprintf("invalid input: %v", err)}, nil
	}

	if t.Scheduler == nil {
		return ToolResult{Error: "cron scheduling is not available"}, nil
	}

	switch in.Action {
	case "add":
		return t.addJob(in)
	case "list":
		return t.listJobs()
	default:
		return ToolResult{Error: fmt.Sprintf("unknown action: %q (valid: add, list)", in.Action)}, nil
	}
}

func (t *CronTool) addJob(in cronInput) (ToolResult, error) {
	if in.Name == "" {
		return ToolResult{Error: "name is required for add action"}, nil
	}
	if in.Schedule == "" {
		return ToolResult{Error: "schedule is required for add action"}, nil
	}
	if in.Prompt == "" {
		return ToolResult{Error: "prompt is required for add action"}, nil
	}

	if err := t.Scheduler.AddJob(in.Name, in.Schedule, in.Prompt); err != nil {
		return ToolResult{Error: fmt.Sprintf("failed to schedule job: %v", err)}, nil
	}

	return ToolResult{
		Output: fmt.Sprintf("Scheduled job %q to run every %s", in.Name, in.Schedule),
		Metadata: map[string]any{
			"name":     in.Name,
			"schedule": in.Schedule,
		},
	}, nil
}

func (t *CronTool) listJobs() (ToolResult, error) {
	jobs := t.Scheduler.ListJobs()

	if len(jobs) == 0 {
		return ToolResult{Output: "No scheduled jobs."}, nil
	}

	out, err := json.MarshalIndent(jobs, "", "  ")
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("failed to marshal jobs: %v", err)}, nil
	}

	return ToolResult{
		Output: fmt.Sprintf("%d scheduled job(s):\n%s", len(jobs), string(out)),
		Metadata: map[string]any{
			"count": len(jobs),
		},
	}, nil
}
