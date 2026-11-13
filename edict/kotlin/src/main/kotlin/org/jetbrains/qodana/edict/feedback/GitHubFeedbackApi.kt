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
import org.jetbrains.qodana.edict.ci.ReviewRepository
import org.jetbrains.qodana.edict.ci.github.gitHubPages
import org.jetbrains.qodana.edict.ci.github.gitHubToken
import org.jetbrains.qodana.edict.ci.github.gitHubTransport
import org.jetbrains.qodana.edict.ci.urlPart
import org.jetbrains.qodana.edict.ci.feedbackRecordId
import org.jetbrains.qodana.edict.common.number
import org.jetbrains.qodana.edict.common.obj
import org.jetbrains.qodana.edict.common.text
import org.jetbrains.qodana.edict.core.EdictReply
import org.jetbrains.qodana.edict.core.EdictSignalLabel
import org.jetbrains.qodana.edict.signals.UnifiedDiff
import java.time.DateTimeException
import java.time.Instant

private val logger = KotlinLogging.logger {}

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

/** A review comment of Edict, with the answers it got. */
internal data class FeedbackThread(
    /** The web page of the comment. */
    val url: String,
    /** The label its thumbs give: POSITIVE for 👍 only, NEGATIVE for 👎 only, none for both or neither. */
    val label: EdictSignalLabel?,
    val replies: List<EdictReply>,
)

/** The GitHub client of the workflow run, with the API URL and the token GitHub Actions gives it. */
internal fun actionsFeedbackApi(): GitHubFeedbackApi {
    val apiUrl = System.getenv("GITHUB_API_URL")?.takeIf(String::isNotBlank)
        ?: error("GITHUB_API_URL is not set; Edict feedback runs in GitHub Actions")
    val token = gitHubToken().getOrElse { throw IllegalStateException("Edict feedback requires a token to comment: ${it.message}", it) }
    return GitHubFeedbackApi(DefaultCiHttpTransport(CiProviderId.GITHUB, apiUrl, token))
}

/** The GitHub client of the Edict server for [repository]; none on a provider other than GitHub. */
internal fun serverFeedbackApi(repository: ReviewRepository): GitHubFeedbackApi? = when (repository.provider) {
    CiProviderId.GITHUB -> GitHubFeedbackApi(gitHubTransport(repository))
    CiProviderId.SPACE -> null
}

/** Reads what pull requests change and comments on their lines, on GitHub only, where Edict feedback runs. */
internal class GitHubFeedbackApi(private val http: CiHttpTransport) {
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

    /**
     * The pull requests of [repository] updated at [since] or later, by number, true for an open one: the [limit] most
     * recently updated at most. Pages are read only as far as the pull requests are recent enough.
     */
    fun recentPullRequests(repository: ReviewRepository, since: Instant, limit: Int): Result<Map<String, Boolean>> = gitHubRead {
        val endpoint = "/repos/${urlPart(repository.owner)}/${urlPart(repository.repository)}/pulls"
        http.gitHubPages(endpoint, mapOf("state" to "all", "sort" to "updated", "direction" to "desc"))
            .takeWhile { Instant.parse(it.text("updated_at")) >= since }
            .take(limit)
            .associate { pullRequest ->
                val state = pullRequest.text("state")
                // GitHub documents only these two states; another one must not count as closed.
                require(state == "open" || state == "closed") { "GitHub listed ${pullRequest.text("html_url")} in the unknown state '$state'" }
                pullRequest.number("number").toString() to (state == "open")
            }
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
