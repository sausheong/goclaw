package main

import (
	"bufio"
	_ "embed"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"fyne.io/systray"

	"time"

	"github.com/sausheong/goclaw/internal/config"
	"github.com/sausheong/goclaw/internal/startup"
)

//go:embed icon.png
var iconBytes []byte

var (
	version = "dev"
	commit  = "none"
)

func main() {
	// Write logs to a file so crashes are diagnosable (macOS .app has no stderr,
	// Windows GUI apps have no console).
	initLogFile()

	// macOS .app bundles don't inherit shell env vars (e.g. API keys).
	// Source the user's shell profile to pick them up.
	loadShellEnv()
	systray.Run(onReady, onQuit)
}

// initLogFile redirects slog to a log file in the data directory.
func initLogFile() {
	dir := config.DefaultDataDir()
	os.MkdirAll(dir, 0o755)
	f, err := os.OpenFile(filepath.Join(dir, "goclaw-app.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(f, nil)))
}

// loadShellEnv runs an interactive login shell to dump its environment,
// then sets any missing variables in the current process.
func loadShellEnv() {
	if runtime.GOOS != "darwin" {
		return
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	out, err := exec.Command(shell, "-ilc", "env").Output()
	if err != nil {
		slog.Debug("failed to load shell env", "error", err)
		return
	}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		k, v, ok := strings.Cut(line, "=")
		if !ok || k == "" {
			continue
		}
		// Always override PATH so Homebrew/user paths are available.
		// For other vars, only set if not already present.
		if k == "PATH" || os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}

func onReady() {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic in onReady", "error", r)
			showError(fmt.Sprintf("GoClaw crashed: %v", r))
			systray.Quit()
		}
	}()

	if runtime.GOOS == "darwin" {
		systray.SetTemplateIcon(iconBytes, iconBytes)
	} else {
		systray.SetIcon(iconBytes)
	}
	systray.SetTooltip("GoClaw")

	// Start gateway in the background
	result, err := startup.StartGateway("", version, startup.Options{
		ConnectTimeout: 30 * time.Second,
	})
	if err != nil {
		slog.Error("failed to start gateway", "error", err)
		showError(fmt.Sprintf("GoClaw failed to start:\n\n%v\n\nCheck config at:\n%s", err, config.DefaultConfigPath()))
		systray.Quit()
		return
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic in gateway server", "error", r)
			}
		}()
		if err := result.Server.Start(); err != nil && err != http.ErrServerClosed {
			slog.Error("gateway error", "error", err)
		}
	}()

	port := result.Config.Gateway.Port
	if port == 0 {
		port = 18789
	}

	// Menu items
	mChat := systray.AddMenuItem("Chat", "Open chat in browser")
	mJobs := systray.AddMenuItem("Jobs", "Open jobs in browser")
	mLogs := systray.AddMenuItem("Logs", "Open logs in browser")
	mSettings := systray.AddMenuItem("Settings", "Open config file")
	mRestart := systray.AddMenuItem("Restart", "Restart the gateway")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Shut down and exit")

	go func() {
		for {
			select {
			case <-mChat.ClickedCh:
				openURL("http://localhost:" + itoa(port) + "/chat")
			case <-mJobs.ClickedCh:
				openURL("http://localhost:" + itoa(port) + "/jobs")
			case <-mLogs.ClickedCh:
				openURL("http://localhost:" + itoa(port) + "/logs")
			case <-mSettings.ClickedCh:
				openFile(config.DefaultConfigPath())
			case <-mRestart.ClickedCh:
				slog.Info("restarting gateway")
				result.Cleanup()
				newResult, err := startup.StartGateway("", version, startup.Options{
					ConnectTimeout: 30 * time.Second,
				})
				if err != nil {
					slog.Error("failed to restart gateway", "error", err)
					continue
				}
				go func() {
					defer func() {
						if r := recover(); r != nil {
							slog.Error("panic in gateway server", "error", r)
						}
					}()
					if err := newResult.Server.Start(); err != nil && err != http.ErrServerClosed {
						slog.Error("gateway error", "error", err)
					}
				}()
				result = newResult
				port = newResult.Config.Gateway.Port
				if port == 0 {
					port = 18789
				}
				slog.Info("gateway restarted", "port", port)
			case <-mQuit.ClickedCh:
				result.Cleanup()
				systray.Quit()
				return
			}
		}
	}()
}

func onQuit() {
	os.Exit(0)
}

func openURL(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		// Use rundll32 to avoid cmd /c start title-parsing issues with URLs
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		slog.Warn("unsupported OS for opening URL", "os", runtime.GOOS)
		return
	}
	if err := cmd.Start(); err != nil {
		slog.Error("failed to open URL", "url", url, "error", err)
	}
}

func openFile(path string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "linux":
		cmd = exec.Command("xdg-open", path)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", path)
	default:
		slog.Warn("unsupported OS for opening file", "os", runtime.GOOS)
		return
	}
	if err := cmd.Start(); err != nil {
		slog.Error("failed to open file", "path", path, "error", err)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
