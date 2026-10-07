package org.jetbrains.qodana.edict.promotion

import org.jetbrains.qodana.edict.ci.CreateReviewRequest
import org.jetbrains.qodana.edict.ci.ReviewClient
import org.jetbrains.qodana.edict.ci.ReviewManagementApi
import org.jetbrains.qodana.edict.ci.ReviewReference
import org.jetbrains.qodana.edict.ci.ReviewState
import org.jetbrains.qodana.edict.common.EdictConfiguration
import org.jetbrains.qodana.edict.edictnext.EdictNextClusterStatus
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalStrength
import org.jetbrains.qodana.edict.edictnext.EdictRepository
import org.jetbrains.qodana.edict.edictnext.InspectionPromotion
import org.jetbrains.qodana.edict.edictnext.PromotionPrState
import org.jetbrains.qodana.edict.edictnext.sha256Hex
import java.time.Clock
import java.time.Instant
import kotlin.io.path.isRegularFile
import kotlin.io.path.readBytes

internal class PromotionService(
  private val repository: () -> EdictRepository,
  private val configuration: EdictConfiguration = EdictConfiguration(),
  private val reviews: ReviewManagementApi = ReviewClient(),
  private val clock: Clock = Clock.systemUTC(),
) {
  fun promote(clusterIds: List<String>? = null): PromotionPromoteResponse {
    require(clusterIds == null || clusterIds.isNotEmpty()) { "clusterIds must not be empty when provided" }
    require(clusterIds == null || clusterIds.distinct().size == clusterIds.size) { "clusterIds must be unique" }
    val ci = requireNotNull(configuration.ci) { "Inspection promotion requires complete edict.ci configuration" }
    val promotionConfiguration = requireNotNull(configuration.promotion) {
      "Inspection promotion requires edict.promotion.reviewer configuration"
    }
    val targetBranch by lazy { reviews.resolveTargetBranch(ci.reviewRepository, promotionConfiguration.targetBranch) }
    val repository = repository()
    val created = mutableListOf<InspectionPromotion>()
    val failures = mutableListOf<PromotionFailure>()
    val available = repository.loadClusters()
    val clusters = if (clusterIds == null) available else {
      val byId = available.associateBy { it.id }
      clusterIds.mapNotNull { clusterId ->
        byId[clusterId] ?: run {
          failures += PromotionFailure(clusterId, "cluster does not exist")
          null
        }
      }
    }
    clusters.forEach { cluster ->
      if (cluster.manifest.status != EdictNextClusterStatus.Generated) {
        return@forEach
      }
      val strongSignals = cluster.signals.count { it.strength == EdictNextSignalStrength.STRONG }
      if (strongSignals < MIN_STRONG_SIGNALS) {
        return@forEach
      }
      val inspectionPath = repository.paths.inspectionPath(cluster.id)
      if (!inspectionPath.isRegularFile()) {
        failures += PromotionFailure(cluster.id, "generated inspection is missing")
        return@forEach
      }
      val content = inspectionPath.readBytes()
      val digest = sha256Hex(content)
      val targetPath = "${promotionConfiguration.inspectionsDirectory.trim('/')}/${cluster.id}.inspection.kts"
      val alreadyPromoted = cluster.manifest.promotions.any {
        it.inspectionDigest == digest && it.targetBranch == targetBranch && it.targetPath == targetPath &&
          it.pullRequest.provider == ci.provider && it.pullRequest.owner == ci.owner &&
          it.pullRequest.repository == ci.repository
      }
      if (alreadyPromoted) return@forEach
      val promotionId = "p-${sha256Hex("${cluster.id}\u0000$digest\u0000${ci.provider}\u0000${ci.owner}\u0000${ci.repository}\u0000$targetBranch\u0000$targetPath").take(24)}"
      try {
        val remote = reviews.ensureReview(
          CreateReviewRequest(
            operationId = promotionId,
            clusterId = cluster.id,
            repository = ci.reviewRepository,
            targetBranch = targetBranch,
            reviewer = promotionConfiguration.reviewer,
            targetPath = targetPath,
            inspectionDigest = digest,
            inspectionContent = content,
          ),
        )
        val now = Instant.now(clock).toString()
        val promotion = InspectionPromotion(
          id = promotionId,
          inspectionDigest = digest,
          targetBranch = targetBranch,
          targetPath = targetPath,
          pullRequest = ReviewReference(ci.provider, ci.owner, ci.repository, remote.id, remote.url),
          state = PromotionPrState.ON_REVIEW,
          createdAt = now,
          updatedAt = now,
        )
        created += repository.appendPromotion(cluster.id, digest, promotion)
      }
      catch (e: Exception) {
        failures += PromotionFailure(cluster.id, e.message ?: e.javaClass.simpleName)
      }
    }
    return PromotionPromoteResponse(created, failures)
  }

  fun checkReviews(): PromotionCheckReviewsResponse {
    val repository = repository()
    val accepted = mutableListOf<InspectionPromotion>()
    val undecided = mutableListOf<UndecidedPromotionReview>()
    val failures = mutableListOf<PromotionReviewFailure>()

    repository.loadClusters().forEach { cluster ->
      cluster.manifest.promotions.filter {
        it.state == PromotionPrState.ON_REVIEW ||
          it.state == PromotionPrState.CLOSED && cluster.manifest.status == EdictNextClusterStatus.Generated
      }.forEach { promotion ->
        try {
          val review = reviews.review(promotion.pullRequest)
          if (promotion.state == PromotionPrState.ON_REVIEW) {
            when (review.state) {
              ReviewState.OPEN -> Unit
              ReviewState.MERGED -> accepted += repository.applyResolvedPromotion(
                cluster.id,
                promotion.id,
                PromotionPrState.ACCEPTED,
                Instant.now(clock).toString(),
              )
              ReviewState.CLOSED_UNMERGED -> {
                undecided += UndecidedPromotionReview(
                  cluster.id,
                  promotion.id,
                  promotion.pullRequest.url,
                  review.evidence.take(MAX_EVIDENCE_CHARS),
                )
              }
            }
          }
          else {
            undecided += UndecidedPromotionReview(
                cluster.id,
                promotion.id,
                promotion.pullRequest.url,
                if (review.state == ReviewState.CLOSED_UNMERGED) review.evidence.take(MAX_EVIDENCE_CHARS)
                else "Remote review state is ${review.state}",
              )
          }
        }
        catch (e: Exception) {
          failures += PromotionReviewFailure(cluster.id, promotion.id, e.message ?: e.javaClass.simpleName)
          if (promotion.state == PromotionPrState.CLOSED) {
            undecided += UndecidedPromotionReview(cluster.id, promotion.id, promotion.pullRequest.url, "")
          }
        }
      }
    }
    return PromotionCheckReviewsResponse(
      accepted,
      undecided.sortedWith(compareBy(UndecidedPromotionReview::clusterId, UndecidedPromotionReview::promotionId)),
      failures,
    )
  }

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
    val repository = repository()
    val updated = mutableListOf<InspectionPromotion>()
    val failures = mutableListOf<PromotionReviewFailure>()
    decisions.forEach { decision ->
      try {
        val cluster = repository.loadCluster(decision.clusterId)
        val promotion = cluster.manifest.promotions.singleOrNull { it.id == decision.promotionId }
          ?: error("Unknown promotion '${decision.promotionId}' in cluster '${decision.clusterId}'")
        require(promotion.state in setOf(PromotionPrState.ON_REVIEW, PromotionPrState.CLOSED)) {
          "Promotion '${decision.promotionId}' is not awaiting a closed-review decision"
        }
        val remote = reviews.review(promotion.pullRequest)
        require(remote.state == ReviewState.CLOSED_UNMERGED) {
          "Promotion '${decision.promotionId}' remote review is ${remote.state}, not CLOSED_UNMERGED"
        }
        when (decision.decision) {
          ClosedReviewDecision.MOVE_CLUSTER_TO_PENDING -> updated += repository.applyResolvedPromotion(
            clusterId = cluster.id,
            promotionId = promotion.id,
            resolvedState = PromotionPrState.CLOSED,
            updatedAt = Instant.now(clock).toString(),
            expectedInspectionDigest = promotion.inspectionDigest,
            targetStatus = EdictNextClusterStatus.Pending,
            rationale = decision.rationale,
          )
          ClosedReviewDecision.DISCONTINUE_CLUSTER -> updated += repository.applyResolvedPromotion(
            clusterId = cluster.id,
            promotionId = promotion.id,
            resolvedState = PromotionPrState.CLOSED,
            updatedAt = Instant.now(clock).toString(),
            expectedInspectionDigest = promotion.inspectionDigest,
            targetStatus = EdictNextClusterStatus.Discontinued,
            rationale = decision.rationale,
          )
        }
      }
      catch (e: Exception) {
        failures += PromotionReviewFailure(decision.clusterId, decision.promotionId, e.message ?: e.javaClass.simpleName)
      }
    }
    return PromotionDecideReviewsResponse(updated, failures)
  }

  private companion object {
    const val MIN_STRONG_SIGNALS = 6
    const val MAX_EVIDENCE_CHARS = 32_000
    const val MAX_RATIONALE_CHARS = 4_000
    const val MAX_DECISION_BATCH_SIZE = 10
  }
}
