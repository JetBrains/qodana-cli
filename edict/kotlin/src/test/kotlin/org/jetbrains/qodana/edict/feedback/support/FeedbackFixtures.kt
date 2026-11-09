// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback.support

import org.jetbrains.qodana.edict.ci.CiProviderId
import org.jetbrains.qodana.edict.ci.ReviewReference
import org.jetbrains.qodana.edict.ci.ReviewRepository
import org.jetbrains.qodana.edict.edictnext.EdictNextLineRange
import org.jetbrains.qodana.edict.edictnext.EdictRepositoryDirectory
import org.jetbrains.qodana.edict.feedback.ActionsPullRequest
import org.jetbrains.qodana.edict.feedback.FeedbackRecord
import org.jetbrains.qodana.edict.feedback.FeedbackRecords
import org.jetbrains.qodana.edict.feedback.fileName
import org.jetbrains.qodana.edict.feedback.fileText
import org.jetbrains.qodana.edict.feedback.GitHubFeedbackApi
import org.jetbrains.qodana.edict.ci.support.Exchange
import org.jetbrains.qodana.edict.ci.support.ProviderRequest
import org.jetbrains.qodana.edict.ci.support.Recording
import org.jetbrains.qodana.edict.ci.support.answer
import org.jetbrains.qodana.edict.ci.CiHttpResponse
import org.jetbrains.qodana.edict.feedback.feedbackComment
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonObject
import org.jetbrains.qodana.edict.promotion.support.ClusterFixture
import org.jetbrains.qodana.edict.promotion.support.INSPECTION_NAME
import org.jetbrains.qodana.edict.promotion.support.INSPECTION_SUMMARY
import org.jetbrains.qodana.edict.promotion.support.promotionRepository
import org.jetbrains.qodana.edict.promotion.support.store
import kotlinx.serialization.json.JsonNull
import java.nio.file.Path
import kotlin.io.path.isRegularFile
import kotlin.io.path.readText
import kotlin.io.path.relativeTo
import kotlin.io.path.walk
import kotlin.io.path.createDirectories
import kotlin.io.path.writeText

/** The repository the fixture pull requests belong to. */
internal val PROJECT = ReviewRepository(CiProviderId.GITHUB, "github.com", "owner", "project")

/** Pull request [number] of [PROJECT]. */
internal fun pullRequest(number: Int): ReviewReference = PROJECT.reference("$number", "https://github.com/owner/project/pull/$number")

/** The pull request the fixture workflows run for. */
internal val PULL_REQUEST = pullRequest(7)

/** A commit of [PULL_REQUEST]. */
internal const val HEAD_REVISION = "d0d0cafed0d0cafed0d0cafed0d0cafed0d0cafe"

/** [PULL_REQUEST] as the workflow event of a pull request of the repository itself has it. */
internal val OWN_PULL_REQUEST = ActionsPullRequest.Own(PULL_REQUEST)

/** Record `f-<index>` of a finding of [clusterId] on [pullRequest]. */
internal fun feedbackRecord(index: Int, clusterId: String = "rule", pullRequest: ReviewReference = PULL_REQUEST): FeedbackRecord =
  FeedbackRecord(
    id = "f-%02d".format(index),
    clusterId = clusterId,
    pullRequest = pullRequest,
    revision = "c0ffee00c0ffee00c0ffee00c0ffee00c0ffee00",
    path = "src/Main.kt",
    lines = EdictNextLineRange(3, 3),
    commentLine = 3,
    inspectionName = INSPECTION_NAME,
    inspectionDescription = INSPECTION_SUMMARY,
    message = "Blocking call on the worker thread",
    createdAt = "2026-11-05T10:00:00Z",
  )

/** Stores [records] as awaiting an answer. */
internal fun EdictRepositoryDirectory.storeActive(vararg records: FeedbackRecord) {
  records.forEach(FeedbackRecords(this)::addActive)
}

/** Stores [records] as having left the active ones in October 2026, as the routine moves them. */
internal fun EdictRepositoryDirectory.storeHistory(vararg records: FeedbackRecord) {
  val month = feedbackHistoryDirectory.resolve("2026-10").createDirectories()
  records.forEach { month.resolve(it.fileName).writeText(it.fileText()) }
}

/** The files of a state holding [cluster], built in [directory], contents by their path in the state. */
internal fun stateFiles(directory: Path, cluster: ClusterFixture): Map<String, String> {
  val state = promotionRepository(directory).apply { store(cluster) }.paths.root
  return state.walk().filter(Path::isRegularFile).associate { it.relativeTo(state).toString() to it.readText() }
}

/** The pull request endpoint of [PULL_REQUEST]. */
private val PULL_REQUEST_ENDPOINT = "/repos/${PROJECT.owner}/${PROJECT.repository}/pulls/${PULL_REQUEST.id}"

/** The first page of the files of [PULL_REQUEST]. */
internal val PULL_REQUEST_FILES = ProviderRequest("GET", "$PULL_REQUEST_ENDPOINT/files", mapOf("per_page" to "100", "page" to "1"))

/** The read of [PULL_REQUEST] itself, which says its state. */
internal val PULL_REQUEST_READ = ProviderRequest("GET", PULL_REQUEST_ENDPOINT)

/** The first page of the review comments of [PULL_REQUEST]. */
internal val PULL_REQUEST_COMMENTS = ProviderRequest("GET", "$PULL_REQUEST_ENDPOINT/comments", mapOf("per_page" to "100", "page" to "1"))

/** The web page of the review comment [id] of [PULL_REQUEST]. */
internal fun commentUrl(id: Int): String = "${PULL_REQUEST.url}#discussion_r$id"

/** The review comment [id] of Edict on [PULL_REQUEST] for [record], with [thumbsUp] 👍 and [thumbsDown] 👎. */
internal fun edictComment(id: Int, record: FeedbackRecord, thumbsUp: Int = 0, thumbsDown: Int = 0): String = buildJsonObject {
  put("id", id)
  put("body", feedbackComment(record))
  put("html_url", commentUrl(id))
  putJsonObject("user") { put("login", "github-actions[bot]") }
  putJsonObject("reactions") {
    put("total_count", thumbsUp + thumbsDown)
    put("+1", thumbsUp)
    put("-1", thumbsDown)
  }
}.toString()

/** The reply [id] of the user [login] to the review comment [to] of [PULL_REQUEST]. */
internal fun reply(id: Int, to: Int, login: String, body: String): String = buildJsonObject {
  put("id", id)
  put("in_reply_to_id", to)
  put("body", body)
  put("html_url", commentUrl(id))
  putJsonObject("user") { put("login", login) }
}.toString()

/** [PULL_REQUEST] as GitHub reads it in [state], `open` or `closed`. */
internal fun pullRequestIn(state: String): CiHttpResponse = answer(200, """{"number": 7, "state": "$state"}""")

/** The answer that lists [comments] as the review comments of [PULL_REQUEST]. */
internal fun reviewComments(vararg comments: String): CiHttpResponse = answer(200, comments.joinToString(",\n", "[", "]"))

/** A call of the feedback client that sends exactly [exchanges], answered as a script has them: no real pull request yet. */
internal fun feedbackScripted(vararg exchanges: Exchange): Recording<GitHubFeedbackApi> =
  Recording(exchanges.toList(), JsonNull, ::GitHubFeedbackApi, scripted = true)
