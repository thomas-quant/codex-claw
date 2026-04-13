# Phase 6: MCP Allowlists And Agent Scoping Design

## Goal

Make PicoClaw-managed MCP tools obey explicit agent boundaries. Phase 6 turns `mcpServers` from parsed metadata into enforced runtime policy while keeping the existing MCP manager, transport, deferred loading, and artifact behavior intact.

## Scope

This phase adds:

- strict per-agent MCP server allowlists from AGENT frontmatter
- MCP tool registration filtered by agent scope
- cron and background runs inheriting the owning agent's MCP scope
- warning-level handling for unknown `mcpServers` entries
- agent-visible tool discovery that naturally stays inside the allowed MCP subset

This phase does not add:

- per-agent MCP managers
- Codex-native MCP integration
- new MCP discovery behavior
- changes to artifact storage or oversized MCP result shaping
- wildcard or implicit default MCP access

## Recommended Approach

Enforce allowlists at MCP tool registration time.

Alternatives considered:

1. Execution-time enforcement only. This would still expose disallowed MCP tools to the model and pollute discovery/search.
2. Per-agent MCP managers. This gives stronger isolation, but it is heavier than needed and works against the lean-runtime goal.
3. Registration-time enforcement. Recommended.

Under the recommended approach:

- `pkg/mcp` continues to own server startup, transport, and live tool discovery
- `pkg/agent/loop_mcp.go` decides which agents receive which MCP tools
- each agent's tool registry becomes the effective MCP security boundary for both interactive and background work

## Default Access Policy

`mcpServers` becomes strict.

If an agent frontmatter omits `mcpServers`, that agent receives no MCP tools by default. This matches the explicit opt-in requirement and prevents silent tool sprawl across unrelated agents.

If an agent lists one or more server names, only tools from those configured servers are registered for that agent.

Unknown server names should not crash startup. They should produce a warning that includes the agent id and the unknown server names so configuration mistakes are visible without taking down the runtime.

## Registration Model

The global MCP initialization path stays intact:

1. load configured MCP servers once
2. connect and fetch each server's tool catalog
3. register tools onto agents

Phase 6 only changes step 3.

For each discovered MCP server:

- check whether the current agent explicitly allowlists that server
- if not, skip registration for that agent
- if yes, register the server's tools onto that agent exactly as today, including deferred/hidden registration rules and workspace-aware `MCPTool` construction

This keeps the runtime lean by avoiding a new MCP control plane while making the tool registry reflect the true per-agent contract.

## Discovery And Deferred Tools

Deferred MCP registration continues to work as it does today. The difference is that discovery tools will only see MCP tools already registered on that agent.

That means:

- an agent without allowed MCP servers sees no MCP tools in discovery
- an agent with a subset of allowed servers only sees that subset
- no extra discovery filtering layer is needed if registration is already scoped correctly

This keeps the implementation small and avoids two competing policy layers.

## Cron And Background Runs

Cron and other background runs should inherit MCP access from the agent they execute as. No new policy surface is needed if they already use the agent's tool registry.

The practical rule is:

- if the agent allowlists an MCP server, cron/background runs for that agent can use it
- if the agent does not allowlist an MCP server, cron/background runs for that agent cannot use it

This matches the earlier decision that MCP access should be opt-in per agent, including automation.

## Layer Responsibilities

### `pkg/agent/definition.go`

- continue parsing `mcpServers` from AGENT frontmatter
- no schema expansion beyond what already exists

### `pkg/agent/loop_mcp.go`

- enforce the allowlist during MCP tool registration
- collect and log unknown-server warnings per agent
- keep deferred/discovery registration behavior unchanged for allowed servers

### `pkg/mcp`

- no policy changes
- continue handling server startup, transport, and live tool catalogs

### `pkg/tools/mcp_tool.go`

- no behavior changes
- keep current workspace artifact handling for oversized MCP results

## Testing Strategy

Keep verification focused on scoping behavior.

Required coverage:

- agent with no `mcpServers` receives no MCP tools
- agent with `mcpServers: ["server-a"]` receives only `server-a` tools
- agent with multiple allowed servers receives the union of those servers' tools
- unknown `mcpServers` entries warn and do not crash initialization
- deferred MCP tools remain hidden for allowed servers only
- discovery tools only surface MCP tools registered to that agent
- cron/background execution keeps the agent-scoped MCP registry

Touched-package verification should stay narrow:

- `./pkg/agent`
- `./pkg/mcp` only if a test helper needs adjustment
- `./pkg/tools` only if a registry-facing test needs adjustment

## Risks

- accidentally treating omitted `mcpServers` as "all servers" instead of "none"
- adding a second filtering layer in discovery that drifts from registration behavior
- letting unknown-server handling fail silently
- breaking agents that implicitly relied on the old global MCP exposure

The mitigation is to make the default policy explicit in code, keep enforcement at registration time only, and add direct tests for the no-default-access rule.
