// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback.support

import org.jetbrains.qodana.edict.ci.CiProviderId
import org.jetbrains.qodana.edict.ci.ReviewReference
import org.jetbrains.qodana.edict.ci.ReviewRepository
import org.jetbrains.qodana.edict.core.EdictLineRange
import org.jetbrains.qodana.edict.core.EdictRepositoryDirectory
import org.jetbrains.qodana.edict.feedback.ActionsPullRequest
import org.jetbrains.qodana.edict.common.DEFAULT_TARGET_PRECISION
import org.jetbrains.qodana.edict.feedback.FeedbackInspections
import org.jetbrains.qodana.edict.feedback.chooseFeedbackInspections
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
import org.jetbrains.qodana.edict.promotion.support.INSPECTION_DIGEST
import org.jetbrains.qodana.edict.promotion.support.INSPECTION_NAME
import org.jetbrains.qodana.edict.promotion.support.INSPECTION_SUMMARY
import org.jetbrains.qodana.edict.promotion.support.promotionRepository
import org.jetbrains.qodana.edict.promotion.support.store
import kotlinx.serialization.json.JsonNull
import java.nio.file.Path
import java.time.Instant
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

/** Record `f-<index>` of a finding of [clusterId], of the inspection bytes of [inspectionDigest], on [pullRequest]. */
internal fun feedbackRecord(
  index: Int,
  clusterId: String = "rule",
  pullRequest: ReviewReference = PULL_REQUEST,
  inspectionDigest: String = INSPECTION_DIGEST,
): FeedbackRecord =
  FeedbackRecord(
    id = "f-%02d".format(index),
    clusterId = clusterId,
    inspectionDigest = inspectionDigest,
    pullRequest = pullRequest,
    revision = "c0ffee00c0ffee00c0ffee00c0ffee00c0ffee00",
    path = "src/Main.kt",
    lines = EdictLineRange(3, 3),
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

/**
 * The files of a state holding [cluster], built in [directory], contents by their path in the state, with the feedback
 * inspections a push of it chooses.
 */
internal fun stateFiles(directory: Path, cluster: ClusterFixture): Map<String, String> {
  val repository = promotionRepository(directory).apply { store(cluster) }
  FeedbackInspections(repository.paths).save(chooseFeedbackInspections(repository, DEFAULT_TARGET_PRECISION))
  val state = repository.paths.root
  return state.walk().filter(Path::isRegularFile).associate { it.relativeTo(state).toString() to it.readText() }
}

/** The endpoint of the pull requests of [PROJECT]. */
private val PULLS_ENDPOINT = "/repos/${PROJECT.owner}/${PROJECT.repository}/pulls"

/** The pull request endpoint of [PULL_REQUEST]. */
private val PULL_REQUEST_ENDPOINT = "$PULLS_ENDPOINT/${PULL_REQUEST.id}"

/** The first page of the files of [PULL_REQUEST]. */
internal val PULL_REQUEST_FILES = ProviderRequest("GET", "$PULL_REQUEST_ENDPOINT/files", mapOf("per_page" to "100", "page" to "1"))

/** When the fixture routine collects the answers: the records it keeps waiting are of pull requests updated since a week. */
internal val COLLECTED_AT: Instant = Instant.parse("2026-11-10T12:00:00Z")

/** Page [page] of the pull requests of [PROJECT], the most recently updated first. */
internal fun recentPullRequests(page: Int = 1): ProviderRequest = ProviderRequest(
  "GET",
  PULLS_ENDPOINT,
  mapOf("state" to "all", "sort" to "updated", "direction" to "desc", "per_page" to "100", "page" to "$page"),
)

/** Pull request [number] of [PROJECT] as GitHub lists it in [state], `open` or `closed`, last updated at [updatedAt]. */
internal fun listedPullRequest(number: Int, state: String, updatedAt: String = "2026-11-09T08:00:00Z"): String = buildJsonObject {
  put("number", number)
  put("state", state)
  put("updated_at", updatedAt)
  put("html_url", pullRequest(number).url)
}.toString()

/** The first page of the review comments of [pullRequest]. */
internal fun commentsOf(pullRequest: ReviewReference): ProviderRequest = ProviderRequest(
  "GET",
  "$PULLS_ENDPOINT/${pullRequest.id}/comments",
  mapOf("per_page" to "100", "page" to "1"),
)

/** The first page of the review comments of [PULL_REQUEST]. */
internal val PULL_REQUEST_COMMENTS = commentsOf(PULL_REQUEST)

/** The web page of the review comment [id] of [pullRequest]. */
internal fun commentUrl(id: Int, pullRequest: ReviewReference = PULL_REQUEST): String = "${pullRequest.url}#discussion_r$id"

/** The review comment [id] of Edict on the pull request of [record], with [thumbsUp] 👍 and [thumbsDown] 👎. */
internal fun edictComment(id: Int, record: FeedbackRecord, thumbsUp: Int = 0, thumbsDown: Int = 0): String = buildJsonObject {
  put("id", id)
  put("body", feedbackComment(record))
  put("html_url", commentUrl(id, record.pullRequest))
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

/** The answer that lists [items], such as pull requests or review comments, as one page of a GitHub list. */
internal fun listAnswer(vararg items: String): CiHttpResponse = answer(200, items.joinToString(",\n", "[", "]"))

/** A call of the feedback client that sends exactly [exchanges], answered as a script has them: no real pull request yet. */
internal fun feedbackScripted(vararg exchanges: Exchange): Recording<GitHubFeedbackApi> =
  Recording(exchanges.toList(), JsonNull, ::GitHubFeedbackApi, scripted = true)
