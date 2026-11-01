package org.jetbrains.qodana.edict.promotion

import org.jetbrains.qodana.edict.edictnext.EdictNextSignal
import org.jetbrains.qodana.edict.edictnext.EdictNextEvaluation
import org.jetbrains.qodana.edict.edictnext.exampleSetDigest
import org.jetbrains.qodana.edict.ci.CiProviderId
import org.jetbrains.qodana.edict.common.EdictCIConfiguration
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalLabel
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalSource
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalStrength
import org.jetbrains.qodana.edict.edictnext.EdictNextStoredCluster
import org.jetbrains.qodana.edict.signals.validSourcePath
import org.jetbrains.qodana.edict.signals.validRevision
import java.net.URI

internal const val MIN_PROMOTION_SOURCE_FILES = 5
private const val MAX_DESCRIPTION_SOURCE_FILES = 5

/** One satisfied, referenced strong positive representative per source path, regardless of revision. */
internal fun EdictNextStoredCluster.promotionEvidence(
  ci: EdictCIConfiguration? = null,
  evaluation: EdictNextEvaluation? = null,
): List<EdictNextSignal> = signals
  .filter {
    it.strength == EdictNextSignalStrength.STRONG && it.label == EdictNextSignalLabel.POSITIVE && it.referenceUrl(ci) != null &&
      evaluation?.examples?.get(it.syntheticExampleId)?.let { example ->
        example.strong && example.label == EdictNextSignalLabel.POSITIVE && example.satisfied
      } == true
  }
  .sortedBy { it.id }
  .distinctBy { it.fileRevision.path }

/** Only historical evidence with a usable external reference can support a promotion PR. */
internal fun promotionDescription(
  cluster: EdictNextStoredCluster,
  targetPath: String,
  digest: String,
  ci: EdictCIConfiguration? = null,
  evaluation: EdictNextEvaluation? = null,
): String {
  require(evaluation != null && evaluation.inspectionHash == digest && evaluation.exampleSetDigest == exampleSetDigest(cluster)) {
    "Promotion requires a current evaluation matching the inspection and examples"
  }
  val evidence = cluster.promotionEvidence(ci, evaluation)
  require(evidence.size >= MIN_PROMOTION_SOURCE_FILES) {
    "Promotion requires strong positive Signals from at least $MIN_PROMOTION_SOURCE_FILES distinct source files with satisfied examples and file/line URLs; found ${evidence.size}"
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
    appendLine("Showing referenced STRONG POSITIVE signals from ${selected.size} of ${evidence.size} distinct source files, one per file. POSITIVE marks problematic code.")
    appendLine()
    selected.forEachIndexed { index, signal ->
      val url = checkNotNull(signal.referenceUrl(ci))
      val source = when (val origin = signal.source) {
        is EdictNextSignalSource.FromPR -> "PR #${origin.prNumber}: ${origin.title.take(200).markdownText()}"
        is EdictNextSignalSource.FromCommit -> "Commit ${origin.commitRevision}"
        else -> error("Unsupported promotion evidence")
      }
      val file = signal.fileRevision
      val ranges = file.expectedRanges.orEmpty().joinToString(", ") {
        if (it.start == it.end) "${it.start}" else "${it.start}-${it.end}"
      }
      appendLine("${index + 1}. **${signal.label}** — ${signal.description.markdownText()}  ")
      appendLine("   Source: [$source](<$url>)  ")
      appendLine("   Evidence: [${file.path.markdownText()}](<$url>) at `${file.revision}`${if (ranges.isEmpty()) "" else ", lines $ranges"}; signal `${signal.id}`.")
      appendLine()
    }
    appendLine("Inspection SHA-256: `$digest`.")
  }
}

internal fun EdictNextSignal.referenceUrl(ci: EdictCIConfiguration? = null): String? {
  val originUrl = when (val origin = source) {
    is EdictNextSignalSource.FromPR -> origin.url
    is EdictNextSignalSource.FromCommit -> origin.url?.takeIf { it.isNotBlank() } ?: ci?.url
    else -> null
  } ?: return null
  val originUri = runCatching { URI(originUrl) }.getOrNull() ?: return null
  if (originUri.scheme?.lowercase() !in setOf("http", "https") || originUri.host.isNullOrBlank() ||
    originUri.userInfo != null || originUrl.any { it.isWhitespace() || it in "<>" }) return null
  if (!validRevision(fileRevision.revision)) return null
  val ranges = fileRevision.expectedRanges?.takeIf { it.isNotEmpty() } ?: return null
  val range = ranges.first()
  if (range.start <= 0 || range.end < range.start) return null
  val sourcePath = fileRevision.path
  if (!validSourcePath(sourcePath)) return null
  // Prefer the evidence repository over the publication repository when the source URL identifies it.
  val sourceRepository = Regex("^(https?://[^/]+/p/[^/]+/repositories/[^/]+)(?:/|$)").find(originUrl)?.groupValues?.get(1)
    ?: Regex("^(https?://[^/]+/[^/]+/[^/]+)/(?:pull|commit|blob)/").find(originUrl)?.groupValues?.get(1)
  val repository = sourceRepository ?: ci?.url ?: return null
  val config = runCatching { EdictCIConfiguration(repository) }.getOrNull() ?: return null
  val path = URI(null, null, "/$sourcePath", null).rawPath.removePrefix("/")
  val root = repository.trimEnd('/').removeSuffix(".git")
  val url = when (config.provider) {
    CiProviderId.GITHUB -> "$root/blob/${fileRevision.revision}/$path#L${range.start}" +
      if (range.end == range.start) "" else "-L${range.end}"
    CiProviderId.SPACE -> "$root/files/${fileRevision.revision}/$path?line=${range.start}"
  }
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
