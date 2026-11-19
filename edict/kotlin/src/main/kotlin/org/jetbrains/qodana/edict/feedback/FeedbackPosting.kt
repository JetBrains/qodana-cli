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
 * Shows [review] the findings of the SARIF report at [sarif] that the inspections of `prepare` reported in what it changes, at
 * the commit the report names as analysed, as many as [limits] allow. First reserves the findings that [api] lets the review
 * show: records them in the state directory of [layout], which `prepare` checked out, and pushes them to [stateBranch]; then
 * shows them through [api]. Without a review, the run gets no feedback. Finally removes the checkout of the state, if it is a
 * worktree of the project, which the next run takes from origin again.
 */
internal fun postFeedback(
    layout: EdictLayout,
    stateBranch: String,
    review: ReviewReference?,
    api: FeedbackApi,
    sarif: Path,
    clock: Clock,
    limits: FeedbackConfiguration,
) {
    val branch = StateBranch(layout, stateBranch)
    try {
        if (review != null) reserveAndShow(layout, branch, review, api, sarif, clock, limits)
    }
    catch (e: Throwable) {
        runCatching { branch.remove() }.exceptionOrNull()?.let(e::addSuppressed)
        throw e
    }
    branch.remove()
}

private fun reserveAndShow(
    layout: EdictLayout,
    branch: StateBranch,
    review: ReviewReference,
    api: FeedbackApi,
    sarif: Path,
    clock: Clock,
    limits: FeedbackConfiguration,
) {
    val (revision, findings) = feedbackReport(sarif)
    if (findings.isEmpty()) {
        logger.info { "No Edict feedback inspection found a problem on ${review.url}" }
        return
    }
    check(validRevision(revision)) { "$sarif names no full commit SHA as the analysed revision: '$revision'" }
    val paths = EdictRepositoryDirectory(layout.stateDirectory)
    reserve(branch.worktree(), paths, review, revision, findings, api, clock, limits).forEach(api::show)
}

/**
 * Reserves the records of the [findings] that [api] lets [review] show on the state as origin has it, selecting again whenever
 * another run pushed first, and returns them.
 */
private fun reserve(
    worktree: StateBranch.Worktree,
    paths: EdictRepositoryDirectory,
    review: ReviewReference,
    revision: String,
    findings: List<InternalFinding>,
    api: FeedbackApi,
    clock: Clock,
    limits: FeedbackConfiguration,
): List<FeedbackRecord> {
    repeat(MAX_RESERVATION_ATTEMPTS) {
        // The records as origin has them now, which other runs pushed during the scan or since the last attempt; drops the
        // reservation of an attempt that lost, which is committed but not pushed.
        val base = worktree.resetToOrigin()
        val records = FeedbackRecords(paths)
        // The records decide which findings the review may show: those another run reserved meanwhile may be kept from it.
        val candidates = api.candidates(review, findings, records, clock.instant())
        if (candidates.isEmpty()) {
            logger.info { "No Edict feedback finding in what ${review.url} changes is left to show" }
            return emptyList()
        }
        val inspections = FeedbackInspections(paths).load()
        val reserved = reserveFeedback(inspections, records, review, revision, candidates, clock.instant(), limits)
        if (reserved.isEmpty()) {
            logger.info { "${review.url} gets no more Edict feedback" }
            return reserved
        }
        reserved.forEach(records::addActive)
        if (worktree.publish("Reserve Edict feedback on ${review.url}", base)) return reserved
        logger.info { "Another run pushed the Edict state meanwhile; selecting the feedback again" }
    }
    error("Could not reserve Edict feedback: other runs kept pushing the state")
}
