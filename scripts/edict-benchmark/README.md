# Kotlin skills generation benchmark

TeamCity configuration: `StaticAnalysis_Edict_Benchmarks_JenkinsKotlinSkills`.
The fixture VCS root checks out `qodana/edict-jenkins` into `project`; the source
VCS root checks out `JetBrains/qodana-cli`, branch `avafanasev/edict-master`, into
`qodana-cli`. Scripts never fetch or check out repositories.

The pipeline has four steps:

1. `install-codex.sh` installs pinned Codex and configures LiteLLM using the secure
   `LITELLM_API_KEY` environment parameter.
2. `prepare.sh` installs the standard Edict application and its Kotlin skills,
   imports required fixture examples as SubmittedFeedback, and starts the Edict
   and native inspections MCP servers. Both must be ready before execution.
3. `generate.sh` executes Codex directly with `process inbox and generate new rules`.
   Its exit trap stops both servers, including on failure. Raw Codex transcripts
   stay private because MCP responses contain temporary delegation capabilities.
4. A TeamCity **Gradle runner** executes `:benchmark:report`. Kotlin runs the accepted
   inspections natively, writes SARIF, and compares it with the immutable fixture
   snapshot and gold SARIF. No benchmark Kotlin controller runs before Codex.

The pinned ARM64 Qodana distribution comes from build **1070981529**. The matching
CLI comes from its dependency **1070964192**. TeamCity downloads these artifacts;
`QODANA_DIST` points to the extracted native distribution and `QODANA_CLI` to the
CLI executable. No container is used. Separate `QODANA_CONF` directories prevent
MCP and analysis IDE instances from contending for the same configuration lock.

Generation can compile examples with inspections MCP and use `inspect-project.sh`
for complete project findings in scratch. State is writable only through Edict MCP.
Optional examples and gold are denied to Codex. Required examples preserve source
revisions, labels, ranges, and original feedback. Comparison follows feedback
provenance instead of prescribing cluster names. Scoring copies namespace the
literal first `InspectionKts` descriptor ID to `EdictBenchmark<RuleId>` to avoid
built-in inspection collisions; accepted scripts remain unchanged. Unsupported
or ambiguous descriptor/cluster mappings fail explicitly.

`BENCHMARK_RULES` is an optional comma-separated selection; `BENCHMARK_LIMIT=0`
selects all fixtures. `BENCHMARK_MINUTES` bounds Codex execution (default 240).
The job timeout is 300 minutes. Reports, persisted state, native analysis logs,
and the exact Bash scripts are build artifacts; credentials and raw Codex sessions
are not published.

Reapply the checked-in TeamCity configuration with an authenticated `teamcity` CLI:

```sh
./edict/kotlin/gradlew -p edict/kotlin :benchmark:configureTeamCity
```

Run native reporting locally after generation:

```sh
QODANA_DIST=/path/to/native/distribution QODANA_CLI=/path/to/qodana \
  ./edict/kotlin/gradlew -p edict/kotlin :benchmark:report \
  -PsourceProjectDir=/path/to/project \
  -PbenchmarkDir=/path/to/project/benchmark \
  -PgenerationDir=/path/to/benchmark-output
```

`:benchmark:compare` remains available to compare an existing SARIF without running
another analysis. Metrics and report fields follow `GenerationBenchmarkScript2.kt`.
