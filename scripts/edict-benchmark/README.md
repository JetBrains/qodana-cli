# Kotlin managed-skills benchmark

TeamCity configuration:
[Jenkins: Kotlin managed skills](https://buildserver.labs.intellij.net/buildConfiguration/StaticAnalysis_Edict_Benchmarks_JenkinsKotlinSkills)
under `StaticAnalysis_Edict_Benchmarks`.

The build uses the Jenkins dataset from the reference benchmark, the assembled
Qodana image `registry.jetbrains.team/p/sa/containers/qodana-jvm:263.SNAPSHOT.149`
from build `1070975243`, and the Kotlin `edict-cli.jar` from build `1070533116`
(`1.0.1`). Neither dependency triggers an upstream build. Codex `0.155.1` is
installed in the build output because the image's bundled `0.117` lacks the
permission profiles required by the Kotlin host.
The assembled image is used directly; there is no custom Dockerfile or image build.
Python orchestration runs on the Linux TeamCity agent, and Java, Codex and Qodana
run in the container with the checkout mounted at the same absolute path. Both
MCP endpoints use loopback through Docker host networking.

`BenchmarkHost.java` invokes the published Kotlin `CodexRunner`, bundled skills,
`Store`, MCP server and agent logger. It checks scratch/state filesystem isolation
before model execution. `benchmark.py` starts Qodana's `mcp-server` scenario,
waits for its HTTP endpoint and verifies all four generic inspection tools before
starting the host. The client maintains the GET event stream required by IntelliJ
to keep its MCP session alive. Preflight compiles and executes a sentinel fixture,
runs a full-project scan while MCP is still running, and checks the session again
after the server's 15-second pending-session deadline.
A local proxy exposes only these generic tools and an isolated full-project scan;
legacy Edict state-mutating tools are excluded. Server and
container cleanup also runs on failure.
The outer Docker seccomp/AppArmor profiles are unconfined so Codex's inner
Bubblewrap sandbox can create mounts. The Kotlin state-write isolation probe
must still pass before any model execution; a failed probe stops the build.

Project scans run serially against a reused source copy and warm cache. Every IDE
process has a separate configuration directory as well as a separate system cache,
so it cannot collide with the running MCP server's configuration lock. Each scan
replaces the previous candidate scripts and writes separate results. MCP transport
failures and failed Qodana processes stop model execution promptly. Tool timings
and failures are recorded in `log/inspection-mcp.jsonl`; `progress.json` preserves
cluster statuses. The generation step exports SARIF even when the managed run
ends with incomplete tasks, so completed inspections can still be compared.

Each original rule becomes one Pending cluster. Its required labelled source
examples are passed to the normal example-generation workers as transient
feedback evidence. They are not fabricated commit/PR signals. Generation uses
the normal Kotlin skill hierarchy and independent reviews. Optional examples and
gold SARIF are held out of the generation input and used for scoring. Direct
reads of the original benchmark directory and complete fixture manifest are
denied in the agent permission profile; the source Git repository remains
available for exact-revision source reads.

The original `BusyWait` fixture labels the same source range both positive and
negative. This conflict is preserved and can produce a Discontinued outcome.
The runner never relabels fixtures to make a score pass.

Accepted scripts use `EdictBenchmark<OriginalRuleId>` IDs, avoiding collisions
with built-in IDE inspections. A final full-project Qodana scan enables only the
accepted scripts. Scoring retains the reference's region-overlap / two-line
matching and its optional thresholds (precision 0.8, recall 0.7, F1 0.1).
After generation and container shutdown, a separate TeamCity step checks out a
pinned commit of `JetBrains/qodana-cli`, then runs
`edict/kotlin/gradlew :benchmark:compare` with the fixture and generation directories.
The independent [Kotlin subproject](../../edict/kotlin/benchmark/README.md) produces
the reference report format, aggregate metrics, accepted scripts and
specification-versus-gold comparisons. The checkout requires no model credentials.
Scores use the original immutable specifications; unlike the old generator,
generation does not rewrite those specifications. The managed workflow uses its
own repair/review loop rather than the old `genIterationsCount` parameter.

Build parameters:

| Parameter | Default | Purpose |
| --- | --- | --- |
| `benchmark.image` | `…:263.SNAPSHOT.149` | Assembled inspections runtime |
| `env.BENCHMARK_COMPARISON_REVISION` | pinned commit SHA | qodana-cli source checked out after generation |
| `env.BENCHMARK_MODEL` | `gpt-5.6-sol` | Model used by Kotlin CodexRunner |
| `env.BENCHMARK_CODEX_VERSION` | `0.155.1` | Compatible Codex version |
| `env.BENCHMARK_MINUTES` | `240` | Total managed generation timeout |
| `env.BENCHMARK_LIMIT` | `0` | First N alphabetically sorted rules; 0 means all 20 |
| `env.BENCHMARK_RULES` | empty | Optional comma-separated original rule IDs for focused runs |
| `env.BENCHMARK_PREFLIGHT` | `false` | Check compiler, project scan, both servers and sandbox without model execution |

The copied secure `liteLLMToken` and `env.QODANA_TOKEN` supply credentials through
environment variables. Artifacts include input revision and image/JAR provenance,
scores, SARIF, managed state, redacted agent logs, MCP diagnostics and the exact
runner sources. Raw Codex sessions, authentication configuration and npm caches
are not published.

To update the dedicated configuration with these runner files:

```sh
python3 scripts/edict-benchmark/configure_teamcity.py --comparison-revision <published-full-commit-sha>
```

For a new server configuration, add `--create` once; this copies the reference
configuration within TeamCity so its secure parameters need not be retrieved.
The generation scripts are embedded in the build step. The Kotlin comparison
subproject must be available at the supplied published commit, because TeamCity
checks it out only after generation. JDK 21 is selected through the agent
`JDK_21_0` environment variable.

Validation:

```sh
python3 -m unittest discover -s scripts/edict-benchmark -p 'test_*.py' -v
bash -n scripts/edict-benchmark/run.sh
bash -n scripts/edict-benchmark/generate.sh
bash -n scripts/edict-benchmark/compare.sh
(cd edict/kotlin && ./gradlew :benchmark:test)
```
