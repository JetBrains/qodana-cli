package org.jetbrains.qodana.edict.history.enrichment

import kotlinx.serialization.Serializable
@Serializable
internal data class EdictHistoryContext(
  val clusterId: String,
  val range: EdictHistoryRange,
)

/** Exact, newest-first commit window. Filtering a query must never broaden this set. */
@Serializable
internal data class EdictHistoryRange(
  val headRevision: String,
  /** The date cutoff, an ISO-8601 UTC instant: older commits are outside the window. */
  val since: String,
  /** How many revisions [revisionsFile] lists; 0 for an empty window. */
  val revisionCount: Int,
  /** Absolute path of the window's revisions, newest first, one full SHA per line. */
  val revisionsFile: String,
)
