// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

import kotlinx.serialization.Serializable
import org.jetbrains.qodana.edict.core.EdictFileRevision
import org.jetbrains.qodana.edict.core.EdictReply
import org.jetbrains.qodana.edict.core.EdictRepositoryState
import org.jetbrains.qodana.edict.core.EdictRepositoryState.SignalPublication
import org.jetbrains.qodana.edict.core.EdictSignal
import org.jetbrains.qodana.edict.core.EdictSignalLabel
import org.jetbrains.qodana.edict.core.EdictSignalProvenance
import org.jetbrains.qodana.edict.core.EdictSignalSource
import org.jetbrains.qodana.edict.core.EdictSignalStrength
import org.jetbrains.qodana.edict.core.stableSignalId
import java.time.Clock
import java.util.concurrent.ConcurrentHashMap

/** An active [record] whose review comment at [url] got the thumbs that give [label], and the [replies] it got. */
@Serializable
internal data class FeedbackAnswer(
    val record: FeedbackRecord,
    val label: EdictSignalLabel,
    val url: String,
    val replies: List<EdictReply>,
)

/** A pull request at [url] whose answers GitHub could not give, and the [failure] it gave instead. */
@Serializable
internal data class UnreadPullRequest(val url: String, val failure: String)

/** The [answers] a collection found, and the pull requests it could not read, whose records stay active. */
@Serializable
internal data class FeedbackCollection(val answers: List<FeedbackAnswer>, val unread: List<UnreadPullRequest>)

/**
 * The Signals of the answers to the findings of the active feedback [records], as [api] reads them at the time [clock] tells,
 * in the inbox of [store].
 */
// TODO: think about misrouting of these signals
internal class FeedbackSignals(
    private val records: FeedbackRecords,
    private val api: FeedbackApi,
    private val store: EdictRepositoryState,
    private val clock: Clock,
) {
    /** The answers [collect] returned last, by record id: the only ones [publish] takes. */
    private val collected = ConcurrentHashMap<String, FeedbackAnswer>()

    /**
     * Returns the active records whose findings got answers that give a label, and moves to history those that get no more
     * answers, such as those of closed reviews; the rest stay active, as do the records
     * of a review whose answers could not be read.
     */
    fun collect(): FeedbackCollection {
        val outcomes = api.outcomes(records.active(), clock.instant())
        outcomes.filterIsInstance<RecordOutcome.Retired>().forEach { records.moveToHistory(it.record) }
        val answers = outcomes.filterIsInstance<RecordOutcome.Answered>().map(RecordOutcome.Answered::answer)
        collected.clear()
        collected.putAll(answers.associateBy { it.record.id })
        val unread = outcomes.filterIsInstance<RecordOutcome.Unread>().map {
            UnreadPullRequest(it.review.url, it.failure.message.orEmpty())
        }
        return FeedbackCollection(answers, unread)
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
 * The STRONG Signal of [answer]: the record's lines at its revision, labelled by the thumbs, with the replies as their authors
 * wrote them. The agent supplies only what the record lacks: the one-line [description].
 */
private fun feedbackSignal(answer: FeedbackAnswer, description: String): EdictSignal {
    val record = answer.record
    val key = "edict-feedback:${record.id}"
    return EdictSignal(
        id = stableSignalId(key),
        idempotencyKey = key,
        fileRevision = EdictFileRevision(record.path, record.revision, listOf(record.lines)),
        source = EdictSignalSource.GitHubEvaluatedFinding(
            inspectionName = record.inspectionName,
            inspectionDescription = record.inspectionDescription,
            problemMessage = record.message,
            url = answer.url,
            replies = answer.replies,
            inspectionDigest = record.inspectionDigest,
        ),
        label = answer.label,
        description = description,
        strength = EdictSignalStrength.STRONG,
        provenance = EdictSignalProvenance(record.id),
    )
}
