// PicoClaw - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package agent

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/mcp"
	"github.com/sipeed/picoclaw/pkg/tools"
)

type mcpRuntime struct {
	initOnce sync.Once
	mu       sync.Mutex
	manager  *mcp.Manager
	initErr  error
}

type agentMCPScope struct {
	Allowed                  map[string]struct{}
	Unknown                  []string
	LegacyNoAllowlistWarning bool
}

type mcpRegistrationResult struct {
	UniqueTools        int
	TotalRegistrations int
	DeferredAgents     map[string]bool
}

func (r *mcpRuntime) setManager(manager *mcp.Manager) {
	r.mu.Lock()
	r.manager = manager
	r.initErr = nil
	r.mu.Unlock()
}

func (r *mcpRuntime) setInitErr(err error) {
	r.mu.Lock()
	r.initErr = err
	r.mu.Unlock()
}

func (r *mcpRuntime) getInitErr() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.initErr
}

func (r *mcpRuntime) takeManager() *mcp.Manager {
	r.mu.Lock()
	defer r.mu.Unlock()
	manager := r.manager
	r.manager = nil
	return manager
}

func (r *mcpRuntime) hasManager() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.manager != nil
}

// ensureMCPInitialized loads MCP servers/tools once so both Run() and direct
// agent mode share the same initialization path.
func (al *AgentLoop) ensureMCPInitialized(ctx context.Context) error {
	if !al.cfg.Tools.IsToolEnabled("mcp") {
		return nil
	}

	if al.cfg.Tools.MCP.Servers == nil || len(al.cfg.Tools.MCP.Servers) == 0 {
		logger.WarnCF("agent", "MCP is enabled but no servers are configured, skipping MCP initialization", nil)
		return nil
	}

	findValidServer := false
	for _, serverCfg := range al.cfg.Tools.MCP.Servers {
		if serverCfg.Enabled {
			findValidServer = true
		}
	}
	if !findValidServer {
		logger.WarnCF("agent", "MCP is enabled but no valid servers are configured, skipping MCP initialization", nil)
		return nil
	}

	al.mcp.initOnce.Do(func() {
		mcpManager := mcp.NewManager()

		defaultAgent := al.registry.GetDefaultAgent()
		workspacePath := al.cfg.WorkspacePath()
		if defaultAgent != nil && defaultAgent.Workspace != "" {
			workspacePath = defaultAgent.Workspace
		}

		if err := mcpManager.LoadFromMCPConfig(ctx, al.cfg.Tools.MCP, workspacePath); err != nil {
			logger.WarnCF("agent", "Failed to load MCP servers, MCP tools will not be available",
				map[string]any{
					"error": err.Error(),
				})
			if closeErr := mcpManager.Close(); closeErr != nil {
				logger.ErrorCF("agent", "Failed to close MCP manager",
					map[string]any{
						"error": closeErr.Error(),
					})
			}
			return
		}

		// Register MCP tools for all agents
		servers := mcpManager.GetServers()
		agentIDs := al.registry.ListAgentIDs()
		agentCount := len(agentIDs)
		agents := make(map[string]*AgentInstance, agentCount)
		for _, agentID := range agentIDs {
			agent, ok := al.registry.GetAgent(agentID)
			if !ok {
				continue
			}
			agents[agentID] = agent
		}

		registration := registerMCPToolsForAgents(
			mcpManager,
			agents,
			al.cfg.Tools.MCP.Servers,
			servers,
			al.cfg.Tools.MCP.Discovery.Enabled,
			al.cfg.Tools.MCP.GetMaxInlineTextChars(),
		)
		logger.InfoCF("agent", "MCP tools registered successfully",
			map[string]any{
				"server_count":        len(servers),
				"unique_tools":        registration.UniqueTools,
				"total_registrations": registration.TotalRegistrations,
				"agent_count":         agentCount,
			})

		// Initializes Discovery Tools only if enabled by configuration
		if al.cfg.Tools.MCP.Enabled && al.cfg.Tools.MCP.Discovery.Enabled {
			if err := registerDiscoveryTools(agents, registration.DeferredAgents, al.cfg.Tools.MCP.Discovery); err != nil {
				al.mcp.setInitErr(err)
				if closeErr := mcpManager.Close(); closeErr != nil {
					logger.ErrorCF("agent", "Failed to close MCP manager",
						map[string]any{
							"error": closeErr.Error(),
						})
				}
				return
			}
		}

		al.mcp.setManager(mcpManager)
	})

	return al.mcp.getInitErr()
}

// serverIsDeferred reports whether an MCP server's tools should be registered
// as hidden (deferred/discovery mode).
//
// Global discovery must be enabled before any server can be deferred. When it
// is enabled, the per-server Deferred field overrides the default; otherwise,
// servers fall back to deferred-by-default.
func serverIsDeferred(discoveryEnabled bool, serverCfg config.MCPServerConfig) bool {
	if !discoveryEnabled {
		return false
	}
	if serverCfg.Deferred != nil {
		return *serverCfg.Deferred
	}
	return true
}

func resolveAgentMCPScope(agent *AgentInstance, configured map[string]config.MCPServerConfig) agentMCPScope {
	scope := agentMCPScope{
		Allowed: make(map[string]struct{}),
	}
	if agent == nil || strings.TrimSpace(agent.Workspace) == "" {
		return scope
	}

	definition := loadAgentDefinition(agent.Workspace)
	if definition.Source == AgentDefinitionSourceAgents && hasEnabledMCPServers(configured) {
		scope.LegacyNoAllowlistWarning = true
		return scope
	}
	if definition.Agent == nil || len(definition.Agent.Frontmatter.MCPServers) == 0 {
		return scope
	}

	unknownSet := make(map[string]struct{})
	for _, name := range definition.Agent.Frontmatter.MCPServers {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if serverCfg, ok := configured[name]; ok && serverCfg.Enabled {
			scope.Allowed[name] = struct{}{}
			continue
		}
		if _, ok := unknownSet[name]; ok {
			continue
		}
		unknownSet[name] = struct{}{}
		scope.Unknown = append(scope.Unknown, name)
	}

	slices.Sort(scope.Unknown)
	return scope
}

func hasEnabledMCPServers(configured map[string]config.MCPServerConfig) bool {
	for _, serverCfg := range configured {
		if serverCfg.Enabled {
			return true
		}
	}
	return false
}

func registerMCPToolsForAgents(
	manager tools.MCPManager,
	agents map[string]*AgentInstance,
	serverCfgs map[string]config.MCPServerConfig,
	servers map[string]*mcp.ServerConnection,
	discoveryEnabled bool,
	maxInlineTextChars int,
) mcpRegistrationResult {
	scopes := make(map[string]agentMCPScope, len(agents))
	for agentID, agent := range agents {
		scope := resolveAgentMCPScope(agent, serverCfgs)
		scopes[agentID] = scope
		if scope.LegacyNoAllowlistWarning {
			logger.WarnCF("agent", "Legacy AGENTS.md agent cannot declare MCP allowlists; no MCP tools will be registered", map[string]any{
				"agent_id": agentID,
			})
		}
		if len(scope.Unknown) > 0 {
			logger.WarnCF("agent", "Agent references unknown or unavailable MCP servers", map[string]any{
				"agent_id": agentID,
				"servers":  scope.Unknown,
			})
		}
	}

	result := mcpRegistrationResult{
		DeferredAgents: make(map[string]bool),
	}
	for serverName, conn := range servers {
		if conn == nil {
			continue
		}
		result.UniqueTools += len(conn.Tools)
		serverCfg := serverCfgs[serverName]
		registerAsHidden := serverIsDeferred(discoveryEnabled, serverCfg)

		for agentID, agent := range agents {
			if agent == nil || agent.Tools == nil {
				continue
			}
			if _, ok := scopes[agentID].Allowed[serverName]; !ok {
				continue
			}

			registeredHidden := false
			for _, tool := range conn.Tools {
				mcpTool := tools.NewMCPTool(manager, serverName, tool)
				mcpTool.SetWorkspace(agent.Workspace)
				mcpTool.SetMaxInlineTextRunes(maxInlineTextChars)

				if registerAsHidden {
					agent.Tools.RegisterHidden(mcpTool)
					registeredHidden = true
				} else {
					agent.Tools.Register(mcpTool)
				}

				result.TotalRegistrations++
				logger.DebugCF("agent", "Registered MCP tool",
					map[string]any{
						"agent_id": agentID,
						"server":   serverName,
						"tool":     tool.Name,
						"name":     mcpTool.Name(),
						"deferred": registerAsHidden,
					})
			}
			if registeredHidden {
				result.DeferredAgents[agentID] = true
			}
		}
	}

	return result
}

func registerDiscoveryTools(
	agents map[string]*AgentInstance,
	deferredAgents map[string]bool,
	discovery config.ToolDiscoveryConfig,
) error {
	if !discovery.Enabled {
		return nil
	}
	if !discovery.UseBM25 && !discovery.UseRegex {
		return fmt.Errorf("tool discovery is enabled but neither 'use_bm25' nor 'use_regex' is set to true in the configuration")
	}

	ttl := discovery.TTL
	if ttl <= 0 {
		ttl = 5
	}
	maxSearchResults := discovery.MaxSearchResults
	if maxSearchResults <= 0 {
		maxSearchResults = 5
	}

	logger.InfoCF("agent", "Initializing tool discovery", map[string]any{
		"bm25": discovery.UseBM25, "regex": discovery.UseRegex, "ttl": ttl, "max_results": maxSearchResults,
	})

	for agentID, agent := range agents {
		if !deferredAgents[agentID] || agent == nil || agent.Tools == nil {
			continue
		}
		if discovery.UseRegex {
			agent.Tools.Register(tools.NewRegexSearchTool(agent.Tools, ttl, maxSearchResults))
		}
		if discovery.UseBM25 {
			agent.Tools.Register(tools.NewBM25SearchTool(agent.Tools, ttl, maxSearchResults))
		}
	}

	return nil
}
