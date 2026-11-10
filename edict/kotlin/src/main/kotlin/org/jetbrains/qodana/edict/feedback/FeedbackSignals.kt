// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

import kotlinx.serialization.Serializable
import org.jetbrains.qodana.edict.ci.ReviewReference
import org.jetbrains.qodana.edict.edictnext.EdictNextFileRevision
import org.jetbrains.qodana.edict.edictnext.EdictNextReply
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState.SignalPublication
import org.jetbrains.qodana.edict.edictnext.EdictNextSignal
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalLabel
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalProvenance
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalSource
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalStrength
import org.jetbrains.qodana.edict.edictnext.stableSignalId
import java.time.Clock
import java.time.Duration
import java.util.concurrent.ConcurrentHashMap

/** How long after its pull request was last updated a record still waits for an answer. */
internal val FEEDBACK_ANSWER_WINDOW: Duration = Duration.ofDays(7)

/** Pull requests of a repository whose records wait for answers at most: the most recently updated ones. */
internal const val MAX_ANSWERED_PULL_REQUESTS = 200

/** An active [record] whose review comment at [url] got the thumbs that give [label], and the [replies] it got. */
@Serializable
internal data class FeedbackAnswer(
    val record: FeedbackRecord,
    val label: EdictNextSignalLabel,
    val url: String,
    val replies: List<EdictNextReply>,
)

/** A pull request at [url] whose answers GitHub could not give, and the [failure] it gave instead. */
@Serializable
internal data class UnreadPullRequest(val url: String, val failure: String)

/** The [answers] a collection found, and the pull requests it could not read, whose records stay active. */
@Serializable
internal data class FeedbackCollection(val answers: List<FeedbackAnswer>, val unread: List<UnreadPullRequest>)

/**
 * The Signals of the answers to the review comments of the active feedback [records], as GitHub has them through [api] at the
 * time [clock] tells, in the inbox of [store].
 */
// TODO: think about misrouting of these signals
internal class FeedbackSignals(
    private val records: FeedbackRecords,
    private val api: GitHubFeedbackApi,
    private val store: EdictNextRepositoryState,
    private val clock: Clock,
) {
    /** The answers [collect] returned last, by record id: the only ones [publish] takes. */
    private val collected = ConcurrentHashMap<String, FeedbackAnswer>()

    /**
     * Returns the active records whose comments got thumbs that give a label. Moves to history the other records of closed pull
     * requests, and every record of a pull request outside the [MAX_ANSWERED_PULL_REQUESTS] last updated in
     * [FEEDBACK_ANSWER_WINDOW], which wait for no more answers; the rest stay active, as do the records of a pull request
     * GitHub could not read.
     */
    fun collect(): FeedbackCollection {
        val since = clock.instant().minus(FEEDBACK_ANSWER_WINDOW)
        val outcomes = records.active().groupBy { it.pullRequest.reviewRepository }.flatMap { (repository, recordsOfRepository) ->
            val recordsByPullRequest = recordsOfRepository.groupBy(FeedbackRecord::pullRequest)
            api.recentPullRequests(repository, since, MAX_ANSWERED_PULL_REQUESTS).fold(
                onSuccess = { recent ->
                    recordsByPullRequest.flatMap { (pullRequest, recordsOfPullRequest) ->
                        recent[pullRequest.id]?.let { open -> pullRequestOutcomes(pullRequest, open, recordsOfPullRequest) }
                            ?: recordsOfPullRequest.map(RecordOutcome::Retired)
                    }
                },
                onFailure = { failure -> recordsByPullRequest.keys.map { RecordOutcome.Unread(it, failure) } },
            )
        }
        outcomes.filterIsInstance<RecordOutcome.Retired>().forEach { records.moveToHistory(it.record) }
        val answers = outcomes.filterIsInstance<RecordOutcome.Answered>().map(RecordOutcome.Answered::answer)
        collected.clear()
        collected.putAll(answers.associateBy { it.record.id })
        val unread = outcomes.filterIsInstance<RecordOutcome.Unread>().map {
            UnreadPullRequest(it.pullRequest.url, it.failure.message.orEmpty())
        }
        return FeedbackCollection(answers, unread)
    }

    /** What becomes of the [recordsOfPullRequest] on [pullRequest], which is [open] or closed. */
    private fun pullRequestOutcomes(
        pullRequest: ReviewReference,
        open: Boolean,
        recordsOfPullRequest: List<FeedbackRecord>,
    ): List<RecordOutcome> =
        api.threads(pullRequest).fold(
            onSuccess = { threads -> recordsOfPullRequest.map { record -> outcome(record, threads[record.id], open) } },
            onFailure = { failure -> listOf(RecordOutcome.Unread(pullRequest, failure)) },
        )

    /**
     * Publishes with [token] the Signal of the answer to the record [recordId] that [collect] returned, and moves the record
     * to history. See [feedbackSignal] for [description].
     */
    fun publish(token: String, recordId: String, description: String): SignalPublication {
        val answer = requireNotNull(collected[recordId]) { "No collected answer to feedback record $recordId" }
        return store.publishSignal(token, feedbackSignal(answer, description)).also {
            records.moveToHistory(answer.record)
            collected.remove(recordId)
        }
    }
}

/**
 * The STRONG Signal of [answer]: the record's lines at its revision, labelled by the thumbs, with the replies as their authors
 * wrote them. The agent supplies only what the record lacks: the one-line [description].
 */
private fun feedbackSignal(answer: FeedbackAnswer, description: String): EdictNextSignal {
    val record = answer.record
    val key = "edict-feedback:${record.id}"
    return EdictNextSignal(
        id = stableSignalId(key),
        idempotencyKey = key,
        fileRevision = EdictNextFileRevision(record.path, record.revision, listOf(record.lines)),
        source = EdictNextSignalSource.GitHubEvaluatedFinding(
            inspectionName = record.inspectionName,
            inspectionDescription = record.inspectionDescription,
            problemMessage = record.message,
            url = answer.url,
            replies = answer.replies,
        ),
        label = answer.label,
        description = description,
        strength = EdictNextSignalStrength.STRONG,
        provenance = EdictNextSignalProvenance(record.id),
    )
}

/** What becomes of active records once their pull request is read. */
private sealed interface RecordOutcome {
    /** Its comment got thumbs that give a label. */
    data class Answered(val answer: FeedbackAnswer) : RecordOutcome

    /** It has no answer yet, on an open pull request: it stays active. */
    data object Waiting : RecordOutcome

    /** It gets no more answers, on a closed or no longer updated pull request: it moves to history. */
    data class Retired(val record: FeedbackRecord) : RecordOutcome

    /** GitHub could not tell the answers on [pullRequest], failing with [failure]: its records stay active. */
    data class Unread(val pullRequest: ReviewReference, val failure: Throwable) : RecordOutcome
}

private fun outcome(record: FeedbackRecord, thread: FeedbackThread?, open: Boolean): RecordOutcome {
    val answer = thread?.let { answered -> answered.label?.let { FeedbackAnswer(record, it, answered.url, answered.replies) } }
    return when {
        answer != null -> RecordOutcome.Answered(answer)
        open -> RecordOutcome.Waiting
        else -> RecordOutcome.Retired(record)
    }
}

