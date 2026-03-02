// Package startup provides shared gateway startup logic used by both the
// CLI (cmd/goclaw) and the menu bar app (cmd/goclaw-app).
package startup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

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

// Result holds the running gateway components.
type Result struct {
	Server    *gateway.Server
	Config    *config.Config
	Cleanup   func() // call to gracefully shut down everything
}

// ResolveProviderOpts builds ProviderOptions for a given provider name,
// merging config file settings with environment variables.
// Env vars take precedence over config values.
func ResolveProviderOpts(name string, cfg *config.Config) llm.ProviderOptions {
	pcfg := cfg.GetProvider(name)
	opts := llm.ProviderOptions{
		APIKey:  pcfg.APIKey,
		BaseURL: pcfg.BaseURL,
		Kind:    pcfg.Kind,
	}

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

// InitProviders creates LLM providers from config.
func InitProviders(cfg *config.Config) map[string]llm.LLMProvider {
	providers := make(map[string]llm.LLMProvider)

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
		opts := ResolveProviderOpts(name, cfg)

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

// CronSchedulerAdapter adapts cron.Scheduler to the tools.JobScheduler interface.
type CronSchedulerAdapter struct {
	Scheduler    *cron.Scheduler
	Ctx          context.Context
	AgentFactory func(name string) func(context.Context, string) (string, error)
	OutputFn     cron.OutputFunc
}

func (a *CronSchedulerAdapter) AddJob(name, schedule, prompt string) error {
	var agentFn func(context.Context, string) (string, error)
	if a.AgentFactory != nil {
		agentFn = a.AgentFactory(name)
	} else {
		agentFn = func(ctx context.Context, p string) (string, error) {
			slog.Info("dynamic cron job executed (no agent)", "name", name)
			return "OK", nil
		}
	}

	err := a.Scheduler.Add(cron.Job{
		Name:     name,
		Schedule: schedule,
		Prompt:   prompt,
		AgentFn:  agentFn,
		OutputFn: a.OutputFn,
	})
	if err != nil {
		return err
	}
	a.Scheduler.Start(a.Ctx)
	return nil
}

func (a *CronSchedulerAdapter) RemoveJob(name string) error {
	return a.Scheduler.Remove(name)
}

func (a *CronSchedulerAdapter) ListJobs() []tools.JobInfo {
	jobs := a.Scheduler.Jobs()
	infos := make([]tools.JobInfo, len(jobs))
	for i, j := range jobs {
		infos[i] = tools.JobInfo{
			Name:     j.Name,
			Schedule: j.Schedule,
			Prompt:   j.Prompt,
			Paused:   j.Paused,
		}
	}
	return infos
}

func (a *CronSchedulerAdapter) PauseJob(name string) error {
	return a.Scheduler.Pause(name)
}

func (a *CronSchedulerAdapter) ResumeJob(name string) error {
	return a.Scheduler.Resume(name)
}

func (a *CronSchedulerAdapter) UpdateJobSchedule(name, schedule string) error {
	return a.Scheduler.UpdateSchedule(name, schedule)
}

// Options configures gateway startup behavior.
type Options struct {
	// ConnectTimeout is the per-channel connect timeout. Zero means no
	// timeout (channels block until connected). Set this for headless
	// environments where interactive channel setup is not possible.
	ConnectTimeout time.Duration
}

// StartGateway starts the full gateway server and returns the result.
// The caller is responsible for calling Result.Cleanup() on shutdown and
// starting the HTTP server via Result.Server.Start() in a goroutine.
func StartGateway(configPath, version string, opts ...Options) (*Result, error) {
	var opt Options
	if len(opts) > 0 {
		opt = opts[0]
	}
	// Install a tee log handler that captures entries for the /logs page
	// while forwarding to a stderr handler. We create a fresh TextHandler
	// rather than using slog.Default().Handler() because in Go 1.22+ the
	// default handler routes through log.Logger which routes back through
	// slog.Default(), creating a deadlock when LogBuffer replaces it.
	logBuf := gateway.NewLogBuffer(2000, slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(slog.New(logBuf))

	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	// Ensure data directories exist
	dataDir := config.DefaultDataDir()
	os.MkdirAll(filepath.Join(dataDir, "sessions"), 0o755)
	os.MkdirAll(filepath.Join(dataDir, "memory"), 0o755)
	os.MkdirAll(filepath.Join(dataDir, "skills"), 0o755)

	// Init components
	providers := InitProviders(cfg)
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
	chanMgr := gateway.NewChannelManager(msgRouter, providers, toolReg, sessionStore, cfg, cfg.Security.DMPolicy.UnknownSenders)
	chanMgr.SetSkills(skillLoader)
	chanMgr.SetMemory(memMgr)
	if opt.ConnectTimeout > 0 {
		chanMgr.SetConnectTimeout(opt.ConnectTimeout)
	}

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
		defaultDB := filepath.Join(dataDir, "whatsapp.db")
		if _, err := os.Stat(defaultDB); err == nil {
			waDBPath = defaultDB
		}
	}
	if waDBPath != "" {
		waChan := channel.NewWhatsAppChannel(waDBPath, cfg.Channels.WhatsApp.AllowedSenders)
		chanMgr.Register(waChan)
		slog.Info("whatsapp channel registered")
	}

	// Register send_message tool with channel manager as the sender
	tools.RegisterSendMessage(toolReg, chanMgr)

	// Register ask_agent tool for inter-agent delegation
	agentRunner := gateway.NewAgentRunner(providers, cfg, sessionStore)
	agentRunner.SetSender(chanMgr)
	agentRunner.SetSkills(skillLoader)
	agentRunner.SetMemory(memMgr)
	tools.RegisterAskAgent(toolReg, agentRunner)

	// Config hot-reload
	var configWatcher *config.Watcher
	if cfg.Path() != "" {
		watcher, err := config.NewWatcher(cfg.Path(), func(newCfg *config.Config) {
			wsHandler.UpdateConfig(newCfg)
			slog.Info("config hot-reloaded")
		})
		if err == nil {
			watcher.Start()
			configWatcher = watcher
		} else {
			slog.Warn("config watcher not started", "error", err)
		}
	}

	// Start channel manager
	ctx := context.Background()
	if err := chanMgr.Start(ctx); err != nil {
		return nil, fmt.Errorf("start channel manager: %w", err)
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
			agentMaxTurns := agentCfg.MaxTurns

			agentSystemPrompt := agentCfg.SystemPrompt
			agentName := agentCfg.Name
			agentFn := func(ctx context.Context, prompt string) (string, error) {
				sess := session.NewSession(agentID, "heartbeat")
				hbToolReg := tools.NewRegistry()
				tools.RegisterCoreTools(hbToolReg, agentWorkspace)
				tools.RegisterSendMessage(hbToolReg, chanMgr)

				rt := &agent.Runtime{
					LLM:          provider,
					Tools:        hbToolReg,
					Session:      sess,
					AgentID:      agentID,
					AgentName:    agentName,
					Model:        modelName,
					Workspace:    agentWorkspace,
					MaxTurns:     agentMaxTurns,
					SystemPrompt: agentSystemPrompt,
					Skills:       skillLoader,
					Memory:       memMgr,
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
			agentMaxTurns := agentCfg.MaxTurns
			jobPrompt := cronJob.Prompt

			agentSystemPrompt := agentCfg.SystemPrompt
			agentName := agentCfg.Name
			agentFn := func(ctx context.Context, prompt string) (string, error) {
				sess := session.NewSession(agentID, "cron_"+cronJob.Name)
				cronToolReg := tools.NewRegistry()
				tools.RegisterCoreTools(cronToolReg, agentWorkspace)
				tools.RegisterSendMessage(cronToolReg, chanMgr)
				rt := &agent.Runtime{
					LLM:          provider,
					Tools:        cronToolReg,
					Session:      sess,
					AgentID:      agentID,
					AgentName:    agentName,
					Model:        modelName,
					Workspace:    agentWorkspace,
					MaxTurns:     agentMaxTurns,
					SystemPrompt: agentSystemPrompt,
					Skills:       skillLoader,
					Memory:       memMgr,
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

	cronAdapter := &CronSchedulerAdapter{
		Scheduler: cronScheduler,
		Ctx:       ctx,
		AgentFactory: func(jobName string) func(context.Context, string) (string, error) {
			return func(ctx context.Context, prompt string) (string, error) {
				defaultCfg := cfg.Agents.List[0]
				pName, mName := llm.ParseProviderModel(defaultCfg.Model)
				p, ok := providers[pName]
				if !ok {
					return "", fmt.Errorf("provider %q not available", pName)
				}
				cronSess := session.NewSession(defaultCfg.ID, "cron_"+jobName)
				cronToolReg := tools.NewRegistry()
				tools.RegisterCoreTools(cronToolReg, defaultCfg.Workspace)
				tools.RegisterSendMessage(cronToolReg, chanMgr)
				rt := &agent.Runtime{
					LLM:          p,
					Tools:        cronToolReg,
					Session:      cronSess,
					AgentID:      defaultCfg.ID,
					AgentName:    defaultCfg.Name,
					Model:        mName,
					Workspace:    defaultCfg.Workspace,
					MaxTurns:     defaultCfg.MaxTurns,
					SystemPrompt: defaultCfg.SystemPrompt,
					Skills:       skillLoader,
					Memory:       memMgr,
				}
				return rt.RunSync(ctx, prompt, nil)
			}
		},
	}
	tools.RegisterCron(toolReg, cronAdapter)
	wsHandler.SetJobScheduler(cronAdapter)

	if len(cronScheduler.Jobs()) > 0 {
		cronScheduler.Start(ctx)
	}

	// Init metrics
	metrics := gateway.NewMetrics()

	// Start gateway HTTP server
	port := cfg.Gateway.Port
	srv := gateway.NewServer(cfg.Gateway.Host, port, wsHandler, gateway.ServerOptions{
		AuthToken:      cfg.Gateway.Auth.Token,
		MetricsHandler: metrics.Handler(),
		UIHandler:      gateway.NewUIHandler(cfg, version),
		ChatHandler:    gateway.NewChatHandler(port),
		JobsHandler:    gateway.NewJobsHandler(port),
		LogBuffer:      logBuf,
	})

	cleanup := func() {
		cronScheduler.Stop()
		for _, hb := range heartbeats {
			hb.Stop()
		}
		chanMgr.Stop()
		if configWatcher != nil {
			configWatcher.Stop()
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}

	return &Result{
		Server:  srv,
		Config:  cfg,
		Cleanup: cleanup,
	}, nil
}
