# Contributing to Edict

This guide covers building and testing Edict and how its code is organized. To use Edict, see the
[README](README.md).

## Architecture

Edict has three parts:

- **Go commands** in [`edict/cmd`](../cmd): `qodana edict install`, `mcp start|stop`, and `state push`. They start the
  Kotlin application with Qodana's embedded JBR in the project directory. The hidden `qodana edict ide-mcp` helper runs
  the native IDE as a child process. It prints one line when the IDE is ready and stops the IDE when its stdin closes.
- **Kotlin application** (this directory): a standalone Gradle project. It needs JDK 21+ and Git, and uses the Kotlin
  MCP SDK with Ktor for the Streamable HTTP transport. It doesn't depend on IntelliJ, the Qodana CLI, Go, or Bazel.
  It owns the authoritative state, task and capability management, and state validation.
- **Managed skills** in [`src/main/resources/skills`](src/main/resources/skills): 19 skills with their agent metadata
  and shared references. The workers are named `edict-*`, and the root skill is `edict_manager`. Codex runs them as
  native agents, so orchestration and task lifecycle stay in the agents.

The Kotlin application doesn't compile or run inspections itself. It does that through the generic inspection MCP of
IntelliJ Ultimate, and contains only the lifecycle and HTTP client for it.

## Building

Standalone:

```sh
cd edict/kotlin
./gradlew test -PexcludeIntegrationTests=true installDist
build/install/edict/bin/edict --help
# or: ./gradlew bundledJar && java -jar build/libs/edict-cli.jar --help
```

Embedded in the Qodana CLI, from the repository root:

```sh
go generate ./internal/tooling/...
go build -o qodana ./cli
```

`go generate` does the following:

- Runs `./gradlew bundledJar` and embeds the JAR together with the other Java tools.
- Updates the module set of the bundled runtime. Modules are detected from the portable JAR, and multi-release JARs
  are read.
- Writes one JAR per target to `internal/tooling/edict/libs/<os>-<arch>/`. Each JAR keeps only that target's ONNX
  Runtime native libraries, and Go build constraints embed only the matching JAR. This lets parallel
  cross-compilation run without pulling in other platforms' libraries.

ONNX Runtime 1.22.0 has no Windows ARM64 natives, so local embeddings are unavailable on that target. The JAR name
includes a content hash and the runtime name includes a module hash, so stale extracted tools are never reused.

## Source layout

Production code is under `src/main/kotlin/org/jetbrains/qodana/edict`:

| Package | Responsibility |
| --- | --- |
| (root) | `Main.kt` (the `org.jetbrains.qodana.edict.MainKt` entry point) and `EdictServer.kt` (the shared HTTP MCP server) |
| `core` | Signal model, execution state, distribution, generation, embeddings, and the IDE/inspection MCP clients |
| `skills` | Managed skill registry, invocation policies, and classpath loading |
| `signals` | Structural Signal and path validation, unified-diff parsing |
| `extraction` | Commit Signal extraction (`git`) and prepared PR analysis (`reviews`) |
| `history` | Git-history enrichment of a cluster |
| `ci` | GitHub and Space review clients and models |
| `promotion` | Inspection promotion PRs |
| `setup` | Project-local Codex setup: skills, `.codex/config.toml`, server port |
| `runtime` | Codex price analysis and reports |
| `logging` | Redacted, correlated agent activity logs |
| `common` | Configuration, file layout (`EdictLayout.kt`), Git access, state branch, hashing, JSON, subprocesses |

`common/EdictLayout.kt` defines all run paths. The `edict.log.dir` JVM property moves the `log` folder (relative to
the project), and `edict.run.id` fixes the run id. Logging is configured in `src/main/resources/logback.xml`.

### Skills

Edit the skill sources in `src/main/resources/skills` directly. Gradle packages them in the JAR and generates their
resource index. The CLI and the test Codex runner both install them through the `Skills` classpath loader. Neither
reads the parent project's skill directories.

## Boundaries and invariants

- **State isolation.** The host must keep the state root read-only to all agents and run the server outside their
  sandbox. The JVM checks reject path traversal, symlinks, and unsupported artifact names. These checks don't replace
  host filesystem isolation against a process that swaps directories concurrently.
- **Capabilities.** Capabilities are bearer secrets. They are held only as hashes in memory, never saved or logged,
  and they are redacted from agent logs, including after revocation. Keep raw runtime traces and other agents'
  conversations away from workers.
- **Authority.** Creating a plan is the only unauthenticated mutation, and the first caller to do it becomes the
  manager once per server lifetime. Read tools are public. Each worker must fetch its assignment and declare its
  registered skill before it starts. The worker's `agentId` records the identity the runtime asserts.
- **Persistence.**
  - Plans are persistent, and files are replaced atomically with SHA-256 concurrency checks.
  - After a restart, interrupted tasks go back to pending and completed results are kept. A run resumes from its
    original request and top-level steps.
  - Writes that span several files are not transactional. Publish and verify the destination before removing an inbox
    source, and reconcile partial progress on retry.
- **Evidence.**
  - PR extractors keep a unified-diff excerpt of at most 4000 characters in `diff.excerpt`. The server checks its
    lines, paths, and coordinates against the full provider diff before storing it as `diffPositiveToNegative`.
  - `GitRepository.validateEvidence` checks commit Signals against actual Git objects, canonical diff bytes, and
    historical file lengths.
  - Provider credentials are never tool arguments, HTTP redirects are disabled, and incomplete provider evidence fails
    validation.
  - The server validates structure and consistency. Whether a Signal is semantically correct still depends on the
    worker's review.

## Tests

### Go

```sh
go test ./edict/cmd/...
```

The Go tests launch the real embedded JAR and JBR. They cover the following:

- Local installation and bundled resources.
- HTTP MCP on the configured port.
- Redacted logs, errors, shutdown, and release of the state lock.

They need no model, Distillery checkout, or IDE.

### Kotlin

- Unit tests are in `src/test/kotlin` and follow the production package layout. Unit checks of the integration
  harness are also there.
- Integration tests and their shared harness are in `src/integrationTest/kotlin`, under
  `org.jetbrains.qodana.edict.integration` and its `support` subpackages.

The standard `test` task runs both. Integration tests run by default and use a real model.

```sh
# Unit tests only: no Distillery checkout or model access needed.
./gradlew test -PexcludeIntegrationTests=true
# All tests.
ULTIMATE_EDICT_REPO=/path/to/ultimate.edict.master ./gradlew test --console=plain
# A single integration test (also runnable from the IDE).
./gradlew test --tests '*LiveCommitExtractionTest' --console=plain
./gradlew test --tests '*LiveThreeCommitExtractionTest' --tests '*LivePrExtractionTest' --console=plain
CODEX_MODEL=gpt-5.6-terra ULTIMATE_EDICT_REPO=/path/to/ultimate.edict.master \
  ./gradlew test --tests '*LivePipelineTest' --console=plain
./gradlew test --tests '*LiveEnrichmentTest' --console=plain
```

Run Gradle tests one at a time in each checkout, including runs started from the IDE. Overlapping builds share
`build/test-results/test/binary`. One build can finalize `in-progress-results-generic.bin` while another still needs
it, which makes `:test` fail with `NoSuchFileException` even when the tests pass. If that happens, wait for the
running builds to finish, then rerun. Use separate checkouts for concurrent runs.

### Integration harness

Every integration test extends `IntegrationTest`, which prepares an `IntegrationWorkspace` before each test method.
The workspace works as follows:

- **Output.** Output goes to `out/integration/<test-class>/<method>-<hash>/`. Only that test's earlier output is
  cleared, and the latest run is kept whether it passes or fails. A per-test file lock blocks cleanup of an active
  run, and cleanup refuses to follow symlinks.
- **Fixture.** The harness clones Distillery with `git clone --no-local`. It uses `DISTILLERY_TEST_REPO` (a local
  clone or Git URL) when set, then `~/prj/examples/distillery-test` if it exists, and otherwise
  `ssh://git@git.jetbrains.team/sa/distillery-test.git`.
- **Pinned commit.** It checks out the test's pinned commit on `main` (by default
  `16f44bad3587e0f95ec5ff711ba213820de9ed35`), then resets and cleans the disposable clone. Your original checkout is
  never modified.
- **Checks.** After the test, it verifies that tracked fixture content and HEAD are unchanged and that new files stay
  inside the fixture's state directory.

The tests install Edict into the fixture the same way `qodana edict install` does.

- **Codex home.** Each test uses an isolated Codex home that keeps only the host's active provider definition and the
  project trust. `auth.json` is copied when needed. Host hooks, MCP servers, and permissions are not inherited.
- **Test settings.** Settings that only tests need (the server URL, enabled tools, extra writable roots) are passed as
  `codex -c` overrides.
- **Server.** The Kotlin MCP server runs outside the agent sandbox.
- **Model.** `CODEX_BIN` overrides the Codex executable, and `CODEX_MODEL` overrides the default model
  (`gpt-5.6-sol`, high reasoning effort).

A test fails if the provider or the fixture is not accessible.

| Test | What it covers |
| --- | --- |
| `LiveCommitExtractionTest` | Asks only "Extract signals from the latest commit." and checks the `PollingWaiter.java` correction (`Thread.sleep(1_000)` becomes `workerFinished.await(1, TimeUnit.SECONDS)`). Verifies both Signals, exact revisions, canonical diff bytes, and changed-line evidence. Requires a completed batch and a separate native evidence worker. |
| `LiveThreeCommitExtractionTest` | Six Signals from three corrections: string equality, locale-independent normalization, and integer multiplication overflow. |
| `LivePrExtractionTest` | GitHub and Space extraction through local authenticated HTTP fixtures, using the real provider clients. Each provider must produce both sides of the correction with complete provenance. |
| `LivePipelineTest` | Extracts, clusters, and generates an accepted inspection. Starts Ultimate's inspection MCP from `ULTIMATE_EDICT_REPO` with Bazel and records compiler calls in `log/inspection-mcp.jsonl`. Recompiles the accepted bytes against the original evidence and checks that the reviews accepted that exact hash. The time limit is 60 minutes; extraction tests have 20. |
| `LiveEnrichmentTest` | Enriches a prepared cluster from `testEnrichSignalFromGitHistory`. The history range is the intersection of the latest 100 commits and the last 90 days, anchored to HEAD. Verifies the Git evidence and that every analyzed commit is inside the range. Doesn't distribute, generate, or start IntelliJ. |

Each test writes logs to `log/process-log/<run-id>/`:

| File | Contents |
| --- | --- |
| `edict-agents.log` | Manager and worker messages, task assignments, and full MCP details |
| `edict-agent-short.log` | The same messages with short MCP summaries |
| `tasks/<task-id>.log` | One log for each managed task |
| `edict-mcp.log`, `edict-mcp-system.log` | Tool activity and protocol details |

The agent logs update during the run and wrap lines at 120 characters. They map native workers to skill and task
identities and redact capability tokens. User messages, reasoning, and unrelated sessions are left out, and available
output is still written when a test fails. Raw traces stay in private `trace/` and `codex-home/sessions/`
directories, which agents can't read.

## History

The managed API and policies were ported from the former Go implementation (`edict/managed`), and the skills come from
`edict/skills/managed`. Both have since been removed; they are in the Git history. These parts of the Ultimate
reference implementation shaped the boundaries:

- `predict/agent/codex/commit-analysis-utils.kt`: bounded commit work items, stable identities, and evidence tied to
  the selected side of a diff.
- `predict/agent/codex/PRAnalysisMcpService.kt` and `ExtractionMcpToolset.kt`: prepared discussions, complete
  inspection coverage, and exact revision reads.
- `edict/store/repository-validation-utils.kt`: stable Signal IDs, inbox receipts, and unified-diff and range
  validation.

Some IntelliJ components were replaced:

- IntelliJ `PatchReader` became `UnifiedDiff`.
- EEL and project services became Java NIO, Git subprocesses called with argument arrays, and explicit host objects.
