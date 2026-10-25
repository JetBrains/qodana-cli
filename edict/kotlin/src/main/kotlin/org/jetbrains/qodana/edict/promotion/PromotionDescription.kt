package org.jetbrains.qodana.edict.promotion

import org.jetbrains.qodana.edict.edictnext.EdictNextSignal
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalLabel
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalSource
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalStrength
import org.jetbrains.qodana.edict.edictnext.EdictNextStoredCluster
import java.net.URI

internal const val MIN_PROMOTION_SOURCE_FILES = 5
private const val MAX_DESCRIPTION_SOURCE_FILES = 10

/** One referenced strong representative per source path, regardless of revision or label. */
internal fun EdictNextStoredCluster.promotionEvidence(): List<EdictNextSignal> = signals
  .filter { it.strength == EdictNextSignalStrength.STRONG && it.referenceUrl() != null }
  .sortedWith(compareBy({ it.label != EdictNextSignalLabel.POSITIVE }, { it.id }))
  .distinctBy { it.fileRevision.path }

/** Only historical evidence with a usable external reference can support a promotion PR. */
internal fun promotionDescription(cluster: EdictNextStoredCluster, targetPath: String, digest: String): String {
  val evidence = cluster.promotionEvidence()
  require(evidence.size >= MIN_PROMOTION_SOURCE_FILES) {
    "Promotion requires strong Signals from at least $MIN_PROMOTION_SOURCE_FILES distinct source files with PR or commit diff URLs; found ${evidence.size}"
  }
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
    appendLine("Showing referenced STRONG signals from ${selected.size} of ${evidence.size} distinct source files, one per file. POSITIVE marks problematic code; NEGATIVE marks corrected or acceptable code.")
    appendLine()
    selected.forEachIndexed { index, signal ->
      val url = checkNotNull(signal.referenceUrl())
      val source = when (val origin = signal.source) {
        is EdictNextSignalSource.FromPR -> "PR #${origin.prNumber}: ${origin.title.take(200).markdownText()}"
        is EdictNextSignalSource.FromCommit -> "Commit diff ${origin.commitRevision}"
        else -> error("Unsupported promotion evidence")
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
    else -> null
  }?.takeIf { it.isNotBlank() } ?: return null
  val uri = runCatching { URI(url) }.getOrNull() ?: return null
  return url.takeIf {
    uri.scheme?.lowercase() in setOf("http", "https") && !uri.host.isNullOrBlank() && uri.userInfo == null &&
      url.none { it.isWhitespace() || it in "<>" }
  }
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
