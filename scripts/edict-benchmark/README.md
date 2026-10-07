# Kotlin skills generation benchmark

TeamCity configuration: `StaticAnalysis_Edict_Benchmarks_JenkinsKotlinSkills`.
The fixture VCS root checks out `qodana/edict-jenkins` into `project`; the source
VCS root checks out `JetBrains/qodana-cli`, branch `avafanasev/edict-master`, into
`qodana-cli`. Scripts never fetch or check out repositories.

The pipeline has six steps:

1. `install-codex.sh` installs pinned Codex and writes a temporary user-level `CODEX_HOME` holding only
   the LiteLLM provider (using the secure `LITELLM_API_KEY` environment parameter) and the trust
   entry for `project`, without which Codex ignores `project/.codex/config.toml`. `common.sh` exports
   it as `CODEX_HOME` for every step, so the trust check of `qodana edict install` sees that entry too.
2. `install-qodana.sh` generates the embedded tooling, builds a static Qodana CLI directly
   from the source checkout with `CGO_ENABLED=0 go build`, and installs it under the
   benchmark tooling directory. TeamCity runs this step in the repository's published
   devcontainer image, so the host agent does not need a Go installation and does not
   need to match the container's glibc version.
3. `install-intellij.sh` verifies and extracts the native IntelliJ distribution supplied
   by the TeamCity artifact dependency.
4. `prepare.sh` runs `qodana edict install` in `project`, which writes the skills, sandbox permissions,
   agent limits and `edict-mcp` address into `project/.codex`; `--deny` adds the benchmark answers and
   Codex session records. The script then starts a shared `qodana edict mcp start` HTTP server in
   `project` on `edict.mcpPort` from the fixture's `qodana.yaml` (27182 when unset), using the checked-out
   project's **`project/.edict`** as both its Edict repository and managed state directory, with logs in
   `project/log`. The server starts the native
   `idea mcpServer` headless entry point from `QODANA_DIST` on the first inspection call,
   with the benchmark's IDE properties and a 20-minute readiness allowance, and forwards
   the IntelliJ tools, so Codex configures only `edict-mcp`.
   Distribution embeds Signals in the MCP JVM with ONNX Runtime; the GTE model is
   downloaded once into `${XDG_CACHE_HOME:-~/.cache}/JetBrains/Qodana/edict/models`. The existing
   `.edict/inbox` files are used directly, without importing, copying, or filtering
   signals. The shared managed endpoint remains
   available to isolated Codex workers, and MCP tool calls are auto-approved.
5. `generate.sh` executes Codex in `project` with `$edict_manager process inbox and generate new rules`.
   The prompt carries no paths: skills read the Edict state, inspected project, and private scratch
   paths from the `edict_context` MCP tool.
   Generation uses TeamCity’s normal execution mode so cancellation remains
   interruptible. TeamCity cleans up server processes when the build finishes.
6. A TeamCity **Gradle runner** executes `:benchmark:report`. Kotlin runs the accepted
   inspections from `.edict/inspections` natively, writes SARIF into `benchmark-output`,
   and compares it with `.edict/gold.sarif.json`. During this analysis it prices every
   manager and subagent session using current official OpenAI Standard rates, prints
   the token-type and stage breakdown, writes
   `benchmark-output/log/edict/edict-price-report.json`, embeds the complete price
   report in `report.json`, and publishes `edict.priceUsd` and `edict.totalTokens` as
   TeamCity statistics. Missing plan, session, pricing, or usage data fails reporting.
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
reserving slots for their reviewers and example workers. A cluster repeats the cheap
cycle of generation, strong-example validation and shallow review as often as needed;
the managed server allows each cluster three project analyses (followed by weak-signal
review) per run, `edict.generation.maxProjectAnalyses` in `qodana.yaml`. Weak examples
target recall and never block publication. Strong examples and exact-candidate
provenance remain mandatory.
State is writable only through Edict MCP.
The MCP server logs each task transition as `[cluster:taskId] task name started/finished`,
including nested reviewers and example workers. Tasks outside a cluster use `-` as the
cluster. Failed or cancelled tasks also emit `finished`; their outcome remains in the
plan and detailed MCP log. These messages go to server stderr and `edict-tasks.log` in the server's run folder
`project/log/process-log/<run-id>` (the run id is its start time; preparation records the folder in
`benchmark-output/edict-process-log`), which the generation step streams to the TeamCity console.
Full assignment, MCP response, commentary, and final-message records for each managed
task are also retained in `tasks/<full-task-id>.log` there. Agents cannot read
`project/log/process-log`; their scratch lives in `project/log/agent-work/<run-id>/scratch`.
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

The job timeout is 300 minutes. Reports, persisted state, server logs (`project/log`; the TeamCity
artifact rules must include it), native analysis logs, and the exact Bash scripts are build artifacts; credentials and Codex session
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
