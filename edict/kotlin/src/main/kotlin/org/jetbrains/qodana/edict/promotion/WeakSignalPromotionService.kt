package org.jetbrains.qodana.edict.promotion

import kotlinx.serialization.Serializable
import org.jetbrains.qodana.edict.ci.CreateReviewRequest
import org.jetbrains.qodana.edict.ci.ReviewManagementApi
import org.jetbrains.qodana.edict.ci.ReviewReference
import org.jetbrains.qodana.edict.ci.ReviewState
import org.jetbrains.qodana.edict.ci.SourceFixChanges
import org.jetbrains.qodana.edict.ci.MergedReview
import org.jetbrains.qodana.edict.ci.ReviewExtractionApi
import org.jetbrains.qodana.edict.ci.ReviewRepository
import org.jetbrains.qodana.edict.ci.ReviewFileChange
import org.jetbrains.qodana.edict.common.EdictConfiguration
import org.jetbrains.qodana.edict.edictnext.*
import org.jetbrains.qodana.edict.signals.SignalValidation
import org.jetbrains.qodana.edict.signals.UnifiedDiff
import org.jetbrains.qodana.edict.signals.validRevision
import org.jetbrains.qodana.edict.signals.validSourcePath
import java.nio.file.NoSuchFileException
import kotlin.io.path.isRegularFile
import kotlin.io.path.readBytes

internal const val MAX_WEAK_SIGNAL_FIXES = 5
private const val MAX_WEAK_SIGNAL_REVIEW_EVIDENCE_CHARS = 32_000

@Serializable
internal data class WeakSignalPromotionCandidate(
  val clusterId: String,
  val inspectionDigest: String,
  val projectRevision: String,
  val inspectionDescription: String,
  val findings: List<EdictNextProjectFinding>,
)

@Serializable
internal data class WeakSignalPreparationResponse(
  val candidates: List<WeakSignalPromotionCandidate>,
  val failures: List<PromotionFailure>,
)

@Serializable
internal data class CheckedWeakSignalReview(
  val clusterId: String,
  val promotion: WeakSignalPromotion,
  val reviewEvidence: String,
  val inspectionDigest: String?,
  val mergedReview: MergedReview? = null,
  val diffs: List<String> = emptyList(),
)

@Serializable
internal data class WeakSignalPromotionCheckReviewsResponse(
  val reviews: List<CheckedWeakSignalReview> = emptyList(),
  val failures: List<PromotionReviewFailure> = emptyList(),
)

/** One complete replacement file can repair several selected locations. Lines refer to the original revision. */
@Serializable
internal data class WeakSignalFix(
  val path: String,
  val lines: List<Int>,
  val content: String,
  val explanation: String,
)

internal class WeakSignalPromotionService(
  private val repository: () -> EdictRepository,
  private val configuration: EdictConfiguration,
  private val reviews: ReviewManagementApi,
) {
  fun checkReviews(redactEvidence: (String) -> String = { it }): WeakSignalPromotionCheckReviewsResponse {
    val repository = repository()
    val checked = mutableListOf<CheckedWeakSignalReview>()
    val failures = mutableListOf<PromotionReviewFailure>()
    repository.loadActiveWeakSignalPromotions().forEach { (clusterId, promotions) ->
      promotions.forEach { promotion ->
        try {
          val review = reviews.reviewWithEvidence(promotion.pullRequest)
          val state = when (review.state) {
            ReviewState.OPEN -> PromotionPrState.ON_REVIEW
            ReviewState.MERGED -> PromotionPrState.ACCEPTED
            ReviewState.CLOSED_UNMERGED -> PromotionPrState.CLOSED
          }
          val updated = repository.recordWeakSignalReview(
            clusterId, promotion.pullRequest, state,
          )
          var inspectionDigest: String? = null
          var merged: MergedReview? = null
          var diffs = emptyList<String>()
          if (state == PromotionPrState.ACCEPTED) {
            try {
              val cluster = repository.loadCluster(clusterId)
              inspectionDigest = repository.paths.inspectionPath(cluster.manifest.predecessorId ?: clusterId)
                .takeIf { it.isRegularFile() }?.let { sha256Hex(it.readBytes()) }
              merged = mergedReview(promotion.pullRequest)
              diffs = merged.changes.filter { change ->
                EdictNextLanguage.fromPathOrNull(change.beforePath) == cluster.manifest.language ||
                  EdictNextLanguage.fromPathOrNull(change.afterPath) == cluster.manifest.language
              }.map { change ->
                source().diff(reviewRepository(promotion.pullRequest), merged.beforeRevision, merged.mergedRevision, change.beforePath, change.afterPath)
              }
              merged = merged.copy(title = redactEvidence(merged.title), discussionMessages = merged.discussionMessages.map(redactEvidence))
            } catch (e: Exception) {
              failures += PromotionReviewFailure(clusterId, promotion.pullRequest.id, e.message ?: e.javaClass.simpleName)
              merged = null
            }
          }
          checked += CheckedWeakSignalReview(clusterId, updated,
            redactEvidence(review.evidence).take(MAX_WEAK_SIGNAL_REVIEW_EVIDENCE_CHARS), inspectionDigest, merged, diffs)
        }
        catch (e: Exception) {
          failures += PromotionReviewFailure(clusterId, promotion.pullRequest.id, e.message ?: e.javaClass.simpleName)
        }
      }
    }
    return WeakSignalPromotionCheckReviewsResponse(checked, failures)
  }

  fun prepare(clusterIds: List<String>? = null): WeakSignalPreparationResponse {
    require(clusterIds == null || clusterIds.isNotEmpty() && clusterIds.distinct().size == clusterIds.size) {
      "clusterIds must be nonempty and unique when provided"
    }
    val repository = repository()
    val candidates = mutableListOf<WeakSignalPromotionCandidate>()
    val failures = mutableListOf<PromotionFailure>()
    val clusters = repository.loadClusters().associateBy { it.id }
    val ci = configuration.ci
    val active = repository.loadActiveWeakSignalPromotions()
    (clusterIds ?: clusters.keys.toList()).forEach { id ->
      try {
        val cluster = clusters[id] ?: error("Cluster does not exist")
        if (!eligible(cluster)) return@forEach
        val candidate = candidate(repository, cluster)
        if (ci != null && active[id].orEmpty().any {
          it.pullRequest.provider == ci.provider && it.pullRequest.owner == ci.owner &&
            it.pullRequest.repository == ci.repository
        }) return@forEach
        if (candidate.findings.isNotEmpty()) candidates += candidate
      }
      catch (e: Exception) {
        failures += PromotionFailure(id, e.message ?: e.javaClass.simpleName)
      }
    }
    return WeakSignalPreparationResponse(candidates, failures)
  }

  fun publish(clusterId: String, inspectionDigest: String, fixes: List<WeakSignalFix>, validation: String): WeakSignalPromotion {
    val ci = requireNotNull(configuration.ci) { "Weak-signal promotion requires complete edict.ci configuration" }
    val config = requireNotNull(configuration.promotion) { "Weak-signal promotion requires edict.promotion.reviewer configuration" }
    val repository = repository()
    val cluster = repository.loadCluster(clusterId)
    require(eligible(cluster)) { "Cluster '$clusterId' must be Generated with referenced strong positive Signals from fewer than five distinct source files" }
    val candidate = candidate(repository, cluster)
    require(candidate.inspectionDigest == inspectionDigest) { "Inspection changed since weak-signal selection" }
    require(fixes.isNotEmpty() && fixes.map { it.path }.distinct().size == fixes.size) { "Supply unique, nonempty source fixes" }
    require(validation.isNotBlank() && validation.length <= 8_000) { "Describe validation results in at most 8000 characters" }
    val selected = candidate.findings.map { it.fileRevision.path to it.line() }.toSet()
    val fixed = fixes.flatMap { fix ->
      require(validSourcePath(fix.path) && fix.content.isNotBlank() && '\u0000' !in fix.content) { "Invalid source fix for '${fix.path}'" }
      require(fix.explanation.isNotBlank() && fix.explanation.length <= 4_000) { "Every fixed file needs a bounded explanation" }
      require(fix.lines.isNotEmpty() && fix.lines.distinct().size == fix.lines.size) { "Supply unique fixed lines for '${fix.path}'" }
      fix.lines.map { fix.path to it }
    }
    require(fixed.size in 1..MAX_WEAK_SIGNAL_FIXES && fixed.all { it in selected }) {
      "Fix only 1 to $MAX_WEAK_SIGNAL_FIXES selected true-positive locations"
    }
    val targetBranch = reviews.resolveTargetBranch(ci.reviewRepository, config.targetBranch)
    repository.loadActiveWeakSignalPromotions()[clusterId].orEmpty().firstOrNull {
      it.pullRequest.provider == ci.provider &&
        it.pullRequest.owner == ci.owner && it.pullRequest.repository == ci.repository
    }?.let { return it }
    // Identical fixes reuse their branch; changed fixes can recover from an interrupted publication on a new branch.
    val fixesDigest = sha256Hex(
      fixes.sortedBy { it.path }.joinToString("\u0000") { "${it.path}\u0000${sha256Hex(it.content)}" },
    )
    val id = "w-${sha256Hex("$clusterId\u0000$inspectionDigest\u0000${candidate.projectRevision}\u0000${ci.provider}\u0000${ci.owner}\u0000${ci.repository}\u0000$targetBranch\u0000$fixesDigest").take(24)}"
    val description = buildString {
      appendLine("## Inspection")
      appendLine()
      appendLine(candidate.inspectionDescription.markdownText())
      appendLine()
      appendLine("Inspection `$clusterId` is generated, but has referenced strong positive Signals from fewer than five distinct source files for normal promotion. These source fixes seek feedback on its true-positive findings.")
      appendLine()
      appendLine("## Proposed improvements")
      appendLine()
      fixes.forEach { fix ->
        appendLine("- ${fix.path.markdownText()}, original lines ${fix.lines.sorted().joinToString()}: ${fix.explanation.markdownText()}")
      }
      appendLine()
      appendLine("## Validation")
      appendLine()
      appendLine(validation.markdownText())
      appendLine()
      appendLine("Please review these improvements and accept/merge this PR if you agree with the changes. If you disagree, please explain which findings or fixes are unsuitable in the review.")
      appendLine()
      appendLine("Analyzed project revision: `${candidate.projectRevision}`. Inspection SHA-256: `$inspectionDigest`.")
    }
    val remote = reviews.ensureReview(CreateReviewRequest(
      operationId = id,
      clusterId = clusterId,
      repository = ci.reviewRepository,
      targetBranch = targetBranch,
      reviewer = config.reviewer,
      targetPath = "",
      inspectionDigest = inspectionDigest,
      inspectionContent = byteArrayOf(),
      description = description,
      sourceFix = SourceFixChanges(candidate.projectRevision, fixes.associate { it.path to it.content }),
    ))
    return repository.appendWeakSignalPromotion(clusterId, inspectionDigest, WeakSignalPromotion(
      ReviewReference(ci.provider, ci.owner, ci.repository, remote.id, remote.url),
    ))
  }

  fun recordSignals(
    clusterId: String,
    pullRequest: ReviewReference,
    inspectionDigest: String,
    signals: List<EdictNextSignal>,
    redactEvidence: (String) -> String = { it },
  ): List<EdictNextSignal> {
    val repository = repository()
    require(repository.loadActiveWeakSignalPromotions()[clusterId].orEmpty().any { it.pullRequest == pullRequest && it.state == PromotionPrState.ACCEPTED }) {
      "Review does not belong to source cluster '$clusterId'"
    }
    require(signals.isNotEmpty()) { "Supply actual merged corrections, not proposed fixes" }
    val merged = mergedReview(pullRequest)
    val repo = reviewRepository(pullRequest)
    val canonical = mutableMapOf<ReviewFileChange, String>()
    val workItem = "weak-promotion:${pullRequest.provider}:${pullRequest.owner}/${pullRequest.repository}:${pullRequest.id}:${merged.mergedRevision}"
    signals.forEach { draft ->
      SignalValidation.validate("clusters/$clusterId/signals/${draft.id}.json", EdictNextJson.encodeToString(EdictNextSignal.serializer(), draft))
      val pr = requireNotNull(draft.source as? EdictNextSignalSource.FromPR) { "Expected regular FromPR signal" }
      require(draft.strength == EdictNextSignalStrength.STRONG && pr.prNumber.toString() == pullRequest.id && pr.url == pullRequest.url &&
        pr.title == redactEvidence(merged.title) && pr.discussionMessages ==
        merged.discussionMessages.ifEmpty { listOf("Source-fix review merged: ${pullRequest.url}") }.map(redactEvidence)) { "Signal must preserve actual merged review metadata" }
      val revision = if (draft.label == EdictNextSignalLabel.POSITIVE) merged.beforeRevision else merged.mergedRevision
      require(draft.fileRevision.revision == revision) { "Signal must use the actual merged before/after revision" }
      val change = merged.changes.singleOrNull {
        (if (draft.label == EdictNextSignalLabel.POSITIVE) it.beforePath else it.afterPath) == draft.fileRevision.path
      } ?: error("Signal path is absent from the actual merged change")
      UnifiedDiff.validateExcerpt(pr.diffPositiveToNegative, canonical.getOrPut(change) {
        source().diff(repo, merged.beforeRevision, merged.mergedRevision, change.beforePath, change.afterPath)
      })
      val content = try { source().file(repo, revision, draft.fileRevision.path) } catch (_: NoSuchFileException) {
        error("Cannot create a signal for absent code")
      }
      require(draft.fileRevision.expectedRanges.orEmpty().all { it.end <= content.lines().size }) { "Signal range exceeds actual source" }
      val ranges = draft.fileRevision.expectedRanges.orEmpty().joinToString(",") { "${it.start}-${it.end}" }
      val key = "$workItem:${draft.label}:${draft.fileRevision.path}:$revision:$ranges"
      require(draft.idempotencyKey == key && draft.provenance.workItemId == workItem) { "Use the stable merged-review signal key and work item" }
    }
    repository.recordWeakSignalPromotionSignals(clusterId, pullRequest, inspectionDigest, signals)
    return signals
  }

  fun completeReview(clusterId: String, pullRequest: ReviewReference) = repository().completeWeakSignalPromotion(clusterId, pullRequest)

  private fun source(): ReviewExtractionApi = requireNotNull(reviews as? ReviewExtractionApi) { "Provider cannot read exact merged source snapshots" }

  private fun reviewRepository(reference: ReviewReference) = ReviewRepository(reference.provider, reference.owner, reference.repository)

  private fun mergedReview(reference: ReviewReference): MergedReview = reviews.mergedReview(reference).also { merged ->
    require(merged.title.isNotBlank() && validRevision(merged.beforeRevision) && validRevision(merged.mergedRevision) &&
      merged.beforeRevision != merged.mergedRevision && merged.changes.isNotEmpty() && merged.changes.distinct().size == merged.changes.size &&
      merged.changes.all { validSourcePath(it.beforePath) && validSourcePath(it.afterPath) }) { "Invalid actual merged-change evidence" }
  }

  private fun eligible(cluster: EdictNextStoredCluster): Boolean =
    cluster.manifest.status == EdictNextClusterStatus.Generated &&
      cluster.promotionEvidence().size < MIN_PROMOTION_SOURCE_FILES

  private fun candidate(repository: EdictRepository, cluster: EdictNextStoredCluster): WeakSignalPromotionCandidate {
    val digest = sha256Hex(repository.paths.inspectionPath(cluster.id).readBytes())
    val evaluation = requireNotNull(repository.loadEvaluation(cluster)) { "No persisted evaluation; regenerate the inspection to review project findings" }
    require(evaluation.inspectionHash == digest) { "Evaluation does not match the generated inspection" }
    val review = requireNotNull(evaluation.projectReview) { "No project review for this inspection; regenerate it before promoting weak signals" }
    require(validRevision(review.projectRevision) && review.inspectionDescription.isNotBlank()) {
      "No persisted inspection description or exact project revision; regenerate the inspection"
    }
    val findings = review.truePositiveFindings.filter {
      validSourcePath(it.fileRevision.path) && it.fileRevision.revision == review.projectRevision && it.line() > 0
    }.distinctBy { it.fileRevision.path to it.line() }
      .sortedWith(compareBy({ it.fileRevision.path }, { it.line() }))
      .take(MAX_WEAK_SIGNAL_FIXES)
    return WeakSignalPromotionCandidate(cluster.id, digest, review.projectRevision, review.inspectionDescription, findings)
  }
}

private fun EdictNextProjectFinding.line(): Int = fileRevision.expectedRanges?.firstOrNull()?.start ?: 0
