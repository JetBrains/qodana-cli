// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

import kotlinx.serialization.Serializable
import org.jetbrains.qodana.edict.edictnext.EdictNextFileRevision
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState.SignalPublication
import org.jetbrains.qodana.edict.edictnext.EdictNextSignal
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalLabel
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalProvenance
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalSource
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalStrength
import org.jetbrains.qodana.edict.edictnext.stableSignalId
import java.util.concurrent.ConcurrentHashMap

/** An active [record] whose review comment at [url] got the thumbs that give [label], and the [replies] it got. */
@Serializable
internal data class FeedbackAnswer(
    val record: FeedbackRecord,
    val label: EdictNextSignalLabel,
    val url: String,
    val replies: List<FeedbackReply>,
)

/**
 * The Signals of the answers to the review comments of the active feedback [records], as GitHub has them through [api],
 * in the inbox of [store].
 */
internal class FeedbackSignals(
    private val records: FeedbackRecords,
    private val api: GitHubFeedbackApi,
    private val store: EdictNextRepositoryState,
) {
    /** The answers [collect] returned last, by record id: the only ones [publish] takes. */
    private val collected = ConcurrentHashMap<String, FeedbackAnswer>()

    /**
     * Returns the active records whose comments got thumbs that give a label, and moves to history the other records of
     * closed pull requests, which wait for no more answers; the rest stay active.
     */
    fun collect(): List<FeedbackAnswer> {
        val outcomes = records.active().groupBy(FeedbackRecord::pullRequest).flatMap { (pullRequest, recordsOfPullRequest) ->
            val pullRequestThreads = api.threads(pullRequest)
            recordsOfPullRequest.map(pullRequestThreads::outcome)
        }
        outcomes.filterIsInstance<RecordOutcome.Retired>().forEach { records.moveToHistory(it.record) }
        val answers = outcomes.filterIsInstance<RecordOutcome.Answered>().map(RecordOutcome.Answered::answer)
        collected.clear()
        collected.putAll(answers.associateBy { it.record.id })
        return answers
    }

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
 * The STRONG Signal of [answer]: the record's lines at its revision, labelled by the thumbs, with the replies, as their authors
 * wrote them, as the user's message. The agent supplies only what the record lacks: the one-line [description].
 */
private fun feedbackSignal(answer: FeedbackAnswer, description: String): EdictNextSignal {
    val record = answer.record
    val key = "edict-feedback:${record.id}"
    return EdictNextSignal(
        id = stableSignalId(key),
        idempotencyKey = key,
        fileRevision = EdictNextFileRevision(record.path, record.revision, listOf(record.lines)),
        source = EdictNextSignalSource.SubmittedFeedback(
            inspectionName = record.inspectionName,
            inspectionDescription = record.inspectionDescription,
            problemMessage = record.message,
            message = answer.replies.joinToString("\n\n") { "${it.author}: ${it.body}" }.ifEmpty { null },
            url = answer.url,
        ),
        label = answer.label,
        description = description,
        strength = EdictNextSignalStrength.STRONG,
        provenance = EdictNextSignalProvenance(record.id),
    )
}

/** What becomes of an active record once its pull request is read. */
private sealed interface RecordOutcome {
    /** Its comment got thumbs that give a label. */
    data class Answered(val answer: FeedbackAnswer) : RecordOutcome

    /** It has no answer yet, on an open pull request: it stays active. */
    data object Waiting : RecordOutcome

    /** It has no answer, on a closed pull request, which gets no more: it moves to history. */
    data class Retired(val record: FeedbackRecord) : RecordOutcome
}

private fun PullRequestThreads.outcome(record: FeedbackRecord): RecordOutcome {
    val answer = byRecordId[record.id]?.let { thread -> thread.label?.let { FeedbackAnswer(record, it, thread.url, thread.replies) } }
    return when {
        answer != null -> RecordOutcome.Answered(answer)
        open -> RecordOutcome.Waiting
        else -> RecordOutcome.Retired(record)
    }
}
