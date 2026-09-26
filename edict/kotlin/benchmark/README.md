# Native Edict benchmark reporting

This Gradle subproject runs **after** Codex generation. `:benchmark:report` executes
accepted inspections with the native `QODANA_DIST`, writes SARIF, and compares it
with `.edict/gold.sarif.json`. Both tasks
read the project's existing `.edict` state via `-PedictStateDir`; report artifacts
go to `-PbenchmarkOutputDir`. `:benchmark:compare` can
re-evaluate existing SARIF. The subproject has no generation controller or MCP proxy.

Pipeline setup, parameters, artifact provenance, and commands are documented in
[`scripts/edict-benchmark/README.md`](../../../scripts/edict-benchmark/README.md).

Comparison follows `GenerationBenchmarkScript2.kt`: TP uses exact character
intersection or a two-line tolerance, and FN uses exact character intersection.
Only Generated inspections contribute quality metrics; every selected rule remains
in the generation-outcome report.
