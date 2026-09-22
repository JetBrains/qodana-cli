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
Each handler also logs its complete response body, or error, below a named
`edict_<tool> response:` entry. Bodies use readable YAML, expanding JSON-encoded
file contents and worker reports and preserving multiline evidence. These are
display copies; the MCP response itself is unchanged. Protocol envelopes are omitted.
Detailed JSON request/response payloads, protocol fields, timings, and SDK diagnostics
go to `edict-mcp-system.log`. Each activity line starts with the timestamp, skill,
and the first eight characters of the task ID, for example:

```text
2026/09/21 11:30:19 [edict-signal-analysis task=1a2b3c4d] Started task; assignment fetched and skill verified
```

Activity logs omit `next-` from displayed skill names. The prefix always identifies
the caller, except the explicit task prompt record, which uses the child's
skill/task ID and names the assigning parent. Read tools accept an optional `token` for caller
attribution; managed skills supply their own token on every call after bootstrap.
Tokenless reads use `[anonymous task=-]` because the caller is unknown. A shared
MCP connection cannot identify individual workers. Manager events use
`[edict_manager task=-]` because the manager has no worker task. Only capability
tokens are redacted, including known tokens in free-form text; IDs and hashes
remain visible. Structured worker reports appear in full in readable responses,
including plan reads and task completion. Both readable logs wrap at 120 characters.
Keep the project log directory inaccessible to children.

Hosts launching managed workflows through `edict.RunCodex` can capture the main
agent's and all nested workers' emitted commentary and final messages in
`edict-agents.log`, in the same log directory. `edict-agent-short.log` keeps the
same commentary and final messages, with only concise MCP activity and task
status summaries. It omits full response bodies and replaces task prompts with
a brief delegation entry. Pass
`managed.NewAgentLogger(store, logs.Agents, logs.AgentsShort)` as `CodexRunConfig.AgentLogger` and the
fourth argument to `managed.NewServer`, using the existing store and `managed.OpenLogs`
result. The server mirrors readable MCP activity and complete response bodies into this combined log with an
`mcp:` label. The managed integration harness
enables this automatically; no log-directory CLI flag is needed. Capture tails
Codex session traces during execution and flushes on success, failure, or
cancellation. Every record starts with its timestamp, `[skill/shortTaskId]`, and
`commentary:`, `final:`, or `mcp:`. Lines wrap at 120 Unicode characters including
the prefix, with indented continuation lines. Existing multiline formatting and
full messages remain available; long paths are wrapped too. Runtime agent IDs
are used internally for attribution and omitted from this readable log. Issued
tokens remain redacted after revocation.

Only the invocation's root thread and its descendants are included. Native runtime prompts,
tool payloads, reasoning events, and unrelated sessions are excluded; Edict MCP
response bodies come directly from the server handlers. The full task assignment submitted to `edict_delegate` is logged under the child's
skill/task ID. The worker fetches those exact instructions through `edict_task_get`;
its response is logged too. Only the short launch message carries credentials,
which remain redacted. Early
worker output waits for `edict_task_start` to associate the runtime agent with
its managed task. If a worker exits before starting, its output is retained as
`[unassigned/-]`. The standalone MCP server sees tool
traffic only; an external host must supply its agent output to `AgentLogger`.

`edict_delegate` requires complete token-free `prompt` instructions whose first
line is exactly `$managed-<registered skill>`, followed by the skill file path and
bounded inputs. The plan persists these instructions unchanged. The returned
`prompt` is a short launch message declaring that skill and telling the child to
call `edict_task_get` with its delegated token. Parents pass only this launch
message to native `spawn_agent`.

`edict_task_get(token)` selects the assignment bound to that capability and returns
`taskId`, `skill`, `skillPath`, and the full `prompt`. Workers read it and the assigned
skill file, then call `edict_task_start(token, agentId, skill)`. Startup is rejected
until that delegation has fetched its assignment and the declared skill matches.
Retries must fetch their new assignment. There is no prompt echo or string comparison.
The real Codex tests verify the child's own MCP read before startup and actual skill-file
reads. They also reject failed lifecycle calls and any cancellation or failed task
completion, even if a later retry succeeds. The root manager skill does not apply
to delegated workers, even when it is the only implicitly discoverable skill.

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
  go test ./edict -run '^TestManagedEdictExtractSignalsFrom(OneCommit|ThreeCommits)$' -v -timeout 35m
```

The deterministic integration test uses real Git history and MCP requests, plus
an actual Codex sandbox write-denial check when Codex is installed. The real-model
tests ask only "Extract signals from the latest commit." or "Extract signals from
the latest three commits." Each runs from the
source project with MCP and scratch space configured by the test environment,
without protocol instructions in the prompt. They run the manager and native subagents,
check one evidence worker per commit beneath the batch task, and validate extracted
signals against each fixture commit's exact revisions and diff. The three-commit
scenario uses `distillery-test/testExtractSignalsFromThreeCommits`: a baseline
followed by independent string-equality, locale, and integer-overflow corrections.
It expects six signals, with one positive/negative pair for each commit and none
from outside the selection. Both scenarios require Codex and the configured model
provider; neither runs IntelliJ generation. Unit/protocol tests cover capability attenuation,
call-graph restrictions, revocation, plan recovery, stale writes, filesystem
containment, concurrent access, and clean CLI stdio/shutdown.

The managed model tests default to `gpt-5.6-terra`, with high reasoning effort.
Their native workers inherit the selected model. Set `CODEX_MODEL=gpt-5.6-sol`
(or another supported model) to override it for comparisons or provider access.
Other Codex integrations retain their existing model defaults. Each test prints
its selected model alongside the result and elapsed time.

Integration artifacts are retained in `<qodana-cli>/out/<test-name>/`, including
the fixture checkout, Codex traces, readable `log/edict/edict-mcp.log`, detailed
`log/edict/edict-mcp-system.log`, and runtime output in `log/edict/edict-agents.log`
and `log/edict/edict-agent-short.log`
for managed Codex tests.
Each test clears its own output directory before the next execution; the latest
run remains available for debugging after success or failure. Raw Codex traces may
contain capabilities, so the per-test directory is private and must not be shared
with child agents.
