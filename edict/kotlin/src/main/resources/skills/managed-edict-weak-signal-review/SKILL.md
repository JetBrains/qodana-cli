---
name: managed-edict-weak-signal-review
description: Review a managed inspection attempt's sampled findings and delegate negative examples for confident false positives.
---

# Managed Weak Signal Review

Follow [the managed protocol](../edict_manager/references/protocol.md). Registry ID: `edict-weak-signal-review`.
Start the task. Your example-write capability is for delegation; you do not write persisted state yourself.

Read the supplied scratch attempt manifest. Verify its candidate hash against the stored candidate. Read complete
cluster description/history, every signal and referenced example through MCP, then the complete candidate and sampled
findings. Infer the intended general rule and positive/negative boundaries from source evidence; description and
observed detector behavior alone cannot prove the rule.

Inspect every sampled finding at its exact recorded revision and relevant range, including enough surrounding code and
PSI/symbol context to assess the rule. Classify as TP (violates the intended rule), FP (must not be reported), or
UNCERTAIN (necessary evidence missing or ambiguous). Record uncertainty and continue; never infer classification from a
filename, API name, message, or detector output alone.

For each confident FP, create a transient NEGATIVE signal only in private scratch with unique ID, finding
`fileRevision`, generated source explaining the review, precise description, strength WEAK, and null syntheticExampleId.
Delegate `edict-code-example` with `example.write` scoped to this cluster's example directory and no signal-write
permission. Pass the transient scratch signal. Require its completed result and verified example ID.

Write one scratch false-positive report with exact path/revision/ranges, relevant snippet, semantic reason, example ID,
and whether created or reused. Do not create a signal/example for uncertain evidence or a report for TP. Never copy
transient weak signals into the persisted cluster.

Write the manifest's scratch output with candidate hash, sampled findings path, total/reviewed counts, every
false-positive report path, and every unresolved finding/reason. A zero-FP sample does not prove universal precision.
Return its path and finish the task after all example children complete.
