// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

import org.jetbrains.qodana.edict.ci.ReviewReference
import org.jetbrains.qodana.edict.common.sha256
import org.jetbrains.qodana.edict.edictnext.EdictRepository
import org.jetbrains.qodana.edict.edictnext.EvaluatedInspection
import java.time.Instant

/** A [finding] that a pull request touches, with [line], the first of its lines the pull request adds, to comment on. */
internal data class FeedbackCandidate(val finding: InternalFinding, val line: Int)

/**
 * The records to reserve for [candidates] on [pullRequest], whose findings are of the commit with the full SHA [revision]: the
 * first candidate of each inspection that [selectFeedbackInspections] picks, the most precise inspection first, as many as the
 * pull request has left.
 */
internal fun reserveFeedback(
    repository: EdictRepository,
    records: FeedbackRecords,
    pullRequest: ReviewReference,
    revision: String,
    candidates: List<FeedbackCandidate>,
    createdAt: Instant,
): List<FeedbackRecord> {
    val selection = selectFeedbackInspections(repository, records, pullRequest)
    val candidatesByCluster = candidates.groupBy { it.finding.clusterId }
    return selection.inspections
        .mapNotNull { inspection ->
            val first = candidatesByCluster[inspection.clusterId]?.minWith(compareBy({ it.finding.path }, FeedbackCandidate::line))
            first?.let { feedbackRecord(inspection, it, pullRequest, revision, createdAt) }
        }
        .take(selection.remaining)
}

/**
 * The candidates of [findings] that have a line the pull request adds, as [addedLines] has them by path: a finding may span
 * a whole declaration of which the pull request changes one line.
 */
internal fun onAddedLines(findings: List<InternalFinding>, addedLines: Map<String, Set<Int>>): List<FeedbackCandidate> =
    findings.mapNotNull { finding ->
        val range = finding.lines.start..finding.lines.end
        addedLines[finding.path].orEmpty().filter { it in range }.minOrNull()?.let { FeedbackCandidate(finding, it) }
    }

/** The record of [candidate] of [inspection] on [pullRequest]; its id is the same whichever run reserves the finding. */
private fun feedbackRecord(
    inspection: EvaluatedInspection,
    candidate: FeedbackCandidate,
    pullRequest: ReviewReference,
    revision: String,
    createdAt: Instant,
): FeedbackRecord {
    val finding = candidate.finding
    val identity = listOf(pullRequest.url, finding.clusterId, finding.path, finding.lines.start.toString()).joinToString("\u0000")
    return FeedbackRecord(
        id = "f-${sha256(identity).take(24)}",
        clusterId = finding.clusterId,
        pullRequest = pullRequest,
        revision = revision,
        path = finding.path,
        lines = finding.lines,
        commentLine = candidate.line,
        inspectionName = inspection.evaluation.inspectionName.ifBlank { inspection.clusterId },
        inspectionDescription = inspection.evaluation.inspectionDescription,
        message = finding.message,
        createdAt = createdAt.toString(),
    )
}
