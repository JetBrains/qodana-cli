// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.promotion.support

import org.jetbrains.qodana.edict.ci.CiProviderId
import org.jetbrains.qodana.edict.support.resourceText
import org.jetbrains.qodana.edict.ci.CreateReviewRequest
import org.jetbrains.qodana.edict.ci.ReviewApi
import org.jetbrains.qodana.edict.ci.ReviewReference
import org.jetbrains.qodana.edict.ci.ReviewRepository
import org.jetbrains.qodana.edict.common.EdictConfiguration
import org.jetbrains.qodana.edict.common.PromotionConfiguration
import org.jetbrains.qodana.edict.edictnext.EdictRepository
import org.jetbrains.qodana.edict.edictnext.InspectionPromotion
import org.jetbrains.qodana.edict.edictnext.PromotionPrState
import org.jetbrains.qodana.edict.edictnext.TokenRedactor
import org.jetbrains.qodana.edict.edictnext.sha256Hex
import org.jetbrains.qodana.edict.promotion.PromotionService
import org.jetbrains.qodana.edict.promotion.WeakSignalPromotionService
import org.jetbrains.qodana.edict.support.NoRedaction
import java.time.Clock
import java.time.Instant
import java.time.ZoneOffset

/** Where the promotion tests open reviews. */
internal val PROMOTION_REPOSITORY = ReviewRepository(CiProviderId.GITHUB, "github.com", "JetBrains", "qodana-cli")

/** A repository promotions were opened in before the configuration moved to [PROMOTION_REPOSITORY]. */
internal val FOREIGN_REPOSITORY = ReviewRepository(CiProviderId.GITHUB, "github.com", "JetBrains", "kotlin")

internal val PROMOTION = PromotionConfiguration("reviewer", "main")

/** When a stored promotion was created, before the tests run. */
internal const val CREATED_AT = "2026-10-01T00:00:00Z"

/** The time of every test run, as the services' clock tells it. */
internal const val NOW = "2026-10-04T12:00:00Z"

/** The text of every generated inspection the fixtures store. */
internal const val INSPECTION = "inspection"

internal val INSPECTION_DIGEST = sha256Hex(INSPECTION)

/** The id of promoting [INSPECTION] of cluster `rule` to `inspections/rule.inspection.kts` on `main` of [PROMOTION_REPOSITORY]. */
internal const val RULE_PROMOTION_ID = "p-fdbb61978378fb3349d2c557"

/** The id of promoting [INSPECTION] of cluster `other-rule` to its inspection path on `main` of [PROMOTION_REPOSITORY]. */
internal const val OTHER_RULE_PROMOTION_ID = "p-48adbdaa3a8a96a86f94f838"

/** The id of promoting [INSPECTION] of cluster `rule` to `inspections/rule.inspection.kts` on `trunk` of [PROMOTION_REPOSITORY]. */
internal const val RULE_TRUNK_PROMOTION_ID = "p-f2b783e56b53dad6470dead1"

/** A secret the redacting services hide as `[redacted]`. */
internal const val SECRET = "secret-token"

internal val SECRET_REDACTION = TokenRedactor { it.replace(SECRET, "[redacted]") }

/** The review numbered [number] in [PROMOTION_REPOSITORY]; the fake opens them from 1 on. */
internal fun review(number: Int): ReviewReference =
  PROMOTION_REPOSITORY.reference("$number", "https://github.com/JetBrains/qodana-cli/pull/$number")

internal fun foreignReview(number: Int): ReviewReference =
  FOREIGN_REPOSITORY.reference("$number", "https://github.com/JetBrains/kotlin/pull/$number")

/** A promotion of [INSPECTION] of cluster [clusterId] to its path in the inspections directory. */
internal fun inspectionPromotion(
  state: PromotionPrState = PromotionPrState.ON_REVIEW,
  id: String = RULE_PROMOTION_ID,
  clusterId: String = "rule",
  pullRequest: ReviewReference = review(1),
  inspectionDigest: String = INSPECTION_DIGEST,
  targetBranch: String = "main",
  createdAt: String = CREATED_AT,
): InspectionPromotion = InspectionPromotion(
  id, inspectionDigest, targetBranch, "inspections/$clusterId.inspection.kts", pullRequest, state, createdAt, createdAt,
)

/** What promoting cluster `rule` as the first review creates. */
internal val PROMOTED_RULE = inspectionPromotion(createdAt = NOW)

/** The review request promoting cluster `rule` sends. */
internal val RULE_REVIEW_REQUEST = CreateReviewRequest(
  operationId = RULE_PROMOTION_ID,
  clusterId = "rule",
  targetBranch = "main",
  reviewer = "reviewer",
  targetPath = "inspections/rule.inspection.kts",
  inspectionDigest = INSPECTION_DIGEST,
  inspectionContent = INSPECTION,
  description = expectedText("rule-description.md"),
)

/** The promotion service of [PROMOTION_REPOSITORY], whose clock shows [NOW]. */
internal fun promotionService(
  repository: EdictRepository,
  reviews: ReviewApi?,
  promotion: PromotionConfiguration? = PROMOTION,
  redactor: TokenRedactor = NoRedaction,
): PromotionService = PromotionService(
  repository, EdictConfiguration(ci = PROMOTION_REPOSITORY, promotion = promotion), reviews, redactor, Clock.fixed(Instant.parse(NOW), ZoneOffset.UTC),
)

/** The weak-signal promotion service of [PROMOTION_REPOSITORY]. */
internal fun weakSignalService(
  repository: EdictRepository,
  reviews: ReviewApi?,
  promotion: PromotionConfiguration? = PROMOTION,
  redactor: TokenRedactor = NoRedaction,
): WeakSignalPromotionService =
  WeakSignalPromotionService(repository, EdictConfiguration(ci = PROMOTION_REPOSITORY, promotion = promotion), reviews, redactor)

/** An expected text from the `promotion` test resources, by its file [name]. */
internal fun expectedText(name: String): String = resourceText("promotion/$name")
