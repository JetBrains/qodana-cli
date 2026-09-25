# Native Edict benchmark reporting

This Gradle subproject runs **after** Codex generation. `:benchmark:report` executes
accepted inspections with the native `QODANA_DIST`, writes SARIF, and compares it
with the original benchmark specifications and gold. `:benchmark:compare` can
re-evaluate existing SARIF. The subproject has no generation controller or MCP proxy.

Pipeline setup, parameters, artifact provenance, and commands are documented in
[`scripts/edict-benchmark/README.md`](../../../scripts/edict-benchmark/README.md).

Comparison follows `GenerationBenchmarkScript2.kt`: TP uses exact character
intersection or a two-line tolerance, FN uses exact character intersection,
required examples must all pass, and optional defaults are precision 0.8,
recall 0.7, F1 0.1. Only Generated inspections contribute quality metrics; every
selected specification remains in the generation-outcome report.
