package org.jetbrains.qodana.edict.promotion

import kotlinx.serialization.Serializable
import org.jetbrains.qodana.edict.edictnext.InspectionPromotion

@Serializable
internal data class PromotionFailure(val clusterId: String, val message: String)

@Serializable
internal data class PromotionPromoteResponse(
  val created: List<InspectionPromotion> = emptyList(),
  val failures: List<PromotionFailure> = emptyList(),
)

@Serializable
internal enum class ClosedReviewDecision {
  MOVE_CLUSTER_TO_PENDING,
  DISCONTINUE_CLUSTER,
}

@Serializable
internal data class PromotionReviewDecision(
  val clusterId: String,
  val promotionId: String,
  val decision: ClosedReviewDecision,
  val rationale: String,
)

@Serializable
internal data class UndecidedPromotionReview(
  val clusterId: String,
  val promotionId: String,
  val pullRequestUrl: String,
  val evidence: String,
)

@Serializable
internal data class PromotionReviewFailure(
  val clusterId: String,
  val promotionId: String,
  val message: String,
)

@Serializable
internal data class PromotionCheckReviewsResponse(
  val accepted: List<InspectionPromotion> = emptyList(),
  val undecided: List<UndecidedPromotionReview> = emptyList(),
  val failures: List<PromotionReviewFailure> = emptyList(),
)

@Serializable
internal data class PromotionDecideReviewsResponse(
  val updated: List<InspectionPromotion> = emptyList(),
  val failures: List<PromotionReviewFailure> = emptyList(),
)
