// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import org.jetbrains.qodana.edict.ci.CiHttpTransport
import org.jetbrains.qodana.edict.ci.CiProviderId
import org.jetbrains.qodana.edict.ci.DefaultCiHttpTransport
import org.jetbrains.qodana.edict.ci.ReviewReference
import org.jetbrains.qodana.edict.ci.github.gitHubPages
import org.jetbrains.qodana.edict.ci.github.gitHubToken
import org.jetbrains.qodana.edict.ci.urlPart
import org.jetbrains.qodana.edict.common.text
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

/** The GitHub client of the workflow run, with the API URL and the token GitHub Actions gives it. */
internal fun actionsFeedbackApi(): GitHubFeedbackApi {
    val apiUrl = System.getenv("GITHUB_API_URL")?.takeIf(String::isNotBlank)
        ?: error("GITHUB_API_URL is not set; Edict feedback runs in GitHub Actions")
    val token = gitHubToken().getOrElse { throw IllegalStateException("Edict feedback requires a token to comment: ${it.message}", it) }
    return GitHubFeedbackApi(DefaultCiHttpTransport(CiProviderId.GITHUB, apiUrl, token))
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

    private fun pullRequestEndpoint(pullRequest: ReviewReference): String =
        "/repos/${urlPart(pullRequest.owner)}/${urlPart(pullRequest.repository)}/pulls/${urlPart(pullRequest.id)}"
}
