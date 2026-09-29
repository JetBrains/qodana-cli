---
name: edict-weak-signal-review
description: Review a managed inspection attempt's sampled findings and delegate negative examples for confident false positives.
---

# Weak Signal Review

Follow [the managed protocol](../edict_manager/references/protocol.md). Registry ID: `edict-weak-signal-review`.
Start the task. Your example-write capability is for delegation; you do not write persisted state yourself.

The task supplies one absolute `Review config` path to a private scratch attempt manifest. Read it first and verify its
candidate hash against the stored candidate. It identifies this exact analysis attempt, the state-relative cluster and
candidate paths, inspected source project, sampled findings, private scratch, and review output. Read complete
cluster description/history, every signal and referenced example through MCP, then the complete candidate and sampled
findings. Infer the intended general rule and positive/negative boundaries from source evidence; description and
observed detector behavior alone cannot prove the rule.

Inspect every sampled finding at its exact recorded revision and relevant range, including enough surrounding code and
PSI/symbol context to assess the rule. Use read-only Git or an available exact-revision reader when the linked example
is not sufficient. For inspection-server calls, pass the inspected IntelliJ project as `projectPath`.

Classify each sampled finding as:

- `TP`: the reported code violates the intended general rule.
- `FP`: the code must not be reported by a rule satisfying the cluster Signals.
- `UNCERTAIN`: required source, semantic, or rule evidence is unavailable or ambiguous.

Record unresolved evidence with a reason and continue; never infer classification from
a filename, API name, wording, cluster description, or the candidate's behavior alone.

For each confident FP, create a transient NEGATIVE signal only in private scratch with unique ID, finding
`fileRevision`, generated source explaining the review, precise description, strength WEAK, and null syntheticExampleId.
Use the same JSON shape as a cluster Signal. Delegate `edict-code-example` with `example.write` scoped to this
cluster's example directory and no signal-write permission. Pass the transient scratch signal. Require its completed
result and verified example ID.

Run example children one at a time and close/dispose each after collecting its result, preserving the other cluster's
reserved agent slots.

After its example is validated, write `<scratch>/weak-signal-review/false-positive-<index>.md` with exact
path/revision/ranges, the exact relevant code snippet, why a rule satisfying the cluster Signals must not report it,
assigned example ID, whether created or reused, and severity (`BLOCKER` or `MAJOR`). Use BLOCKER when the evidence
invalidates the core rule or demonstrates an unsafe recommendation; use MAJOR for a bounded precision gap that leaves the core rule usable.
BLOCKER findings require repairs while another cluster iteration remains. MAJOR findings are publishable limitations
and do not request another iteration. Neither severity independently blocks publication after iteration five when the
exact candidate passes the mandatory compilation, example, and provenance checks; the cluster worker records those
findings as published known problems. Do not create a signal/example for uncertain evidence or a report for TP. Never copy
transient weak signals into the persisted cluster.

After every code-example worker finishes, call `edict_validate_cluster_examples(clusterId)`. Repair reported persisted
metadata, range, or label-link issues through bounded example workers and repeat until it succeeds. This state validator
does not replace each example worker's Ultimate PSI/parser check or semantic review.

Write the manifest's scratch output with candidate hash, sampled findings path, total/reviewed counts, every
absolute false-positive report path, and every unresolved finding/reason. A zero-FP sample does not prove universal precision.
Return its path and finish the task after all example children complete.
