# Edict — managed skills for Kotlin/JVM

Standalone Gradle application for the managed part of Edict. Requires JDK 21+ and
Git; the Gradle wrapper downloads the pinned distribution. The only runtime
libraries beyond Kotlin are `kotlinx.serialization` and the TOML configuration
parser `tomlj`. There are no IntelliJ, Qodana
CLI, Go, or Bazel dependencies.

```sh
cd edict/kotlin
./gradlew test -PexcludeIntegrationTests=true installDist
build/install/edict/bin/edict --help
build/install/edict/bin/edict install-skills --directory /path/to/codex-home/skills
build/install/edict/bin/edict mcp --project-dir /path/to/source --state-dir /path/to/state
```

`mcp` uses newline-delimited MCP JSON-RPC on stdio. Stdout contains only protocol
messages; activity and redacted tool details are written to
`<project-dir>/log/edict/edict-mcp{,-system}.log`. Add `--http-port 0` to start a
shared Streamable HTTP endpoint on a dynamically allocated loopback port (printed
to stderr), or choose a fixed port. HTTP is useful when multiple native agents
must share one server and one state lock.

## What is ported

- All 13 managed skills, their invocation policies, and shared references.
  Registry IDs use `edict-*`; installed worker names use `managed-edict-*`.
  The manager remains `edict_manager`. The `-next-` component is removed from
  names, references, prompts, and registry policies in this project.
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
  extraction boundary with an injectable semantic analyzer.
- GitHub and Space review readers, bounded selection, pagination, bot filtering,
  complete discussion provenance, exact source snapshots, task-bound batches,
  ordered coverage validation, and publication receipts tied to exact bytes.
- An isolated Codex runner for executing the bundled managed skills with a real
  model and native subagents.

The managed skill sources live in this subproject under
[`src/main/resources/skills`](src/main/resources/skills), including their agent
metadata and shared references. Edit these local copies directly. Gradle packages
them in the application JAR and generates their resource index; Kotlin's `Skills`
loader reads and installs the bundle from the classpath. Both the CLI and Codex
runner use that loader, with no access to the parent project's skill directory.

The original Go implementation and unmanaged skill bundle remain outside this
project. The inspection-generation skills are bundled as orchestration clients;
compiling/running inspection scripts requires separately supplied inspection
tools. This project does not embed an IDE or implement an inspection compiler.

Use a new state root when switching from the old `edict-next-*` registry: existing
inbox, cluster, and inspection artifacts retain their layout, but old execution
plans are not automatically migrated to the renamed registry.

## Tests

Unit tests live in `src/test/kotlin`. Integration tests and their shared harness
live in `src/integrationTest/kotlin`, in the `org.jetbrains.qodana.edict.integration`
package. Both run through the standard `test` task; there is no separate live-test
task. Integration tests run by default, including the real Codex extraction test.

```sh
# Unit tests only; no Distillery checkout or model access required.
./gradlew test -PexcludeIntegrationTests=true
# All tests, including real model-backed extraction.
./gradlew test --console=plain
# One integration test, also runnable directly from the IDE.
./gradlew test --tests '*LiveCommitExtractionTest' --console=plain
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
- Checks out pinned commit `16f44bad3587e0f95ec5ff711ba213820de9ed35` on `main`,
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
the bundled skills in an isolated Codex home, runs a shared Kotlin MCP server
outside the agent sandbox, and requires a completed batch and distinct native
evidence worker. `CODEX_BIN` overrides the CLI executable; `CODEX_MODEL` overrides
the default `gpt-5.6-sol` model (high reasoning effort). The runner inherits only
the host's active provider definition. Without a custom provider,
`LITELLM_API_KEY` selects LiteLLM; otherwise Codex uses its default provider.
Existing `auth.json` is copied when needed. Host hooks, MCP servers and permissions
are not inherited. Missing provider or fixture access fails the integration test.

Each test's `log/edict/` directory contains:

- `edict-agents.log`: manager and worker commentary/final messages, task
  assignments, MCP lifecycle activity and complete response details.
- `edict-agent-short.log`: the same agent messages with concise MCP summaries.
- `edict-mcp.log` and `edict-mcp-system.log`: tool activity and protocol details.

Agent logs update during execution, correlate native workers with skill/task
identities, wrap lines at 120 characters, and redact issued capability tokens,
including revoked tokens. Runtime user-message events, reasoning and unrelated sessions
are excluded. Failures still flush available agent output. Raw runtime traces
remain in private `trace/` and `codex-home/sessions/` directories. Agents cannot
read those traces or the shared logs. The CLI accepts `--log-dir` to place its
`edict/` logs outside the source checkout.

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
capability is a bearer secret. The supplied runner denies agent access to its
trace and session directories and grants writes only to scratch space.

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

The managed API and policies were ported from `../managed`, and the local skill
copies originated from `../skills/managed`. Neither is a build or runtime
dependency. The reference implementation in Ultimate informed these
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
Distillery clone fixture and do not depend on Ultimate.
