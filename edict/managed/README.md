# Managed Edict

`edict-mcp` is the Qodana CLI's state and execution-plan server. It does not launch
an IDE. `qodana edict mcp start` still launches the separate IntelliJ inspection
server.

Install the bundled manager and managed skill copies:

```sh
qodana edict setup-codex --managed
```

The source bundle is `edict/skills/managed`. Installed children use
`managed-edict-next-*` names so legacy skills can coexist. Only `edict_manager`
allows implicit invocation. Its default pipeline analyzes a bounded selection of
PRs/commits and then runs preparation, distribution, and generation. Extraction
only and existing-inbox requests select the corresponding subset. Every stage,
review, and evidence-analysis task runs in a fresh native subagent. The manager
does not automatically commit or publish changes.

## Server and host setup

Configure the MCP host to launch:

```sh
qodana edict edict-mcp --project-dir /path/to/project
```

The default state root is `<project-dir>/.edict`; use `--state-dir` for an existing
rules checkout such as `ultimate-edict-next`. No bootstrap token file is needed.
The first successful `edict_plan_create(request, steps)` call returns `{plan, token}`
and grants its caller manager authority. It can succeed only once per server
lifetime, even after every task completes. Invalid requests do not consume the
claim. Concurrent callers are serialized so exactly one can become manager.
Standard output contains only MCP protocol messages. The MCP client owns the
stdio connection and server lifetime. Logging needs no extra CLI parameter: logs
are appended under `<project-dir>/log/edict/`. Tool handlers write readable progress
to `edict-mcp.log`: plan creation, task titles and outcomes, file changes, and errors.
Detailed JSON request/response payloads, protocol fields, timings, and SDK diagnostics
go to `edict-mcp-system.log`. Each activity line starts with the timestamp, skill,
and the first eight characters of the task ID, for example:

```text
2026/09/21 11:30:19 [edict-signal-analysis task=1a2b3c4d] Started task "Review the corrective commit"
```

Activity logs omit `next-` from displayed skill names. The prefix always identifies
the caller; adding, delegating, or cancelling a child includes the child's skill
and short task ID in the message. Read tools accept an optional `token` for caller
attribution; managed skills supply their own token on every call after bootstrap.
Tokenless reads use `[anonymous task=-]` because the caller is unknown. A shared
MCP connection cannot identify individual workers. Manager events use
`[edict_manager task=-]` because the manager has no worker task. Only capability
tokens are redacted, including known tokens in free-form text; IDs and hashes
remain visible. Structured worker reports stay in the system log, with a readable
summary in the activity log.
Keep the project log directory inaccessible to children.

The trusted host must make the state root read-only to **all** agents, including
unmanaged skills, while the server retains write access. Run the server outside
that agent sandbox. Have the manager claim the plan before starting children; deny
children access to its parent conversation and any logs containing parent/sibling
tokens. Give each child only its own token, with no inherited conversation.
Scratch directories remain writable outside the state root. A capability is a
bearer secret: this server cannot identify the agent behind a stolen token,
enforce native spawning, or stop shell writes by an unrestricted process. The
runtime provides those isolation guarantees; `agentId` records its asserted
native identity rather than authenticating the runtime.

The inspection server also needs read-only access to authoritative state; give
inspection tools scratch snapshots and do not configure a legacy Edict writer
session against that root. Otherwise it would provide a second mutation path.
Allow at least five levels of nested worker delegation and six simultaneous
agent frames for the deepest generation path (manager → run → generation →
cluster → weak-signal review → example). Batch-only extraction needs two nested
levels. The runtime must support fresh workers with isolated conversation
history, and release finished workers so later tasks can start.

## Authorization and persistence

The closed registry in `registry.go` is loaded before serving requests. There is
no runtime registration. Plan creation is the sole unauthenticated mutation:
whoever successfully calls it first becomes manager for that server run. `edict_registry`
exposes each skill's permitted calls, operations it can delegate, and operations
it can execute itself. Orchestrators can delegate state rights without being
allowed to use those rights directly. Unregistered skills cannot receive grants.

The server issues random 256-bit tokens and keeps only their SHA-256 digests in
memory. After plan creation, every mutation checks the active grant, task lifecycle,
registered skill, operation, and relative path scope. A delegated grant must be a subset of its
parent's operations and paths and the child's registered policy. Tokens cannot
be reused after completion, cancellation, parent failure, or server restart.
Read tools require no token and never expose capability values.

Plans live at `plans/<id>.json`. Task creation, delegation, start, completion, and
cancellation atomically update the plan and its revision. A parent cannot complete
with unfinished or failed descendants. Lost workers can be cancelled by their
direct coordinator, then retried with fresh tokens. Restart loads the active plan,
preserves completed results, and resets interrupted tasks to pending. The manager
claims it with a fresh token by calling `edict_plan_create` with the persisted
request and ordered top-level skill/title steps; mismatched requests are rejected
while an unfinished plan exists. Start a new server to create another plan after
the previous plan becomes terminal. Historical plan files remain.

State writes require the current SHA-256 hash, or an empty hash for creation.
Each replacement is atomic; a stale hash fails without changing the file. Deletes
require an existing hash. Writes cannot target plans, registry, Git metadata, or
arbitrary files. Example workers can change only `syntheticExampleId` in a signal.
Inbox and cluster signal writes validate required fields, stable IDs, evidence
revisions, and repository-relative paths/ranges against the supplied diff. Invalid
signals return an artifact/field-specific error without creating or replacing a file.
Symlinks, traversal, and unsupported artifact paths are rejected; `os.Root`
confines filesystem access and an OS lock prevents concurrent servers for one
root. Multi-file transitions are **not** transactions: distribution copies and
verifies evidence before deleting its inbox copy, and generation sets the final
status after its artifacts are durable. Workers reconcile partial progress on
retry. Authorization does not establish the semantic correctness of evidence or
inspection code; managed skills perform those checks using scratch-only tooling.

## Tests

```sh
go test -race ./edict/managed
go test ./internal/cmd -run 'TestManaged|TestEdictManaged'
DISTILLERY_TEST_REPO=/path/to/distillery-test \
  go test ./edict -run '^TestManagedEdictDistilleryWorkflow$' -v
EDICT_MANAGED_CODEX_TEST=1 DISTILLERY_TEST_REPO=/path/to/distillery-test \
  go test ./edict -run '^TestManagedEdictManagerSkill$' -v -timeout 18m
```

The deterministic integration test uses real Git history and MCP requests, plus
an actual Codex sandbox write-denial check when Codex is installed. The opt-in
model test asks only "Extract signals from the latest commit." It runs from the
source project with MCP and scratch space configured by the test environment,
without protocol instructions in the prompt. It runs the manager and native subagents, checks their runtime IDs against
the persisted plan, and validates extracted signals against the fixture's exact
revisions and diff. It requires Codex and the configured model provider. It does
not run IntelliJ generation. Unit/protocol tests cover capability attenuation,
call-graph restrictions, revocation, plan recovery, stale writes, filesystem
containment, concurrent access, and clean CLI stdio/shutdown.

Integration artifacts are retained in `<qodana-cli>/out/<test-name>/`, including
the fixture checkout, Codex traces, readable `log/edict/edict-mcp.log`, and detailed
`log/edict/edict-mcp-system.log` for managed tests.
Each test clears its own output directory before the next execution; the latest
run remains available for debugging after success or failure. Raw Codex traces may
contain capabilities, so the per-test directory is private and must not be shared
with child agents.
