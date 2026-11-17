package org.jetbrains.qodana.edict.promotion

import org.jetbrains.qodana.edict.core.EdictClusterStatus
import org.jetbrains.qodana.edict.core.EdictSignal
import org.jetbrains.qodana.edict.core.EdictEvaluation
import org.jetbrains.qodana.edict.core.isCurrentEvaluation
import org.jetbrains.qodana.edict.ci.CiProviderId
import org.jetbrains.qodana.edict.ci.ReviewRepository
import org.jetbrains.qodana.edict.common.reviewRepository
import org.jetbrains.qodana.edict.core.EdictSignalLabel
import org.jetbrains.qodana.edict.core.EdictSignalSource
import org.jetbrains.qodana.edict.core.EdictSignalStrength
import org.jetbrains.qodana.edict.core.EdictStoredCluster
import org.jetbrains.qodana.edict.signals.validSourcePath
import org.jetbrains.qodana.edict.signals.validRevision
import java.net.URI
import java.net.URLEncoder
import java.util.Locale

private const val MAX_DESCRIPTION_SOURCE_FILES = 5

/**
 * What decides how a cluster's inspection is promoted. An inspection PR needs `edict.promotion.minSourceFiles` source files
 * of both kinds; a weak-signal source-fix review is for an inspection with fewer [referencedSourceFiles].
 */
internal data class PromotionEligibility(
  /** The inspection is generated and awaits review; other statuses are pending, decided or discontinued. */
  val generated: Boolean,
  /** Distinct source files with a strong positive Signal: how widely the problem was observed. */
  val strongPositiveSourceFiles: Int,
  /** Distinct source files with a strong positive Signal that the evaluation satisfies and that links to its lines: what a PR can cite. */
  val referencedSourceFiles: Int,
)

/** Eligibility under the [evaluation] of the cluster's inspection, with commit Signals without a URL linked in the [ci] repository. */
internal fun EdictStoredCluster.promotionEligibility(ci: ReviewRepository?, evaluation: EdictEvaluation?): PromotionEligibility =
  PromotionEligibility(
    generated = manifest.status == EdictClusterStatus.Generated,
    strongPositiveSourceFiles = signals.filter(EdictSignal::isStrongPositive).distinctBy { it.fileRevision.path }.size,
    referencedSourceFiles = promotionEvidence(ci, evaluation).size,
  )

/** One satisfied, referenced strong positive representative per source path, regardless of revision. */
internal fun EdictStoredCluster.promotionEvidence(ci: ReviewRepository?, evaluation: EdictEvaluation?): List<EdictSignal> = signals
  .filter {
    it.isStrongPositive() && it.referenceUrl(ci) != null &&
      evaluation?.examples?.get(it.syntheticExampleId)?.satisfied == true
  }
  .sortedBy { it.id }
  .distinctBy { it.fileRevision.path }

private fun EdictSignal.isStrongPositive(): Boolean =
  strength == EdictSignalStrength.STRONG && label == EdictSignalLabel.POSITIVE

/** Describes a cluster with [minSourceFiles] referenced source files; only those can support a promotion PR. */
internal fun promotionDescription(
  cluster: EdictStoredCluster,
  digest: String,
  ci: ReviewRepository?,
  evaluation: EdictEvaluation?,
  minSourceFiles: Int,
): String {
  require(evaluation != null && cluster.isCurrentEvaluation(evaluation, digest)) {
    "Promotion requires a current evaluation matching the inspection and examples"
  }
  require(evaluation.inspectionName.isNotBlank() && evaluation.inspectionDescription.isNotBlank()) {
    "Promotion requires inspection name and description; re-evaluate the inspection"
  }
  val evidence = cluster.promotionEvidence(ci, evaluation)
  require(evidence.size >= minSourceFiles) {
    "Promotion requires strong positive Signals from at least $minSourceFiles distinct source files with satisfied examples and file/line URLs; found ${evidence.size}"
  }
  val selected = evidence.take(MAX_DESCRIPTION_SOURCE_FILES)
  fun percent(value: Double): String = String.format(Locale.ROOT, "%.1f%%", value * 100)
  return buildString {
    appendLine("## Description")
    appendLine()
    appendLine("**${evaluation.inspectionName.markdownText()}**")
    appendLine()
    appendLine("Rule ID: `${cluster.id}`")
    appendLine()
    appendLine(evaluation.inspectionDescription.markdownText())
    appendLine()
    appendLine("Estimated precision: **${percent(evaluation.precision)}** · recall: **${percent(evaluation.recall)}** · F1: **${percent(evaluation.f1)}**")
    appendLine()
    evaluation.projectReview?.let { review ->
      val count = "${review.totalFindings} findings"
      val findings = review.reportUrl?.takeIf { org.jetbrains.qodana.edict.core.validReportUrl(it) }
        ?.let { "[$count]($it)" } ?: count
      appendLine("Last project analysis reported $findings; ${review.reviewedFindings} were reviewed.")
      appendLine()
    }
    appendLine("## Evidence")
    appendLine()
    selected.forEachIndexed { index, signal ->
      val citation = when (val source = signal.source) {
        is EdictSignalSource.FromPR -> {
          val sourceUri = URI(source.url)
          val url = if (ci?.provider == CiProviderId.SPACE && sourceUri.host?.lowercase() == ci.host) {
            val project = Regex("/p/([^/]+)/").find(sourceUri.path)?.groupValues?.get(1) ?: ci.owner
            "${sourceUri.scheme}://${sourceUri.rawAuthority}/p/$project/reviews/${source.prNumber}"
          }
          else source.url
          Citation("Fixed", "PR #${source.prNumber}", url)
        }
        is EdictSignalSource.FromCommit -> {
          val commitUrl = source.url?.takeIf { it.isNotBlank() } ?: run {
            val repository = checkNotNull(ci).webUrl()
            when (ci.provider) {
              CiProviderId.GITHUB -> "$repository/commit/${source.commitRevision}"
              CiProviderId.SPACE -> "$repository/revision/${source.commitRevision}"
            }
          }
          val uri = URI(commitUrl)
          val url = if (Regex("/p/[^/]+/repositories/[^/]+/revision/").containsMatchIn(uri.path)) {
            val file = URLEncoder.encode(signal.fileRevision.path, Charsets.UTF_8)
            val range = checkNotNull(signal.fileRevision.expectedRanges).first()
            // Historical evidence describes the problematic code on the old side of the fix.
            "$commitUrl?file=$file&from-line=OLD:${range.start}" +
              if (range.end == range.start) "" else "&to-line=OLD:${range.end}"
          }
          else commitUrl
          Citation("Fixed", source.commitRevision.take(7), url)
        }
        is EdictSignalSource.GitHubEvaluatedFinding -> Citation("Confirmed", "a review comment", source.url)
        is EdictSignalSource.SubmittedFeedback, is EdictSignalSource.Generated -> error("Unsupported promotion evidence")
      }
      appendLine("${index + 1}. ${citation.verb}: \"${signal.description.markdownText()}\" in [${citation.linkText}](${citation.url}).")
      appendLine()
    }
  }
}

/** How a promotion PR cites one Signal: what was done with the code ([verb]), and the [url] it links as [linkText]. */
private data class Citation(val verb: String, val linkText: String, val url: String)

internal fun EdictSignal.referenceUrl(ci: ReviewRepository?): String? {
  val originUrl = when (val origin = source) {
    is EdictSignalSource.FromPR -> origin.url
    is EdictSignalSource.FromCommit -> origin.url?.takeIf { it.isNotBlank() } ?: ci?.webUrl()
    is EdictSignalSource.GitHubEvaluatedFinding -> origin.url
    is EdictSignalSource.SubmittedFeedback, is EdictSignalSource.Generated -> null
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
  val repository = sourceRepository?.let { runCatching { reviewRepository(it, "Evidence URL") }.getOrNull() ?: return null } ?: ci ?: return null
  val path = URI(null, null, "/$sourcePath", null).rawPath.removePrefix("/")
  val root = repository.webUrl()
  val url = when (repository.provider) {
    CiProviderId.GITHUB -> "$root/blob/${fileRevision.revision}/$path#L${range.start}" +
      if (range.end == range.start) "" else "-L${range.end}"
    CiProviderId.SPACE -> "$root/files/${fileRevision.revision}/$path?line=${range.start}"
  }
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
