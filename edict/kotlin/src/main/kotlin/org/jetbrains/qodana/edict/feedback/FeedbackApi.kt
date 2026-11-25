// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

import org.jetbrains.qodana.edict.ci.CiProviderId
import org.jetbrains.qodana.edict.ci.ReviewReference
import org.jetbrains.qodana.edict.ci.ReviewRepository
import org.jetbrains.qodana.edict.ci.github.gitHubTransport
import java.nio.file.Path
import java.time.Instant

/**
 * Feedback on the reviews of a code host: which review a CI run analyses, which findings it may show, how a reserved finding
 * reaches the review's people, and what becomes of the records once their answers can be read.
 */
internal interface FeedbackApi {
    /** The review that the CI run of this process analyses; none, after a log saying why, when the run gets no feedback. */
    fun analysedReview(): ReviewReference?

    /**
     * The [findings] that [review] may show, each with the line to show it on: those in what it changes, and none that the
     * state, which [records] reads, keeps from being shown again at [now].
     */
    fun candidates(review: ReviewReference, findings: List<InternalFinding>, records: FeedbackRecords, now: Instant): List<FeedbackCandidate>

    /** Shows the reserved [record] to the people of its review; a refusal only warns, and the record stays, which collecting later retires. */
    fun show(record: FeedbackRecord)

    /** What becomes of the active [records] once their answers are read at [now]. */
    fun outcomes(records: List<FeedbackRecord>, now: Instant): List<RecordOutcome>
}

/** What becomes of active records once their review is read. */
internal sealed interface RecordOutcome {
    /** Its finding got an answer that gives a label. */
    data class Answered(val answer: FeedbackAnswer) : RecordOutcome

    /** It has no answer yet, on an open review: it stays active. */
    data object Waiting : RecordOutcome

    /** It gets no more answers here, on a closed or no longer updated review: it moves to history. */
    data class Retired(val record: FeedbackRecord) : RecordOutcome

    /** The answers on [review] could not be read, failing with [failure]: its records stay active. */
    data class Unread(val review: ReviewReference, val failure: Throwable) : RecordOutcome
}

/**
 * The feedback of the CI run in [project]: of a Space SafePush when [ci], edict.ci, is on Space, of a GitHub Actions run when
 * it is on GitHub, and of the CI the environment shows when it is not set.
 */
internal fun ciRunFeedbackApi(ci: ReviewRepository?, project: Path): FeedbackApi = when (ci?.provider) {
    CiProviderId.SPACE -> spaceFeedbackApi(ci, project)
    CiProviderId.GITHUB -> actionsFeedbackApi()
    null -> feedbackApiFromEnvironment()
}

/** The feedback of the CI run the environment shows, which only GitHub Actions does: Space needs edict.ci for its repository. */
private fun feedbackApiFromEnvironment(): FeedbackApi = when {
    runsInGitHubActions() -> actionsFeedbackApi()
    else -> error("edict.ci is not set in qodana.yaml, and the environment shows no CI run that Edict feedback knows")
}

/** The feedback of the Edict server for [repository], edict.ci, checked out in [project]. */
internal fun serverFeedbackApi(repository: ReviewRepository, project: Path): FeedbackApi = when (repository.provider) {
    // Without a token the server cannot read answers; it fails to start rather than when the routine collects them.
    CiProviderId.GITHUB -> GitHubFeedbackApi(lazyOf(gitHubTransport(repository)))
    CiProviderId.SPACE -> spaceFeedbackApi(repository, project)
}
