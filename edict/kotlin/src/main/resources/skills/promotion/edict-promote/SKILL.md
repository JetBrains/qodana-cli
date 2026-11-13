---
name: edict-promote
description: Managed worker that promotes eligible generated inspections through configured pull requests.
---

# Inspection promotion

Run only as a delegated managed subagent.
Follow [the managed task protocol](../edict_manager/references/task-protocol.md).
Registry ID: `edict-promote`.

1. Call `edict_select_promotion_clusters` with your token to discover candidates across all Generated clusters,
   including earlier runs. Omit `count` to select all eligible clusters, or supply a positive `count` to limit the
   number returned. Apply any task restriction on cluster IDs when choosing which clusters to publish.
   The configured precision, recall and F1 thresholds are available in this call's `thresholds` response field.
   Selection applies these targets, requires a current
   evaluation and referenced STRONG POSITIVE Signals from at least the configured minimum of distinct source files, and excludes digests
   already published to the configured target. It returns inspection paths, digests, evaluations and F1 without creating PRs.
2. Review the returned candidates and their evidence. Decide which inspections to publish based on their metrics,
   examples, inspection code and the task's instructions. You may choose a subset. You may also deliberately choose
   another Generated cluster outside the suggested candidates when its evidence supports publication; explain why.
   Selection thresholds are recommendations for this decision, not enforced again by publication.
3. Call `edict_promote_clusters` with your token and an explicit nonempty `clusterIds` list of your chosen clusters.
   Omitted IDs are never published. If you choose none, do not call publication. Unchosen clusters remain Generated.
   Publication requires an inspection file and a current evaluation matching the inspection and examples, and strong positive Signals with satisfied strong positive examples
   from at least that minimum of distinct source files. It does not reapply metric thresholds.
   The server builds fix links from the source evidence, using the source repository or configured repository.
   Negative Signals, generated evidence, `SubmittedFeedback`, and weak Signals do not count toward the reference
   requirement; a `GitHubEvaluatedFinding` 👍 counts, linked to its review comment. Never invent URLs to bypass this check. A repository-relative path counts once across
   revisions and reviews. The server builds each PR description from up to five source files, with one representative
   positive Signal per file, inspection name and description, estimated precision/recall/F1, and fix links labeled with
   PR numbers or short commit hashes, and confirmation links to review comments. It includes the last project analysis counts when available, linking the findings
   count to that evaluation's uploaded Qodana Cloud report when one exists. Never invent a report URL or a current count.

Repository identity, target branch, reviewer, and target inspections directory come from the server's `qodana.yaml`
configuration and must never be overridden in a tool call. The operation does not promote the same inspection digest
to the same target twice.

Review every per-cluster failure and report chosen, deferred, created and failed clusters, with reasons for your decisions. Provider credentials remain in the MCP server environment; never request, print, or persist them.
