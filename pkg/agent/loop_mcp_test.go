// PicoClaw - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package agent

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sipeed/picoclaw/pkg/config"
	picoclawmcp "github.com/sipeed/picoclaw/pkg/mcp"
	"github.com/sipeed/picoclaw/pkg/tools"
)

func boolPtr(b bool) *bool { return &b }

func writeAgentFile(t *testing.T, workspace, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(workspace, "AGENT.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(AGENT.md) error = %v", err)
	}
}

func TestServerIsDeferred(t *testing.T) {
	tests := []struct {
		name             string
		discoveryEnabled bool
		serverDeferred   *bool
		want             bool
	}{
		// --- global false always wins: per-server deferred is ignored ---
		{
			name:             "global false: per-server deferred=true is ignored",
			discoveryEnabled: false,
			serverDeferred:   boolPtr(true),
			want:             false,
		},
		{
			name:             "global false: per-server deferred=false stays false",
			discoveryEnabled: false,
			serverDeferred:   boolPtr(false),
			want:             false,
		},
		// --- global true: per-server override applies ---
		{
			name:             "global true: per-server deferred=false opts out",
			discoveryEnabled: true,
			serverDeferred:   boolPtr(false),
			want:             false,
		},
		{
			name:             "global true: per-server deferred=true stays true",
			discoveryEnabled: true,
			serverDeferred:   boolPtr(true),
			want:             true,
		},
		// --- no per-server override: fall back to global ---
		{
			name:             "no per-server field, global discovery enabled",
			discoveryEnabled: true,
			serverDeferred:   nil,
			want:             true,
		},
		{
			name:             "no per-server field, global discovery disabled",
			discoveryEnabled: false,
			serverDeferred:   nil,
			want:             false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverCfg := config.MCPServerConfig{Deferred: tt.serverDeferred}
			got := serverIsDeferred(tt.discoveryEnabled, serverCfg)
			if got != tt.want {
				t.Errorf("serverIsDeferred(discoveryEnabled=%v, deferred=%v) = %v, want %v",
					tt.discoveryEnabled, tt.serverDeferred, got, tt.want)
			}
		})
	}
}

func TestResolveAgentMCPScope_NoAllowlistMeansNoServers(t *testing.T) {
	workspace := t.TempDir()
	writeAgentFile(t, workspace, "---\nname: coder\n---\n# agent\n")

	scope := resolveAgentMCPScope(&AgentInstance{
		ID:        "coder",
		Workspace: workspace,
	}, map[string]config.MCPServerConfig{
		"github":     {Enabled: true},
		"filesystem": {Enabled: true},
	})

	if len(scope.Allowed) != 0 {
		t.Fatalf("Allowed = %#v, want empty", scope.Allowed)
	}
	if len(scope.Unknown) != 0 {
		t.Fatalf("Unknown = %#v, want empty", scope.Unknown)
	}
}

func TestResolveAgentMCPScope_ReturnsAllowedAndUnknownServers(t *testing.T) {
	workspace := t.TempDir()
	writeAgentFile(t, workspace, "---\nmcpServers:\n  - github\n  - ghost\n---\n# agent\n")

	scope := resolveAgentMCPScope(&AgentInstance{
		ID:        "coder",
		Workspace: workspace,
	}, map[string]config.MCPServerConfig{
		"github": {Enabled: true},
	})

	if _, ok := scope.Allowed["github"]; !ok {
		t.Fatalf("Allowed = %#v, want github", scope.Allowed)
	}
	if !reflect.DeepEqual(scope.Unknown, []string{"ghost"}) {
		t.Fatalf("Unknown = %#v, want [ghost]", scope.Unknown)
	}
}

func TestResolveAgentMCPScope_DisabledServersAreUnavailable(t *testing.T) {
	workspace := t.TempDir()
	writeAgentFile(t, workspace, "---\nmcpServers:\n  - github\n---\n# agent\n")

	scope := resolveAgentMCPScope(&AgentInstance{
		ID:        "coder",
		Workspace: workspace,
	}, map[string]config.MCPServerConfig{
		"github": {Enabled: false},
	})

	if len(scope.Allowed) != 0 {
		t.Fatalf("Allowed = %#v, want empty", scope.Allowed)
	}
	if !reflect.DeepEqual(scope.Unknown, []string{"github"}) {
		t.Fatalf("Unknown = %#v, want [github]", scope.Unknown)
	}
}

func TestResolveAgentMCPScope_LegacyAgentsWarnWhenMCPConfigured(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "AGENTS.md"), []byte("# legacy agent\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(AGENTS.md) error = %v", err)
	}

	scope := resolveAgentMCPScope(&AgentInstance{
		ID:        "legacy",
		Workspace: workspace,
	}, map[string]config.MCPServerConfig{
		"github": {Enabled: true},
	})

	if len(scope.Allowed) != 0 {
		t.Fatalf("Allowed = %#v, want empty", scope.Allowed)
	}
	if len(scope.Unknown) != 0 {
		t.Fatalf("Unknown = %#v, want empty", scope.Unknown)
	}
	if !scope.LegacyNoAllowlistWarning {
		t.Fatalf("LegacyNoAllowlistWarning = false, want true")
	}
}

func TestRegisterMCPToolsForAgents_RegistersOnlyAllowedServers(t *testing.T) {
	githubAgent := &AgentInstance{ID: "github-agent", Workspace: t.TempDir(), Tools: tools.NewToolRegistry()}
	writeAgentFile(t, githubAgent.Workspace, "---\nmcpServers:\n  - github\n---\n# agent\n")

	noMCPAgent := &AgentInstance{ID: "plain-agent", Workspace: t.TempDir(), Tools: tools.NewToolRegistry()}
	writeAgentFile(t, noMCPAgent.Workspace, "---\nname: plain\n---\n# agent\n")

	servers := map[string]*picoclawmcp.ServerConnection{
		"github": {
			Name: "github",
			Tools: []*sdkmcp.Tool{
				{Name: "issues", Description: "List issues", InputSchema: map[string]any{"type": "object"}},
			},
		},
		"filesystem": {
			Name: "filesystem",
			Tools: []*sdkmcp.Tool{
				{Name: "read_file", Description: "Read file", InputSchema: map[string]any{"type": "object"}},
			},
		},
	}

	stats := registerMCPToolsForAgents(
		nil,
		map[string]*AgentInstance{
			"github-agent": githubAgent,
			"plain-agent":  noMCPAgent,
		},
		map[string]config.MCPServerConfig{
			"github":     {Enabled: true, Deferred: boolPtr(false)},
			"filesystem": {Enabled: true, Deferred: boolPtr(false)},
		},
		servers,
		false,
		16384,
	)

	if stats.TotalRegistrations != 1 {
		t.Fatalf("TotalRegistrations = %d, want 1", stats.TotalRegistrations)
	}
	if got := githubAgent.Tools.List(); !slices.Contains(got, "mcp_github_issues") {
		t.Fatalf("githubAgent tools = %v, want mcp_github_issues", got)
	}
	if got := noMCPAgent.Tools.List(); slices.Contains(got, "mcp_github_issues") {
		t.Fatalf("plain agent should not receive MCP tools, got %v", got)
	}
}

func TestRegisterMCPToolsForAgents_DeferredToolsStayScoped(t *testing.T) {
	agent := &AgentInstance{ID: "coder", Workspace: t.TempDir(), Tools: tools.NewToolRegistry()}
	writeAgentFile(t, agent.Workspace, "---\nmcpServers:\n  - github\n---\n# agent\n")

	registerMCPToolsForAgents(
		nil,
		map[string]*AgentInstance{"coder": agent},
		map[string]config.MCPServerConfig{
			"github": {Enabled: true, Deferred: boolPtr(true)},
		},
		map[string]*picoclawmcp.ServerConnection{
			"github": {
				Name: "github",
				Tools: []*sdkmcp.Tool{
					{Name: "issues", Description: "List issues", InputSchema: map[string]any{"type": "object"}},
				},
			},
		},
		true,
		16384,
	)

	if _, ok := agent.Tools.Get("mcp_github_issues"); ok {
		t.Fatalf("deferred MCP tool should not be callable before discovery promotion")
	}
	if hidden := agent.Tools.SnapshotHiddenTools().Docs; len(hidden) != 1 || hidden[0].Name != "mcp_github_issues" {
		t.Fatalf("hidden docs = %#v, want mcp_github_issues", hidden)
	}
}

func TestRegisterMCPToolsForAgents_DiscoveryToolsOnlyForAgentsWithDeferredMCP(t *testing.T) {
	deferredAgent := &AgentInstance{ID: "coder", Workspace: t.TempDir(), Tools: tools.NewToolRegistry()}
	writeAgentFile(t, deferredAgent.Workspace, "---\nmcpServers:\n  - github\n---\n# agent\n")

	noMCPAgent := &AgentInstance{ID: "plain", Workspace: t.TempDir(), Tools: tools.NewToolRegistry()}
	writeAgentFile(t, noMCPAgent.Workspace, "---\nname: plain\n---\n# agent\n")

	result := registerMCPToolsForAgents(
		nil,
		map[string]*AgentInstance{"coder": deferredAgent, "plain": noMCPAgent},
		map[string]config.MCPServerConfig{
			"github": {Enabled: true, Deferred: boolPtr(true)},
		},
		map[string]*picoclawmcp.ServerConnection{
			"github": {
				Name: "github",
				Tools: []*sdkmcp.Tool{
					{Name: "issues", Description: "List issues", InputSchema: map[string]any{"type": "object"}},
				},
			},
		},
		true,
		16384,
	)

	if err := registerDiscoveryTools(
		map[string]*AgentInstance{"coder": deferredAgent, "plain": noMCPAgent},
		result.DeferredAgents,
		config.ToolDiscoveryConfig{Enabled: true, UseBM25: true, TTL: 5, MaxSearchResults: 5},
	); err != nil {
		t.Fatalf("registerDiscoveryTools() error = %v", err)
	}

	if result.TotalRegistrations != 1 {
		t.Fatalf("TotalRegistrations = %d, want 1", result.TotalRegistrations)
	}
	if got := deferredAgent.Tools.List(); !slices.Contains(got, "tool_search_tool_bm25") {
		t.Fatalf("deferred agent tools = %v, want tool_search_tool_bm25", got)
	}
	if got := noMCPAgent.Tools.List(); slices.Contains(got, "tool_search_tool_bm25") {
		t.Fatalf("plain agent should not receive discovery tool, got %v", got)
	}
}
