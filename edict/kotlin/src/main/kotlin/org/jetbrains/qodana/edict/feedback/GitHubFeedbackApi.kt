// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

import io.github.oshai.kotlinlogging.KotlinLogging
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import org.jetbrains.qodana.edict.ci.CiHttpTransport
import org.jetbrains.qodana.edict.ci.CiProviderId
import org.jetbrains.qodana.edict.ci.DefaultCiHttpTransport
import org.jetbrains.qodana.edict.ci.ReviewReference
import org.jetbrains.qodana.edict.ci.github.expectObject
import org.jetbrains.qodana.edict.ci.github.gitHubPages
import org.jetbrains.qodana.edict.ci.github.gitHubToken
import org.jetbrains.qodana.edict.ci.urlPart
import org.jetbrains.qodana.edict.ci.feedbackRecordId
import org.jetbrains.qodana.edict.common.number
import org.jetbrains.qodana.edict.common.obj
import org.jetbrains.qodana.edict.common.text
import org.jetbrains.qodana.edict.core.EdictReply
import org.jetbrains.qodana.edict.core.EdictSignalLabel
import org.jetbrains.qodana.edict.signals.UnifiedDiff
import java.time.DateTimeException
import java.time.Duration
import java.time.Instant

private val logger = KotlinLogging.logger {}

/** How long after its pull request was last updated a record still waits for an answer. */
internal val FEEDBACK_ANSWER_WINDOW: Duration = Duration.ofDays(7)

/** What GitHub did with a review comment. */
internal sealed interface PostedComment {
    /** GitHub shows the comment at [url]. */
    data class Posted(val url: String) : PostedComment

    /**
     * GitHub refused the comment, saying [message]: with [status] 403 the token may not comment or GitHub limits its rate,
     * with 422 the line is no longer in the diff, with 429 GitHub limits its rate.
     */
    data class Refused(val status: Int, val message: String) : PostedComment
}

/** A pull request that is [open] or closed, last updated at [updatedAt]. */
internal data class PullRequestState(val open: Boolean, val updatedAt: Instant)

/** A review comment of Edict, with the answers it got. */
internal data class FeedbackThread(
    /** The web page of the comment. */
    val url: String,
    /** The label its thumbs give: POSITIVE for 👍 only, NEGATIVE for 👎 only, none for both or neither. */
    val label: EdictSignalLabel?,
    val replies: List<EdictReply>,
)

/** Whether the process runs in GitHub Actions, which sets `GITHUB_ACTIONS` to `true` in every run. */
internal fun runsInGitHubActions(): Boolean = System.getenv("GITHUB_ACTIONS") == "true"

/** The feedback of the GitHub Actions run the process is in. */
internal fun actionsFeedbackApi(): GitHubFeedbackApi = GitHubFeedbackApi(lazy(::actionsTransport))

/** The API of GitHub for the workflow run, at the URL and with the token GitHub Actions gives it. */
internal fun actionsTransport(): CiHttpTransport {
    val apiUrl = System.getenv("GITHUB_API_URL")?.takeIf(String::isNotBlank)
        ?: error("GITHUB_API_URL is not set; Edict feedback runs in GitHub Actions")
    val token = gitHubToken().getOrElse { throw IllegalStateException("Edict feedback requires a token to comment: ${it.message}", it) }
    return DefaultCiHttpTransport(CiProviderId.GITHUB, apiUrl, token)
}

/**
 * The feedback on GitHub pull requests through [http], made once first used, as `prepare` runs without a token: review comments
 * on the lines a pull request of the repository itself adds, answered by their thumbs and replies.
 */
internal class GitHubFeedbackApi(http: Lazy<CiHttpTransport>) : FeedbackApi {
    private val http by http

    /** The lines each pull request adds, which stay the same while the run selects its findings again. */
    private val addedLinesByPullRequest = HashMap<ReviewReference, Map<String, Set<Int>>>()

    /** The pull request of the workflow run, which gets feedback only when it is between branches of the repository itself. */
    override fun analysedReview(): ReviewReference? = actionsPullRequest(actionsEventPath()).feedbackPullRequest()

    override fun candidates(
        review: ReviewReference,
        findings: List<InternalFinding>,
        records: FeedbackRecords,
        now: Instant,
    ): List<FeedbackCandidate> = onAddedLines(findings, addedLinesByPullRequest.getOrPut(review) { addedLines(review) })

    override fun show(record: FeedbackRecord) {
        when (val comment = comment(record.pullRequest, record.revision, record.path, record.commentLine, feedbackComment(record))) {
            is PostedComment.Posted -> logger.info { "Posted Edict feedback ${record.id} of ${record.clusterId} at ${comment.url}" }
            is PostedComment.Refused -> logger.warn {
                "$FEEDBACK_UNAVAILABLE: GitHub refused feedback ${record.id} with HTTP ${comment.status}: ${comment.message}"
            }
        }
    }

    /**
     * The answers of the records on each of their pull requests updated in the [FEEDBACK_ANSWER_WINDOW] before [now]; the
     * other records of a closed one, and every record of one updated earlier, retire.
     */
    override fun outcomes(records: List<FeedbackRecord>, now: Instant): List<RecordOutcome> =
        records.groupBy(FeedbackRecord::pullRequest).flatMap { (pullRequest, recordsOfPullRequest) ->
            pullRequestOutcomes(pullRequest, recordsOfPullRequest, now.minus(FEEDBACK_ANSWER_WINDOW))
        }

    /** What becomes of the [recordsOfPullRequest] on [pullRequest], whose answers are read when it was updated at [since] or later. */
    private fun pullRequestOutcomes(pullRequest: ReviewReference, recordsOfPullRequest: List<FeedbackRecord>, since: Instant): List<RecordOutcome> {
        val state = state(pullRequest).getOrElse { return listOf(RecordOutcome.Unread(pullRequest, it)) }
        if (state.updatedAt < since) return recordsOfPullRequest.map(RecordOutcome::Retired)
        val threads = threads(pullRequest).getOrElse { return listOf(RecordOutcome.Unread(pullRequest, it)) }
        return recordsOfPullRequest.map { record -> outcome(record, threads[record.id], state.open) }
    }

    /**
     * The lines [pullRequest] adds, by repository-relative path. A file whose patch GitHub leaves out, a binary or a very
     * large one, adds none, and GitHub lists at most the first 3000 files.
     */
    fun addedLines(pullRequest: ReviewReference): Map<String, Set<Int>> =
        http.gitHubPages("${pullRequestEndpoint(pullRequest)}/files", emptyMap())
            .filter { it.text("patch").isNotEmpty() }
            .associate { file -> file.text("filename") to UnifiedDiff.addedLines(file.text("patch")).toSet() }

    /** Posts [body] on the line [line] of [path] as [pullRequest] has it at the full commit SHA [revision]. */
    fun comment(pullRequest: ReviewReference, revision: String, path: String, line: Int, body: String): PostedComment {
        val request = buildJsonObject {
            put("body", body)
            put("commit_id", revision)
            put("path", path)
            put("line", line)
            put("side", "RIGHT")
        }
        val response = http.send("POST", "${pullRequestEndpoint(pullRequest)}/comments", body = request)
        val answer = response.body as? JsonObject ?: JsonObject(emptyMap())
        return when (response.status) {
            201 -> PostedComment.Posted(answer.text("html_url"))
            403, 422, 429 -> PostedComment.Refused(response.status, answer.text("message"))
            else -> error("GitHub answered a review comment on ${pullRequest.url} with HTTP ${response.status}: ${answer.text("message")}")
        }
    }

    /** Whether [pullRequest] is open, and when it was last updated. */
    fun state(pullRequest: ReviewReference): Result<PullRequestState> = gitHubRead {
        val endpoint = pullRequestEndpoint(pullRequest)
        val read = expectObject(http.send("GET", endpoint), "GitHub read of $endpoint", 200)
        val state = read.text("state")
        // GitHub documents only these two states; another one must not count as closed.
        require(state == "open" || state == "closed") { "GitHub has ${pullRequest.url} in the unknown state '$state'" }
        PullRequestState(state == "open", Instant.parse(read.text("updated_at")))
    }

    /** The review comments of Edict on [pullRequest] with their answers, by the record id their marker names. */
    fun threads(pullRequest: ReviewReference): Result<Map<String, FeedbackThread>> = gitHubRead {
        // Every reply names the comment that starts its thread.
        val (replies, comments) = http.gitHubPages("${pullRequestEndpoint(pullRequest)}/comments", emptyMap())
            .partition { it.number("in_reply_to_id") != 0L }
        val repliesByComment = replies.groupBy { it.number("in_reply_to_id") }
        comments.mapNotNull { comment ->
            feedbackRecordId(comment.text("body"))?.let { it to feedbackThread(comment, repliesByComment[comment.number("id")].orEmpty()) }
        }.toMap()
    }

    private fun pullRequestEndpoint(pullRequest: ReviewReference): String =
        "/repos/${urlPart(pullRequest.owner)}/${urlPart(pullRequest.repository)}/pulls/${urlPart(pullRequest.id)}"
}

private fun feedbackThread(comment: JsonObject, replies: List<JsonObject>): FeedbackThread {
    val reactions = comment.obj("reactions")
    val thumbsUp = reactions.number("+1")
    val thumbsDown = reactions.number("-1")
    val label = when {
        thumbsUp > 0 && thumbsDown == 0L -> EdictSignalLabel.POSITIVE
        thumbsDown > 0 && thumbsUp == 0L -> EdictSignalLabel.NEGATIVE
        else -> null
    }
    return FeedbackThread(comment.text("html_url"), label, replies.map { EdictReply(it.obj("user").text("login"), it.text("body")) })
}

private fun outcome(record: FeedbackRecord, thread: FeedbackThread?, open: Boolean): RecordOutcome {
    val answer = thread?.let { answered -> answered.label?.let { FeedbackAnswer(record, it, answered.url, answered.replies) } }
    return when {
        answer != null -> RecordOutcome.Answered(answer)
        open -> RecordOutcome.Waiting
        else -> RecordOutcome.Retired(record)
    }
}

/**
 * What [read] returned from GitHub, or how it failed: GitHub answered otherwise than a read expects, with a status, a body or
 * a date it doesn't, or did not answer at all. The failure is logged with its cause.
 */
private fun <T> gitHubRead(read: () -> T): Result<T> {
    val failure = try {
        return Result.success(read())
    }
    catch (e: IllegalArgumentException) {
        e
    }
    catch (e: IllegalStateException) {
        e
    }
    catch (e: DateTimeException) {
        e
    }
    logger.warn(failure) { "GitHub could not give the Edict feedback answers: ${failure.message}" }
    return Result.failure(failure)
}
