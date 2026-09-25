# Kotlin skills generation benchmark

TeamCity configuration: `StaticAnalysis_Edict_Benchmarks_JenkinsKotlinSkills`.
The fixture VCS root checks out `qodana/edict-jenkins` into `project`; the source
VCS root checks out `JetBrains/qodana-cli`, branch `avafanasev/edict-master`, into
`qodana-cli`. Scripts never fetch or check out repositories.

The pipeline has four steps:

1. `install-codex.sh` installs pinned Codex and configures LiteLLM using the secure
   `LITELLM_API_KEY` environment parameter.
2. `prepare.sh` installs the standard Edict application and its Kotlin skills,
   and starts Edict MCP using the checked-out project's **`project/.edict`** as its
   state directory, alongside the native inspections MCP server. The existing
   `.edict/inbox` files are used directly, without importing, copying, or filtering
   signals. The supplied archive uses its built-in
   `idea mcpServer` headless entry point; its AI Assistant plugin predates the
   Qodana-specific `mcp-server` script. Both must be ready before execution.
3. `generate.sh` executes Codex directly with `process inbox and generate new rules`.
   Its exit trap stops both servers, including on failure or cancellation. Generation
   uses TeamCity’s normal execution mode so cancellation remains interruptible. Raw Codex transcripts
   stay private because MCP responses contain temporary delegation capabilities.
4. A TeamCity **Gradle runner** executes `:benchmark:report`. Kotlin runs the accepted
   inspections from `.edict/inspections` natively, writes SARIF into `benchmark-output`,
   and compares it with the checked-in specifications and `.edict/gold.sarif.json`.
   No benchmark Kotlin controller runs before Codex.

The pinned ARM64 Qodana distribution comes from build **1070981529**. The matching
CLI comes from its dependency **1070964192**. TeamCity downloads these artifacts;
`QODANA_DIST` points to the extracted native distribution and `QODANA_CLI` to the
CLI executable. No container is used. Separate `QODANA_CONF` directories prevent
MCP and analysis IDE instances from contending for the same configuration lock.

Generation can compile examples with inspections MCP and use `inspect-project.sh`
for complete project findings in scratch, reusing a serialized project workspace
and IDE cache across candidates. State is writable only through Edict MCP.
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

`BENCHMARK_MINUTES` bounds Codex execution (default 240).
The job timeout is 300 minutes. Reports, persisted state, native analysis logs,
and the exact Bash scripts are build artifacts; credentials and raw Codex sessions
are not published.

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
