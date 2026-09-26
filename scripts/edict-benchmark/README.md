# Kotlin skills generation benchmark

TeamCity configuration: `StaticAnalysis_Edict_Benchmarks_JenkinsKotlinSkills`.
The fixture VCS root checks out `qodana/edict-jenkins` into `project`; the source
VCS root checks out `JetBrains/qodana-cli`, branch `avafanasev/edict-master`, into
`qodana-cli`. Scripts never fetch or check out repositories.

The pipeline has four steps:

1. `install-codex.sh` installs pinned Codex, configures LiteLLM using the secure
   `LITELLM_API_KEY` environment parameter, extracts the native distribution, and
   installs a supplied assembled Qodana CLI or builds it from the checkout, including
   the current embedded Kotlin application and skills.
2. `prepare.sh` runs `qodana edict install`, enables every installed skill in
   Codex configuration, and starts `qodana edict mcp start` using the checked-out
   project's **`project/.edict`** as its state directory. It starts inspections with
   `qodana edict linter-mcp start`, selecting native execution through `QODANA_DIST`. The existing
   `.edict/inbox` files are used directly, without importing, copying, or filtering
   signals. The CLI launches the native `idea mcpServer` headless entry point
   and waits for readiness.
   Both servers must be ready before execution. MCP tool calls are auto-approved.
3. `generate.sh` executes Codex directly with `process inbox and generate new rules`.
   The prompt also supplies the existing state and private scratch paths.
   Generation uses TeamCity’s normal execution mode so cancellation remains
   interruptible. TeamCity cleans up server processes when the build finishes.
4. A TeamCity **Gradle runner** executes `:benchmark:report`. Kotlin runs the accepted
   inspections from `.edict/inspections` natively, writes SARIF into `benchmark-output`,
   and compares it with the checked-in specifications and `.edict/gold.sarif.json`.
   No benchmark Kotlin controller runs before Codex.

The ARM64 Qodana distribution comes from the latest successful `qodana-jvm: edict`
build (`ijplatform_master_QodanaJvmEdict`, main branch). TeamCity downloads the
archive and its checksum; the first step discovers and unpacks it, then sets
`env.QODANA_DIST` for subsequent steps. The CLI is built from the source VCS checkout so its
embedded skills and managed state server match that revision.
`QODANA_DIST` points to the extracted native distribution and `QODANA_CLI` to the
CLI executable. No container is used. Separate `QODANA_CONF` directories prevent
MCP and analysis IDE instances from contending for the same configuration lock.

For a custom-branch run, first assemble `ijplatform_master_QodanaCliAll` (Snapshot
2026.3 CLI) at the branch's exact commit. In the benchmark custom run, pin the CLI
source VCS root to that same branch and commit, retain the native distribution
dependency, and add a per-run artifact dependency on that successful CLI build:
`cli_linux_arm64_v8.0/qodana => cli-artifacts`.
Set `env.BENCHMARK_CLI_PATH=cli-artifacts/qodana`,
`env.BENCHMARK_CLI_REVISION=<full commit SHA>`, and
`env.BENCHMARK_CLI_BUILD_ID=<assembly build ID>`. The install step checks the source
revision, installs the assembled binary, and skips Go generation/build. It records
the build ID, revision, and binary checksum in the benchmark output. These custom
run overrides do not change the default benchmark branch or its dependencies.

Generation can compile and execute examples with inspections MCP.
Codex permits 50 simultaneous agents. Generation keeps up to 15 cluster workers active,
reserving slots for their reviewers and example workers. Each cluster gets at most
three review iterations including its initial candidate; the managed server caps each
review stage at three tasks per cluster worker, preserving that count across restarts.
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
The benchmark specification directory and `.edict/gold.sarif.json` are denied to
Codex. All existing inbox signals remain available, including optional examples
already supplied by the repository. Their IDs, revisions, labels, ranges and original
feedback are preserved. Setup creates no signals or clusters: Edict creates clusters
while processing the inbox in place. Kotlin accepts the checked-in SubmittedFeedback
format (`inspectionName`, `inspectionDescription`, `codeSnippet`, `reason`,
`suggestionId`) directly. Comparison discovers the resulting membership from each
signal's `source.suggestionId` (or managed feedback provenance). Split clusters are
scored together for their source specification, with duplicate findings counted once;
merged clusters are evaluated against each contributing specification. Missing clusters
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
