// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

import org.jetbrains.qodana.edict.ci.ReviewReference
import org.jetbrains.qodana.edict.common.FeedbackConfiguration
import org.jetbrains.qodana.edict.core.EvaluatedInspection
import kotlin.sequences.sortedWith

/** Inspections one run analyses for feedback at most; each adds to the analysis time. */
internal const val MAX_FEEDBACK_INSPECTIONS = 50

/** The inspections whose findings may become review comments on a pull request, and how many comments it has [remaining]. */
internal data class FeedbackSelection(val inspections: List<FeedbackInspection>, val remaining: Int)

/**
 * What [pullRequest] may still get under [limits]: nothing once it got [FeedbackConfiguration.maxPerPullRequest] records, else
 * the rest of them, from the [inspections] the state chose, in their order, that have no record on it and of whose bytes fewer
 * than [FeedbackConfiguration.maxPerInspection] records await an answer, up to [MAX_FEEDBACK_INSPECTIONS].
 */
internal fun selectFeedbackInspections(
    inspections: List<FeedbackInspection>,
    records: FeedbackRecords,
    pullRequest: ReviewReference,
    limits: FeedbackConfiguration,
): FeedbackSelection {
    val active = records.active()
    // Answered records leave active: without the history, a pull request whose comments got answers would get new ones.
    val commented = (active + records.history()).filter { it.pullRequest == pullRequest }
    if (commented.size >= limits.maxPerPullRequest) return FeedbackSelection(emptyList(), 0)
    val commentedInspections = commented.map(FeedbackRecord::inspectionDigest).toSet()
    val activeByInspection = active.groupingBy(FeedbackRecord::inspectionDigest).eachCount()
    // select inspections so that max 1 comment per inspection in one PR, less than maxPerInspection overall
    // inspections sorted in best precision order by chooseFeedbackInspection
    val selected = inspections
        .filter { it.digest !in commentedInspections }
        .filter { activeByInspection.getOrDefault(it.digest, 0) < limits.maxPerInspection }
        .sortedWith(compareByDescending<FeedbackInspection> { it.precision }.thenBy(FeedbackInspection::clusterId))
        .take(MAX_FEEDBACK_INSPECTIONS)
    return FeedbackSelection(selected, limits.maxPerPullRequest - commented.size)
}
