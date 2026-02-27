package main

import (
	"bufio"
	_ "embed"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"fyne.io/systray"

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
	// macOS .app bundles don't inherit shell env vars (e.g. API keys).
	// Source the user's shell profile to pick them up.
	loadShellEnv()
	systray.Run(onReady, onQuit)
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
		// Only set vars that aren't already present
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}

func onReady() {
	systray.SetIcon(iconBytes)
	systray.SetTooltip("GoClaw")

	// Start gateway in the background
	result, err := startup.StartGateway("", version)
	if err != nil {
		slog.Error("failed to start gateway", "error", err)
		systray.Quit()
		return
	}

	go func() {
		if err := result.Server.Start(); err != nil && err != http.ErrServerClosed {
			slog.Error("gateway error", "error", err)
		}
	}()

	port := result.Config.Gateway.Port
	if port == 0 {
		port = 18789
	}

	// Menu items
	mChat := systray.AddMenuItem("Open GoClaw Chat", "Open chat in browser")
	mSettings := systray.AddMenuItem("Settings", "Open config file")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit GoClaw", "Shut down and exit")

	go func() {
		for {
			select {
			case <-mChat.ClickedCh:
				openURL("http://localhost:" + itoa(port) + "/chat")
			case <-mSettings.ClickedCh:
				openFile(config.DefaultConfigPath())
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
		cmd = exec.Command("cmd", "/c", "start", url)
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
		cmd = exec.Command("cmd", "/c", "start", path)
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
