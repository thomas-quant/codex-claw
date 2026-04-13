# Phase 6: MCP Allowlists And Agent Scoping Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make MCP access explicit per agent by enforcing `mcpServers` as a strict allowlist, with no MCP-by-default, while preserving the existing global MCP manager and deferred loading behavior.

**Architecture:** Keep `pkg/mcp` unchanged as the global transport and discovery layer. Move the new policy entirely into `pkg/agent/loop_mcp.go`: resolve each agent's MCP scope from AGENT frontmatter once, register only allowed server tools into that agent's tool registry, and only add discovery tools where hidden MCP tools actually exist.

**Tech Stack:** Go 1.25, PicoClaw agent/runtime packages, MCP manager/catalog types, `go test`

---

## File Structure

### Modify

- `pkg/agent/loop_mcp.go` — add MCP allowlist resolution helpers, scoped registration, unknown-server warnings, and discovery-tool gating
- `pkg/agent/loop_mcp_test.go` — focused coverage for allowlist resolution, scoped registration, deferred behavior, and discovery scoping
- `docs/configuration.md` — make the strict `mcpServers` opt-in default explicit
- `docs/tools_configuration.md` — document that MCP discovery/search tools are only attached to agents that actually receive deferred MCP tools
- `docs/troubleshooting.md` — clarify the “no `mcpServers` means no MCP tools” rule

### Keep As-Is

- `pkg/mcp/manager.go` — no policy changes
- `pkg/tools/mcp_tool.go` — no artifact/result-shaping changes
- `pkg/agent/definition.go` — keep parsing `mcpServers` exactly as it already does

## Task 1: Add Agent MCP Scope Helpers

**Files:**
- Modify: `pkg/agent/loop_mcp.go`
- Modify: `pkg/agent/loop_mcp_test.go`

- [ ] **Step 1: Write failing tests for strict MCP scope resolution**

Add focused tests to `pkg/agent/loop_mcp_test.go` that pin the no-default-access rule and unknown-server handling:

```go
func TestResolveAgentMCPScope_NoAllowlistMeansNoServers(t *testing.T) {
	workspace := t.TempDir()
	writeAgentFile(t, workspace, "---\nname: coder\n---\n# agent\n")

	scope := resolveAgentMCPScope(&AgentInstance{
		ID:        "coder",
		Workspace: workspace,
	}, map[string]config.MCPServerConfig{
		"github":    {Enabled: true},
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
```

Use a small helper in the test file:

```go
func writeAgentFile(t *testing.T, workspace, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(workspace, "AGENT.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(AGENT.md) error = %v", err)
	}
}
```

- [ ] **Step 2: Run the new scope tests and confirm they fail**

Run:

```bash
PATH=/tmp/go-toolchain/go/bin:$PATH go test ./pkg/agent -run 'TestResolveAgentMCPScope_' -count=1
```

Expected: FAIL because `resolveAgentMCPScope` and `agentMCPScope` do not exist yet.

- [ ] **Step 3: Implement MCP scope resolution helpers in `loop_mcp.go`**

Add a small internal helper type and resolver:

```go
type agentMCPScope struct {
	Allowed map[string]struct{}
	Unknown []string
}

func resolveAgentMCPScope(agent *AgentInstance, configured map[string]config.MCPServerConfig) agentMCPScope {
	scope := agentMCPScope{Allowed: make(map[string]struct{})}
	if agent == nil || strings.TrimSpace(agent.Workspace) == "" {
		return scope
	}

	definition := loadAgentDefinition(agent.Workspace)
	allowlist := definition.Agent
	if allowlist == nil || len(allowlist.Frontmatter.MCPServers) == 0 {
		return scope
	}

	for _, name := range allowlist.Frontmatter.MCPServers {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := configured[name]; ok {
			scope.Allowed[name] = struct{}{}
			continue
		}
		scope.Unknown = append(scope.Unknown, name)
	}
	slices.Sort(scope.Unknown)
	return scope
}
```

- [ ] **Step 4: Re-run the scope tests**

Run:

```bash
PATH=/tmp/go-toolchain/go/bin:$PATH go test ./pkg/agent -run 'TestResolveAgentMCPScope_' -count=1
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/agent/loop_mcp.go pkg/agent/loop_mcp_test.go
git commit -m "feat(mcp): add per-agent scope resolution"
```

## Task 2: Scope MCP Tool Registration By Agent

**Files:**
- Modify: `pkg/agent/loop_mcp.go`
- Modify: `pkg/agent/loop_mcp_test.go`

- [ ] **Step 1: Write failing tests for scoped MCP registration**

Add tests that exercise the registration logic without needing live MCP servers:

```go
func TestRegisterMCPToolsForAgents_RegistersOnlyAllowedServers(t *testing.T) {
	githubAgent := &AgentInstance{ID: "github-agent", Workspace: t.TempDir(), Tools: tools.NewToolRegistry()}
	writeAgentFile(t, githubAgent.Workspace, "---\nmcpServers:\n  - github\n---\n# agent\n")

	noMCPAgent := &AgentInstance{ID: "plain-agent", Workspace: t.TempDir(), Tools: tools.NewToolRegistry()}
	writeAgentFile(t, noMCPAgent.Workspace, "---\nname: plain\n---\n# agent\n")

	servers := map[string]*mcp.ServerConnection{
		"github": {
			Name: "github",
			Tools: []*mcp.Tool{
				{Name: "issues", Description: "List issues", InputSchema: map[string]any{"type": "object"}},
			},
		},
		"filesystem": {
			Name: "filesystem",
			Tools: []*mcp.Tool{
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
			"github":    {Enabled: true, Deferred: boolPtr(false)},
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
		map[string]*mcp.ServerConnection{
			"github": {
				Name: "github",
				Tools: []*mcp.Tool{
					{Name: "issues", Description: "List issues", InputSchema: map[string]any{"type": "object"}},
				},
			},
		},
		true,
		16384,
	)

	if got := agent.Tools.List(); slices.Contains(got, "mcp_github_issues") {
		t.Fatalf("deferred MCP tool should not be core-visible, got %v", got)
	}
	if hidden := agent.Tools.SnapshotHiddenTools().Docs; len(hidden) != 1 || hidden[0].Name != "mcp_github_issues" {
		t.Fatalf("hidden docs = %#v, want mcp_github_issues", hidden)
	}
}
```

- [ ] **Step 2: Run the registration tests to verify they fail**

Run:

```bash
PATH=/tmp/go-toolchain/go/bin:$PATH go test ./pkg/agent -run 'TestRegisterMCPToolsForAgents_' -count=1
```

Expected: FAIL because `registerMCPToolsForAgents` does not exist and `ensureMCPInitialized` still registers every MCP tool on every agent.

- [ ] **Step 3: Extract and implement scoped registration**

Refactor the registration loop inside `ensureMCPInitialized` into a helper that resolves agent scopes once, logs unknown entries once, and only registers allowed tools:

```go
type mcpRegistrationResult struct {
	UniqueTools        int
	TotalRegistrations int
	DeferredAgents     map[string]bool
}

func registerMCPToolsForAgents(
	manager *mcp.Manager,
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
		if len(scope.Unknown) > 0 {
			logger.WarnCF("agent", "Agent references unknown MCP servers", map[string]any{
				"agent_id": agentID,
				"servers":  scope.Unknown,
			})
		}
	}

	result := mcpRegistrationResult{
		DeferredAgents: make(map[string]bool),
	}
	for serverName, conn := range servers {
		result.UniqueTools += len(conn.Tools)
		serverCfg := serverCfgs[serverName]
		registerAsHidden := serverIsDeferred(discoveryEnabled, serverCfg)

		for agentID, agent := range agents {
			if _, ok := scopes[agentID].Allowed[serverName]; !ok {
				continue
			}
			for _, tool := range conn.Tools {
				mcpTool := tools.NewMCPTool(manager, serverName, tool)
				mcpTool.SetWorkspace(agent.Workspace)
				mcpTool.SetMaxInlineTextRunes(maxInlineTextChars)
				if registerAsHidden {
					agent.Tools.RegisterHidden(mcpTool)
				} else {
					agent.Tools.Register(mcpTool)
				}
				result.TotalRegistrations++
			}
			if registerAsHidden {
				result.DeferredAgents[agentID] = true
			}
		}
	}
	return result
}
```

Then replace the old nested “register for all agents” block in `ensureMCPInitialized` with a call to this helper.

- [ ] **Step 4: Re-run the registration tests**

Run:

```bash
PATH=/tmp/go-toolchain/go/bin:$PATH go test ./pkg/agent -run 'TestRegisterMCPToolsForAgents_' -count=1
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/agent/loop_mcp.go pkg/agent/loop_mcp_test.go
git commit -m "feat(mcp): scope tool registration per agent"
```

## Task 3: Gate Discovery Tools To Agents With Deferred MCP Access

**Files:**
- Modify: `pkg/agent/loop_mcp.go`
- Modify: `pkg/agent/loop_mcp_test.go`

- [ ] **Step 1: Write failing tests for discovery-tool scoping**

Add tests that pin the lean behavior: discovery helpers should only be present where deferred MCP tools were actually registered.

```go
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
		map[string]*mcp.ServerConnection{
			"github": {
				Name: "github",
				Tools: []*mcp.Tool{
					{Name: "issues", Description: "List issues", InputSchema: map[string]any{"type": "object"}},
				},
			},
		},
		true,
		16384,
	)

	registerDiscoveryTools(
		map[string]*AgentInstance{"coder": deferredAgent, "plain": noMCPAgent},
		result.DeferredAgents,
		config.MCPDiscoveryConfig{Enabled: true, UseBM25: true, TTL: 5, MaxSearchResults: 5},
	)

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
```

- [ ] **Step 2: Run the discovery tests and confirm they fail**

Run:

```bash
PATH=/tmp/go-toolchain/go/bin:$PATH go test ./pkg/agent -run 'TestRegisterMCPToolsForAgents_DiscoveryToolsOnlyForAgentsWithDeferredMCP' -count=1
```

Expected: FAIL because discovery tools are still registered for every agent whenever discovery is globally enabled.

- [ ] **Step 3: Extract discovery registration and scope it**

Split the current discovery-tool loop into a helper that only touches agents with deferred MCP tools:

```go
func registerDiscoveryTools(
	agents map[string]*AgentInstance,
	deferredAgents map[string]bool,
	discovery config.MCPDiscoveryConfig,
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

	for agentID, agent := range agents {
		if !deferredAgents[agentID] {
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
```

Wire `ensureMCPInitialized` so it reuses the deferred-agent information produced during MCP registration instead of rebuilding policy a second time.

- [ ] **Step 4: Run the focused MCP tests**

Run:

```bash
PATH=/tmp/go-toolchain/go/bin:$PATH go test ./pkg/agent -run 'Test(ServerIsDeferred|ResolveAgentMCPScope_|RegisterMCPToolsForAgents_)' -count=1
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/agent/loop_mcp.go pkg/agent/loop_mcp_test.go
git commit -m "feat(mcp): scope discovery tools by agent"
```

## Task 4: Update The Docs And Run Guardrail Verification

**Files:**
- Modify: `docs/configuration.md`
- Modify: `docs/tools_configuration.md`
- Modify: `docs/troubleshooting.md`

- [ ] **Step 1: Update the user-facing MCP docs**

Make the new default explicit in the three docs:

`docs/configuration.md`

```md
Use agent-level `mcpServers` allowlists when you want to expose MCP tools to an agent. If `mcpServers` is omitted, that agent receives no MCP tools.
```

`docs/tools_configuration.md`

```md
MCP server startup remains global, but tool exposure is per agent. Only MCP servers listed in an agent's `mcpServers` frontmatter are registered into that agent's tool registry. Discovery/search tools are only attached to agents that actually receive deferred MCP tools.
```

`docs/troubleshooting.md`

```md
- if the agent does not list the server in `mcpServers`, PicoClaw will not register that server's tools for the agent
- if `mcpServers` is omitted entirely, the agent receives no MCP tools by default
```

- [ ] **Step 2: Run the targeted MCP verification suite**

Run:

```bash
PATH=/tmp/go-toolchain/go/bin:$PATH go test ./pkg/agent -run 'Test(ServerIsDeferred|ResolveAgentMCPScope_|RegisterMCPToolsForAgents_)' -count=1
PATH=/tmp/go-toolchain/go/bin:$PATH go test ./pkg/agent -count=1
```

Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add docs/configuration.md docs/tools_configuration.md docs/troubleshooting.md pkg/agent/loop_mcp.go pkg/agent/loop_mcp_test.go
git commit -m "docs(mcp): document strict agent allowlists"
```

## Self-Review

- Spec coverage: this plan covers strict `mcpServers`, scoped MCP registration, deferred/discovery scoping, unknown-server handling, and cron/background inheritance via the agent tool registry boundary.
- Placeholder scan: no `TODO`/`TBD` placeholders remain.
- Type consistency: the plan consistently uses `agentMCPScope`, `resolveAgentMCPScope`, `registerMCPToolsForAgents`, and `registerDiscoveryTools` as the new internal helpers.
