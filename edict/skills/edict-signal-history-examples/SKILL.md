---
name: edict-signal-history-examples
description: Enrich incoming Edict inspection-signal JSON files with verified positive and negative examples from corrective commits in the analyzed repository's Git history. Use when asked to find historical examples for signals or enrich an Edict income directory; do not use to discover unrelated rules.
---

# Edict Signal History Examples

Enrich each Edict `InspectionSpecification` JSON file in the supplied income directory. If no
directory is supplied, use `income/` below the current repository root. Search only the current
repository's history and edit only signal files in that income directory.

The result mirrors Edict's `HistoricalEnricher`: a historical correction contributes its parent
state to `optionalPositiveExamples` and its corrected state to `optionalNegativeExamples`. Do not
change the rule, its primary examples, or its source provenance.

## Search contract

For each signal:

1. Read `ruleId`, `ruleName`, `description`, `language`, `source`, and all existing examples. Reject
   malformed JSON rather than guessing missing fields.
2. Derive a small set of literal and regex search terms from the precise code pattern: relevant API
   or type names, the problematic construct, the corrected construct, and useful words from the
   source context. Do not rely on the rule ID or commit-message wording alone.
3. Search at most the latest 50,000 commits reachable from `HEAD`. Use `git log -S`, `git log -G`,
   and focused `git log --grep` searches to identify candidate corrections. Use `git grep` against
   candidate and parent revisions to locate and compare actual occurrences, for example:

   ```sh
   git grep -n -F -e 'Thread.sleep' <revision> -- '*.java' '*.kt'
   ```

   Quote patterns and place revision/pathspec separators correctly. A search hit is only a
   candidate; it is not evidence by itself.
4. Skip the originating correction and every revision already present in the signal. For a
   `FromCommit` source, the originating correction is `source.commitHash`. Also exclude roots,
   merges, reverts, automated changes, and commits whose relevant source cannot be read.
5. Inspect each candidate's complete message and relevant parent-to-commit diff. Accept it only when
   the commit fixes the same semantic and statically detectable problem described by the signal.
   Shared tokens, the same file, or a superficially similar edit are insufficient.
6. Resolve both sides with full immutable commit hashes. Follow renames when necessary. The positive
   side is the first-parent file state containing the violation; the negative side is the correcting
   commit's file state. Determine minimal, one-based inclusive positive ranges from the actual parent
   file. Use `expectedProblemRanges: null` on the negative side when the violating expression was
   removed and no meaningful corrected range remains.
7. Keep at most five new pairs per signal. Deduplicate by path, revision, and range, including
   examples already present. Append accepted pairs in newest-first order, keeping corresponding
   positive and negative entries in the same relative order.

## Write contract

Preserve every existing JSON field. Create `optionalPositiveExamples` and
`optionalNegativeExamples` when absent, otherwise append to them. Each added entry has the exact
`FileRevision` shape:

```json
{
  "path": "repository-relative/source/File.java",
  "revision": "full-commit-hash",
  "expectedProblemRanges": [{"start": 1, "end": 1}]
}
```

Before finishing, parse every edited file again and verify that every added revision and path exists
in Git, every positive range is within that revision's file, no originating revision was added, and
the two optional arrays received the same number of new entries. If no qualifying correction exists,
leave the signal byte-for-byte unchanged. Report edited signal paths and accepted correcting commits.

