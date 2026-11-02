package org.jetbrains.qodana.edict.promotion

import org.jetbrains.qodana.edict.edictnext.EdictNextClusterStatus
import org.jetbrains.qodana.edict.edictnext.EdictNextSignal
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalLabel
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalSource
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalStrength
import org.jetbrains.qodana.edict.edictnext.EdictNextStoredCluster
import java.net.URI

internal const val MIN_PROMOTION_SOURCE_FILES = 5
private const val MAX_DESCRIPTION_SOURCE_FILES = 10

/**
 * What decides how a cluster's inspection is promoted. An inspection PR needs [MIN_PROMOTION_SOURCE_FILES] source files
 * of both kinds; a weak-signal source-fix review is for an inspection with fewer [referencedSourceFiles].
 */
internal data class PromotionEligibility(
  /** The inspection is generated and awaits review; other statuses are pending, decided or discontinued. */
  val generated: Boolean,
  /** Distinct source files with a strong positive Signal: how widely the problem was observed. */
  val strongPositiveSourceFiles: Int,
  /** Distinct source files with a strong positive Signal that links to its PR or commit diff: what a PR can cite. */
  val referencedSourceFiles: Int,
)

internal fun EdictNextStoredCluster.promotionEligibility(): PromotionEligibility = PromotionEligibility(
  generated = manifest.status == EdictNextClusterStatus.Generated,
  strongPositiveSourceFiles = signals.filter(EdictNextSignal::isStrongPositive).distinctBy { it.fileRevision.path }.size,
  referencedSourceFiles = promotionEvidence().size,
)

/** One referenced strong positive representative per source path, regardless of revision. */
internal fun EdictNextStoredCluster.promotionEvidence(): List<EdictNextSignal> = signals
  .filter { it.isStrongPositive() && it.referenceUrl() != null }
  .sortedBy { it.id }
  .distinctBy { it.fileRevision.path }

private fun EdictNextSignal.isStrongPositive(): Boolean =
  strength == EdictNextSignalStrength.STRONG && label == EdictNextSignalLabel.POSITIVE

/** Describes a cluster with [MIN_PROMOTION_SOURCE_FILES] referenced source files; only those can support a promotion PR. */
internal fun promotionDescription(cluster: EdictNextStoredCluster, targetPath: String, digest: String): String {
  val evidence = cluster.promotionEvidence()
  val selected = evidence.take(MAX_DESCRIPTION_SOURCE_FILES)
  return buildString {
    appendLine("## Description")
    appendLine()
    appendLine("Adds the generated ${cluster.manifest.language} inspection `${cluster.id}` at `$targetPath`.")
    appendLine()
    appendLine("Representative evidence: ${selected.first().description.markdownText()}")
    appendLine()
    appendLine("## Strong signals")
    appendLine()
    appendLine("Showing referenced STRONG POSITIVE signals from ${selected.size} of ${evidence.size} distinct source files, one per file. POSITIVE marks problematic code.")
    appendLine()
    selected.forEachIndexed { index, signal ->
      val url = checkNotNull(signal.referenceUrl())
      val source = when (val origin = signal.source) {
        is EdictNextSignalSource.FromPR -> "PR #${origin.prNumber}: ${origin.title.take(200).markdownText()}"
        is EdictNextSignalSource.FromCommit -> "Commit diff ${origin.commitRevision}"
        is EdictNextSignalSource.SubmittedFeedback, is EdictNextSignalSource.Generated -> error("Unsupported promotion evidence")
      }
      val file = signal.fileRevision
      val ranges = file.expectedRanges.orEmpty().joinToString(", ") {
        if (it.start == it.end) "${it.start}" else "${it.start}-${it.end}"
      }
      appendLine("${index + 1}. **${signal.label}** — ${signal.description.markdownText()}  ")
      appendLine("   Source: [$source](<$url>)  ")
      appendLine("   Evidence: ${file.path.markdownText()} at `${file.revision}`${if (ranges.isEmpty()) "" else ", lines $ranges"}; signal `${signal.id}`.")
      appendLine()
    }
    appendLine("Inspection SHA-256: `$digest`.")
  }
}

internal fun EdictNextSignal.referenceUrl(): String? {
  val url = when (val origin = source) {
    is EdictNextSignalSource.FromPR -> origin.url
    is EdictNextSignalSource.FromCommit -> origin.url
    is EdictNextSignalSource.SubmittedFeedback, is EdictNextSignalSource.Generated -> null
  } ?: return null
  val uri = runCatching { URI(url) }.getOrNull() ?: return null
  return url.takeIf { uri.scheme?.lowercase() in setOf("http", "https") && !uri.host.isNullOrBlank() && uri.userInfo == null }
}

internal fun String.markdownText(): String = buildString {
  for (character in this@markdownText) {
    when (character) {
      '\n', '\r' -> append(' ')
      '&' -> append("&amp;")
      '<' -> append("&lt;")
      '>' -> append("&gt;")
      '\\', '`', '*', '_', '[', ']', '#' -> append('\\').append(character)
      else -> append(character)
    }
  }
}
