package org.jetbrains.qodana.edict.promotion

import kotlinx.serialization.Serializable
import org.jetbrains.qodana.edict.ci.CreateReviewRequest
import org.jetbrains.qodana.edict.ci.ReviewApi
import org.jetbrains.qodana.edict.ci.ReviewReference
import org.jetbrains.qodana.edict.ci.ReviewState
import org.jetbrains.qodana.edict.ci.SourceFixChanges
import org.jetbrains.qodana.edict.ci.MergedReview
import org.jetbrains.qodana.edict.ci.ReviewFileChange
import org.jetbrains.qodana.edict.common.EdictConfiguration
import org.jetbrains.qodana.edict.core.*
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
  val findings: List<EdictProjectFinding>,
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
  private val repository: EdictRepository,
  private val configuration: EdictConfiguration,
  private val promotionReviews: ReviewApi?,
  /** Review text and review failures reach agents, so they are redacted, before they are bounded and when compared. */
  private val redactor: TokenRedactor,
  /** The branch of the project the run checked out; source fixes are proposed against it. */
  private val executionBranch: () -> String = { error("The project execution branch is unavailable") },
) {
  fun checkReviews(): WeakSignalPromotionCheckReviewsResponse {
    val outcomes = repository.loadActiveWeakSignalPromotions().flatMap { (clusterId, promotions) ->
      promotions.map { promotion -> checkReview(clusterId, promotion) }
    }
    return WeakSignalPromotionCheckReviewsResponse(
      outcomes.mapNotNull {
        when (it) {
          is WeakSignalReviewOutcome.Checked -> it.review
          is WeakSignalReviewOutcome.MergedChangeUnreadable -> it.review
          is WeakSignalReviewOutcome.Failed -> null
        }
      },
      outcomes.mapNotNull {
        when (it) {
          is WeakSignalReviewOutcome.Checked -> null
          is WeakSignalReviewOutcome.MergedChangeUnreadable -> it.failure
          is WeakSignalReviewOutcome.Failed -> it.failure
        }
      },
    )
  }

  private fun checkReview(clusterId: String, promotion: WeakSignalPromotion): WeakSignalReviewOutcome = try {
    val review = reviews().reviewWithEvidence(promotion.pullRequest)
    val state = when (review.state) {
      ReviewState.OPEN -> PromotionPrState.ON_REVIEW
      ReviewState.MERGED -> PromotionPrState.ACCEPTED
      ReviewState.CLOSED_UNMERGED -> PromotionPrState.CLOSED
    }
    val updated = repository.recordWeakSignalReview(clusterId, promotion.pullRequest, state)
    val checked = CheckedWeakSignalReview(
      clusterId, updated, redactor.redact(review.evidence).take(MAX_WEAK_SIGNAL_REVIEW_EVIDENCE_CHARS), inspectionDigest = null,
    )
    if (state != PromotionPrState.ACCEPTED) WeakSignalReviewOutcome.Checked(checked)
    else try {
      WeakSignalReviewOutcome.Checked(withMergedChange(checked))
    }
    catch (e: Exception) {
      WeakSignalReviewOutcome.MergedChangeUnreadable(checked, reviewFailure(clusterId, promotion, e))
    }
  }
  catch (e: Exception) {
    WeakSignalReviewOutcome.Failed(reviewFailure(clusterId, promotion, e))
  }

  /** Adds what turns an accepted review into Signals: the source inspection's digest and the merged change in the cluster's language. */
  private fun withMergedChange(checked: CheckedWeakSignalReview): CheckedWeakSignalReview {
    val cluster = repository.loadCluster(checked.clusterId)
    val inspectionDigest = repository.paths.inspectionPath(cluster.manifest.predecessorId ?: cluster.id)
      .takeIf { it.isRegularFile() }?.let { sha256Hex(it.readBytes()) }
    val merged = mergedReview(checked.promotion.pullRequest)
    val diffs = merged.changes.filter { change ->
      EdictLanguage.fromPathOrNull(change.beforePath) == cluster.manifest.language ||
        EdictLanguage.fromPathOrNull(change.afterPath) == cluster.manifest.language
    }.map { change ->
      reviews().diff(merged.beforeRevision, merged.mergedRevision, change.beforePath, change.afterPath)
    }
    return checked.copy(
      inspectionDigest = inspectionDigest,
      mergedReview = merged,
      diffs = diffs,
    )
  }

  private fun reviewFailure(clusterId: String, promotion: WeakSignalPromotion, e: Exception) =
    PromotionReviewFailure(clusterId, promotion.pullRequest.id, redactor.redact(e.message ?: e.javaClass.simpleName))

  fun prepare(clusterIds: List<String>? = null): WeakSignalPreparationResponse {
    require(clusterIds == null || clusterIds.isNotEmpty() && clusterIds.distinct().size == clusterIds.size) {
      "clusterIds must be nonempty and unique when provided"
    }
    val clusters = repository.loadClusters().associateBy { it.id }
    // The server starts only when every active promotion belongs to the promotion repository.
    val active = repository.loadActiveWeakSignalPromotions()
    val outcomes = (clusterIds ?: clusters.keys.toList()).map { id ->
      try {
        val cluster = clusters[id] ?: error("Cluster does not exist")
        preparedCandidate(cluster, hasActiveReview = !active[id].isNullOrEmpty())
          ?.let(PreparationOutcome::Prepared) ?: PreparationOutcome.Skipped
      }
      catch (e: Exception) {
        PreparationOutcome.Failed(PromotionFailure(id, e.message ?: e.javaClass.simpleName))
      }
    }
    return WeakSignalPreparationResponse(
      outcomes.filterIsInstance<PreparationOutcome.Prepared>().map { it.candidate },
      outcomes.filterIsInstance<PreparationOutcome.Failed>().map { it.failure },
    )
  }

  /** The cluster's source-fix candidate; null when it isn't eligible, already has an active review, or has nothing to fix. */
  private fun preparedCandidate(cluster: EdictStoredCluster, hasActiveReview: Boolean): WeakSignalPromotionCandidate? {
    if (!eligible(cluster)) return null
    // Built even while a review is active, so a broken evaluation is still reported.
    val candidate = candidate(cluster)
    return candidate.takeIf { !hasActiveReview && it.findings.isNotEmpty() }
  }

  fun publish(clusterId: String, inspectionDigest: String, fixes: List<WeakSignalFix>, validation: String): WeakSignalPromotion {
    val reviews = reviews()
    val config = requireNotNull(configuration.promotion) { "Weak-signal promotion requires edict.promotion.reviewer configuration" }
    val cluster = repository.loadCluster(clusterId)
    require(eligible(cluster)) { "Cluster '$clusterId' must be Generated with satisfied, referenced strong positive Signals from fewer than five distinct source files" }
    val candidate = candidate(cluster)
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
    require(fixed.all { it in selected }) { "Fix only selected true-positive locations" }
    val targetBranch = reviews.resolveTargetBranch(executionBranch())
    repository.loadActiveWeakSignalPromotions()[clusterId]?.firstOrNull()?.let { return it }
    val target = reviews.repository
    // Identical fixes reuse their branch; changed fixes can recover from an interrupted publication on a new branch.
    val fixesDigest = sha256Hex(
      fixes.sortedBy { it.path }.joinToString("\u0000") { "${it.path}\u0000${sha256Hex(it.content)}" },
    )
    val id = "w-${sha256Hex("$clusterId\u0000$inspectionDigest\u0000${candidate.projectRevision}\u0000${target.label()}\u0000$targetBranch\u0000$fixesDigest").take(24)}"
    val description = buildString {
      appendLine("## Inspection")
      appendLine()
      appendLine(candidate.inspectionDescription.markdownText())
      appendLine()
      appendLine("Inspection `$clusterId` is generated, but has satisfied, referenced strong positive Signals from fewer than five distinct source files for normal promotion. These source fixes seek feedback on its true-positive findings.")
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
    val review = reviews.findOrCreateReview(CreateReviewRequest(
      operationId = id,
      clusterId = clusterId,
      targetBranch = targetBranch,
      reviewer = config.reviewer,
      targetPath = "",
      inspectionDigest = inspectionDigest,
      inspectionContent = "",
      description = description,
      sourceFix = SourceFixChanges(candidate.projectRevision, fixes.associate { it.path to it.content }),
    ))
    return repository.appendWeakSignalPromotion(clusterId, inspectionDigest, WeakSignalPromotion(review))
  }

  fun recordSignals(
    clusterId: String,
    pullRequest: ReviewReference,
    inspectionDigest: String,
    signals: List<EdictSignal>,
  ): List<EdictSignal> {
    val promotion = requireNotNull(repository.loadActiveWeakSignalPromotions()[clusterId].orEmpty().firstOrNull { it.pullRequest == pullRequest }) {
      "Review does not belong to source cluster '$clusterId'"
    }
    require(promotion.state == PromotionPrState.ACCEPTED) { "Review of source cluster '$clusterId' is not accepted" }
    require(signals.isNotEmpty()) { "Supply actual merged corrections, not proposed fixes" }
    val merged = mergedReview(pullRequest)
    val canonical = mutableMapOf<ReviewFileChange, String>()
    val workItem = "weak-promotion:${pullRequest.provider}:${pullRequest.owner}/${pullRequest.repository}:${pullRequest.id}:${merged.mergedRevision}"
    signals.forEach { draft ->
      SignalValidation.validate("clusters/$clusterId/signals/${draft.id}.json", EdictJson.encodeToString(EdictSignal.serializer(), draft))
      val pr = requireNotNull(draft.source as? EdictSignalSource.FromPR) { "Expected regular FromPR signal" }
      require(draft.strength == EdictSignalStrength.STRONG && pr.prNumber.toString() == pullRequest.id && pr.url == pullRequest.url &&
        pr.title == merged.title && pr.discussionMessages ==
        merged.discussionMessages.ifEmpty { listOf("Source-fix review merged: ${pullRequest.url}") }) { "Signal must preserve actual merged review metadata" }
      val revision = if (draft.label == EdictSignalLabel.POSITIVE) merged.beforeRevision else merged.mergedRevision
      require(draft.fileRevision.revision == revision) { "Signal must use the actual merged before/after revision" }
      val change = merged.changes.singleOrNull {
        (if (draft.label == EdictSignalLabel.POSITIVE) it.beforePath else it.afterPath) == draft.fileRevision.path
      } ?: error("Signal path is absent from the actual merged change")
      UnifiedDiff.validateExcerpt(pr.diffPositiveToNegative, canonical.getOrPut(change) {
        reviews().diff(merged.beforeRevision, merged.mergedRevision, change.beforePath, change.afterPath)
      })
      val content = try { reviews().file(revision, draft.fileRevision.path) } catch (_: NoSuchFileException) {
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

  fun completeReview(clusterId: String, pullRequest: ReviewReference) = repository.completeWeakSignalPromotion(clusterId, pullRequest)

  private fun reviews(): ReviewApi = requireNotNull(promotionReviews) {
    "Weak-signal promotion requires edict.promotion.url or edict.ci.url configuration"
  }

  /** The merged change, with its title and messages redacted as agents see them. */
  private fun mergedReview(reference: ReviewReference): MergedReview {
    val merged = reviews().mergedReview(reference)
    require(merged.title.isNotBlank() && validRevision(merged.beforeRevision) && validRevision(merged.mergedRevision) &&
      merged.beforeRevision != merged.mergedRevision && merged.changes.isNotEmpty() && merged.changes.distinct().size == merged.changes.size &&
      merged.changes.all { validSourcePath(it.beforePath) && validSourcePath(it.afterPath) }) { "Invalid actual merged-change evidence" }
    return merged.copy(title = redactor.redact(merged.title), discussionMessages = merged.discussionMessages.map(redactor::redact))
  }

  private fun eligible(cluster: EdictStoredCluster): Boolean =
    cluster.promotionEligibility(configuration.ci, repository.loadEvaluation(cluster))
      .let { it.generated && it.referencedSourceFiles < configuration.minPromotionSourceFiles }

  private fun candidate(cluster: EdictStoredCluster): WeakSignalPromotionCandidate {
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

private fun EdictProjectFinding.line(): Int = fileRevision.expectedRanges?.firstOrNull()?.start ?: 0

/** What preparing one cluster produced. */
private sealed interface PreparationOutcome {
  data class Prepared(val candidate: WeakSignalPromotionCandidate) : PreparationOutcome
  data class Failed(val failure: PromotionFailure) : PreparationOutcome
  /** Not eligible, already on review, or nothing to fix. */
  data object Skipped : PreparationOutcome
}

/** What checking one weak-signal review produced. */
private sealed interface WeakSignalReviewOutcome {
  data class Checked(val review: CheckedWeakSignalReview) : WeakSignalReviewOutcome
  /** Accepted, and its state is recorded, but its merged change cannot be read yet; the next check reads it again. */
  data class MergedChangeUnreadable(val review: CheckedWeakSignalReview, val failure: PromotionReviewFailure) : WeakSignalReviewOutcome
  data class Failed(val failure: PromotionReviewFailure) : WeakSignalReviewOutcome
}
