# Inspection generation benchmark comparison

This standalone Gradle subproject compares completed Kotlin managed-skill generation
artifacts with the Jenkins benchmark fixtures. It has no dependency on the Edict
runtime or IntelliJ platform and does not run models or inspections.

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
