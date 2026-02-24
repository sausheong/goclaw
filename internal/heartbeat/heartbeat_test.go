package heartbeat

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDaemonTick(t *testing.T) {
	dir := t.TempDir()

	// Write a HEARTBEAT.md
	heartbeatPath := filepath.Join(dir, "HEARTBEAT.md")
	os.WriteFile(heartbeatPath, []byte("- Check for new emails"), 0o644)

	var callCount atomic.Int32

	agentFn := func(ctx context.Context, prompt string) (string, error) {
		callCount.Add(1)
		assert.Contains(t, prompt, "Check for new emails")
		return "HEARTBEAT_OK", nil
	}

	daemon := NewDaemon(dir, 50*time.Millisecond, agentFn)
	daemon.Start(context.Background())

	// Wait for at least one tick
	time.Sleep(200 * time.Millisecond)
	daemon.Stop()

	assert.GreaterOrEqual(t, callCount.Load(), int32(1))
}

func TestDaemonNoHeartbeatFile(t *testing.T) {
	dir := t.TempDir() // no HEARTBEAT.md

	var callCount atomic.Int32

	agentFn := func(ctx context.Context, prompt string) (string, error) {
		callCount.Add(1)
		return "HEARTBEAT_OK", nil
	}

	daemon := NewDaemon(dir, 50*time.Millisecond, agentFn)
	daemon.Start(context.Background())
	time.Sleep(200 * time.Millisecond)
	daemon.Stop()

	// Should not have called the agent since there's no heartbeat file
	assert.Equal(t, int32(0), callCount.Load())
}

func TestDaemonEmptyHeartbeatFile(t *testing.T) {
	dir := t.TempDir()
	heartbeatPath := filepath.Join(dir, "HEARTBEAT.md")
	os.WriteFile(heartbeatPath, []byte(""), 0o644)

	var callCount atomic.Int32

	agentFn := func(ctx context.Context, prompt string) (string, error) {
		callCount.Add(1)
		return "HEARTBEAT_OK", nil
	}

	daemon := NewDaemon(dir, 50*time.Millisecond, agentFn)
	daemon.Start(context.Background())
	time.Sleep(200 * time.Millisecond)
	daemon.Stop()

	assert.Equal(t, int32(0), callCount.Load())
}

func TestDaemonStop(t *testing.T) {
	dir := t.TempDir()
	heartbeatPath := filepath.Join(dir, "HEARTBEAT.md")
	os.WriteFile(heartbeatPath, []byte("- Check tasks"), 0o644)

	agentFn := func(ctx context.Context, prompt string) (string, error) {
		return "HEARTBEAT_OK", nil
	}

	daemon := NewDaemon(dir, 100*time.Millisecond, agentFn)
	daemon.Start(context.Background())

	// Stopping should not hang
	done := make(chan struct{})
	go func() {
		daemon.Stop()
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not return in time")
	}
}

func TestDaemonCreation(t *testing.T) {
	agentFn := func(ctx context.Context, prompt string) (string, error) {
		return "HEARTBEAT_OK", nil
	}

	daemon := NewDaemon("/tmp/test", 30*time.Minute, agentFn)
	require.NotNil(t, daemon)
	assert.Equal(t, 30*time.Minute, daemon.interval)
}
