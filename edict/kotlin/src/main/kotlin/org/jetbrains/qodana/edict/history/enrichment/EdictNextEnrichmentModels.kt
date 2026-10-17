package org.jetbrains.qodana.edict.history.enrichment

import kotlinx.serialization.Serializable
@Serializable
internal data class EdictNextHistoryContext(
  val clusterId: String,
  val range: EdictNextHistoryRange,
)

/** Exact, newest-first commit window. Filtering a query must never broaden this set. */
@Serializable
internal data class EdictNextHistoryRange(
  val headRevision: String,
  val since: String,
  val commitRevisions: List<String>,
)
