// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

import io.github.oshai.kotlinlogging.KotlinLogging
import org.jetbrains.qodana.edict.ci.ReviewReference
import org.jetbrains.qodana.edict.common.EdictLayout
import org.jetbrains.qodana.edict.common.FeedbackConfiguration
import org.jetbrains.qodana.edict.common.StateBranch
import org.jetbrains.qodana.edict.core.EdictRepositoryDirectory
import org.jetbrains.qodana.edict.signals.validRevision
import java.nio.file.Path
import java.time.Clock

private val logger = KotlinLogging.logger {}

/** Runs that may reserve at once before one gives up: each that pushes first makes the others select again. */
private const val MAX_RESERVATION_ATTEMPTS = 5

/**
 * Comments on the workflow's [event] with the findings of the SARIF report at [sarif] that the inspections of `prepare` reported
 * on lines the pull request adds, at the commit the report names as analysed, as many as [limits] allow. First reserves the
 * findings: records them in the
 * state directory of [layout], which `prepare` checked out, and pushes them to [stateBranch]; then posts them through [api]. A
 * comment GitHub refuses leaves its record, which the routine later finds unanswered. Finally removes the checkout of the
 * state, which the next run takes from origin again.
 */
internal fun postFeedback(
    layout: EdictLayout,
    stateBranch: String,
    event: ActionsPullRequest,
    sarif: Path,
    api: GitHubFeedbackApi,
    clock: Clock,
    limits: FeedbackConfiguration,
) {
    val branch = StateBranch(layout, stateBranch)
    try {
        reserveAndComment(layout, branch, event, sarif, api, clock, limits)
    }
    catch (e: Throwable) {
        runCatching { branch.remove() }.exceptionOrNull()?.let(e::addSuppressed)
        throw e
    }
    branch.remove()
}

private fun reserveAndComment(
    layout: EdictLayout,
    branch: StateBranch,
    event: ActionsPullRequest,
    sarif: Path,
    api: GitHubFeedbackApi,
    clock: Clock,
    limits: FeedbackConfiguration,
) {
    val pullRequest = event.feedbackPullRequest() ?: return
    val (revision, findings) = feedbackReport(sarif)
    if (findings.isEmpty()) {
        logger.info { "No Edict feedback inspection found a problem on ${pullRequest.url}" }
        return
    }
    check(validRevision(revision)) { "$sarif names no full commit SHA as the analysed revision: '$revision'" }
    val candidates = onAddedLines(findings, api.addedLines(pullRequest))
    if (candidates.isEmpty()) {
        logger.info { "No Edict feedback finding is on a line ${pullRequest.url} adds" }
        return
    }
    val worktree = branch.worktree()
    val reserved = reserve(worktree, EdictRepositoryDirectory(layout.stateDirectory), pullRequest, revision, candidates, clock, limits)
    reserved.forEach { record ->
        val comment = api.comment(pullRequest, record.revision, record.path, record.commentLine, feedbackComment(record))
        when (comment) {
            is PostedComment.Posted -> logger.info { "Posted Edict feedback ${record.id} of ${record.clusterId} at ${comment.url}" }
            is PostedComment.Refused -> logger.warn {
                "$FEEDBACK_UNAVAILABLE: GitHub refused feedback ${record.id} " +
                    "with HTTP ${comment.status}: ${comment.message}"
            }
        }
    }
}

/**
 * Reserves the records of [candidates] on the state as origin has it, selecting again whenever another run pushed first,
 * and returns them.
 */
private fun reserve(
    worktree: StateBranch.Worktree,
    paths: EdictRepositoryDirectory,
    pullRequest: ReviewReference,
    revision: String,
    candidates: List<FeedbackCandidate>,
    clock: Clock,
    limits: FeedbackConfiguration,
): List<FeedbackRecord> {
    repeat(MAX_RESERVATION_ATTEMPTS) {
        // Drops the reservation of an attempt that lost, which is committed but not pushed.
        val base = worktree.resetToOrigin()
        val records = FeedbackRecords(paths)
        val inspections = FeedbackInspections(paths).load()
        val reserved = reserveFeedback(inspections, records, pullRequest, revision, candidates, clock.instant(), limits)
        if (reserved.isEmpty()) {
            logger.info { "${pullRequest.url} gets no more Edict feedback" }
            return reserved
        }
        reserved.forEach(records::addActive)
        if (worktree.publish("Reserve Edict feedback on ${pullRequest.url}", base)) return reserved
        logger.info { "Another run pushed the Edict state meanwhile; selecting the feedback again" }
    }
    error("Could not reserve Edict feedback: other runs kept pushing the state")
}
