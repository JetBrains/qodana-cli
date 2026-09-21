/*
 * Copyright 2026 JetBrains s.r.o.
 * Licensed under the Apache License, Version 2.0 (the "License");
 */

package managed

import (
	"context"
	"io"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// NewServer exposes the store through MCP. Authorization belongs to Store, so
// every transport and every write follows the same capability checks.
// Activity is readable tool progress; system is detailed, redacted protocol traffic.
// The caller retains ownership of both writers; nil disables either log.
// An optional shared AgentLogger also receives readable MCP activity.
func NewServer(store *Store, activity, system io.Writer, agents ...*AgentLogger) *mcp.Server {
	if activity == nil {
		activity = io.Discard
	}
	if system == nil {
		system = io.Discard
	}
	handlers := toolHandlers{store: store, log: newActivityLogger(activity)}
	if len(agents) > 0 {
		handlers.log.agents = agents[0]
	}
	logger := slog.New(slog.NewJSONHandler(system, nil))
	server := mcp.NewServer(&mcp.Implementation{Name: "edict-mcp", Version: "1.0.0"}, &mcp.ServerOptions{
		Logger:       logger,
		Instructions: "Managed Edict state and execution plans. Root requests enter through edict_manager. Delegated workers must load their assigned managed skill, identified by the delegation's skill and skillPath, instead of invoking edict_manager. Read that SKILL.md before edict_task_start and supply its registered skill ID when starting. Run every task in a subagent with its delegated token. Include your own token in every call, including read tools, so logs identify the caller. Reads may omit it during manager bootstrap and remain public. The first successful edict_plan_create call takes no token and returns the manager token; subsequent calls are forbidden for this server. Other mutations require a capability token. Never persist tokens or include them in task results. This server does not run IntelliJ inspections.",
	})
	server.AddReceivingMiddleware(requestLogging(logger, handlers.log))
	addStoreTool(server, "edict_registry", "Read the immutable startup policy: registered skills, permitted state operations, and allowed child skills. Include your token when available to identify the caller in logs.", true,
		handlers.registry)
	addStoreTool(server, "edict_read", "Read a persisted Edict state file and its SHA-256 hash for optimistic concurrency. Include your token when available to identify the caller in logs.", true,
		handlers.read)
	addStoreTool(server, "edict_list", "List persisted Edict state files below a relative path prefix. Include your token when available to identify the caller in logs.", true,
		handlers.list)
	addStoreTool(server, "edict_plan_get", "Read the current execution plan, including persisted task status and results. Include your token when available to identify the caller in logs.", true,
		handlers.planGet)
	addStoreTool(server, "edict_plan_create", "Create an execution plan and become edict_manager without supplying a token. Returns {plan, token}; keep the manager token private. Only one successful call is allowed per server lifetime, including after task completion. After restart, resume an unfinished plan by supplying its original request and top-level steps. The default pipeline analyzes signals and then generates inspections.", false,
		handlers.planCreate)
	addStoreTool(server, "edict_task_add", "Add a child task for a skill permitted by the caller's registered delegation policy.", false,
		handlers.taskAdd)
	addStoreTool(server, "edict_delegate", "Mint a unique child capability for a pending task. Returns token, taskId, assigned skill, and skillPath relative to the host's installed skills directory. Give the child the resolved absolute SKILL.md path and require it to read that file before starting. Operations and path scope may only narrow the parent's grant. Give this token only to that subagent.", false,
		handlers.delegate)
	addStoreTool(server, "edict_task_start", "Start a delegated task after reading its assigned managed SKILL.md. The declared skill must match the capability's registered skill; edict_manager cannot substitute for a worker skill. Record the subagent ID before any state mutation.", false,
		handlers.taskStart)
	addStoreTool(server, "edict_task_finish", "Persist a task's completed or failed status and result; revoke its capability and all descendant capabilities.", false,
		handlers.taskFinish)
	addStoreTool(server, "edict_task_cancel", "Mark a direct child task and its unfinished descendants failed when its subagent is lost or cannot finish. Revoke their capabilities.", false,
		handlers.taskCancel)
	addStoreTool(server, "edict_state_write", "Create or replace an allowed state file. Use an empty expectedHash for creation, or the exact current hash for replacement. Signal writes validate required fields, ID, revisions, repository-relative paths, and changed-line ranges against the supplied diff; failures identify the field and leave the file unchanged. Execution plans and registry cannot be overwritten with this tool.", false,
		handlers.stateWrite)
	addStoreTool(server, "edict_state_delete", "Delete an allowed state file using its exact current hash. Execution plans and registry cannot be deleted with this tool.", false,
		handlers.stateDelete)
	return server
}

func addStoreTool[In any](server *mcp.Server, name, description string, readOnly bool, handler func(In) (any, error)) {
	closedWorld := false
	mcp.AddTool(server, &mcp.Tool{
		Name: name, Description: description,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: readOnly, OpenWorldHint: &closedWorld},
	}, func(_ context.Context, _ *mcp.CallToolRequest, input In) (*mcp.CallToolResult, any, error) {
		output, err := handler(input)
		return nil, output, err
	})
}

type callerInput struct {
	Token string `json:"token,omitempty" jsonschema:"Your own capability token for caller attribution; omit only when no token is available"`
}

type readInput struct {
	Token string `json:"token,omitempty" jsonschema:"Your own capability token for caller attribution; omit only when no token is available"`
	Path  string `json:"path" jsonschema:"State file path relative to the configured state directory"`
}

type listInput struct {
	Token  string `json:"token,omitempty" jsonschema:"Your own capability token for caller attribution; omit only when no token is available"`
	Prefix string `json:"prefix,omitempty" jsonschema:"Optional relative directory prefix"`
}

type createPlanInput struct {
	Request string `json:"request" jsonschema:"The user's requested outcome"`
	Steps   []Step `json:"steps" jsonschema:"Ordered top-level skill executions, each with a registered skill ID and concrete title"`
}

type addTaskInput struct {
	Token string `json:"token" jsonschema:"The parent task's capability token"`
	Skill string `json:"skill" jsonschema:"Registered managed skill name permitted by the parent's policy"`
	Title string `json:"title" jsonschema:"Concrete outcome expected from this subagent"`
}

type delegateInput struct {
	Token      string   `json:"token" jsonschema:"The parent task's capability token"`
	TaskID     string   `json:"taskId" jsonschema:"Pending child task ID"`
	Operations []string `json:"operations,omitempty" jsonschema:"Explicit subset of the parent and child policy operations; omitted grants no state mutations"`
	Scope      []string `json:"scope,omitempty" jsonschema:"Explicit narrowed relative state path prefixes; required when granting mutation operations"`
}

type startTaskInput struct {
	Token   string `json:"token" jsonschema:"The delegated token held by this subagent"`
	AgentID string `json:"agentId" jsonschema:"The actual subagent ID returned by the agent runtime"`
	Skill   string `json:"skill" jsonschema:"Assigned registry skill ID from the delegation, after reading its managed SKILL.md; must match this task"`
}

type finishTaskInput struct {
	Token  string `json:"token" jsonschema:"The running subagent's delegated token"`
	Status string `json:"status" jsonschema:"Terminal task status: completed or failed"`
	Result string `json:"result" jsonschema:"Summary, evidence, outputs, or failure details; never include tokens"`
}

type cancelTaskInput struct {
	Token  string `json:"token" jsonschema:"The parent task's capability token"`
	TaskID string `json:"taskId" jsonschema:"A direct child task whose execution cannot complete"`
	Result string `json:"result" jsonschema:"Why the child failed or was cancelled; never include tokens"`
}

type writeInput struct {
	Token        string `json:"token" jsonschema:"A running managed subagent's capability token"`
	Path         string `json:"path" jsonschema:"Allowed state file path relative to the state directory"`
	Content      string `json:"content" jsonschema:"Complete replacement file content"`
	ExpectedHash string `json:"expectedHash" jsonschema:"Current file SHA-256 hash, or empty string when creating a file"`
}

type deleteInput struct {
	Token        string `json:"token" jsonschema:"A running managed subagent's capability token"`
	Path         string `json:"path" jsonschema:"Allowed state file path relative to the state directory"`
	ExpectedHash string `json:"expectedHash" jsonschema:"Exact current file SHA-256 hash"`
}
