package config

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWatcherDetectsChanges(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "goclaw.json5")

	// Write initial valid config
	initialConfig := `{
		"gateway": {"host": "127.0.0.1", "port": 18789},
		"agents": {"list": [{"id": "default", "name": "Test", "model": "openai/gpt-4o"}]}
	}`
	err := os.WriteFile(cfgPath, []byte(initialConfig), 0o644)
	require.NoError(t, err)

	var callbackFired atomic.Int32

	w, err := NewWatcher(cfgPath, func(cfg *Config) {
		callbackFired.Add(1)
	})
	require.NoError(t, err)

	w.Start()
	defer w.Stop()

	// Give the watcher time to start
	time.Sleep(100 * time.Millisecond)

	// Modify the file
	updatedConfig := `{
		"gateway": {"host": "127.0.0.1", "port": 19000},
		"agents": {"list": [{"id": "default", "name": "Updated", "model": "openai/gpt-4o"}]}
	}`
	err = os.WriteFile(cfgPath, []byte(updatedConfig), 0o644)
	require.NoError(t, err)

	// Wait for debounce (500ms) + some buffer
	assert.Eventually(t, func() bool {
		return callbackFired.Load() > 0
	}, 3*time.Second, 100*time.Millisecond, "callback should fire after file change")
}

func TestWatcherStopDoesNotHang(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "goclaw.json5")

	initialConfig := `{
		"gateway": {"host": "127.0.0.1", "port": 18789},
		"agents": {"list": [{"id": "default", "name": "Test", "model": "openai/gpt-4o"}]}
	}`
	err := os.WriteFile(cfgPath, []byte(initialConfig), 0o644)
	require.NoError(t, err)

	w, err := NewWatcher(cfgPath, func(cfg *Config) {})
	require.NoError(t, err)

	w.Start()

	done := make(chan struct{})
	go func() {
		w.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Stop returned promptly
	case <-time.After(3 * time.Second):
		t.Fatal("Stop() hung for more than 3 seconds")
	}
}
