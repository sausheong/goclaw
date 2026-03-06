//go:build !windows

package main

import "log/slog"

// showError logs the error. On non-Windows platforms, the log file
// is the primary error output (macOS .app has no stderr either).
func showError(msg string) {
	slog.Error(msg)
}
