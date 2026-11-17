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
  val since: String,
  val commitRevisions: List<String>,
)
