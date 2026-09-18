---
name: edict-qodana-history-fix-signals
description: Compare a cluster's Qodana inspection at an earlier Git snapshot and at HEAD, investigate absent findings, and emit verified before/after Edict signals for intentional fixes. Use when given a cluster ID and an Edict directory with the standard clusters, inspections, and inbox layout, and a separate Qodana execution skill is available.
---

# Edict Qodana History Fix Signals

Analyze the project in the current working directory. Accept these inputs:

- a required cluster ID;
- a required Edict directory;
- an optional date, defaulting to the current local date;
- an optional period, defaulting to three calendar months.

Do not accept a separate project, cluster, inspection, inbox, Qodana checkout, command, or report
directory. Resolve the cluster and inspection exclusively from this fixed Edict layout:

```text
<edict-directory>/clusters/<cluster-id>/description.json
<edict-directory>/inspections/<cluster-id>.inspection.kts
<edict-directory>/inbox/
```

Stop if either input file is missing, the inspection filename differs from the cluster ID, or their
contents do not describe the same problem. Treat the cluster as the semantic authority and the
inspection as its detector. Resolve the current working directory's Git root for repository state
and Signal paths, but run both whole-project analyses on the current working directory itself.

## Qodana execution dependency

This skill coordinates comparison and investigation; it does not know how Qodana is launched.
Use a separate installed Qodana execution skill selected or supplied by the caller. That runner must
support a project directory and result directory, plus an optional baseline SARIF path and an
include-absent mode, and must return the resulting SARIF path after a successful analysis.

Delegate both Qodana runs to that skill. Do not construct, prescribe, or execute a Qodana CLI,
Docker, Bazel, or other launcher command yourself. Stop before changing the repository if no such
runner skill is available or its contract cannot provide baseline absent results.

## Repository safety

This workflow temporarily checks out an older revision. Before changing the repository:

1. Record the full original `HEAD`, its branch or detached state, and `git status --porcelain`.
2. Stop if there are tracked changes or untracked files outside `<edict-directory>/inbox`. Never
   stash, clean, reset, or overwrite user work.
3. Copy the cluster and inspection bytes to scratch outside the Git worktree so they remain
   available at the historical checkout.
4. Derive a cutoff by subtracting the requested period from the requested date. Select the newest
   first-parent commit reachable from the original `HEAD` whose commit time is on or before that
   cutoff. Stop if none exists or it equals the original `HEAD`.
5. Check out that commit detached, perform the snapshot analysis, remove temporary project files,
   and restore the exact original branch and `HEAD` before continuing. On every failure, restore
   the repository first. Verify the original state again at the end.

Before each delegated analysis, make the captured inspection available to Qodana under the current
project's `inspections` directory. Prefer
`<project>/inspections/<cluster-id>.inspection.kts`. If the same bytes already exist there under a
different filename, reuse that file instead of installing a duplicate inspection ID. Record and
restore any pre-existing destination bytes and remove directories created only for this purpose.
Use the same captured inspection bytes for both revisions.

Do not modify sources, commits, branches, remotes, cluster data, or the Edict inspection. Write only
new Signal JSON files below `<edict-directory>/inbox`. Put Qodana reports and all other scratch data
in a private temporary directory outside the Git worktree, honor `TMPDIR` when it is set, and retain
the reports for verification.

## Comparison

Ask the Qodana runner to analyze the complete current project at the selected snapshot without a
baseline. Require a successful, parseable SARIF report and confirm it contains results for the
inspection ID declared by the captured inspection.

After restoring the original `HEAD`, ask the same runner to analyze the complete project again,
using the snapshot SARIF as its baseline and enabling absent results. Require another successful,
parseable SARIF report. Collect only rows for the exact inspection ID whose `baselineState` is
`absent`; ignore new, unchanged, and unrelated results. Absence makes a finding a candidate, not an
intentional fix.

## Fix investigation

For every absent result, use its snapshot path, range, message, and fingerprints to trace the file
between the snapshot and original `HEAD`, following renames. Inspect complete commit messages and
parent-to-commit diffs. Identify the first commit that removes that concrete finding and verify:

- its first-parent state still contains the reported violation;
- the commit state removes the violation and shows the replacement or intentional deletion;
- the diff fixes the same semantic problem described by the cluster and inspection;
- the commit message, available review metadata, or an unambiguous focused diff demonstrates
  deliberate remediation rather than incidental deletion, movement, formatting, generated churn,
  a revert, or an unrelated refactor.

Reject ambiguous candidates and report why. Do not infer intent merely from disappearance.
Deduplicate absent rows removed by the same before/after construct.

## Signal output

For each accepted fix, create exactly two Edict Next pending Signal files:

- `POSITIVE`: the fixing commit's first-parent revision and the minimal one-based inclusive range
  containing the violation;
- `NEGATIVE`: the fixing commit revision and the minimal one-based inclusive range containing the
  compliant replacement. Use `expectedRanges: null` if the file or construct was intentionally
  removed and no replacement range exists.

Both use `strength: "STRONG"` and `syntheticExampleId: null`. Paths are relative to the Git root,
even when the project is a subdirectory. Edict Next infers language from the path, so omit the
legacy `language` field. Use this source for both sides:

```json
{
  "type": "FromCommit",
  "commitRevision": "full-fixing-commit-hash",
  "message": "complete commit message"
}
```

Make each description state what that side proves. Compute the ID as `s-` plus the first ten
lowercase hexadecimal characters of SHA-256 over this UTF-8 material, with literal newlines:

```text
<cluster-id>
<label>
<repository-relative-path>
<full-revision>
<ranges as start:end comma-separated, or null>
```

Write `<edict-directory>/inbox/<signal-id>.json`. Never overwrite different content; identical
content is an idempotent success. Parse and verify every new file against Git before finishing.
Report the cutoff and snapshot revision, original `HEAD`, both SARIF paths, absent count, accepted
fix commits, rejected candidates with reasons, and created Signal paths.
