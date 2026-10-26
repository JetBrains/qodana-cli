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
/path/to/edict/kotlin/build/install/edict/bin/edict mcp
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
limits, and the `edict-mcp` server on loopback port `edict.mcpPort`. The port and state path come from the
`edict` section of the project's `qodana.yml` or `qodana.yaml` (27182 and `.edict` when absent); re-run
`install` after changing the port. Installation logs the installed names. Existing unrelated skills are preserved.
Then run Codex in the project with an explicit `$edict_manager <request>`; skills read the run's
paths from the `edict_context` tool.

From the repository root:

```sh
go generate ./internal/tooling/...
go build -o qodana ./cli
cd /path/to/project
/path/to/qodana edict install
/path/to/qodana edict mcp start
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
`intellij-mcp.log` in the run's log folder, and the IDE's own logs to `intellij-mcp/log/idea.log` next to it.

Set `edict.projectAnalysisTimeoutMinutes` in `qodana.yaml` to override the 30-minute timeout for each whole-project
inspection analysis. The value must be a positive whole number of minutes. Its HTTP request timeout is this value
plus 10 minutes; other IDE HTTP requests default to 60 minutes.

Distribution uses `edict.distribution.defaultSignalCount` in `qodana.yaml`, defaulting to 300 Signals per run,
independently of `edict.extraction.defaultSignalCount` (100 by default). The distribution limit can only be set in
`qodana.yaml`; `edict_next_next_signal` takes no count argument.

```yaml
edict:
  extraction:
    defaultSignalCount: 100
  distribution:
    defaultSignalCount: 300
```

`mcp` serves one shared Streamable HTTP endpoint on loopback port `edict.mcpPort`, so every native
agent shares one server and one state lock. It fails when `.codex/config.toml` is missing or names
another port, or when the port is busy; it never moves to another port. State is read from
`edict.statePath` in qodana.yaml (default `.edict`) and is also the reference repository for distribution checks; the
project's Git repository validates commit Signals. Every process is a run named by its start time (UTC, for example
`2026-10-06T14-03-12.345Z`) and logs to `log/process-log/<run-id>/`: activity and redacted tool details in
`edict-mcp{,-system}.log`, and INFO and above, with stack traces, in `edict.log`, which stdout also shows
(`src/main/resources/logback.xml`). The server prints its folder as `Process log: <path>`. Each managed task receives
separate scratch at `log/agent-work/<run-id>/scratch/<task-id>` (`scratch/manager` for the root manager).
`common/EdictLayout.kt` names all of these paths; the `edict.log.dir` JVM system
property moves the `log` folder (relative to the project), and `edict.run.id` fixes the run id.

Optional price reporting is enabled with `edict.calculatePrice: true` in `qodana.yaml`. It exposes the manager-only
`edict_calculate_price` MCP tool, which reads each model from the Codex session log, uses the current official OpenAI
Standard rates, and writes
`edict-price-report.json` beside the other private process logs while also printing a compact breakdown to
`edict-tasks.log` (the stream CI follows) and `edict.log`. The breakdown shows USD and token totals for every pipeline
step, cluster, and descendant task, including each task's own cost when it has children. Inclusive costs contain all
descendants and must not be added to their child rows. Detailed token categories remain in the JSON report.
The manager includes only the run total in its final response after the pipeline completes.

## What is ported

- All 18 managed skills, their invocation policies, and shared references.
  Worker registry IDs and installed skill names both use `edict-*`.
  The manager remains `edict_manager`.
- Promoting weak signals: `edict-promote-weak-signal` selects up to five reviewed true-positive locations per Generated
  inspection with referenced strong positive Signals from fewer than five distinct source files, attempts focused source fixes, and opens a GitHub or Space
  PR asking users to accept the improvements if they agree. It runs after normal promotion in the default pipeline.
  Review evidence is persisted in `evaluation.json`; older inspections need regeneration to populate it. Source-fix
  PRs require the configured target to remain at the analyzed revision, and are recorded separately from inspection
  promotion so repeated runs do not create duplicate PRs.
  The default pipeline runs `edict-check-weak-signal-promotion` as a separate step alongside signal extraction. It refreshes created
  weak-signal source-fix PRs. Promotion records contain only review references and states; actual merged corrections
  become regular strong Signals. Covered evidence goes directly to the originating cluster, implicitly processed;
  other evidence goes to the inbox for normal distribution. Active reviews are indexed under `promotions/active.json`,
  separately from completed history, so checks do not search all clusters. Extraction can continue while checks run or fail.
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
- Task-bound distribution with same-language embedding candidates, context receipts, one guarded Signal-to-cluster
  transition, and post-distribution validation. The manager chooses generation clusters using read-only suggestions or
  the user's requested targets, then runs one `edict-git-history-signal-enrichment` task per `clusterId`. Each task searches
  bounded Git history, analyzes all member Signals and candidate corrections in the same agent without child workers,
  and assigns newly found strong Signals directly to
  that cluster. The manager alone keeps the chosen cluster list and directly launches one cluster-generation worker
  per target. Selection creates no state. No second distribution pass is needed.
  A durable vector cache supports distribution. Code-example,
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
# Enrich a selected Signal's cluster, without an inspection compiler or generation.
./gradlew :test --tests '*LiveEnrichmentTest' --console=plain
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

`LiveEnrichmentTest` loads the inspected project and prepared `.edict` state from the dedicated
`testEnrichSignalFromGitHistory` folder in the pinned `distillery-test` revision, without constructing Signals or clusters
in the test. The harness copies the committed seed to disposable state and leaves the tracked fixture unchanged.
The test uses a short request to enrich its prepared cluster. The real history coordinator reads member Signals through
`edict_get_cluster_signals`, obtains a server-computed history range, and publishes independent strong evidence directly
into that cluster. The range is the intersection of the latest 100 commits and the last 90 days, anchored to an exact
HEAD revision. It verifies actual Git evidence and that every analyzed candidate belongs to the returned range, with no
distribution, generation selection, inspection generation, or IntelliJ startup. No analyzed-commit coverage is persisted;
legacy history-analysis files are ignored.

`WeakSignalPromotionIntegrationTest` and `WeakSignalPromotionReviewIntegrationTest` use the dedicated
`testPromoteWeakSignals` project and its committed `.edict` state from independent pinned Distillery revisions.
They ask only “Promote weak signals” and “Check weak signal promotion reviews”, running the real manager and native workers with
a deterministic review provider. Creation verifies the proposed source fix and persisted PR; checking starts with an
existing PR and verifies its state and actual merged Signals are saved directly in the originating cluster, with covered evidence processed.

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
semantic correctness still requires the worker's review. PR extractors retain a relevant unified-diff excerpt of at
most 4000 characters (including headers and newlines) in `diff.excerpt`. The server verifies its source lines, paths,
and original coordinates against the complete provider diff before persisting it as `diffPositiveToNegative`.
Extractors omit unrelated hunks/context and recount split hunk headers; oversized or altered excerpts fail validation.
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
