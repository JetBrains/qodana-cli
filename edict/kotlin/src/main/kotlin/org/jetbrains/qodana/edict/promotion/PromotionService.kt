package org.jetbrains.qodana.edict.promotion

import org.jetbrains.qodana.edict.ci.CreateReviewRequest
import org.jetbrains.qodana.edict.ci.ReviewApi
import org.jetbrains.qodana.edict.ci.ReviewReference
import org.jetbrains.qodana.edict.ci.ReviewState
import org.jetbrains.qodana.edict.common.EdictConfiguration
import org.jetbrains.qodana.edict.common.PromotionConfiguration
import org.jetbrains.qodana.edict.edictnext.EdictNextClusterStatus
import org.jetbrains.qodana.edict.edictnext.EdictRepository
import org.jetbrains.qodana.edict.edictnext.InspectionPromotion
import org.jetbrains.qodana.edict.edictnext.PromotionPrState
import org.jetbrains.qodana.edict.edictnext.TokenRedactor
import org.jetbrains.qodana.edict.edictnext.WeakSignalPromotion
import org.jetbrains.qodana.edict.edictnext.EdictNextSignal
import org.jetbrains.qodana.edict.edictnext.EdictNextStoredCluster
import org.jetbrains.qodana.edict.edictnext.exampleSetDigest
import org.jetbrains.qodana.edict.edictnext.sha256Hex
import java.time.Clock
import java.time.Instant
import kotlin.io.path.isRegularFile
import kotlin.io.path.readBytes

internal class PromotionService(
  private val repository: EdictRepository,
  private val configuration: EdictConfiguration = EdictConfiguration(),
  // TODO I think we should require non-null promotionReviews. The whole class depends on this. If promotion step itsel is missing we can make promotion service nullable
  /** The promotion repository's client; null when neither `edict.promotion.url` nor `edict.ci.url` is configured. */
  private val promotionReviews: ReviewApi?,
  /** Review text and review failures reach agents, so they are redacted, and before they are bounded. */
  private val redactor: TokenRedactor,
  private val clock: Clock = Clock.systemUTC(),
  executionBranch: () -> String = { error("The project execution branch is unavailable") },
) {
  // TODO: Is signle promotion service which delegates weak signals to other service right architecture? Why not have dedicated services for inpsectins and weak signals?
  private val weakSignals = WeakSignalPromotionService(repository, configuration, promotionReviews, redactor, executionBranch)

  fun prepareWeakSignals(clusterIds: List<String>? = null): WeakSignalPreparationResponse = weakSignals.prepare(clusterIds)

  fun promoteWeakSignals(clusterId: String, inspectionDigest: String, fixes: List<WeakSignalFix>, validation: String): WeakSignalPromotion =
    weakSignals.publish(clusterId, inspectionDigest, fixes, validation)

  fun checkWeakSignalReviews(): WeakSignalPromotionCheckReviewsResponse = weakSignals.checkReviews()

  fun recordWeakSignalPromotionSignals(
    clusterId: String,
    pullRequest: ReviewReference,
    inspectionDigest: String,
    signals: List<EdictNextSignal>,
  ): List<EdictNextSignal> = weakSignals.recordSignals(clusterId, pullRequest, inspectionDigest, signals)

  fun completeWeakSignalReview(clusterId: String, pullRequest: ReviewReference) =
    weakSignals.completeReview(clusterId, pullRequest)

  fun select(count: Int? = null): PromotionSelectionResponse {
    require(count == null || count > 0) { "count must be positive when provided" }
    val reviews = reviews()
    val targets = requireNotNull(configuration.promotion) { "Inspection promotion requires edict.promotion.reviewer configuration" }
    val targetBranch by lazy { reviews.resolveTargetBranch(targets.targetBranch) }
    val candidates = mutableListOf<PromotionCandidate>()
    val failures = mutableListOf<PromotionFailure>()
    repository.loadClusters().sortedBy { it.id }.forEach { cluster ->
      if (count != null && candidates.size >= count) return@forEach
      val id = cluster.id
      try {
        val evaluation = repository.loadEvaluation(cluster)
        val eligibility = cluster.promotionEligibility(configuration.ci, evaluation)
        if (!eligibility.generated || eligibility.strongPositiveSourceFiles < MIN_PROMOTION_SOURCE_FILES) return@forEach
        val path = repository.paths.inspectionPath(id)
        require(path.isRegularFile()) { "generated inspection is missing" }
        val digest = sha256Hex(path.readBytes())
        if (evaluation == null || evaluation.inspectionHash != digest || evaluation.exampleSetDigest != exampleSetDigest(cluster)) return@forEach
        val precision = evaluation.precision
        val recall = evaluation.recall
        val f1 = if (precision == 0.0 || recall == 0.0) 0.0 else 2 / (1 / precision + 1 / recall)
        if (!precision.isFinite() || precision !in 0.0..1.0 || !recall.isFinite() || recall !in 0.0..1.0 ||
          precision <= targets.targetPrecision || recall <= targets.targetRecall || f1 <= targets.targetF1) return@forEach
        val targetPath = "${targets.inspectionsDirectory.trim('/')}/$id.inspection.kts"
        if (cluster.manifest.promotions.any {
          it.inspectionDigest == digest && it.targetBranch == targetBranch && it.targetPath == targetPath &&
            reviews.repository.owns(it.pullRequest)
        }) return@forEach
        // Validate the referenced evidence without publishing anything.
        promotionDescription(cluster, digest, configuration.ci, evaluation)
        candidates += PromotionCandidate(id, path.toString(), digest, eligibility.referencedSourceFiles, evaluation, f1)
      }
      catch (e: Exception) {
        failures += PromotionFailure(id, e.message ?: e.javaClass.simpleName)
      }
    }
    return PromotionSelectionResponse(
      PromotionThresholds(targets.targetPrecision, targets.targetRecall, targets.targetF1), candidates, failures,
    )
  }

  fun promote(clusterIds: List<String>): PromotionPromoteResponse {
    require(clusterIds.isNotEmpty()) { "clusterIds must not be empty" }
    require(clusterIds.distinct().size == clusterIds.size) { "clusterIds must be unique" }
    val reviews = reviews()
    val promotionConfiguration = requireNotNull(configuration.promotion) {
      "Inspection promotion requires edict.promotion.reviewer configuration"
    }
    // Resolved on the first cluster that needs it, so a run with nothing to promote makes no request.
    val targetBranch = lazy { reviews.resolveTargetBranch(promotionConfiguration.targetBranch) }
    val clusters = repository.loadClusters().associateBy { it.id }
    val outcomes = clusterIds.map { clusterId ->
      val cluster = clusters[clusterId] ?: return@map ClusterPromotion.Failed(PromotionFailure(clusterId, "cluster does not exist"))
      if (cluster.manifest.status != EdictNextClusterStatus.Generated) ClusterPromotion.Skipped
      else promote(cluster, reviews, promotionConfiguration, targetBranch)
    }
    return PromotionPromoteResponse(
      outcomes.filterIsInstance<ClusterPromotion.Created>().map { it.promotion },
      outcomes.filterIsInstance<ClusterPromotion.Failed>().map { it.failure },
    )
  }

  private fun promote(
    cluster: EdictNextStoredCluster,
    reviews: ReviewApi,
    promotionConfiguration: PromotionConfiguration,
    resolvedTargetBranch: Lazy<String>,
  ): ClusterPromotion {
    val inspectionPath = repository.paths.inspectionPath(cluster.id)
    if (!inspectionPath.isRegularFile()) return ClusterPromotion.Failed(PromotionFailure(cluster.id, "generated inspection is missing"))
    val bytes = inspectionPath.readBytes()
    val digest = sha256Hex(bytes)
    // Strict decoding, so the pushed text has exactly the digested bytes.
    val content = try {
      bytes.decodeToString(throwOnInvalidSequence = true)
    }
    catch (_: CharacterCodingException) {
      return ClusterPromotion.Failed(PromotionFailure(cluster.id, "generated inspection is not UTF-8 text"))
    }
    val targetBranch = resolvedTargetBranch.value
    val targetPath = "${promotionConfiguration.inspectionsDirectory.trim('/')}/${cluster.id}.inspection.kts"
    val alreadyPromoted = cluster.manifest.promotions.any {
      it.inspectionDigest == digest && it.targetBranch == targetBranch && it.targetPath == targetPath &&
        reviews.repository.owns(it.pullRequest)
    }
    if (alreadyPromoted) return ClusterPromotion.Skipped
    val target = reviews.repository
    val promotionId = "p-${sha256Hex("${cluster.id}\u0000$digest\u0000${target.provider}\u0000${target.owner}\u0000${target.repository}\u0000$targetBranch\u0000$targetPath").take(24)}"
    return try {
      val review = reviews.findOrCreateReview(
        CreateReviewRequest(
          operationId = promotionId,
          clusterId = cluster.id,
          targetBranch = targetBranch,
          reviewer = promotionConfiguration.reviewer,
          targetPath = targetPath,
          inspectionDigest = digest,
          inspectionContent = content,
          description = promotionDescription(cluster, digest, configuration.ci, repository.loadEvaluation(cluster)),
        ),
      )
      val now = Instant.now(clock).toString()
      val promotion = InspectionPromotion(
        id = promotionId,
        inspectionDigest = digest,
        targetBranch = targetBranch,
        targetPath = targetPath,
        pullRequest = review,
        state = PromotionPrState.ON_REVIEW,
        createdAt = now,
        updatedAt = now,
      )
      ClusterPromotion.Created(repository.appendPromotion(cluster.id, digest, promotion))
    }
    catch (e: Exception) {
      ClusterPromotion.Failed(PromotionFailure(cluster.id, e.message ?: e.javaClass.simpleName))
    }
  }

  fun checkReviews(): PromotionCheckReviewsResponse {
    val outcomes = repository.loadClusters().flatMap { cluster ->
      cluster.manifest.promotions.filter {
        it.state == PromotionPrState.ON_REVIEW ||
          it.state == PromotionPrState.CLOSED && cluster.manifest.status == EdictNextClusterStatus.Generated
      }.map { promotion -> checkReview(cluster, promotion) }
    }
    return PromotionCheckReviewsResponse(
      outcomes.filterIsInstance<ReviewOutcome.Resolved>().map { it.promotion },
      outcomes.mapNotNull {
        when (it) {
          is ReviewOutcome.Undecided -> it.review
          is ReviewOutcome.UnreadableClosed -> it.review
          ReviewOutcome.Open, is ReviewOutcome.Resolved, is ReviewOutcome.Failed -> null
        }
      }.sortedWith(compareBy(UndecidedPromotionReview::clusterId, UndecidedPromotionReview::promotionId)),
      outcomes.mapNotNull {
        when (it) {
          is ReviewOutcome.Failed -> it.failure
          is ReviewOutcome.UnreadableClosed -> it.failure
          ReviewOutcome.Open, is ReviewOutcome.Resolved, is ReviewOutcome.Undecided -> null
        }
      },
    )
  }

  /** A promotion on review is accepted once merged; a closed review, of a promotion on review or closed, awaits a decision. */
  private fun checkReview(cluster: EdictNextStoredCluster, promotion: InspectionPromotion): ReviewOutcome = try {
    val review = reviews().review(promotion.pullRequest)
    when {
      review.state == ReviewState.CLOSED_UNMERGED -> ReviewOutcome.Undecided(
        // Only a closed review needs its discussion, which the decision is based on.
        undecided(cluster, promotion, reviews().reviewWithEvidence(promotion.pullRequest).evidence),
      )
      // A decision needs a closed review, so a reopened or merged review of a closed promotion cannot be decided.
      promotion.state == PromotionPrState.CLOSED -> ReviewOutcome.Failed(
        PromotionReviewFailure(cluster.id, promotion.id, "Promotion is CLOSED, but its review is now ${review.state}"),
      )
      review.state == ReviewState.OPEN -> ReviewOutcome.Open
      else -> ReviewOutcome.Resolved(
        repository.applyResolvedPromotion(cluster.id, promotion.id, PromotionPrState.ACCEPTED, Instant.now(clock).toString()),
      )
    }
  }
  catch (e: Exception) {
    val failure = PromotionReviewFailure(cluster.id, promotion.id, redactor.redact(e.message ?: e.javaClass.simpleName))
    if (promotion.state == PromotionPrState.CLOSED) ReviewOutcome.UnreadableClosed(undecided(cluster, promotion, ""), failure)
    else ReviewOutcome.Failed(failure)
  }

  private fun undecided(cluster: EdictNextStoredCluster, promotion: InspectionPromotion, evidence: String) =
    UndecidedPromotionReview(cluster.id, promotion.id, promotion.pullRequest.url, redactor.redact(evidence).take(MAX_EVIDENCE_CHARS))

  fun decideReviews(decisions: List<PromotionReviewDecision>): PromotionDecideReviewsResponse {
    require(decisions.size in 1..MAX_DECISION_BATCH_SIZE) {
      "A decision batch must contain 1 to $MAX_DECISION_BATCH_SIZE promotions"
    }
    require(decisions.distinctBy { it.clusterId to it.promotionId }.size == decisions.size) {
      "Only one decision may be supplied for each promotion"
    }
    decisions.forEach {
      require(it.rationale.isNotBlank()) { "A rationale is required for ${it.decision}" }
      require(it.rationale.length <= MAX_RATIONALE_CHARS) { "Decision rationale exceeds $MAX_RATIONALE_CHARS characters" }
    }
    val outcomes = decisions.map { decision ->
      try {
        ReviewOutcome.Resolved(decide(decision))
      }
      catch (e: Exception) {
        ReviewOutcome.Failed(
          PromotionReviewFailure(decision.clusterId, decision.promotionId, redactor.redact(e.message ?: e.javaClass.simpleName)),
        )
      }
    }
    return PromotionDecideReviewsResponse(
      outcomes.filterIsInstance<ReviewOutcome.Resolved>().map { it.promotion },
      outcomes.filterIsInstance<ReviewOutcome.Failed>().map { it.failure },
    )
  }

  private fun decide(decision: PromotionReviewDecision): InspectionPromotion {
    val cluster = repository.loadCluster(decision.clusterId)
    val promotion = cluster.manifest.promotions.singleOrNull { it.id == decision.promotionId }
      ?: error("Unknown promotion '${decision.promotionId}' in cluster '${decision.clusterId}'")
    require(promotion.state in setOf(PromotionPrState.ON_REVIEW, PromotionPrState.CLOSED)) {
      "Promotion '${decision.promotionId}' is not awaiting a closed-review decision"
    }
    val review = reviews().review(promotion.pullRequest)
    require(review.state == ReviewState.CLOSED_UNMERGED) {
      "Promotion '${decision.promotionId}' remote review is ${review.state}, not CLOSED_UNMERGED"
    }
    return repository.applyResolvedPromotion(
      clusterId = cluster.id,
      promotionId = promotion.id,
      resolvedState = PromotionPrState.CLOSED,
      updatedAt = Instant.now(clock).toString(),
      expectedInspectionDigest = promotion.inspectionDigest,
      targetStatus = when (decision.decision) {
        ClosedReviewDecision.MOVE_CLUSTER_TO_PENDING -> EdictNextClusterStatus.Pending
        ClosedReviewDecision.DISCONTINUE_CLUSTER -> EdictNextClusterStatus.Discontinued
      },
      rationale = decision.rationale,
    )
  }

  private fun reviews(): ReviewApi = requireNotNull(promotionReviews) {
    "Inspection promotion requires edict.promotion.url or edict.ci.url configuration"
  }

  private companion object {
    const val MAX_EVIDENCE_CHARS = 32_000
    const val MAX_RATIONALE_CHARS = 4_000
    const val MAX_DECISION_BATCH_SIZE = 10
  }
}

/** What [PromotionService.promote] did with one requested cluster. */
private sealed interface ClusterPromotion {
  data class Created(val promotion: InspectionPromotion) : ClusterPromotion
  data class Failed(val failure: PromotionFailure) : ClusterPromotion
  /** Not eligible for an inspection PR, or already on review with the same inspection. */
  data object Skipped : ClusterPromotion
}

/** What checking or deciding one promotion's review produced. */
private sealed interface ReviewOutcome {
  /** Still on review. */
  data object Open : ReviewOutcome
  data class Resolved(val promotion: InspectionPromotion) : ReviewOutcome
  /** Closed unmerged: awaits a decision. */
  data class Undecided(val review: UndecidedPromotionReview) : ReviewOutcome
  data class Failed(val failure: PromotionReviewFailure) : ReviewOutcome
  /** A closed promotion whose review cannot be read: it still awaits a decision, and the read failure is reported. */
  data class UnreadableClosed(val review: UndecidedPromotionReview, val failure: PromotionReviewFailure) : ReviewOutcome
}
