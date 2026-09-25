# Inspection generation benchmark

This Gradle subproject runs the embedded Kotlin managed skills and compares their
completed generation artifacts with the Jenkins benchmark fixtures. Generation
uses the assembled Qodana image; the comparison task only reads completed artifacts
and does not start a model or IDE.

`:benchmark:runnerJar` packages the Kotlin controller, embedded Edict runtime and
bundled skills. It starts inspections MCP before generation, keeps its event
stream alive, checks compilation and a concurrent full-project scan, then verifies
the managed sandbox before model execution. Every Qodana process receives an
isolated `QODANA_CONF`. See the [CI runner documentation](../../../scripts/edict-benchmark/README.md)
for configuration, build parameters and diagnostics.

The initial request processes the existing inbox and generates inspection rules.
Required benchmark examples are imported as `SubmittedFeedback`, then the ordinary
`edict-run` workflow prepares, distributes and generates them. Empty Pending
clusters reserve stable rule IDs; optional examples remain held out for comparison.

From `edict/kotlin`, with JDK 21 installed:

```sh
./gradlew :benchmark:compare \
  -PbenchmarkDir=/path/to/jenkins/benchmark \
  -PgenerationDir=/path/to/benchmark-output
```

Optional properties are `benchmarkOutputDir` (defaults to `generationDir`) and
`analysisSarif` (defaults to `generationDir/qodana.sarif.json`). The application
can also be invoked with `:benchmark:run --args='--benchmark-dir ... --generation-dir ...'`.

The generation directory must contain:

- `inputs.json`: the original selected specifications, `clusterToRule` mapping and
  source `revision`, frozen before generation; optional examples stay held out.
- `state/clusters/<cluster>/description.json`: the persisted generation status.
- `state/inspections/<cluster>.inspection.kts`: each accepted Generated inspection.
- `qodana.sarif.json`: full-project analysis of those inspections, registered under
  `EdictBenchmark<OriginalRuleId>` to distinguish them from built-in inspections.

The benchmark directory supplies `gold.sarif.json`. Results go to `report.json`,
`qodana.sarif.json`, `generatedInspections/<rule>.kts` and
`specGoldComparisons/<rule>.json`. The report includes all generation outcomes;
only Generated inspections contribute to inspection metrics and aggregates.
An empty generation writes a report and then exits unsuccessfully.

Calculations and report fields follow
`plugins/qodana/generation-benchmark/src/com/intellij/qodana/generationBenchmark/next/GenerationBenchmarkScript2.kt`
and its `MetricsCalculation.kt`, `ProblemComparison.kt` and `next/utils.kt` helpers:

- TP accepts inclusive character overlap or a start line within two lines.
- FN requires exact character overlap, even when a nearby result counted as TP.
- All required examples must pass; optional precision/recall/F1 defaults are
  0.8/0.7/0.1. Negative ranges ignore unrelated findings in the same file.
- Aggregates include totals, averages, medians and specification satisfaction.
- Specification-versus-gold classification and aggregates match the reference.

Specifications remain immutable; contradictory labels are reported rather than
rewritten. The original in-IDE generator could rewrite specifications during
generation, so this difference should be considered when comparing scores.

```sh
./gradlew :benchmark:test
```
