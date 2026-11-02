// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.promotion.support

import kotlinx.serialization.encodeToString
import org.jetbrains.qodana.edict.ci.CreateReviewRequest
import org.jetbrains.qodana.edict.ci.MergedReview
import org.jetbrains.qodana.edict.ci.ReviewFileChange
import org.jetbrains.qodana.edict.ci.ReviewState
import org.jetbrains.qodana.edict.ci.ReviewStatus
import org.jetbrains.qodana.edict.ci.SourceFixChanges
import org.jetbrains.qodana.edict.edictnext.EdictNextCodeExampleMetadata
import org.jetbrains.qodana.edict.edictnext.EdictNextEvaluation
import org.jetbrains.qodana.edict.edictnext.EdictNextFileRevision
import org.jetbrains.qodana.edict.edictnext.EdictNextInspectionAction
import org.jetbrains.qodana.edict.edictnext.EdictNextJson
import org.jetbrains.qodana.edict.edictnext.EdictNextLineRange
import org.jetbrains.qodana.edict.edictnext.EdictNextProjectFinding
import org.jetbrains.qodana.edict.edictnext.EdictNextProjectReview
import org.jetbrains.qodana.edict.edictnext.EdictNextSignal
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalLabel
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalProvenance
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalSource
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalStrength
import org.jetbrains.qodana.edict.edictnext.EdictRepository
import org.jetbrains.qodana.edict.edictnext.PromotionPrState
import org.jetbrains.qodana.edict.edictnext.WeakSignalPromotion
import org.jetbrains.qodana.edict.edictnext.WeakSignalPromotionIndex
import org.jetbrains.qodana.edict.edictnext.sha256Hex
import org.jetbrains.qodana.edict.edictnext.stableSignalId
import org.jetbrains.qodana.edict.promotion.CheckedWeakSignalReview
import org.jetbrains.qodana.edict.promotion.WeakSignalFix
import org.jetbrains.qodana.edict.promotion.WeakSignalPromotionCandidate
import org.jetbrains.qodana.edict.support.FakeReviewApi
import org.jetbrains.qodana.edict.support.resourceText
import kotlin.io.path.createDirectories
import kotlin.io.path.readText
import kotlin.io.path.writeText

/** The full revision the project review analyzed; the merged source fix starts from it. */
internal const val PROJECT_REVISION = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

/** The full revision the source fix merged as. */
internal const val MERGED_REVISION = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

internal const val INSPECTION_DESCRIPTION = "Avoid busy waiting"

/** A true positive the project review found on [line] of [path] at [PROJECT_REVISION]. */
internal fun finding(line: Int, path: String = "src/Worker.kt"): EdictNextProjectFinding =
  EdictNextProjectFinding(EdictNextFileRevision(path, PROJECT_REVISION, listOf(EdictNextLineRange(line, line))), "Busy waiting")

/** The default project review's true positives: lines 1 and 2 of `src/Worker.kt` and line 3 of `src/Other.kt`. */
internal val FINDINGS = listOf(finding(1), finding(2), finding(3, "src/Other.kt"))

internal fun projectReview(
  findings: List<EdictNextProjectFinding> = FINDINGS,
  /** The analyzed revision; a full SHA unless a test breaks it. */
  revision: String = PROJECT_REVISION,
  description: String = INSPECTION_DESCRIPTION,
): EdictNextProjectReview = EdictNextProjectReview(revision, findings.size, findings.size, 0, 0, 0, description, findings)

/** The cluster with a positive and a negative example and an evaluation of the inspection [inspectionDigest] with [review]. */
internal fun ClusterFixture.withEvaluation(
  review: EdictNextProjectReview?,
  inspectionDigest: String = INSPECTION_DIGEST,
): ClusterFixture = copy(
  examples = listOf(
    EdictNextCodeExampleMetadata("positive", "Example.kt", EdictNextSignalLabel.POSITIVE, listOf(EdictNextLineRange(1, 1))),
    EdictNextCodeExampleMetadata("negative", "Example.kt", EdictNextSignalLabel.NEGATIVE),
  ),
  evaluation = EdictNextEvaluation(
    inspectionDigest, "examples", EdictNextInspectionAction.GENERATE, review?.truePositiveFindings.orEmpty().size, 0, 0, 1.0, 1.0, emptyMap(), review,
  ),
)

/** A weak-signal candidate: a generated cluster with one referenced source file and a project review of [findings]. */
internal fun weakCluster(findings: List<EdictNextProjectFinding> = FINDINGS): ClusterFixture =
  ClusterFixture(listOf(referencedSignal(0).copy(syntheticExampleId = "positive"))).withEvaluation(projectReview(findings))

/** [weakCluster] with a strong negative Signal of the negative example too, so it takes merged corrections of both labels. */
internal fun correctableCluster(): ClusterFixture = weakCluster().let {
  it.copy(signals = it.signals + referencedSignal(1).copy(label = EdictNextSignalLabel.NEGATIVE, syntheticExampleId = "negative"))
}

/** [correctableCluster] once [CORRECTIONS] are recorded in it. */
internal fun correctedCluster(): ClusterFixture = correctableCluster().let { it.copy(signals = it.signals + CORRECTIONS) }

/** The candidate [weakCluster] offers: its findings by path and line. */
internal val CANDIDATE = WeakSignalPromotionCandidate(
  "rule", INSPECTION_DIGEST, PROJECT_REVISION, INSPECTION_DESCRIPTION, listOf(finding(3, "src/Other.kt"), finding(1), finding(2)),
)

internal val WORKER_FIX = WeakSignalFix(
  "src/Worker.kt", listOf(2, 1), "fun waitForResult() = latch.await()\n", "Wait for completion instead of spinning",
)

internal val OTHER_FIX = WeakSignalFix("src/Other.kt", listOf(3), "fun waitForOther() = other.await()\n", "Wait for the other result")

/** Fixes of every [CANDIDATE] finding. */
internal val SOURCE_FIXES = listOf(WORKER_FIX, OTHER_FIX)

/** [SOURCE_FIXES] with another content of `src/Worker.kt`. */
internal val CHANGED_SOURCE_FIXES = listOf(WORKER_FIX.copy(content = "fun waitForResult() = result.get()\n"), OTHER_FIX)

/** The validation results published with the source fixes. */
internal const val VALIDATION = "Reviewed diff; focused tests passed"

/** The id of publishing [SOURCE_FIXES] of cluster `rule` at [PROJECT_REVISION] to `main` of [PROMOTION_REPOSITORY]. */
internal const val SOURCE_FIX_OPERATION_ID = "w-dc9713952433b3216d945d48"

/** The id of publishing [CHANGED_SOURCE_FIXES] like [SOURCE_FIX_OPERATION_ID]. */
internal const val CHANGED_SOURCE_FIX_OPERATION_ID = "w-a0bea3791814c4c3cb05a634"

/** The review request publishing [SOURCE_FIXES] with [VALIDATION] sends. */
internal val SOURCE_FIX_REQUEST = CreateReviewRequest(
  operationId = SOURCE_FIX_OPERATION_ID,
  clusterId = "rule",
  targetBranch = "main",
  reviewer = "reviewer",
  targetPath = "",
  inspectionDigest = INSPECTION_DIGEST,
  inspectionContent = "",
  description = expectedText("source-fix-description.md"),
  sourceFix = SourceFixChanges(PROJECT_REVISION, SOURCE_FIXES.associate { it.path to it.content }),
)

/** The source-fix review of cluster `rule`, the first the fake opens. */
internal val SOURCE_FIX_REVIEW = review(1)

internal val SOURCE_FIX_ON_REVIEW = WeakSignalPromotion(SOURCE_FIX_REVIEW)

internal val SOURCE_FIX_ACCEPTED = WeakSignalPromotion(SOURCE_FIX_REVIEW, PromotionPrState.ACCEPTED)

/** The discussion of the merged [SOURCE_FIX_REVIEW]. */
internal const val REVIEW_FEEDBACK = "Use a blocking wait"

/** How [SOURCE_FIX_REVIEW] merged: it replaced the busy wait in `src/Worker.kt`. */
internal val MERGED_SOURCE_FIX = MergedReview(
  "Fix busy waiting", listOf(REVIEW_FEEDBACK), PROJECT_REVISION, MERGED_REVISION, listOf(ReviewFileChange("src/Worker.kt", "src/Worker.kt")),
)

/** `src/Worker.kt` before and after [MERGED_SOURCE_FIX]. */
private const val SOURCE_BEFORE = "while (!done) {}\n"
private const val SOURCE_AFTER = "latch.await()\n"

/** The diff of `src/Worker.kt` between the revisions of [MERGED_SOURCE_FIX]. */
internal const val MERGED_DIFF = "--- a/src/Worker.kt\n+++ b/src/Worker.kt\n@@ -1 +1 @@\n-while (!done) {}\n+latch.await()\n"

/** Merges [SOURCE_FIX_REVIEW] as [MERGED_SOURCE_FIX], with its discussion and both versions of the changed file. */
internal fun FakeReviewApi.mergeSourceFix() {
  statuses[SOURCE_FIX_REVIEW] = ReviewStatus(ReviewState.MERGED, REVIEW_FEEDBACK)
  merges[SOURCE_FIX_REVIEW] = MERGED_SOURCE_FIX
  files[PROJECT_REVISION to "src/Worker.kt"] = SOURCE_BEFORE
  files[MERGED_REVISION to "src/Worker.kt"] = SOURCE_AFTER
}

/** What checking the merged [SOURCE_FIX_REVIEW] of [weakCluster] reports. */
internal val CHECKED_MERGED_SOURCE_FIX = CheckedWeakSignalReview(
  "rule", SOURCE_FIX_ACCEPTED, REVIEW_FEEDBACK, INSPECTION_DIGEST, MERGED_SOURCE_FIX, listOf(MERGED_DIFF),
)

/** What checking [SOURCE_FIX_REVIEW] reports while it isn't merged. */
internal fun checkedSourceFix(state: PromotionPrState, evidence: String): CheckedWeakSignalReview =
  CheckedWeakSignalReview("rule", WeakSignalPromotion(SOURCE_FIX_REVIEW, state), evidence, inspectionDigest = null)

/** Makes [promotion] the only active weak-signal review, of cluster `rule`. */
internal fun EdictRepository.storeActiveReview(promotion: WeakSignalPromotion) {
  paths.activeWeakSignalPromotionsPath.parent.createDirectories()
  paths.activeWeakSignalPromotionsPath.writeText(activeReviewsText(promotion))
}

/** The active weak-signal review state holding only [promotion], as it is stored. */
internal fun activeReviewsText(promotion: WeakSignalPromotion): String =
  EdictNextJson.encodeToString(WeakSignalPromotionIndex(mapOf("rule" to listOf(promotion))))

/** Archives [promotion] as the only completed weak-signal review of cluster `rule`. */
internal fun EdictRepository.storeArchivedReview(promotion: WeakSignalPromotion) {
  paths.weakSignalPromotionHistoryPath("rule").parent.createDirectories()
  paths.weakSignalPromotionHistoryPath("rule").writeText(EdictNextJson.encodeToString(listOf(promotion)))
}

/** Makes the completed weak-signal reviews of cluster `rule` unreadable. */
internal fun EdictRepository.storeUnreadableArchive() {
  paths.weakSignalPromotionHistoryPath("rule").parent.createDirectories()
  paths.weakSignalPromotionHistoryPath("rule").writeText("not JSON")
}

/**
 * Puts a legacy record of [SOURCE_FIX_ACCEPTED], with the fields it no longer has, into the manifest of cluster `rule`,
 * where weak-signal reviews were stored before `promotions/active.json`.
 */
internal fun EdictRepository.storeLegacyActiveReview() {
  val manifest = loadCluster("rule").directory.manifestPath
  val record = resourceText("promotion/legacy-weak-signal-promotion.json")
  manifest.writeText(manifest.readText().trimEnd().removeSuffix("}") + ",\"weakSignalPromotions\":[$record]}")
}

/** The work item of corrections merged with [SOURCE_FIX_REVIEW]. */
private const val CORRECTION_WORK_ITEM = "weak-promotion:GITHUB:JetBrains/qodana-cli:1:$MERGED_REVISION"

/** A merged correction of `src/Worker.kt` line 1 as the checker records it: the busy wait if [label] is positive, its fix otherwise. */
private fun correction(label: EdictNextSignalLabel): EdictNextSignal {
  val revision = if (label == EdictNextSignalLabel.POSITIVE) PROJECT_REVISION else MERGED_REVISION
  val key = "$CORRECTION_WORK_ITEM:$label:src/Worker.kt:$revision:1-1"
  return EdictNextSignal(
    stableSignalId(key),
    key,
    EdictNextFileRevision("src/Worker.kt", revision, listOf(EdictNextLineRange(1, 1))),
    EdictNextSignalSource.FromPR(1, MERGED_SOURCE_FIX.title, MERGED_SOURCE_FIX.discussionMessages, MERGED_DIFF, SOURCE_FIX_REVIEW.url),
    label,
    "Use a blocking wait instead of spinning",
    syntheticExampleId = label.name.lowercase(),
    provenance = EdictNextSignalProvenance(CORRECTION_WORK_ITEM),
  )
}

internal val POSITIVE_CORRECTION = correction(EdictNextSignalLabel.POSITIVE)

internal val NEGATIVE_CORRECTION = correction(EdictNextSignalLabel.NEGATIVE)

internal val CORRECTIONS = listOf(POSITIVE_CORRECTION, NEGATIVE_CORRECTION)

/** A merged positive correction that no example of the cluster covers, so it goes to the inbox. */
internal val UNCOVERED_CORRECTION = POSITIVE_CORRECTION.copy(syntheticExampleId = null)

/** Clusters that cannot offer a weak-signal candidate, and why. */
internal enum class UnpreparedCluster(val cluster: ClusterFixture, val message: String) {
  NO_EVALUATION(weakCluster().copy(evaluation = null), "No persisted evaluation; regenerate the inspection to review project findings"),
  STALE_EVALUATION(
    weakCluster().withEvaluation(projectReview(), sha256Hex("older inspection")),
    "Evaluation does not match the generated inspection",
  ),
  NO_PROJECT_REVIEW(
    weakCluster().withEvaluation(null),
    "No project review for this inspection; regenerate it before promoting weak signals",
  ),
  NO_DESCRIPTION(
    weakCluster().withEvaluation(projectReview(description = "")),
    "No persisted inspection description or exact project revision; regenerate the inspection",
  ),
  INEXACT_REVISION(
    weakCluster().withEvaluation(projectReview(revision = "HEAD")),
    "No persisted inspection description or exact project revision; regenerate the inspection",
  ),
}

/** Project-review true positives no source fix may select. */
internal enum class UnselectableFinding(val finding: EdictNextProjectFinding) {
  OTHER_REVISION(finding(4).let { it.copy(fileRevision = it.fileRevision.copy(revision = MERGED_REVISION)) }),
  NO_LINE(finding(4).let { it.copy(fileRevision = it.fileRevision.copy(expectedRanges = null)) }),
  PATH_OUTSIDE_REPOSITORY(finding(4, "../Worker.kt")),
}

/** Publications of [CANDIDATE] that differ from [SOURCE_FIXES] and [VALIDATION] only where the review branch doesn't depend. */
internal enum class EquivalentPublication(val fixes: List<WeakSignalFix>, val validation: String) {
  REORDERED(SOURCE_FIXES.reversed(), VALIDATION),
  REEXPLAINED(SOURCE_FIXES.map { it.copy(explanation = "Rechecked fix") }, VALIDATION),
  REVALIDATED(SOURCE_FIXES, "Rechecked diff"),
}

/** Source fixes of [CANDIDATE] publishing rejects, and why. */
internal enum class InvalidPublication(val fixes: List<WeakSignalFix>, val message: String, val validation: String = VALIDATION) {
  NO_FIXES(emptyList(), "Supply unique, nonempty source fixes"),
  REPEATED_FILE(listOf(WORKER_FIX.copy(lines = listOf(1)), WORKER_FIX.copy(lines = listOf(2))), "Supply unique, nonempty source fixes"),
  BLANK_VALIDATION(SOURCE_FIXES, "Describe validation results in at most 8000 characters", validation = " "),
  LONG_VALIDATION(SOURCE_FIXES, "Describe validation results in at most 8000 characters", validation = "x".repeat(8_001)),
  PATH_OUTSIDE_REPOSITORY(listOf(WORKER_FIX.copy(path = "../Worker.kt")), "Invalid source fix for '../Worker.kt'"),
  EMPTY_CONTENT(listOf(WORKER_FIX.copy(content = "")), "Invalid source fix for 'src/Worker.kt'"),
  NUL_IN_CONTENT(listOf(WORKER_FIX.copy(content = "fun a()\u0000\n")), "Invalid source fix for 'src/Worker.kt'"),
  NO_EXPLANATION(listOf(WORKER_FIX.copy(explanation = " ")), "Every fixed file needs a bounded explanation"),
  LONG_EXPLANATION(listOf(WORKER_FIX.copy(explanation = "x".repeat(4_001))), "Every fixed file needs a bounded explanation"),
  NO_LINES(listOf(WORKER_FIX.copy(lines = emptyList())), "Supply unique fixed lines for 'src/Worker.kt'"),
  REPEATED_LINE(listOf(WORKER_FIX.copy(lines = listOf(1, 1))), "Supply unique fixed lines for 'src/Worker.kt'"),
  UNSELECTED_LINE(listOf(WORKER_FIX.copy(lines = listOf(3))), "Fix only selected true-positive locations"),
}

/** Corrections of [MERGED_SOURCE_FIX] that misstate what merged, and why recording them fails. */
internal enum class ForgedCorrection(val correction: EdictNextSignal, val message: String) {
  UNMERGED_REVISION(
    POSITIVE_CORRECTION.copy(fileRevision = POSITIVE_CORRECTION.fileRevision.copy(revision = "c".repeat(40))),
    "Signal must use the actual merged before/after revision",
  ),
  PROPOSED_ONLY_PATH(
    POSITIVE_CORRECTION.copy(fileRevision = POSITIVE_CORRECTION.fileRevision.copy(path = "src/ProposedOnly.kt")),
    "Invalid signal 'clusters/rule/signals/${POSITIVE_CORRECTION.id}.json': fileRevision.path: " +
      "must match a changed POSITIVE path: [src/Worker.kt]",
  ),
  UNCHANGED_LINE(
    POSITIVE_CORRECTION.copy(fileRevision = POSITIVE_CORRECTION.fileRevision.copy(expectedRanges = listOf(EdictNextLineRange(2, 2)))),
    "Invalid signal 'clusters/rule/signals/${POSITIVE_CORRECTION.id}.json': fileRevision.expectedRanges[0]: " +
      "must intersect a changed POSITIVE line in 'src/Worker.kt'",
  ),
  PROPOSED_DIFF(
    POSITIVE_CORRECTION.withSource { it.copy(diffPositiveToNegative = it.diffPositiveToNegative.replace("latch.await()", "proposedButNotMerged()")) },
    "Diff excerpt must preserve canonical paths, source lines and line coordinates",
  ),
  INVENTED_TITLE(POSITIVE_CORRECTION.withSource { it.copy(title = "Invented review title") }, "Signal must preserve actual merged review metadata"),
  WEAK(POSITIVE_CORRECTION.copy(strength = EdictNextSignalStrength.WEAK), "Signal must preserve actual merged review metadata"),
  COMMIT_SOURCE(
    POSITIVE_CORRECTION.copy(source = EdictNextSignalSource.FromCommit(MERGED_REVISION, "https://github.com/JetBrains/qodana-cli/commit/$MERGED_REVISION")),
    "Expected regular FromPR signal",
  ),
}

private fun EdictNextSignal.withSource(change: (EdictNextSignalSource.FromPR) -> EdictNextSignalSource.FromPR) =
  copy(source = change(source as EdictNextSignalSource.FromPR))
