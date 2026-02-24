package heartbeat

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// AgentFunc is a function that sends a heartbeat prompt to an agent and returns its response.
type AgentFunc func(ctx context.Context, prompt string) (string, error)

// Daemon runs periodic heartbeat checks.
type Daemon struct {
	workspace string
	interval  time.Duration
	agentFn   AgentFunc
	cancel    context.CancelFunc
	done      chan struct{}
}

// NewDaemon creates a new heartbeat daemon.
func NewDaemon(workspace string, interval time.Duration, agentFn AgentFunc) *Daemon {
	return &Daemon{
		workspace: workspace,
		interval:  interval,
		agentFn:   agentFn,
		done:      make(chan struct{}),
	}
}

// Start begins the heartbeat loop in a background goroutine.
func (d *Daemon) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	d.cancel = cancel

	go d.run(ctx)
	slog.Info("heartbeat daemon started", "interval", d.interval)
}

// Stop signals the daemon to stop and waits for it to finish.
func (d *Daemon) Stop() {
	if d.cancel != nil {
		d.cancel()
	}
	<-d.done
	slog.Info("heartbeat daemon stopped")
}

func (d *Daemon) run(ctx context.Context) {
	defer close(d.done)

	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.tick(ctx)
		}
	}
}

func (d *Daemon) tick(ctx context.Context) {
	heartbeatPath := filepath.Join(d.workspace, "HEARTBEAT.md")
	data, err := os.ReadFile(heartbeatPath)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("failed to read HEARTBEAT.md", "error", err)
		}
		return
	}

	content := strings.TrimSpace(string(data))
	if content == "" {
		return
	}

	prompt := "HEARTBEAT CHECK: The following is your heartbeat checklist. Review each item and take action if needed. If no action is required, respond with exactly HEARTBEAT_OK.\n\n" + content

	slog.Info("heartbeat tick — sending to agent")

	response, err := d.agentFn(ctx, prompt)
	if err != nil {
		slog.Error("heartbeat agent error", "error", err)
		return
	}

	// Drop HEARTBEAT_OK responses silently
	if strings.TrimSpace(response) == "HEARTBEAT_OK" {
		slog.Debug("heartbeat — no action needed")
		return
	}

	// Non-OK response means the agent took action — log it
	slog.Info("heartbeat — agent acted", "response_length", len(response))
}
