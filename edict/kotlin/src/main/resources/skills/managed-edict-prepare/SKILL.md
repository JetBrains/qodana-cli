---
name: managed-edict-prepare
description: Validate inputs and capture a bounded read-only inbox snapshot for a managed Edict run.
---

# Managed Edict Preparation

Follow [the managed protocol](../edict_manager/references/protocol.md). Registry ID: `edict-prepare`. This leaf has
no state-write operations.

Start the task. Verify the source project and private scratch location, and read `edict_registry` to confirm the
required managed call graph exists. Through `edict_list` and `edict_read`, select at most 100 inbox signal JSON files
alphabetically, unless the parent supplies a narrower set. Return the exact selected paths and hashes. Read every
selected record and reject missing IDs, invalid labels, absent source revisions, and malformed one-based ranges. Do not
repair malformed records here.

List existing cluster descriptions, all cluster member paths, and Pending cluster IDs for downstream navigation. The
distribution worker must read full candidate cluster membership before assignment. Embeddings and a worktree are not
prerequisites for this simpler pipeline. Require inspection capabilities only when generation has actual targets; an
empty run can complete without an inspection server.

Keep summaries in the task result or private scratch. Finish with the prepared snapshot and any prerequisite failure.
