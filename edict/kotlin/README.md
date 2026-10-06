# Edict — managed skills for Kotlin/JVM

Standalone Gradle application for the managed part of Edict. Requires JDK 21+ and
Git; the Gradle wrapper downloads the pinned distribution. It uses the Kotlin MCP
SDK and Ktor for the Streamable HTTP transport. There are no IntelliJ,
Qodana CLI, Go, or Bazel dependencies.

```sh
cd edict/kotlin
./gradlew test -PexcludeIntegrationTests=true installDist
build/install/edict/bin/edict --help
cd /path/to/project
/path/to/edict/kotlin/build/install/edict/bin/edict install
/path/to/edict/kotlin/build/install/edict/bin/edict mcp --state-dir /path/to/state
```

## Qodana CLI integration

The Go commands `qodana edict install` and `qodana edict mcp start` launch this
Kotlin application with Qodana's embedded JBR. Both run in the project directory, which is
also the inspected project.

Setup and run are separate. The user's `$CODEX_HOME/config.toml` holds only the model
provider, created before Edict, and trusts the project (`[projects."<project>"] trust_level =
"trusted"`); without trust Codex ignores the project's `.codex/config.toml`. `install` asks Codex
(`codex mcp list --json`, or `$CODEX_BIN`) whether it loads the config, and removes it and fails if
not, so it needs Codex installed. Edict never writes
`$CODEX_HOME`. `qodana edict install [--deny <path>]...` installs the managed skills into
`./.codex/skills` and writes `./.codex/config.toml`: the `edict` permission profile (project
read-only, `log/agent-work` writable for scratch, `log/process-log` and every `--deny` path denied; the
state root needs no rule because it is read-only to agents and `:read-only` allows reads), agent
limits, and the `edict-mcp` server on loopback port `edict.mcpPort`. The port comes from the
`edict` section of the project's `qodana.yml` or `qodana.yaml` (27182 when absent); re-run
`install` after changing it. Installation logs the installed names. Existing unrelated skills are preserved.
Then run Codex in the project with an explicit `$edict_manager <request>`; skills read the run's
paths from the `edict_context` tool.

From the repository root:

```sh
go generate ./internal/tooling/...
go build -o qodana ./cli
cd /path/to/project
/path/to/qodana edict install
/path/to/qodana edict mcp start --state-dir /path/to/state
codex exec '$edict_manager process inbox and generate new rules'
go test ./internal/cmd -run 'Test(Edict|ManagedMCP)'
```

Generation builds `./gradlew bundledJar`, embeds the JAR with the other Java tools,
and updates the bundled runtime's module set. A content hash in the JAR name and
a module hash in the runtime name prevent reuse of stale extracted tools. No
separate JVM installation is needed to use the Go commands. For standalone use,
run `java -jar build/libs/edict-cli.jar --help` after building `bundledJar`.

The Go proxy tests launch the actual embedded JAR and JBR. They cover the local
installation and bundled resources, HTTP MCP on the configured port, redacted logs, errors, shutdown,
and state-lock release. They need no model, Distillery checkout, or IDE.
The server starts IntelliJ only when an inspection tool first needs it: it runs the
hidden `qodana edict ide-mcp` helper as a child, which prints one readiness line and
stops the IDE when its stdin closes. The IDE is native only: `--ide-dist <path>`, else
`--ide-linter <name>` (downloaded by the CLI), else `QODANA_DIST`. Pass
`--ide-property`/`--ide-wait-timeout` through `edict mcp start`; IDE output goes to
`intellij-mcp.log` in the run's log folder.

`mcp` serves one shared Streamable HTTP endpoint on loopback port `edict.mcpPort`, so every native
agent shares one server and one state lock. It fails when `.codex/config.toml` is missing or names
another port, or when the port is busy; it never moves to another port. State defaults to `.edict`
(`--state-dir` moves it) and is also the reference repository for distribution checks; the
project's Git repository validates commit Signals. Every process is a run named by its start time (UTC, for example
`2026-10-06T14-03-12.345Z`) and logs to `log/process-log/<run-id>/`: activity and redacted tool details in
`edict-mcp{,-system}.log`, and INFO and above, with stack traces, in `edict.log`, which stdout also shows
(`src/main/resources/logback.xml`). The server prints its folder as `Process log: <path>`. Agent scratch is
`log/agent-work/<run-id>/scratch`. `common/EdictLayout.kt` names all of these paths; the `edict.log.dir` JVM system
property moves the `log` folder (relative to the project), and `edict.run.id` fixes the run id.

## What is ported

- All 14 managed skills, their invocation policies, and shared references.
  Worker registry IDs and installed skill names both use `edict-*`.
  The manager remains `edict_manager`.
- Manager claims, task assignment retrieval, native worker startup, attenuated
  capabilities, call-graph restrictions, cancellation, retries, and revocation.
  Capabilities are stored only as hashes in memory and excluded from saved data
  and server logs.
- Persistent plans, exclusive state ownership, restart recovery, atomic file
  replacement, SHA-256 concurrency checks, allowed artifact paths, and restricted
  example-to-signal links. Interrupted tasks return to pending after restart;
  completed results survive. Resume using the original request and top-level steps.
- Signal models, stable IDs, strict unified-diff parsing, changed-side range
  validation, inbox receipts, exact Git revision/source readers, and a commit
  extraction boundary with an injectable semantic analyzer. Managed Git-history
  and retrospective Qodana workflows can discover additional corrective evidence.
- Task-bound sequential distribution with same-language embedding candidates, context receipts, one guarded
  Signal-to-cluster transition, a durable vector cache, and post-distribution validation. Code-example,
  cluster-example, and generation-state gates remain read-only. Generated inspection compilation and execution run through
  Ultimate's generic inspection MCP; this CLI contains only its lifecycle and HTTP client.
- GitHub and Space review readers, bounded selection, pagination, bot filtering,
  complete discussion provenance, exact source snapshots, task-bound batches,
  ordered coverage validation, publication receipts tied to exact Signal models, and normalized analyzed-PR coverage
  persisted at `extraction/pr-analysis-coverage.json` below the state root.
- An isolated Codex runner for executing the bundled managed skills with a real
  model and native subagents.

The managed skill sources live in this subproject under
[`src/main/resources/skills`](src/main/resources/skills), including their agent
metadata and shared references. Edit these local copies directly. Gradle packages
them in the application JAR and generates their resource index; Kotlin's `Skills`
loader reads and installs the bundle from the classpath. Both the CLI and Codex
runner use that loader, with no access to the parent project's skill directory.
[`EDICT_RUN_FLOW.md`](EDICT_RUN_FLOW.md) describes how these skills and MCP tools form one run.

The original Go implementation remains outside this project. The inspection-generation skills are bundled as orchestration clients;
compiling/running inspection scripts requires separately supplied inspection
tools. This project does not embed an IDE or implement an inspection compiler.

Use a new state root when switching from the old `edict-next-*` registry: existing
inbox, cluster, and inspection artifacts retain their layout, but old execution
plans are not automatically migrated to the renamed registry.

## Source organization

Production code lives under `src/main/kotlin/org/jetbrains/qodana/edict`:

| Package | Responsibility |
| --- | --- |
| `edictnext` | Canonical signal model, execution state, distribution and generation lifecycle |
| `skills` | Managed skill registry, policies and classpath resources |
| `store` | Persistent artifacts, capabilities, lifecycle and inbox receipts |
| `signals` | Structural signal validation and unified-diff parsing |
| `git` | Exact repository evidence and commit extraction |
| `reviews` | GitHub/Space clients, review models and prepared PR analysis |
| `mcp` | Shared HTTP tool transport and dispatch |
| `setup` | Project-local Codex setup: skills, `.codex/config.toml`, server port |
| `runtime` | Isolated Codex execution and native session collection |
| `logging` | Redacted, correlated agent activity logs |
| `common` | JSON configuration/accessors, hashing and bounded subprocesses |

`Main.kt` retains the `org.jetbrains.qodana.edict.MainKt` CLI entrypoint.
The resource paths and persisted JSON contracts are unchanged by package layout.

## Tests

Unit tests live in `src/test/kotlin`. Integration tests and their shared harness
live in `src/integrationTest/kotlin`, in the `org.jetbrains.qodana.edict.integration`
package and its `support` subpackages. The shared clone/runtime harness, review
fixtures, compiler transport and generation assertions live alongside the
integration tests. Unit tests mirror the component packages; unit checks of the
integration harness also stay in `src/test/kotlin` and need no external checkout.
Both run through the standard `test` task; there is no separate live-test
task. Integration tests run by default, including the real Codex extraction test.

```sh
# Unit tests only; no Distillery checkout or model access required.
./gradlew test -PexcludeIntegrationTests=true
# All tests, including real model-backed extraction and generation.
ULTIMATE_EDICT_REPO=/path/to/ultimate.edict.master ./gradlew test --console=plain
# One integration test, also runnable directly from the IDE.
./gradlew test --tests '*LiveCommitExtractionTest' --console=plain
# Three commits and GitHub/Space review extraction, without an inspection compiler.
./gradlew test --tests '*LiveThreeCommitExtractionTest' --tests '*LivePrExtractionTest' --console=plain
# Generation with the same model as the retained Go generation baseline.
CODEX_MODEL=gpt-5.6-terra ULTIMATE_EDICT_REPO=/path/to/ultimate.edict.master \
  ./gradlew test --tests '*LivePipelineTest' --console=plain
```

Every integration test inherits `IntegrationTest`, which prepares an
`IntegrationWorkspace` before each test method. The harness:

- Clears only that test's previous output under
  `out/integration/<test-class>/<method>-<hash>/`, retaining the latest run after
  success or failure. A per-test file lock rejects concurrent cleanup of an active
  run, and cleanup rejects symlink redirects.
- Clones Distillery with `git clone --no-local`. Set `DISTILLERY_TEST_REPO` to a
  local clone or Git URL. By default it uses
  `~/prj/examples/distillery-test` when available, otherwise
  `ssh://git@git.jetbrains.team/sa/distillery-test.git`.
- Checks out the test's pinned commit on `main` (by default
  `16f44bad3587e0f95ec5ff711ba213820de9ed35`),
  resets and cleans the disposable clone before execution, and uses its
  `testHistoryFixSignals` project and `.edict` state. The original checkout,
  including any uncommitted changes, is never cleaned or modified.
- Verifies that tracked fixture content and HEAD remain unchanged and that new
  repository files stay within the fixture's managed state directory.

Scripted MCP tests and the actual model test use the same Distillery correction:
`PollingWaiter.java` changes from `Thread.sleep(1_000)` to
`workerFinished.await(1, TimeUnit.SECONDS)`. Assertions validate both signals,
exact revisions, canonical Git diff bytes and historical changed-line evidence.
CLI and provider integration tests use the same clone and output lifecycle.

The model test asks only **“Extract signals from the latest commit.”** It installs
Edict into the fixture project like `qodana edict install`, keeps only the provider and the
project trust in an isolated Codex home, passes test-only settings (server URL, enabled tools,
extra writable roots) as `codex -c` overrides, runs a shared Kotlin MCP server
outside the agent sandbox, and requires a completed batch and distinct native
evidence worker. `CODEX_BIN` overrides the CLI executable; `CODEX_MODEL` overrides
the default `gpt-5.6-sol` model (high reasoning effort). The runner inherits only
the host's active provider definition; otherwise Codex uses its default provider.
Existing `auth.json` is copied when needed. Host hooks, MCP servers and permissions
are not inherited. Missing provider or fixture access fails the integration test.

`LiveThreeCommitExtractionTest` extracts six signals from three pinned corrections:
string equality, locale-independent normalization, and integer multiplication
overflow. `LivePrExtractionTest` exercises GitHub and Space through local authenticated
HTTP fixtures, using the real provider clients and native managed workers. Each
provider must produce both sides of the review correction with complete provenance.

`LivePipelineTest` extracts the overflow correction, clusters its two signals and
generates an accepted inspection. It launches the external Ultimate inspection MCP
server from `ULTIMATE_EDICT_REPO` using Bazel; this is a test prerequisite, not a JVM
dependency. It uses a disposable compiler project copy and audits compiler calls in
`log/inspection-mcp.jsonl`. The test independently recompiles the accepted bytes
against original Git evidence and synthetic examples, and verifies reviews accepted
that exact inspection hash. Compiler startup output is retained in `log/intellij-mcp.log`.
Generation allows up to 60 minutes for model execution, including candidate revisions
and independent reviews. Extraction tests retain their 20-minute execution limit.

Each test's `log/process-log/<run-id>/` directory contains:

- `edict-agents.log`: manager and worker commentary/final messages, task
  assignments, MCP lifecycle activity and complete response details.
- `edict-agent-short.log`: the same agent messages with concise MCP summaries.
- `tasks/<full-task-id>.log`: one detailed log per managed task/subagent, containing
  only that task's assignment, MCP details, commentary and final message.
- `edict-mcp.log` and `edict-mcp-system.log`: tool activity and protocol details.

Agent logs update during execution, correlate native workers with skill/task
identities, wrap lines at 120 characters, and redact issued capability tokens,
including revoked tokens. Runtime user-message events, reasoning and unrelated sessions
are excluded. Failures still flush available agent output. Raw runtime traces
remain in private `trace/` and `codex-home/sessions/` directories. Agents cannot
read those traces or the shared logs.

Run Gradle tests sequentially within a checkout, including runs started by the IDE.
Overlapping IDE and terminal builds share `build/test-results/test/binary`; one
can finalize `in-progress-results-generic.bin` while another still needs it,
causing `:test` to fail with `NoSuchFileException` even when its tests pass. Let
active builds finish, then rerun the failed command. Do not clean or move build
outputs while a build is running. Use separate checkouts for concurrent runs.

## Host configuration and boundaries

The host must keep the authoritative state root read-only to **all agents** and
run the server outside that sandbox. JVM path checks reject traversal, symlinks,
and unsupported artifact names; they are not a replacement for host filesystem
isolation against a process that can replace directories concurrently. Keep raw
runtime traces and parent/sibling conversations inaccessible to workers: a
capability is a bearer secret. `qodana edict install` denies agents the server logs and grants
writes only to scratch space; the supplied runner also denies its trace and session directories.

Plan creation is the only unauthenticated mutation. The first successful caller
claims manager authority once per server lifetime. Run HTTP only on a trusted
local host; it is not an authenticated multi-user network service. Read tools are
public. Each worker must fetch its assignment and declare its registered skill
before starting; its `agentId` records the runtime's asserted identity.

Multi-file distribution/generation is not transactional. Publish and verify the
destination before removing an inbox source, and reconcile partial progress on
retry. The server validates structure and consistency with the supplied diff;
semantic correctness and source authenticity still require the worker's review.
`GitRepository.validateEvidence` additionally checks a commit signal against
actual Git objects, canonical diff bytes, and historical file lengths.

Provider settings are host environment variables: `GITHUB_TOKEN` (or `GH_TOKEN`),
`SPACE_TOKEN`, and optionally `EDICT_GITHUB_API_URL` / `EDICT_SPACE_URL`. Provider
credentials are never tool arguments. Redirects are disabled and incomplete
provider evidence fails validation.

## Implementation references

The managed API and policies were ported from the former Go implementation
(`edict/managed`), and the local skill copies originated from `edict/skills/managed`.
Both have been removed; see Git history. The reference implementation in Ultimate informed these
standalone boundaries:

- `predict/agent/codex/commit-analysis-utils.kt`: bounded commit work items,
  stable identities, and evidence tied to the selected side.
- `predict/agent/codex/PRAnalysisMcpService.kt` and `ExtractionMcpToolset.kt`:
  prepared discussions, complete inspection coverage, and exact revision reads.
- `edict/store/repository-validation-utils.kt`: stable signal IDs, concise
  descriptions, inbox receipts, and unified-diff/range validation.

IntelliJ `PatchReader` is replaced by `UnifiedDiff`; EEL and project services are
replaced by Java NIO, argument-array Git subprocesses, and explicit host objects.
Unit tests build minimal local Git histories. Integration tests share the pinned
Distillery clone fixture. Only the generation integration test requires an external
Ultimate checkout and its inspection compiler.
