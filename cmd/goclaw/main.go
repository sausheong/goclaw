package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"

	"github.com/sausheong/goclaw/internal/agent"
	"github.com/sausheong/goclaw/internal/channel"
	"github.com/sausheong/goclaw/internal/config"
	"github.com/sausheong/goclaw/internal/cron"
	"github.com/sausheong/goclaw/internal/gateway"
	"github.com/sausheong/goclaw/internal/heartbeat"
	"github.com/sausheong/goclaw/internal/llm"
	"github.com/sausheong/goclaw/internal/memory"
	"github.com/sausheong/goclaw/internal/router"
	"github.com/sausheong/goclaw/internal/session"
	"github.com/sausheong/goclaw/internal/skill"
	"github.com/sausheong/goclaw/internal/tools"
)

var (
	version = "dev"
	commit  = "none"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "goclaw",
		Short: "GoClaw — self-hosted AI agent gateway",
		Long:  "GoClaw is a self-hosted AI agent gateway that connects Telegram and CLI to LLMs.",
	}

	rootCmd.AddCommand(
		startCmd(),
		chatCmd(),
		statusCmd(),
		versionCmd(),
		onboardCmd(),
		doctorCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func startCmd() *cobra.Command {
	var configPath string
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the GoClaw gateway server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStart(configPath)
		},
	}
	cmd.Flags().StringVarP(&configPath, "config", "c", "", "path to config file")
	return cmd
}

func chatCmd() *cobra.Command {
	var configPath, model string
	cmd := &cobra.Command{
		Use:   "chat [agent]",
		Short: "Start an interactive chat session",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			agentID := "default"
			if len(args) > 0 {
				agentID = args[0]
			}
			return runChat(agentID, configPath, model)
		},
	}
	cmd.Flags().StringVarP(&configPath, "config", "c", "", "path to config file")
	cmd.Flags().StringVarP(&model, "model", "m", "", "override model (e.g. anthropic/claude-opus-4-0-20250514)")
	return cmd
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show gateway and agent status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus()
		},
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("goclaw %s (commit: %s)\n", version, commit)
		},
	}
}

// resolveProviderOpts builds ProviderOptions for a given provider name,
// merging config file settings with environment variables.
// Env vars take precedence over config values.
func resolveProviderOpts(name string, cfg *config.Config) llm.ProviderOptions {
	pcfg := cfg.GetProvider(name)
	opts := llm.ProviderOptions{
		APIKey:  pcfg.APIKey,
		BaseURL: pcfg.BaseURL,
		Kind:    pcfg.Kind,
	}

	// Environment variables override config values.
	// Support both PROVIDER_API_KEY and PROVIDER_AUTH_TOKEN patterns.
	envPrefix := strings.ToUpper(name)
	if v := os.Getenv(envPrefix + "_API_KEY"); v != "" {
		opts.APIKey = v
	}
	if v := os.Getenv(envPrefix + "_AUTH_TOKEN"); v != "" {
		opts.APIKey = v
	}
	if v := os.Getenv(envPrefix + "_BASE_URL"); v != "" {
		opts.BaseURL = v
	}

	return opts
}

// initProviders creates LLM providers from config.
func initProviders(cfg *config.Config) map[string]llm.LLMProvider {
	providers := make(map[string]llm.LLMProvider)

	// Collect provider names from agent configs
	needed := make(map[string]bool)
	for _, a := range cfg.Agents.List {
		provName, _ := llm.ParseProviderModel(a.Model)
		if provName != "" {
			needed[provName] = true
		}
		for _, fb := range a.Fallbacks {
			provName, _ = llm.ParseProviderModel(fb)
			if provName != "" {
				needed[provName] = true
			}
		}
	}

	for name := range needed {
		opts := resolveProviderOpts(name, cfg)

		if opts.APIKey == "" {
			slog.Warn("no API key for provider, skipping", "provider", name)
			continue
		}

		if opts.BaseURL != "" {
			slog.Info("using custom base URL for provider", "provider", name, "base_url", opts.BaseURL)
		}

		p, err := llm.NewProvider(name, opts)
		if err != nil {
			slog.Error("failed to create provider", "provider", name, "error", err)
			continue
		}
		providers[name] = p
	}

	return providers
}

func runStart(configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Ensure data directories exist
	dataDir := config.DefaultDataDir()
	os.MkdirAll(filepath.Join(dataDir, "sessions"), 0o755)
	os.MkdirAll(filepath.Join(dataDir, "memory"), 0o755)
	os.MkdirAll(filepath.Join(dataDir, "skills"), 0o755)

	// Init components
	providers := initProviders(cfg)
	sessionStore := session.NewStore(filepath.Join(dataDir, "sessions"))
	toolReg := tools.NewRegistry()
	tools.RegisterCoreTools(toolReg, "")

	// Init skill loader
	skillLoader := skill.NewLoader()
	skillDirs := []string{filepath.Join(dataDir, "skills")}
	for _, a := range cfg.Agents.List {
		skillDirs = append(skillDirs, filepath.Join(a.Workspace, "skills"))
	}
	if err := skillLoader.LoadFrom(skillDirs...); err != nil {
		slog.Warn("failed to load skills", "error", err)
	}

	// Init memory manager
	var memMgr *memory.Manager
	if cfg.Memory.Enabled {
		memMgr = memory.NewManager(filepath.Join(dataDir, "memory"))
		if err := memMgr.Load(); err != nil {
			slog.Warn("failed to load memory", "error", err)
		}
	}

	// Init WebSocket handler
	wsHandler := gateway.NewWebSocketHandler(providers, toolReg, sessionStore, cfg)

	// Init message router
	fallbackAgent := "default"
	if len(cfg.Agents.List) > 0 {
		fallbackAgent = cfg.Agents.List[0].ID
	}
	msgRouter := router.NewRouter(cfg.Bindings, fallbackAgent)

	// Init channel manager
	chanMgr := gateway.NewChannelManager(msgRouter, providers, toolReg, sessionStore, cfg)
	chanMgr.SetSkills(skillLoader)
	chanMgr.SetMemory(memMgr)

	// Register Telegram channel if configured
	if cfg.Channels.Telegram.Token != "" {
		tgChan := channel.NewTelegramChannel(
			cfg.Channels.Telegram.Token,
			cfg.Security.GroupPolicy.RequireMention,
		)
		chanMgr.Register(tgChan)
		slog.Info("telegram channel registered")
	}

	// Register WhatsApp channel if configured
	waDBPath := cfg.Channels.WhatsApp.DBPath
	if waDBPath == "" {
		// Check if a default WhatsApp DB exists (indicating previous setup)
		defaultDB := filepath.Join(dataDir, "whatsapp.db")
		if _, err := os.Stat(defaultDB); err == nil {
			waDBPath = defaultDB
		}
	}
	if waDBPath != "" {
		waChan := channel.NewWhatsAppChannel(waDBPath)
		chanMgr.Register(waChan)
		slog.Info("whatsapp channel registered")
	}

	// Register send_message tool with channel manager as the sender
	tools.RegisterSendMessage(toolReg, chanMgr)

	// Config hot-reload
	if cfg.Path() != "" {
		watcher, err := config.NewWatcher(cfg.Path(), func(newCfg *config.Config) {
			wsHandler.UpdateConfig(newCfg)
			slog.Info("config hot-reloaded")
		})
		if err == nil {
			watcher.Start()
			defer watcher.Stop()
		} else {
			slog.Warn("config watcher not started", "error", err)
		}
	}

	// Start channel manager
	ctx := context.Background()
	if err := chanMgr.Start(ctx); err != nil {
		return fmt.Errorf("start channel manager: %w", err)
	}

	// Start heartbeat daemon for each agent if enabled
	var heartbeats []*heartbeat.Daemon
	if cfg.Heartbeat.Enabled {
		interval, err := time.ParseDuration(cfg.Heartbeat.Interval)
		if err != nil {
			interval = 30 * time.Minute
		}

		for _, agentCfg := range cfg.Agents.List {
			providerName, modelName := llm.ParseProviderModel(agentCfg.Model)
			provider, ok := providers[providerName]
			if !ok {
				continue
			}

			agentWorkspace := agentCfg.Workspace
			agentID := agentCfg.ID

			agentFn := func(ctx context.Context, prompt string) (string, error) {
				sess, err := sessionStore.Load(agentID, "heartbeat")
				if err != nil {
					return "", err
				}

				hbToolReg := tools.NewRegistry()
				tools.RegisterCoreTools(hbToolReg, agentWorkspace)

				rt := &agent.Runtime{
					LLM:       provider,
					Tools:     hbToolReg,
					Session:   sess,
					Model:     modelName,
					Workspace: agentWorkspace,
					Skills:    skillLoader,
					Memory:    memMgr,
				}
				return rt.RunSync(ctx, prompt, nil)
			}

			daemon := heartbeat.NewDaemon(agentCfg.Workspace, interval, agentFn)
			daemon.Start(ctx)
			heartbeats = append(heartbeats, daemon)
		}
	}

	// Start cron scheduler for agents with cron jobs
	cronScheduler := cron.NewScheduler()
	for _, agentCfg := range cfg.Agents.List {
		for _, cronJob := range agentCfg.Cron {
			providerName, modelName := llm.ParseProviderModel(agentCfg.Model)
			provider, ok := providers[providerName]
			if !ok {
				continue
			}
			agentWorkspace := agentCfg.Workspace
			agentID := agentCfg.ID
			jobPrompt := cronJob.Prompt

			agentFn := func(ctx context.Context, prompt string) (string, error) {
				sess, err := sessionStore.Load(agentID, "cron_"+cronJob.Name)
				if err != nil {
					return "", err
				}
				cronToolReg := tools.NewRegistry()
				tools.RegisterCoreTools(cronToolReg, agentWorkspace)
				rt := &agent.Runtime{
					LLM:       provider,
					Tools:     cronToolReg,
					Session:   sess,
					Model:     modelName,
					Workspace: agentWorkspace,
					Skills:    skillLoader,
					Memory:    memMgr,
				}
				return rt.RunSync(ctx, prompt, nil)
			}

			cronScheduler.Add(cron.Job{
				Name:     cronJob.Name,
				Schedule: cronJob.Schedule,
				Prompt:   jobPrompt,
				AgentFn:  agentFn,
			})
		}
	}
	// Register cron tool so the agent can dynamically schedule jobs.
	// The agentFactory creates a real agent runtime for each dynamic job.
	tools.RegisterCron(toolReg, &cronSchedulerAdapter{
		scheduler: cronScheduler,
		ctx:       ctx,
		agentFactory: func(jobName string) func(context.Context, string) (string, error) {
			return func(ctx context.Context, prompt string) (string, error) {
				// Use the default agent for dynamic cron jobs
				defaultCfg := cfg.Agents.List[0]
				pName, mName := llm.ParseProviderModel(defaultCfg.Model)
				p, ok := providers[pName]
				if !ok {
					return "", fmt.Errorf("provider %q not available", pName)
				}
				cronSess, err := sessionStore.Load(defaultCfg.ID, "cron_"+jobName)
				if err != nil {
					return "", err
				}
				cronToolReg := tools.NewRegistry()
				tools.RegisterCoreTools(cronToolReg, defaultCfg.Workspace)
				rt := &agent.Runtime{
					LLM:       p,
					Tools:     cronToolReg,
					Session:   cronSess,
					Model:     mName,
					Workspace: defaultCfg.Workspace,
					Skills:    skillLoader,
					Memory:    memMgr,
				}
				return rt.RunSync(ctx, prompt, nil)
			}
		},
	})

	if len(cronScheduler.Jobs()) > 0 {
		cronScheduler.Start(ctx)
	}

	// Init metrics
	metrics := gateway.NewMetrics()

	// Start gateway HTTP server
	srv := gateway.NewServer(cfg.Gateway.Host, cfg.Gateway.Port, wsHandler, gateway.ServerOptions{
		AuthToken:      cfg.Gateway.Auth.Token,
		MetricsHandler: metrics.Handler(),
		UIHandler:      gateway.NewUIHandler(cfg, version),
	})

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := srv.Start(); err != nil && err != http.ErrServerClosed {
			slog.Error("gateway error", "error", err)
			os.Exit(1)
		}
	}()

	<-stop
	slog.Info("shutting down gateway...")

	// Stop cron, heartbeats, channels, then HTTP server
	cronScheduler.Stop()
	for _, hb := range heartbeats {
		hb.Stop()
	}
	chanMgr.Stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func runChat(agentID, configPath, modelOverride string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	agentCfg, ok := cfg.GetAgent(agentID)
	if !ok {
		return fmt.Errorf("agent %q not found in config", agentID)
	}

	modelStr := agentCfg.Model
	if modelOverride != "" {
		modelStr = modelOverride
	}

	providerName, modelName := llm.ParseProviderModel(modelStr)

	// If no provider prefix in the model string, inherit from the agent's config
	if providerName == "" {
		providerName, _ = llm.ParseProviderModel(agentCfg.Model)
	}
	// Last resort default
	if providerName == "" {
		providerName = "anthropic"
	}

	opts := resolveProviderOpts(providerName, cfg)
	if opts.APIKey == "" {
		return fmt.Errorf("no API key set for provider %q (set %s_API_KEY or %s_AUTH_TOKEN env var)",
			providerName, strings.ToUpper(providerName), strings.ToUpper(providerName))
	}

	if opts.BaseURL != "" {
		slog.Info("using custom base URL", "provider", providerName, "base_url", opts.BaseURL)
	}

	provider, err := llm.NewProvider(providerName, opts)
	if err != nil {
		return fmt.Errorf("create LLM provider: %w", err)
	}

	// Init session
	dataDir := config.DefaultDataDir()
	os.MkdirAll(filepath.Join(dataDir, "sessions"), 0o755)
	sessionStore := session.NewStore(filepath.Join(dataDir, "sessions"))
	sess, err := sessionStore.Load(agentID, "cli_local")
	if err != nil {
		return fmt.Errorf("load session: %w", err)
	}

	// Init skills
	skillLoader := skill.NewLoader()
	skillLoader.LoadFrom(
		filepath.Join(dataDir, "skills"),
		filepath.Join(agentCfg.Workspace, "skills"),
	)

	// Init memory
	var memMgr *memory.Manager
	if cfg.Memory.Enabled {
		memMgr = memory.NewManager(filepath.Join(dataDir, "memory"))
		memMgr.Load()
	}

	// Ensure workspace exists
	os.MkdirAll(agentCfg.Workspace, 0o755)

	// Init tools
	toolReg := tools.NewRegistry()
	tools.RegisterCoreTools(toolReg, agentCfg.Workspace)

	// Init cron scheduler for chat mode so the agent can use the cron tool
	ctx := context.Background()
	cronScheduler := cron.NewScheduler()

	// Build an agent factory for dynamic cron jobs — each job gets its own
	// session and runtime so it can actually execute the prompt via the LLM.
	agentFactory := func(jobName string) func(context.Context, string) (string, error) {
		return func(ctx context.Context, prompt string) (string, error) {
			cronSess, err := sessionStore.Load(agentID, "cron_"+jobName)
			if err != nil {
				return "", err
			}
			cronToolReg := tools.NewRegistry()
			tools.RegisterCoreTools(cronToolReg, agentCfg.Workspace)
			cronRT := &agent.Runtime{
				LLM:       provider,
				Tools:     cronToolReg,
				Session:   cronSess,
				Model:     modelName,
				Workspace: agentCfg.Workspace,
				Skills:    skillLoader,
				Memory:    memMgr,
			}
			return cronRT.RunSync(ctx, prompt, nil)
		}
	}

	// Register static cron jobs from config
	for _, cronJob := range agentCfg.Cron {
		jobPrompt := cronJob.Prompt
		jobName := cronJob.Name
		cronScheduler.Add(cron.Job{
			Name:     cronJob.Name,
			Schedule: cronJob.Schedule,
			Prompt:   jobPrompt,
			AgentFn:  agentFactory(jobName),
		})
	}

	// Register cron tool so the agent can dynamically schedule jobs.
	// In chat mode, print cron job results to the terminal.
	tools.RegisterCron(toolReg, &cronSchedulerAdapter{
		scheduler:    cronScheduler,
		ctx:          ctx,
		agentFactory: agentFactory,
		outputFn: func(jobName, response string) {
			fmt.Printf("\n[cron: %s]\n%s\n\n> ", jobName, response)
		},
	})

	// Apply tool policy from agent config
	policy := tools.Policy{
		Allow: agentCfg.Tools.Allow,
		Deny:  agentCfg.Tools.Deny,
	}
	var toolExecutor tools.Executor = toolReg
	if len(policy.Allow) > 0 || len(policy.Deny) > 0 {
		toolExecutor = tools.NewFilteredRegistry(toolReg, policy)
	}

	// Start cron scheduler if there are any static jobs
	if len(cronScheduler.Jobs()) > 0 {
		cronScheduler.Start(ctx)
	}

	rt := &agent.Runtime{
		LLM:       provider,
		Tools:     toolExecutor,
		Session:   sess,
		Model:     modelName,
		Workspace: agentCfg.Workspace,
		Skills:    skillLoader,
		Memory:    memMgr,
	}

	fmt.Printf("GoClaw chat — agent %q (model: %s)\n", agentID, modelStr)
	fmt.Println("Type /quit to exit.")
	fmt.Println()

	// Interactive REPL

	for {
		fmt.Print("> ")
		var input string
		scanner := make([]byte, 0, 4096)
		buf := make([]byte, 1)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil || n == 0 {
				return nil
			}
			if buf[0] == '\n' {
				break
			}
			scanner = append(scanner, buf[0])
		}
		input = strings.TrimSpace(string(scanner))

		if input == "" {
			continue
		}
		if input == "/quit" || input == "/exit" {
			fmt.Println("Goodbye!")
			return nil
		}

		// Extract image paths from input (supports drag-and-drop)
		text, images := extractImagesFromInput(input)
		if len(images) > 0 {
			fmt.Printf("\033[90m[attached %d image(s)]\033[0m\n", len(images))
		}

		events, err := rt.Run(ctx, text, images)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		var responseText strings.Builder
		for event := range events {
			switch event.Type {
			case agent.EventTextDelta:
				responseText.WriteString(event.Text)
			case agent.EventToolCallStart:
				fmt.Printf("\n\033[36m[tool: %s]\033[0m\n", event.ToolCall.Name)
			case agent.EventToolResult:
				if event.Result.Error != "" {
					fmt.Printf("\033[31m[error: %s]\033[0m\n", event.Result.Error)
				}
			case agent.EventError:
				fmt.Printf("\n\033[31mError: %v\033[0m\n", event.Error)
			case agent.EventDone:
				// Render accumulated markdown
				if responseText.Len() > 0 {
					rendered, err := glamour.Render(responseText.String(), "dark")
					if err != nil {
						fmt.Print(responseText.String())
					} else {
						fmt.Print(rendered)
					}
				}
			}
		}
	}
}

// imageExtensions is the set of file extensions treated as images.
var imageExtensions = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".gif":  "image/gif",
	".webp": "image/webp",
	".bmp":  "image/bmp",
}

// extractImagesFromInput scans the input line for image file paths,
// reads them, and returns the cleaned text plus image contents.
// Supports:
//   - bare paths:        /path/to/image.png
//   - single-quoted paths (drag-and-drop on macOS): '/path/to/my image.png'
//   - backslash-escaped spaces: /path/to/my\ image.png
//   - tilde home dir:    ~/Downloads/image.png
func extractImagesFromInput(input string) (string, []llm.ImageContent) {
	var images []llm.ImageContent
	cleaned := input

	// Pass 1: extract single-quoted paths (drag-and-drop with spaces)
	for {
		start := strings.Index(cleaned, "'")
		if start == -1 {
			break
		}
		end := strings.Index(cleaned[start+1:], "'")
		if end == -1 {
			break
		}
		end += start + 1 // absolute index of closing quote

		quoted := cleaned[start+1 : end]
		path := expandHome(quoted)

		if img, ok := tryReadImage(path); ok {
			images = append(images, img)
			// Remove the quoted path from the text
			cleaned = strings.TrimSpace(cleaned[:start] + cleaned[end+1:])
			continue
		}
		// Not an image, skip past this quoted section to avoid infinite loop
		break
	}

	// Pass 2: extract bare paths and paths with backslash-escaped spaces
	words := splitRespectingEscapes(cleaned)
	var remaining []string
	for _, word := range words {
		// Unescape backslash-spaces
		unescaped := strings.ReplaceAll(word, "\\ ", " ")
		path := expandHome(unescaped)

		if img, ok := tryReadImage(path); ok {
			images = append(images, img)
			continue
		}
		remaining = append(remaining, word)
	}

	text := strings.Join(remaining, " ")
	if text == "" && len(images) > 0 {
		text = "What's in this image?"
	}
	return text, images
}

// tryReadImage checks if a path points to a readable image file and returns its content.
func tryReadImage(path string) (llm.ImageContent, bool) {
	ext := strings.ToLower(filepath.Ext(path))
	mimeType, isImage := imageExtensions[ext]
	if !isImage {
		return llm.ImageContent{}, false
	}

	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return llm.ImageContent{}, false
	}

	// Limit to 10MB
	if info.Size() > 10*1024*1024 {
		slog.Warn("image too large, skipping", "path", path, "size", info.Size())
		return llm.ImageContent{}, false
	}

	data, err := os.ReadFile(path)
	if err != nil {
		slog.Warn("failed to read image", "path", path, "error", err)
		return llm.ImageContent{}, false
	}

	return llm.ImageContent{MimeType: mimeType, Data: data}, true
}

// expandHome expands a leading ~ to the user's home directory.
func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// splitRespectingEscapes splits a string on spaces, but treats "\ " as a literal space
// within the same token (for drag-and-drop paths with escaped spaces).
func splitRespectingEscapes(s string) []string {
	var tokens []string
	var current strings.Builder
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		if runes[i] == '\\' && i+1 < len(runes) && runes[i+1] == ' ' {
			current.WriteString("\\ ")
			i++ // skip the space
		} else if runes[i] == ' ' {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		} else {
			current.WriteRune(runes[i])
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}

func runStatus() error {
	// Connect to running gateway via WebSocket
	conn, _, err := websocket.DefaultDialer.Dial("ws://127.0.0.1:18789/ws", nil)
	if err != nil {
		return fmt.Errorf("cannot connect to gateway (is it running?): %w", err)
	}
	defer conn.Close()

	// Send agent.status request
	req := gateway.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "agent.status",
		ID:      1,
	}
	if err := conn.WriteJSON(req); err != nil {
		return fmt.Errorf("write request: %w", err)
	}

	// Read response
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	var resp gateway.JSONRPCResponse
	if err := json.Unmarshal(msg, &resp); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	// Pretty-print
	out, _ := json.MarshalIndent(resp.Result, "", "  ")
	fmt.Println("Gateway status:")
	fmt.Println(string(out))
	return nil
}

func onboardCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "onboard",
		Short: "Interactive setup wizard for GoClaw",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runOnboard()
		},
	}
}

func runOnboard() error {
	reader := bufio.NewReader(os.Stdin)
	prompt := func(question, defaultVal string) string {
		if defaultVal != "" {
			fmt.Printf("%s [%s]: ", question, defaultVal)
		} else {
			fmt.Printf("%s: ", question)
		}
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(answer)
		if answer == "" {
			return defaultVal
		}
		return answer
	}

	promptSecret := func(question string) string {
		fmt.Printf("%s: ", question)
		answer, _ := reader.ReadString('\n')
		return strings.TrimSpace(answer)
	}

	choose := func(question string, options []string, defaultIdx int) int {
		fmt.Println(question)
		for i, opt := range options {
			marker := "  "
			if i == defaultIdx {
				marker = "* "
			}
			fmt.Printf("  %s%d) %s\n", marker, i+1, opt)
		}
		for {
			choice := prompt("Choose", fmt.Sprintf("%d", defaultIdx+1))
			var idx int
			if _, err := fmt.Sscanf(choice, "%d", &idx); err == nil && idx >= 1 && idx <= len(options) {
				return idx - 1
			}
			fmt.Println("Invalid choice, try again.")
		}
	}

	// Welcome
	fmt.Println()
	fmt.Println("Welcome to GoClaw!")
	fmt.Println("==================")
	fmt.Println()
	fmt.Println("GoClaw is a self-hosted AI agent gateway that connects")
	fmt.Println("Telegram and CLI to LLMs like Claude, GPT, and more.")
	fmt.Println()
	fmt.Println("This wizard will help you set up your configuration.")
	fmt.Println()

	cfg := config.DefaultConfig()

	// Step 1: LLM Provider
	providerIdx := choose(
		"Which LLM provider do you want to use?",
		[]string{
			"Anthropic (Claude)",
			"OpenAI (GPT)",
			"Ollama (local models)",
			"Custom/LiteLLM (OpenAI-compatible endpoint)",
		},
		0,
	)

	providerName := ""
	providerKind := ""
	var baseURL string

	switch providerIdx {
	case 0:
		providerName = "anthropic"
		providerKind = "anthropic"
	case 1:
		providerName = "openai"
		providerKind = "openai"
	case 2:
		providerName = "ollama"
		providerKind = "openai-compatible"
		baseURL = prompt("Ollama base URL", "http://localhost:11434/v1")
	case 3:
		providerName = prompt("Provider name", "litellm")
		providerKind = "openai-compatible"
		baseURL = prompt("Base URL", "http://localhost:4000/v1")
	}

	// Step 2: API Key
	apiKey := ""
	if providerIdx != 2 { // Ollama typically doesn't need an API key
		apiKey = promptSecret(fmt.Sprintf("Enter your %s API key", providerName))
		if apiKey == "" && providerIdx != 2 {
			fmt.Println("Warning: No API key provided. You can set it later via environment variable or config file.")
		}
	}

	// Test connectivity
	if apiKey != "" || providerIdx == 2 {
		fmt.Print("Testing connection... ")
		testOpts := llm.ProviderOptions{
			APIKey:  apiKey,
			BaseURL: baseURL,
			Kind:    providerKind,
		}
		p, err := llm.NewProvider(providerName, testOpts)
		if err != nil {
			fmt.Printf("failed to create provider: %v\n", err)
		} else {
			models := p.Models()
			if len(models) > 0 {
				fmt.Printf("OK (%d models available)\n", len(models))
			} else {
				fmt.Println("OK (connected)")
			}
		}
	}

	// Step 3: Model selection
	fmt.Println()
	var modelStr string
	switch providerIdx {
	case 0:
		modelIdx := choose("Which Claude model?", []string{
			"claude-sonnet-4-5-20250514 (recommended)",
			"claude-opus-4-0-20250514",
			"claude-haiku-3-5-20241022",
		}, 0)
		models := []string{
			"anthropic/claude-sonnet-4-5-20250514",
			"anthropic/claude-opus-4-0-20250514",
			"anthropic/claude-haiku-3-5-20241022",
		}
		modelStr = models[modelIdx]
	case 1:
		modelIdx := choose("Which GPT model?", []string{
			"gpt-4o (recommended)",
			"gpt-4o-mini",
			"gpt-4-turbo",
		}, 0)
		models := []string{
			"openai/gpt-4o",
			"openai/gpt-4o-mini",
			"openai/gpt-4-turbo",
		}
		modelStr = models[modelIdx]
	default:
		modelStr = prompt("Model name (provider/model format)", providerName+"/default")
	}

	// Update config
	cfg.Providers[providerName] = config.ProviderConfig{
		Kind:    providerKind,
		BaseURL: baseURL,
		APIKey:  apiKey,
	}
	cfg.Agents.List[0].Model = modelStr

	// Step 4: Telegram setup (optional)
	fmt.Println()
	setupTelegram := prompt("Set up Telegram bot? (y/n)", "n")
	if strings.ToLower(setupTelegram) == "y" {
		fmt.Println()
		fmt.Println("To create a Telegram bot:")
		fmt.Println("  1. Open Telegram and search for @BotFather")
		fmt.Println("  2. Send /newbot and follow the instructions")
		fmt.Println("  3. Copy the bot token provided by BotFather")
		fmt.Println()

		token := promptSecret("Enter your Telegram bot token")
		if token != "" {
			// Test the token
			fmt.Print("Testing bot token... ")
			tgChan := channel.NewTelegramChannel(token, true)
			testCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err := tgChan.Connect(testCtx)
			cancel()
			if err != nil {
				fmt.Printf("failed: %v\n", err)
				fmt.Println("Token saved anyway. You can fix it later in the config file.")
			} else {
				fmt.Printf("OK (bot: @%s)\n", tgChan.BotUsername())
				tgChan.Disconnect()
			}

			cfg.Channels.Telegram.Token = token
			cfg.Channels.Telegram.Mode = "polling"

			// Add default telegram binding
			cfg.Bindings = append(cfg.Bindings, config.Binding{
				AgentID: "default",
				Match:   config.BindingMatch{Channel: "telegram"},
			})
		}
	}

	// Step 5: WhatsApp setup (optional)
	fmt.Println()
	setupWhatsApp := prompt("Set up WhatsApp? (y/n)", "n")
	if strings.ToLower(setupWhatsApp) == "y" {
		fmt.Println()
		fmt.Println("WhatsApp uses the Web multidevice protocol.")
		fmt.Println("On first 'goclaw start', a QR code will appear in the terminal.")
		fmt.Println("Scan it with WhatsApp on your phone to link this device.")
		fmt.Println()

		waDBPath := prompt("WhatsApp database path", filepath.Join(config.DefaultDataDir(), "whatsapp.db"))
		cfg.Channels.WhatsApp.DBPath = waDBPath

		phoneNumber := prompt("Phone number (for display only, optional)", "")
		if phoneNumber != "" {
			cfg.Channels.WhatsApp.PhoneNumber = phoneNumber
		}

		// Add default whatsapp binding
		cfg.Bindings = append(cfg.Bindings, config.Binding{
			AgentID: "default",
			Match:   config.BindingMatch{Channel: "whatsapp"},
		})

		fmt.Println("WhatsApp configured. QR code will appear on first 'goclaw start'.")
	}

	// Step 6: Write config
	fmt.Println()
	dataDir := config.DefaultDataDir()
	configPath := config.DefaultConfigPath()

	os.MkdirAll(dataDir, 0o755)

	// Check if config exists
	if _, err := os.Stat(configPath); err == nil {
		overwrite := prompt("Config file already exists. Overwrite? (y/n)", "n")
		if strings.ToLower(overwrite) != "y" {
			fmt.Println("Setup cancelled. Existing config preserved.")
			return nil
		}
	}

	// Marshal config to JSON (pretty-printed with comments)
	configJSON, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, configJSON, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	fmt.Printf("Config written to %s\n", configPath)

	// Step 6: Create workspace
	workspace := cfg.Agents.List[0].Workspace
	os.MkdirAll(workspace, 0o755)

	identityPath := filepath.Join(workspace, "IDENTITY.md")
	if _, err := os.Stat(identityPath); os.IsNotExist(err) {
		identity := `You are a helpful AI assistant called GoClaw. You can read files, write files, edit files, execute bash commands on the user's machine, fetch web pages, and search the web. Be concise and helpful. When executing tasks, think step by step and use your tools to accomplish the user's goals.`
		os.WriteFile(identityPath, []byte(identity), 0o644)
		fmt.Printf("Created workspace at %s\n", workspace)
	}

	// Done
	fmt.Println()
	fmt.Println("Setup complete! Next steps:")
	fmt.Println()
	fmt.Println("  goclaw start   — Start the gateway server")
	fmt.Println("  goclaw chat    — Start an interactive chat session")
	fmt.Println()
	if cfg.Channels.Telegram.Token != "" {
		fmt.Println("  Your Telegram bot is configured and will start with 'goclaw start'.")
		fmt.Println()
	}
	if cfg.Channels.WhatsApp.DBPath != "" {
		fmt.Println("  WhatsApp is configured. A QR code will appear on first 'goclaw start'.")
		fmt.Println()
	}

	return nil
}

func doctorCmd() *cobra.Command {
	var configPath string
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Run diagnostic checks on your GoClaw setup",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor(configPath)
		},
	}
	cmd.Flags().StringVarP(&configPath, "config", "c", "", "path to config file")
	return cmd
}

func runDoctor(configPath string) error {
	pass := 0
	fail := 0
	warn := 0

	check := func(name string, fn func() (string, error)) {
		result, err := fn()
		if err != nil {
			fmt.Printf("  FAIL  %s: %v\n", name, err)
			fail++
		} else if result != "" {
			fmt.Printf("  WARN  %s: %s\n", name, result)
			warn++
		} else {
			fmt.Printf("  OK    %s\n", name)
			pass++
		}
	}

	fmt.Println("GoClaw Doctor")
	fmt.Println("=============")
	fmt.Println()

	// Check 1: Config file
	fmt.Println("Configuration:")
	var cfg *config.Config
	check("Config file", func() (string, error) {
		var err error
		cfg, err = config.Load(configPath)
		if err != nil {
			return "", err
		}
		if cfg.Path() != "" {
			if _, err := os.Stat(cfg.Path()); os.IsNotExist(err) {
				return "using defaults (no config file found)", nil
			}
		}
		return "", nil
	})

	if cfg == nil {
		fmt.Println("\nCannot continue without a valid config.")
		return nil
	}

	// Check 2: Data directory
	fmt.Println("\nData directories:")
	dataDir := config.DefaultDataDir()
	for _, sub := range []string{"", "sessions", "memory", "skills"} {
		dir := filepath.Join(dataDir, sub)
		name := dir
		if sub == "" {
			name = dataDir
		}
		check(name, func() (string, error) {
			info, err := os.Stat(dir)
			if os.IsNotExist(err) {
				return "directory does not exist (will be created on start)", nil
			}
			if err != nil {
				return "", err
			}
			if !info.IsDir() {
				return "", fmt.Errorf("path exists but is not a directory")
			}
			return "", nil
		})
	}

	// Check 3: Agent workspaces
	fmt.Println("\nAgent workspaces:")
	for _, a := range cfg.Agents.List {
		agentCfg := a
		check(fmt.Sprintf("Agent %q workspace (%s)", agentCfg.ID, agentCfg.Workspace), func() (string, error) {
			if _, err := os.Stat(agentCfg.Workspace); os.IsNotExist(err) {
				return "workspace does not exist (will be created on start)", nil
			}
			identityPath := filepath.Join(agentCfg.Workspace, "IDENTITY.md")
			if _, err := os.Stat(identityPath); os.IsNotExist(err) {
				return "no IDENTITY.md found (default identity will be used)", nil
			}
			return "", nil
		})
	}

	// Check 4: LLM providers
	fmt.Println("\nLLM providers:")
	for _, a := range cfg.Agents.List {
		agentCfg := a
		check(fmt.Sprintf("Provider for agent %q (%s)", agentCfg.ID, agentCfg.Model), func() (string, error) {
			provName, _ := llm.ParseProviderModel(agentCfg.Model)
			if provName == "" {
				return "", fmt.Errorf("no provider prefix in model name")
			}
			opts := resolveProviderOpts(provName, cfg)
			if opts.APIKey == "" {
				return "", fmt.Errorf("no API key configured (set %s_API_KEY env var or add to config)",
					strings.ToUpper(provName))
			}
			_, err := llm.NewProvider(provName, opts)
			if err != nil {
				return "", fmt.Errorf("failed to create provider: %v", err)
			}
			return "", nil
		})
	}

	// Check 5: Telegram
	fmt.Println("\nChannels:")
	check("Telegram", func() (string, error) {
		if cfg.Channels.Telegram.Token == "" {
			return "not configured (optional)", nil
		}
		return "token configured", nil
	})

	check("WhatsApp", func() (string, error) {
		if cfg.Channels.WhatsApp.DBPath == "" {
			return "not configured (optional)", nil
		}
		if _, err := os.Stat(cfg.Channels.WhatsApp.DBPath); os.IsNotExist(err) {
			return "database not found (will be created on first connect)", nil
		}
		return "database found", nil
	})

	// Check 6: Gateway port
	fmt.Println("\nGateway:")
	check(fmt.Sprintf("Port %d", cfg.Gateway.Port), func() (string, error) {
		addr := net.JoinHostPort(cfg.Gateway.Host, fmt.Sprintf("%d", cfg.Gateway.Port))
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err == nil {
			conn.Close()
			return "port is in use (gateway may already be running)", nil
		}
		return "", nil
	})

	check("Auth token", func() (string, error) {
		if cfg.Gateway.Auth.Token == "" {
			return "no auth token configured (API is unprotected)", nil
		}
		return "", nil
	})

	// Summary
	fmt.Println()
	fmt.Printf("Results: %d passed, %d warnings, %d failed\n", pass, warn, fail)
	if fail > 0 {
		fmt.Println("\nFix the failures above before running 'goclaw start'.")
	} else if warn > 0 {
		fmt.Println("\nSetup looks good with minor warnings.")
	} else {
		fmt.Println("\nAll checks passed!")
	}

	return nil
}

// cronSchedulerAdapter adapts cron.Scheduler to the tools.JobScheduler interface.
type cronSchedulerAdapter struct {
	scheduler    *cron.Scheduler
	ctx          context.Context
	agentFactory func(name string) func(context.Context, string) (string, error) // creates an AgentFn for dynamic jobs
	outputFn     cron.OutputFunc                                                  // optional: called with results
}

func (a *cronSchedulerAdapter) AddJob(name, schedule, prompt string) error {
	var agentFn func(context.Context, string) (string, error)
	if a.agentFactory != nil {
		agentFn = a.agentFactory(name)
	} else {
		agentFn = func(ctx context.Context, p string) (string, error) {
			slog.Info("dynamic cron job executed (no agent)", "name", name)
			return "OK", nil
		}
	}

	err := a.scheduler.Add(cron.Job{
		Name:     name,
		Schedule: schedule,
		Prompt:   prompt,
		AgentFn:  agentFn,
		OutputFn: a.outputFn,
	})
	if err != nil {
		return err
	}
	// Start the scheduler if it has jobs and isn't already running
	a.scheduler.Start(a.ctx)
	return nil
}

func (a *cronSchedulerAdapter) ListJobs() []tools.JobInfo {
	jobs := a.scheduler.Jobs()
	infos := make([]tools.JobInfo, len(jobs))
	for i, j := range jobs {
		infos[i] = tools.JobInfo{
			Name:     j.Name,
			Schedule: j.Schedule,
			Prompt:   j.Prompt,
		}
	}
	return infos
}
