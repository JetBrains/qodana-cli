// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import org.jetbrains.qodana.edict.ci.CiHttpTransport
import org.jetbrains.qodana.edict.ci.CiProviderId
import org.jetbrains.qodana.edict.ci.DefaultCiHttpTransport
import org.jetbrains.qodana.edict.ci.ReviewReference
import org.jetbrains.qodana.edict.ci.ReviewRepository
import org.jetbrains.qodana.edict.ci.github.expectObject
import org.jetbrains.qodana.edict.ci.github.gitHubPages
import org.jetbrains.qodana.edict.ci.github.gitHubToken
import org.jetbrains.qodana.edict.ci.github.gitHubTransport
import org.jetbrains.qodana.edict.ci.urlPart
import org.jetbrains.qodana.edict.common.number
import org.jetbrains.qodana.edict.common.obj
import org.jetbrains.qodana.edict.common.text
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalLabel
import org.jetbrains.qodana.edict.signals.UnifiedDiff

/** What GitHub did with a review comment. */
internal sealed interface PostedComment {
    /** GitHub shows the comment at [url]. */
    data class Posted(val url: String) : PostedComment

    /**
     * GitHub refused the comment, saying [message]: with [status] 403 the token may not comment, with 422 the line is no
     * longer in the diff.
     */
    data class Refused(val status: Int, val message: String) : PostedComment
}

/** A review comment of Edict, with the answers it got. */
internal data class FeedbackThread(
    /** The web page of the comment. */
    val url: String,
    /** The label its thumbs give: POSITIVE for 👍 only, NEGATIVE for 👎 only, none for both or neither. */
    val label: EdictNextSignalLabel?,
    val replies: List<FeedbackReply>,
)

/** A reply to a review comment of Edict: what the user with the login [author] wrote. */
@Serializable
internal data class FeedbackReply(val author: String, val body: String)

/** Whether a pull request is open, and the review comments of Edict on it with their answers. */
internal data class PullRequestThreads(
    val open: Boolean,
    /** The review comments of Edict, by the record id their marker names. */
    val byRecordId: Map<String, FeedbackThread>,
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
        http.gitHubPages("${pullRequestEndpoint(pullRequest)}/files")
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
            403, 422 -> PostedComment.Refused(response.status, answer.text("message"))
            else -> error("GitHub answered a review comment on ${pullRequest.url} with HTTP ${response.status}: ${answer.text("message")}")
        }
    }

    /** Whether [pullRequest] is open, and the review comments of Edict on it with their answers. */
    fun threads(pullRequest: ReviewReference): PullRequestThreads {
        val endpoint = pullRequestEndpoint(pullRequest)
        val state = expectObject(http.send("GET", endpoint), "GitHub pull-request read", 200).text("state")
        require(state == "open" || state == "closed") { "GitHub read ${pullRequest.url} in the unknown state '$state'" }
        // Every reply names the comment that starts its thread.
        val (replies, comments) = http.gitHubPages("$endpoint/comments").partition { it.number("in_reply_to_id") != 0L }
        val repliesByComment = replies.groupBy { it.number("in_reply_to_id") }
        val threads = comments.mapNotNull { comment ->
            feedbackRecordId(comment.text("body"))?.let { it to feedbackThread(comment, repliesByComment[comment.number("id")].orEmpty()) }
        }
        return PullRequestThreads(state == "open", threads.toMap())
    }

    private fun pullRequestEndpoint(pullRequest: ReviewReference): String =
        "/repos/${urlPart(pullRequest.owner)}/${urlPart(pullRequest.repository)}/pulls/${urlPart(pullRequest.id)}"
}

private fun feedbackThread(comment: JsonObject, replies: List<JsonObject>): FeedbackThread {
    val reactions = comment.obj("reactions")
    val thumbsUp = reactions.number("+1")
    val thumbsDown = reactions.number("-1")
    val label = when {
        thumbsUp > 0 && thumbsDown == 0L -> EdictNextSignalLabel.POSITIVE
        thumbsDown > 0 && thumbsUp == 0L -> EdictNextSignalLabel.NEGATIVE
        else -> null
    }
    return FeedbackThread(comment.text("html_url"), label, replies.map { FeedbackReply(it.obj("user").text("login"), it.text("body")) })
}
