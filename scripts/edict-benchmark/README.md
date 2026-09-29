# Kotlin skills generation benchmark

TeamCity configuration: `StaticAnalysis_Edict_Benchmarks_JenkinsKotlinSkills`.
The fixture VCS root checks out `qodana/edict-jenkins` into `project`; the source
VCS root checks out `JetBrains/qodana-cli`, branch `avafanasev/edict-master`, into
`qodana-cli`. Scripts never fetch or check out repositories.

The pipeline has seven steps:

1. `install-codex.sh` installs pinned Codex, configures LiteLLM using the secure
   `LITELLM_API_KEY` environment parameter.
2. `install-qodana.sh` generates the embedded tooling, builds a static Qodana CLI directly
   from the source checkout with `CGO_ENABLED=0 go build`, and installs it under the
   benchmark tooling directory. TeamCity runs this step in the repository's published
   devcontainer image, so the host agent does not need a Go installation and does not
   need to match the container's glibc version.
3. `install-intellij.sh` verifies and extracts the native IntelliJ distribution supplied
   by the TeamCity artifact dependency.
4. `install-embedding-python.sh` provisions Python 3.12 with the pinned embedding
   dependencies. It creates a dedicated Python launcher that preloads the environment's
   OpenMP runtime before importing the embedding stack, avoiding the ARM64 static-TLS
   load-order failure without affecting the MCP JVM or unrelated build steps.
5. `prepare.sh` runs `qodana edict install`; Codex discovers the installed skills
   under `CODEX_HOME` and enables them by default. The script starts a shared
   `qodana edict mcp start` HTTP server using the checked-out
   project's **`project/.edict`** as both its Edict repository and managed state directory. It starts inspections with
   `qodana edict linter-mcp start`, selecting native execution through `QODANA_DIST`.
   The managed server uses the prepared embedding launcher rather than the agent's
   default Python. The existing
   `.edict/inbox` files are used directly, without importing, copying, or filtering
   signals. The CLI launches the native `idea mcpServer` headless entry point
   and waits for readiness.
   Both servers must be ready before execution. The shared managed endpoint remains
   available to isolated Codex workers, and MCP tool calls are auto-approved.
6. `generate.sh` executes Codex directly with `process inbox and generate new rules`.
   The prompt also supplies the Edict repository/state, inspected project, and private scratch paths.
   Generation uses TeamCity’s normal execution mode so cancellation remains
   interruptible. TeamCity cleans up server processes when the build finishes.
7. A TeamCity **Gradle runner** executes `:benchmark:report`. Kotlin runs the accepted
   inspections from `.edict/inspections` natively, writes SARIF into `benchmark-output`,
   and compares it with `.edict/gold.sarif.json`.
   No benchmark Kotlin controller runs before Codex.

The ARM64 Qodana distribution comes from the latest successful `qodana-jvm: edict`
build (`ijplatform_master_QodanaJvmEdict`, main branch). TeamCity downloads the
archive and its checksum; the IntelliJ installation step discovers and unpacks it, then sets
`env.QODANA_DIST` for subsequent steps. The CLI is built from the source VCS checkout so its
embedded skills and managed state server match that revision.
`QODANA_DIST` points to the extracted native distribution and `QODANA_CLI` to the
CLI executable. No container is used. Separate `QODANA_CONF` directories prevent
MCP and analysis IDE instances from contending for the same configuration lock.

For a custom-branch run, pin the CLI source VCS root to that branch and commit. The
benchmark always builds the CLI from that checkout; it does not consume a separately
assembled CLI artifact. The native distribution artifact dependency remains unchanged.

Generation can compile and execute examples with inspections MCP.
Codex permits 50 simultaneous agents. Generation keeps up to 15 cluster workers active,
reserving slots for their reviewers and example workers. Each cluster gets at most
five review iterations including its initial candidate; the managed server caps each
review stage at five tasks per cluster worker, preserving that count across restarts.
Only BLOCKER findings require repairs. MAJOR findings request another iteration while
budget remains but permit downstream checks and publication with recorded limitations.
MINOR findings are recorded without a repair loop. Required compilation/example checks
and exact-candidate provenance remain mandatory.
State is writable only through Edict MCP.
The MCP server logs each task transition as `[cluster:taskId] task name started/finished`,
including nested reviewers and example workers. Tasks outside a cluster use `-` as the
cluster. Failed or cancelled tasks also emit `finished`; their outcome remains in the
plan and detailed MCP log. These messages go to server stderr and `log/edict/edict-tasks.log`,
which the generation step streams to the TeamCity console. MCP stdio stdout stays JSON-RPC.
Full assignment, MCP response, commentary, and final-message records for each managed
task are also retained in `log/edict/tasks/<full-task-id>.log`.
The benchmark specification directory and `.edict/gold.sarif.json` are denied to
Codex. All existing inbox signals remain available, including optional examples
already supplied by the repository. Their IDs, revisions, labels, ranges and original
feedback are preserved. Setup creates no signals or clusters: Edict creates clusters
while processing the inbox in place. Kotlin accepts the checked-in SubmittedFeedback
format (`inspectionName`, `inspectionDescription`, `codeSnippet`, `reason`,
`suggestionId`) directly. Comparison discovers the resulting membership from each
signal's `source.suggestionId` (or managed feedback provenance). Split clusters are
scored together for their source rule, with duplicate findings counted once; merged
clusters are evaluated for each contributing rule against the gold SARIF. Missing clusters
are reported as `NotClustered`; mixed generated and unfinished clusters as
`PartiallyGenerated`. The report includes cluster membership and statuses.
Scoring copies namespace the literal first `InspectionKts` descriptor ID to
`EdictBenchmark<RuleId>` (with a numbered suffix for split clusters) to avoid built-in
inspection collisions; accepted scripts remain unchanged. Unsupported descriptors
fail explicitly.

The job timeout is 300 minutes. Reports, persisted state, native analysis logs,
and the exact Bash scripts are build artifacts; credentials and Codex session
files are not published.

Run native reporting locally after generation:

```sh
QODANA_DIST=/path/to/native/distribution QODANA_CLI=/path/to/qodana \
  ./edict/kotlin/gradlew -p edict/kotlin :benchmark:report \
  -PsourceProjectDir=/path/to/project \
  -PbenchmarkDir=/path/to/project/benchmark \
  -PedictStateDir=/path/to/project/.edict \
  -PbenchmarkOutputDir=/path/to/benchmark-output
```

`:benchmark:compare` remains available to compare an existing SARIF without running
another analysis. Metrics and report fields follow `GenerationBenchmarkScript2.kt`.
The output directory contains `source-revision.txt`, written by preparation, for
report provenance. The state artifact is archived directly from `project/.edict`.
