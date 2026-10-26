# Native Edict benchmark reporting

This Gradle subproject runs **after** Codex generation. `:benchmark:report` executes
accepted inspections with the native `QODANA_DIST`, writes SARIF, and compares it
with `.edict/gold.sarif.json`. Both tasks
read the project's existing `.edict` state via `-PedictStateDir`; report artifacts
go to `-PbenchmarkOutputDir`. `:benchmark:compare` can
re-evaluate existing SARIF. The subproject has no generation controller or MCP proxy.

Pipeline setup, parameters, artifact provenance, and commands are documented in
[`scripts/edict-benchmark/README.md`](../../../scripts/edict-benchmark/README.md).

Comparison uses exact character intersection or a two-line tolerance for both
generated-finding matches and gold-finding coverage.
Each gold result is `POSITIVE`, `NEGATIVE`, or unlabeled (`edictLabel`); gold without any
label lists only expected findings, so all its results are `POSITIVE`. A finding matching a
`POSITIVE` result is a true positive and any other a false positive. False positives also
report `matchedNegative` (matching a `NEGATIVE` result) and `matchedUnlabeled` (matching only
an unlabeled one, possibly a positive nobody labeled yet).
Only Generated inspections contribute quality metrics; every selected rule remains
in the generation-outcome report. A gold rule's `expectedState` property (`generated` or
`discontinued`; `generated` when absent) is the outcome the rule must reach: its TeamCity test
fails otherwise, and `edict.expectedStateMatched` counts the rules that reach it. A Generated
rule is scored even when `discontinued` was expected.
