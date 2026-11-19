// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import org.jetbrains.qodana.edict.ci.CiHttpResponse
import org.jetbrains.qodana.edict.ci.CiHttpTransport
import org.jetbrains.qodana.edict.ci.support.Exchange
import org.jetbrains.qodana.edict.ci.support.ProviderRequest
import org.jetbrains.qodana.edict.ci.support.Recording
import org.jetbrains.qodana.edict.ci.support.answer
import org.jetbrains.qodana.edict.ci.support.answeredWith
import org.jetbrains.qodana.edict.common.DEFAULT_EDICT_STATE_BRANCH
import org.jetbrains.qodana.edict.common.EdictLayout
import org.jetbrains.qodana.edict.common.FeedbackConfiguration
import org.jetbrains.qodana.edict.common.StateBranch
import org.jetbrains.qodana.edict.common.runProcess
import org.jetbrains.qodana.edict.common.sha256
import org.jetbrains.qodana.edict.common.support.GitOrigin
import org.jetbrains.qodana.edict.core.EdictJson
import org.jetbrains.qodana.edict.core.EdictLineRange
import org.jetbrains.qodana.edict.core.EdictRepositoryDirectory
import org.jetbrains.qodana.edict.feedback.support.FEEDBACK_CLUSTER
import org.jetbrains.qodana.edict.feedback.support.FINDING_MESSAGE
import org.jetbrains.qodana.edict.feedback.support.SPACE_PROJECT
import org.jetbrains.qodana.edict.feedback.support.SPACE_REVIEW
import org.jetbrains.qodana.edict.feedback.support.feedbackRecord
import org.jetbrains.qodana.edict.feedback.support.internalResult
import org.jetbrains.qodana.edict.feedback.support.sarifReport
import org.jetbrains.qodana.edict.feedback.support.stateFiles
import org.jetbrains.qodana.edict.feedback.support.storeActive
import org.jetbrains.qodana.edict.feedback.support.storeHistory
import org.jetbrains.qodana.edict.promotion.support.INSPECTION_DIGEST
import org.jetbrains.qodana.edict.promotion.support.INSPECTION_NAME
import org.jetbrains.qodana.edict.promotion.support.INSPECTION_SUMMARY
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import java.time.Clock
import java.time.Instant
import java.time.ZoneOffset
import java.util.Properties
import kotlin.io.path.createDirectories
import kotlin.io.path.outputStream
import kotlin.io.path.writeText
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertIs
import kotlin.test.assertNull

// The Space answers are shaped as Space gave them for MR 236641 and its SafePush on 2026-11-18, cut to the fields read; the
// ids are real. The author's reviews change every day, so the reads are not replayed against Space.
class SpaceFeedbackTest {
  @TempDir
  lateinit var directory: Path

  private val origin by lazy { GitOrigin(directory) }

  @Test
  fun `the review of a SafePush is the first of the author's merge requests whose Safe Merge runs its Patronus build`() {
    val recording = reviewsScripted(
      authorReviews answeredWith answer(200, reviews(review("28hQpA0S4dyM", 236108), commitSet("3ddEto3ss0hy", 234995), review(REVIEW_ID, 236641))),
      titledReviews answeredWith answer(200, reviews(review(REVIEW_ID, 236641))),
      safeMerge("28hQpA0S4dyM") answeredWith answer(200, checks("0aa8b34f-0000-4648-9969-933da7cfb950")),
      safeMerge(REVIEW_ID) answeredWith answer(200, checks(PATRONUS_ID)),
    )

    assertEquals(SPACE_REVIEW, recording.replay { review(AUTHOR, PATRONUS_ID) })
  }

  @Test
  fun `a review titled as the commit is the SafePush's when Space knows no account of the author`() {
    val recording = reviewsScripted(
      authorReviews answeredWith answer(404, """{"error": "not-found", "error_description": "Profile not found"}"""),
      titledReviews answeredWith answer(200, reviews(review(REVIEW_ID, 236641))),
      safeMerge(REVIEW_ID) answeredWith answer(200, checks(PATRONUS_ID)),
    )

    assertEquals(SPACE_REVIEW, recording.replay { review(AUTHOR, PATRONUS_ID) })
  }

  @Test
  fun `a SafePush of no review found is its own review, on its Patronus page`() {
    val recording = reviewsScripted(
      authorReviews answeredWith answer(200, reviews(review("28hQpA0S4dyM", 236108))),
      titledReviews answeredWith answer(200, reviews()),
      // A merge request that never ran a Safe Merge has none.
      safeMerge("28hQpA0S4dyM") answeredWith CiHttpResponse(200, JsonNull),
    )

    val review = recording.replay { review(AUTHOR, PATRONUS_ID) }

    assertEquals(SPACE_PROJECT.reference(PATRONUS_ID, "https://patronus.labs.jb.gg/robot/$PATRONUS_ID"), review)
  }

  @Test
  fun `fails when Space refuses to list reviews, rather than taking the SafePush for one of no review`() {
    val recording = reviewsScripted(authorReviews answeredWith answer(401, """{"error": "invalid_token"}"""))

    val failure = assertFailsWith<IllegalStateException> { recording.replay { review(AUTHOR, PATRONUS_ID) } }

    assertEquals("Space answered the listing of reviews by author with HTTP 401", failure.message)
  }

  @Test
  fun `fails when Space knows no repository to list the reviews of, rather than taking the SafePush for one of no review`() {
    val recording = reviewsScripted(
      authorReviews answeredWith answer(200, reviews()),
      titledReviews answeredWith answer(404, """{"error": "not-found", "error_description": "Repository not found"}"""),
    )

    val failure = assertFailsWith<IllegalStateException> { recording.replay { review(AUTHOR, PATRONUS_ID) } }

    assertEquals("Space knows no ${SPACE_PROJECT.label()} to list the reviews of", failure.message)
  }

  @Test
  fun `fails before reserving anything without a token for the Slack service, which would show the findings to nobody`() {
    val project = safePush()
    val author = SafePushAuthor(git(project, "rev-parse", "HEAD~1"), AUTHOR.email, AUTHOR.subject)
    val safePush = SafePush(SafePushBuild(PATRONUS_ID, git(project, "rev-parse", "HEAD~2"), BUILD_URL), author, SPACE_REVIEW)
    val noToken = IllegalStateException("Edict feedback on Space requires EDICT_NOTIFICATION_TOKEN to notify through the Slack service")
    val api = SpaceFeedbackApi(project, lazyOf(safePush), lazy { throw noToken })

    val failure = assertFailsWith<IllegalStateException> { api.candidates(SPACE_REVIEW, listOf(finding("rule", 2, 3)), emptyRecords(), CHECKED_AT) }

    assertEquals(noToken, failure)
  }

  @Test
  fun `notifies the author through the Slack service`() {
    val recording = notificationsScripted(notification(RECORD, SNIPPET) answeredWith answer(202, ACCEPTED))

    assertEquals(Notification.Sent, recording.replay { notify(RECORD, AUTHOR.email, BUILD_URL, SNIPPET) })
  }

  @Test
  fun `the Slack service may refuse a notification, saying why`() {
    val refused = """{"status": "rejected", "detail": "recordId must look like f-<24 hex>"}"""
    val recording = notificationsScripted(notification(RECORD, SNIPPET) answeredWith answer(400, refused))

    val notification = recording.replay { notify(RECORD, AUTHOR.email, BUILD_URL, SNIPPET) }

    assertEquals(Notification.Refused(400, "recordId must look like f-<24 hex>"), notification)
  }

  @Test
  fun `the snippet of a finding shows the lines around it that the file has`() {
    val file = (1..6).map { "line $it" }

    assertEquals(CodeSnippet(1, "line 1\nline 2\nline 3\nline 4"), codeSnippet(file, EdictLineRange(2, 2)))
    assertEquals(CodeSnippet(2, "line 2\nline 3\nline 4\nline 5\nline 6"), codeSnippet(file, EdictLineRange(4, 5)))
  }

  @Test
  fun `a notification fails without an answer to tell, keeping why`() {
    val cause = IllegalStateException("space API request failed (connection or timeout)")
    val unreachable = object : CiHttpTransport {
      override fun send(method: String, endpoint: String, query: Map<String, String>, body: JsonObject?): CiHttpResponse = throw cause
    }

    val notification = SlackFeedbackNotifications(unreachable).notify(RECORD, AUTHOR.email, BUILD_URL, SNIPPET)

    assertEquals(cause, assertIs<Notification.Failed>(notification).cause)
  }

  @Test
  fun `the author is of the newest commit a person made, under the merge of Patronus`() {
    val project = safePush()

    val author = safePushAuthor(project, git(project, "rev-parse", "HEAD~2"))

    assertEquals(SafePushAuthor(git(project, "rev-parse", "HEAD~1"), AUTHOR.email, AUTHOR.subject), author)
  }

  @Test
  fun `only the files of the person's commits get feedback, not those the merge of Patronus brings`() {
    val project = safePush()
    val base = git(project, "rev-parse", "HEAD~2")
    val author = SafePushAuthor(git(project, "rev-parse", "HEAD~1"), AUTHOR.email, AUTHOR.subject)
    val api = feedbackOn(project, SafePush(SafePushBuild(PATRONUS_ID, base, BUILD_URL), author, SPACE_REVIEW))
    val own = InternalFinding("rule", "src/Own.kt", EdictLineRange(2, 3), FINDING_MESSAGE)
    val merged = InternalFinding("rule", "src/Merged.kt", EdictLineRange(1, 1), FINDING_MESSAGE)

    val candidates = api.candidates(SPACE_REVIEW, listOf(own, merged), emptyRecords(), CHECKED_AT)

    assertEquals(listOf(FeedbackCandidate(own, 2)), candidates)
  }

  @Test
  fun `a SafePush of only bots has no author to notify`() {
    val project = origin.clone("project")
    val base = git(project, "rev-parse", "HEAD")
    commit(project, "src/Merged.kt", "class Merged\n", "patronus-no-reply@jetbrains.com", "Pre-merge `master` changes")

    assertNull(safePushAuthor(project, base))
  }

  @Test
  fun `a finding of an inspection shown at the same lines of the same file within the week is not shown again`() {
    val paths = EdictRepositoryDirectory(directory.resolve("state").createDirectories())
    // A record of another review counts as well.
    val otherReview = SPACE_PROJECT.reference("3x8SkC1fNVwI", "https://code.jetbrains.team/p/ij/reviews/236108")
    val recent = feedbackRecord(1, pullRequest = otherReview).copy(path = "src/Own.kt", createdAt = "2026-11-15T10:00:00Z")
    val old = feedbackRecord(2, clusterId = "old", pullRequest = SPACE_REVIEW).copy(path = "src/Own.kt", createdAt = "2026-11-01T10:00:00Z")
    paths.storeActive(recent)
    paths.storeHistory(old)
    val project = safePush()
    val author = SafePushAuthor(git(project, "rev-parse", "HEAD~1"), AUTHOR.email, AUTHOR.subject)
    val api = feedbackOn(project, SafePush(SafePushBuild(PATRONUS_ID, git(project, "rev-parse", "HEAD~2"), BUILD_URL), author, SPACE_REVIEW))
    val findings = listOf(finding("rule", 3, 3), finding("rule", 3, 4), finding("other", 3, 3), finding("old", 3, 3))

    val candidates = api.candidates(SPACE_REVIEW, findings, FeedbackRecords(paths), CHECKED_AT)

    assertEquals(findings.drop(1).map { FeedbackCandidate(it, 3) }, candidates)
  }

  @Test
  fun `reads the SafePush from the parameters TeamCity writes for the build step`() {
    val system = parameters(
      "system.properties",
      "teamcity.configuration.properties.file" to directory.resolve("configuration.properties").toString(),
      "teamcity.buildType.id" to "ijplatform_master_EdictIncrementalSafePush",
      "teamcity.build.id" to "1133560879",
    )
    parameters(
      "configuration.properties",
      "teamcity.build.branch" to "refs/patronus/yuriy-artamonov-$PATRONUS_ID/checks",
      "baseCommit" to BASE,
      "teamcity.serverUrl" to "https://buildserver.labs.intellij.net/",
    )

    assertEquals(SafePushBuild(PATRONUS_ID, BASE, BUILD_URL), teamCitySafePushBuild(system))
  }

  @Test
  fun `a build of another branch than a SafePush's gets no feedback`() {
    val system = parameters("system.properties", "teamcity.configuration.properties.file" to directory.resolve("configuration.properties").toString())
    parameters("configuration.properties", "teamcity.build.branch" to "refs/heads/master")

    assertNull(teamCitySafePushBuild(system))
  }

  @Test
  fun `posting reserves the person's finding for the review and notifies them, and a later SafePush of the review does not repeat it`() {
    origin.publishState("routine", stateFiles(directory.resolve("routine-state"), FEEDBACK_CLUSTER))
    val project = safePush()
    val base = git(project, "rev-parse", "HEAD~2")
    val revision = git(project, "rev-parse", "HEAD")
    val layout = EdictLayout(project, directory.resolve("state"))
    val sarif = directory.resolve("qodana.sarif.json")
    sarif.writeText(sarifReport(internalResult("rule", "src/Own.kt", startLine = 2, endLine = 3), revision = revision))
    val author = requireNotNull(safePushAuthor(project, base))
    val safePush = SafePush(SafePushBuild(PATRONUS_ID, base, BUILD_URL), author, SPACE_REVIEW)
    val identity = listOf(SPACE_REVIEW.url, INSPECTION_DIGEST, "src/Own.kt", "2").joinToString("\u0000")
    val record = FeedbackRecord(
      id = "f-${sha256(identity).take(24)}",
      clusterId = "rule",
      inspectionDigest = INSPECTION_DIGEST,
      pullRequest = SPACE_REVIEW,
      revision = revision,
      path = "src/Own.kt",
      lines = EdictLineRange(2, 3),
      commentLine = 2,
      inspectionName = INSPECTION_NAME,
      inspectionDescription = INSPECTION_SUMMARY,
      message = FINDING_MESSAGE,
      createdAt = POSTED_AT.toString(),
    )
    val notified = notification(record, CodeSnippet(1, "class Own {\n  fun own() {\n    blockingCall()\n  }\n}")) answeredWith answer(202, ACCEPTED)

    listOf(listOf(notified), emptyList()).forEach { exchanges ->
      StateBranch(layout, DEFAULT_EDICT_STATE_BRANCH).checkOut(shallow = true)
      notificationsScripted(*exchanges.toTypedArray()).replay {
        val api = SpaceFeedbackApi(project, lazyOf(safePush), lazyOf(this))
        postFeedback(layout, DEFAULT_EDICT_STATE_BRANCH, api.analysedReview(), api, sarif, Clock.fixed(POSTED_AT, ZoneOffset.UTC), FeedbackConfiguration())
      }
    }

    assertEquals(listOf(record.id), activeRecordIdsOnOrigin())
  }

  @Test
  fun `posting selects again after another review's run reserved the same finding first, and notifies nobody`() {
    origin.publishState("routine", stateFiles(directory.resolve("routine-state"), FEEDBACK_CLUSTER))
    val project = safePush()
    val base = git(project, "rev-parse", "HEAD~2")
    val revision = git(project, "rev-parse", "HEAD")
    val layout = EdictLayout(project, directory.resolve("state"))
    val sarif = directory.resolve("qodana.sarif.json")
    sarif.writeText(sarifReport(internalResult("rule", "src/Own.kt", startLine = 2, endLine = 3), revision = revision))
    val safePush = SafePush(SafePushBuild(PATRONUS_ID, base, BUILD_URL), requireNotNull(safePushAuthor(project, base)), SPACE_REVIEW)
    // The SafePush of another review, with the same commit, reserves the finding while this run reserves it.
    val otherReview = SPACE_PROJECT.reference("3x8SkC1fNVwI", "https://code.jetbrains.team/p/ij/reviews/236108")
    val other = feedbackRecord(1, pullRequest = otherReview).copy(
      revision = revision,
      path = "src/Own.kt",
      lines = EdictLineRange(2, 3),
      createdAt = POSTED_AT.minusSeconds(60).toString(),
    )
    origin.raceNextStatePush("other", mapOf("feedback/active/${other.fileName}" to other.fileText()))
    StateBranch(layout, DEFAULT_EDICT_STATE_BRANCH).checkOut(shallow = true)

    notificationsScripted().replay {
      val api = SpaceFeedbackApi(project, lazyOf(safePush), lazyOf(this))
      postFeedback(layout, DEFAULT_EDICT_STATE_BRANCH, api.analysedReview(), api, sarif, Clock.fixed(POSTED_AT, ZoneOffset.UTC), FeedbackConfiguration())
    }

    assertEquals(listOf(other.id), activeRecordIdsOnOrigin())
  }

  /** The records of an empty state. */
  private fun emptyRecords(): FeedbackRecords = FeedbackRecords(EdictRepositoryDirectory(directory.resolve("empty-state").createDirectories()))

  /** A clone of origin with a SafePush on top of its commit: the person's commit, then a merge commit of Patronus. */
  private fun safePush(): Path {
    val project = origin.clone("project")
    commit(project, "src/Own.kt", "class Own {\n  fun own() {\n    blockingCall()\n  }\n}\n", AUTHOR.email, AUTHOR.subject)
    commit(project, "src/Merged.kt", "class Merged\n", "patronus-no-reply@jetbrains.com", "Pre-merge `master` changes into temp branch for running tests")
    return project
  }

  /** The ids of the records awaiting an answer on the state branch of origin. */
  private fun activeRecordIdsOnOrigin(): List<String> {
    val active = "$DEFAULT_EDICT_STATE_BRANCH:feedback/active"
    return git(origin.path, "ls-tree", "--name-only", active).lines().filter(String::isNotEmpty).map { file ->
      EdictJson.decodeFromString(FeedbackRecord.serializer(), git(origin.path, "show", "$active/$file")).id
    }
  }

  /** Writes the TeamCity parameters [values] to the properties file [name], as TeamCity writes them, and returns it. */
  private fun parameters(name: String, vararg values: Pair<String, String>): Path {
    val file = directory.resolve(name)
    file.outputStream().use { output -> Properties().apply { values.forEach { (key, value) -> setProperty(key, value) } }.store(output, null) }
    return file
  }
}

private const val PATRONUS_ID = "a88b34fc-d053-4648-9969-933da7cfb950"
private const val REVIEW_ID = "J0wB60npRC0"
private const val BASE = "8a2509a57258dc61083130203bcf211b128ae08f"
private const val BUILD_URL = "https://buildserver.labs.intellij.net/buildConfiguration/ijplatform_master_EdictIncrementalSafePush/1133560879"
private const val ACCEPTED = """{"status": "accepted", "detail": "queued"}"""
private val POSTED_AT = Instant.parse("2026-11-18T12:00:00Z")

private val AUTHOR = SafePushAuthor(
  "eca0f97d063d420cf6cc7971d027b1c6606d3bc4",
  "yuriy.artamonov@jetbrains.com",
  "[jcef] IJPL-262030 Freeze in BalloonImpl.createComponent waits for JCEF init",
)

/** A record of the SafePush, with an id as the Slack service accepts it. */
private val RECORD = feedbackRecord(1, pullRequest = SPACE_REVIEW).copy(id = "f-0123456789abcdef01234567")

private const val REVIEWS = "/projects/key:ij/code-reviews"

/** The listing of the author's merge requests that may be the SafePush's. */
private val authorReviews = ProviderRequest("GET", REVIEWS, reviewsQuery("author" to "username:yuriy.artamonov", top = 30))

/** The listing of the merge requests titled as the commit of the SafePush. */
private val titledReviews = ProviderRequest("GET", REVIEWS, reviewsQuery("text" to AUTHOR.subject, top = 5))

private fun reviewsQuery(filter: Pair<String, String>, top: Int): Map<String, String> = mapOf(
  filter,
  "state" to "null",
  "repository" to "ultimate",
  "sort" to "LastUpdatedDesc",
  "\$top" to "$top",
  "\$fields" to "data(review(id,number))",
)

private fun safeMerge(reviewId: String) =
  ProviderRequest("GET", "$REVIEWS/safe-merge", mapOf("mergeRequestId" to reviewId, "\$fields" to "checks(buildId)"))

private fun reviews(vararg reviews: String): String = reviews.joinToString(",", """{"data": [""", "]}")

private fun review(id: String, number: Int): String = """{"review": {"className": "MergeRequestRecord", "id": "$id", "number": $number}}"""

private fun commitSet(id: String, number: Int): String = """{"review": {"className": "CommitSetReviewRecord", "id": "$id", "number": $number}}"""

private fun checks(buildId: String): String = """{"checks": [{"className": "SafeMergeCheck.External", "buildId": "$buildId"}]}"""

/** The notification of [record], whose code [snippet] shows, to [AUTHOR]. */
private fun notification(record: FeedbackRecord, snippet: CodeSnippet) = ProviderRequest(
  "POST",
  "/api/teamcity/notifications",
  body = buildJsonObject {
    put("email", AUTHOR.email)
    put("inspectionName", record.inspectionName)
    put("inspectionId", record.clusterId)
    put("inspectionDescription", record.inspectionDescription)
    put("filePath", record.path)
    put("startLine", snippet.startLine)
    put("commit", record.revision)
    put("problemDescription", record.message)
    put("buildUrl", BUILD_URL)
    put("codeSnippet", snippet.text)
    put("recordId", record.id)
    put("inspectionDigest", record.inspectionDigest)
  },
)

private val SNIPPET = CodeSnippet(1, "val x = 1")

private val CHECKED_AT = Instant.parse("2026-11-18T10:00:00Z")

private fun finding(clusterId: String, start: Int, end: Int) = InternalFinding(clusterId, "src/Own.kt", EdictLineRange(start, end), FINDING_MESSAGE)

/** The feedback on [safePush], which must notify nobody. */
private fun feedbackOn(project: Path, safePush: SafePush): SpaceFeedbackApi =
  SpaceFeedbackApi(project, lazyOf(safePush), lazyOf(silentNotifications()))

/** A call of the review lookup that sends exactly [exchanges] to Space. */
private fun reviewsScripted(vararg exchanges: Exchange): Recording<SpaceSafePushReviews> =
  Recording(exchanges.toList(), JsonNull, { SpaceSafePushReviews(SPACE_PROJECT, it) }, scripted = true)

/** A call of the Slack notifications that sends exactly [exchanges] to the Slack service. */
private fun notificationsScripted(vararg exchanges: Exchange): Recording<SlackFeedbackNotifications> =
  Recording(exchanges.toList(), JsonNull, ::SlackFeedbackNotifications, scripted = true)

/** Slack notifications that must send nothing. */
private fun silentNotifications(): SlackFeedbackNotifications = notificationsScripted().replay { this }

/** Commits [content] at [path] in [project] as the author [email], with the message [subject]. */
private fun commit(project: Path, path: String, content: String, email: String, subject: String) {
  project.resolve(path).apply { parent.createDirectories() }.writeText(content)
  git(project, "add", "--all")
  git(project, "commit", "--quiet", "--author", "Someone <$email>", "-m", subject)
}

private fun git(directory: Path, vararg arguments: String): String = runProcess(directory, listOf("git", *arguments)).trim()
