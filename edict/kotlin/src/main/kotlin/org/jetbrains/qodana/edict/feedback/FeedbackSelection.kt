// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

import io.github.oshai.kotlinlogging.KotlinLogging
import org.jetbrains.qodana.edict.ci.ReviewReference
import org.jetbrains.qodana.edict.edictnext.EdictNextClusterStatus
import org.jetbrains.qodana.edict.edictnext.EdictNextStoredCluster
import org.jetbrains.qodana.edict.edictnext.EdictRepository
import org.jetbrains.qodana.edict.edictnext.EvaluatedInspection
import org.jetbrains.qodana.edict.edictnext.StoredInspection
import org.jetbrains.qodana.edict.edictnext.storedInspection

private val logger = KotlinLogging.logger {}

/** Review comments a pull request gets at most, over all its runs. */
internal const val MAX_FEEDBACK_PER_PULL_REQUEST = 3

/** Records of one inspection that may await an answer at once. */
internal const val MAX_ACTIVE_FEEDBACK_PER_INSPECTION = 10

/** The precision an inspection's evaluation must exceed for its findings to reach a review. */
internal const val MIN_FEEDBACK_PRECISION = 0.7

/** Inspections one run analyses for feedback at most; each adds to the analysis time. */
internal const val MAX_FEEDBACK_INSPECTIONS = 50

/**
 * The inspections whose findings may become review comments on [pullRequest]: none once it got its
 * [MAX_FEEDBACK_PER_PULL_REQUEST] records, else the generated, not yet promoted ones whose current evaluation exceeds
 * [MIN_FEEDBACK_PRECISION] and that await fewer than [MAX_ACTIVE_FEEDBACK_PER_INSPECTION] answers. The most precise come
 * first, up to [MAX_FEEDBACK_INSPECTIONS].
 */
internal fun selectFeedbackInspections(
    repository: EdictRepository,
    records: FeedbackRecords,
    pullRequest: ReviewReference,
): List<EvaluatedInspection> {
    val active = records.active()
    // Answered records leave active: without the history, a pull request whose comments got answers would get new ones.
    if ((active + records.history()).count { it.pullRequest == pullRequest } >= MAX_FEEDBACK_PER_PULL_REQUEST) return emptyList()
    val activeByCluster = active.groupingBy(FeedbackRecord::clusterId).eachCount()
    return repository.loadClusters()
        .asSequence()
        .filter { it.manifest.status == EdictNextClusterStatus.Generated }
        .filter { activeByCluster.getOrDefault(it.id, 0) < MAX_ACTIVE_FEEDBACK_PER_INSPECTION }
        .mapNotNull { repository.feedbackInspection(it) }
        .filter { it.evaluation.precision > MIN_FEEDBACK_PRECISION }
        .sortedWith(compareByDescending<EvaluatedInspection> { it.evaluation.precision }.thenBy(EvaluatedInspection::clusterId))
        .take(MAX_FEEDBACK_INSPECTIONS)
        .toList()
}

/**
 * The cluster's generated inspection, unless no evaluation of its current bytes and examples ranks it or a promotion proposed
 * these bytes. A cluster without its inspection only loses its feedback, not that of the others.
 */
private fun EdictRepository.feedbackInspection(cluster: EdictNextStoredCluster): EvaluatedInspection? =
    when (val inspection = storedInspection(cluster, loadEvaluation(cluster))) {
        StoredInspection.Missing -> {
            logger.warn { "Edict feedback leaves out ${cluster.id}: its generated inspection is missing" }
            null
        }
        StoredInspection.NotEvaluated -> null
        is EvaluatedInspection -> inspection.takeUnless { cluster.isPromoted(it.digest) }
    }

/**
 * Whether a promotion proposed exactly the inspection bytes of [digest], whatever its state: it awaits its review, was accepted,
 * or was closed and awaits its decision. Only deciding a closed one regenerates the inspection, so the promotions of earlier
 * bytes are decided ones, and the new bytes may get feedback.
 */
private fun EdictNextStoredCluster.isPromoted(digest: String): Boolean = manifest.promotions.any { it.inspectionDigest == digest }
