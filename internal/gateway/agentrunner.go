package gateway

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/sausheong/goclaw/internal/agent"
	"github.com/sausheong/goclaw/internal/config"
	"github.com/sausheong/goclaw/internal/llm"
	"github.com/sausheong/goclaw/internal/memory"
	"github.com/sausheong/goclaw/internal/session"
	"github.com/sausheong/goclaw/internal/skill"
	"github.com/sausheong/goclaw/internal/tools"
)

// AgentRunnerImpl implements tools.AgentRunner by constructing a fresh
// agent runtime for the target agent and running it synchronously.
// The delegated agent gets core tools only — ask_agent is NOT registered
// to prevent infinite recursion.
type AgentRunnerImpl struct {
	providers    map[string]llm.LLMProvider
	config       *config.Config
	sessionStore *session.Store
	sender       tools.MessageSender // optional: for send_message in delegated agents
	skills       *skill.Loader
	memory       *memory.Manager
}

// NewAgentRunner creates an AgentRunnerImpl.
func NewAgentRunner(
	providers map[string]llm.LLMProvider,
	cfg *config.Config,
	sessionStore *session.Store,
) *AgentRunnerImpl {
	return &AgentRunnerImpl{
		providers:    providers,
		config:       cfg,
		sessionStore: sessionStore,
	}
}

// SetSender sets the message sender for delegated agents.
func (r *AgentRunnerImpl) SetSender(sender tools.MessageSender) {
	r.sender = sender
}

// SetSkills sets the skill loader for delegated agents.
func (r *AgentRunnerImpl) SetSkills(skills *skill.Loader) {
	r.skills = skills
}

// SetMemory sets the memory manager for delegated agents.
func (r *AgentRunnerImpl) SetMemory(mem *memory.Manager) {
	r.memory = mem
}

// RunAgent delegates a task to the specified agent and returns the text response.
func (r *AgentRunnerImpl) RunAgent(ctx context.Context, agentID, prompt string) (string, error) {
	agentCfg, ok := r.config.GetAgent(agentID)
	if !ok {
		return "", fmt.Errorf("agent %q not found", agentID)
	}

	providerName, modelName := llm.ParseProviderModel(agentCfg.Model)
	provider, ok := r.providers[providerName]
	if !ok {
		return "", fmt.Errorf("provider %q not available for agent %q", providerName, agentID)
	}

	// Build a fresh tool registry with core tools only — no ask_agent
	// to prevent infinite delegation recursion.
	delegateToolReg := tools.NewRegistry()
	tools.RegisterCoreTools(delegateToolReg, agentCfg.Workspace)

	if r.sender != nil {
		tools.RegisterSendMessage(delegateToolReg, r.sender)
	}

	// Apply the target agent's tool policy
	var executor tools.Executor = delegateToolReg
	if len(agentCfg.Tools.Allow) > 0 || len(agentCfg.Tools.Deny) > 0 {
		executor = tools.NewFilteredRegistry(delegateToolReg, tools.Policy{
			Allow: agentCfg.Tools.Allow,
			Deny:  agentCfg.Tools.Deny,
		})
	}

	// Use a dedicated session so delegated work doesn't pollute channel sessions
	sess := session.NewSession(agentID, fmt.Sprintf("delegate_%s", agentID))

	rt := &agent.Runtime{
		LLM:          provider,
		Tools:        executor,
		Session:      sess,
		AgentID:      agentCfg.ID,
		AgentName:    agentCfg.Name,
		Model:        modelName,
		Workspace:    agentCfg.Workspace,
		MaxTurns:     agentCfg.MaxTurns,
		SystemPrompt: agentCfg.SystemPrompt,
		Skills:       r.skills,
		Memory:       r.memory,
	}

	slog.Info("delegating to agent", "agent", agentID, "prompt_len", len(prompt))
	response, err := rt.RunSync(ctx, prompt, nil)
	if err != nil {
		return "", fmt.Errorf("agent %q execution failed: %w", agentID, err)
	}

	return response, nil
}

// AvailableAgents returns the list of configured agents.
func (r *AgentRunnerImpl) AvailableAgents() []tools.AgentInfo {
	agents := r.config.Agents.List
	infos := make([]tools.AgentInfo, len(agents))
	for i, a := range agents {
		infos[i] = tools.AgentInfo{
			ID:   a.ID,
			Name: a.Name,
		}
	}
	return infos
}
