// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

import io.github.oshai.kotlinlogging.KotlinLogging
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import org.jetbrains.qodana.edict.ci.CiHttpTransport
import org.jetbrains.qodana.edict.ci.CiProviderId
import org.jetbrains.qodana.edict.ci.DefaultCiHttpTransport
import org.jetbrains.qodana.edict.ci.ReviewReference
import org.jetbrains.qodana.edict.ci.ReviewRepository
import org.jetbrains.qodana.edict.ci.space.spaceReviewUrl
import org.jetbrains.qodana.edict.ci.space.spaceTransport
import org.jetbrains.qodana.edict.ci.urlPart
import org.jetbrains.qodana.edict.common.GitRepository
import org.jetbrains.qodana.edict.common.array
import org.jetbrains.qodana.edict.common.number
import org.jetbrains.qodana.edict.common.obj
import org.jetbrains.qodana.edict.common.text
import org.jetbrains.qodana.edict.core.EdictLineRange
import java.nio.file.Path
import java.time.Duration
import java.time.Instant

private val logger = KotlinLogging.logger {}

/** The Slack service that notifies authors of SafePush findings and stores their answers. */
private const val NOTIFICATION_SERVICE_URL = "https://edict-slack-service-public.labs.jb.gg"

/** The reviews of an author, the most recently updated first, that may be the one a SafePush is of, at most. */
private const val MAX_AUTHOR_REVIEWS = 30

/** The reviews whose title matches a commit subject that may be the one a SafePush is of, at most. */
private const val MAX_TITLED_REVIEWS = 5

/** How long a finding shown on Space keeps the same inspection's finding at the same lines of the same file from being shown again. */
internal val SPACE_FEEDBACK_REPEAT_WINDOW: Duration = Duration.ofDays(7)

/**
 * The lines around a finding that its notification shows too, as the context region of Qodana does: the region of a
 * finding names only its first line.
 */
private const val SNIPPET_CONTEXT_LINES = 2

/** The feedback of the SafePush that the TeamCity build of this process analyses in [project], a checkout of [repository]. */
internal fun spaceFeedbackApi(repository: ReviewRepository, project: Path): SpaceFeedbackApi = SpaceFeedbackApi(
    project,
    lazy { analysedSafePush(repository, project, SpaceSafePushReviews(repository, spaceTransport(repository))) },
    lazy(::slackFeedbackNotifications),
)

/**
 * The feedback on Space SafePushes in [project]: Slack notifications through [notifications] to the person who made [safePush],
 * of the findings in the files their commits change, at most once a [SPACE_FEEDBACK_REPEAT_WINDOW] for the same inspection, file
 * and lines. Both are made once first used: the server, which only retires records, needs neither, and `prepare` no
 * notifications. The Slack service keeps the answers, so the records only retire.
 */
internal class SpaceFeedbackApi(
    project: Path,
    safePush: Lazy<SafePush?>,
    private val notifications: Lazy<SlackFeedbackNotifications>,
) : FeedbackApi {
    private val repository by lazy { GitRepository(project) }
    private val safePush by safePush

    override fun analysedReview(): ReviewReference? = safePush?.review

    /**
     * The findings in the files the person's commits change, each at its first line, but for those of an inspection that a
     * record on any review showed at the same lines of the same file within the [SPACE_FEEDBACK_REPEAT_WINDOW].
     */
    override fun candidates(
        review: ReviewReference,
        findings: List<InternalFinding>,
        records: FeedbackRecords,
        now: Instant,
    ): List<FeedbackCandidate> {
        val push = analysed()
        // The findings are reserved before they are shown: without the Slack service, they would be reserved for nobody.
        notifications.value
        // The merge commit of Patronus brings changes of the target branch, which the person did not make.
        val changed = repository.git("diff", "--name-only", "--no-renames", push.build.baseCommit, push.author.revision)
            .lines().toSet()
        val since = now.minus(SPACE_FEEDBACK_REPEAT_WINDOW)
        val shown = (records.active() + records.history())
            .filter { Instant.parse(it.createdAt) >= since }
            .mapTo(HashSet()) { Triple(it.clusterId, it.path, it.lines) }
        return findings.filter { it.path in changed && Triple(it.clusterId, it.path, it.lines) !in shown }
            .map { FeedbackCandidate(it, it.lines.start) }
    }

    override fun show(record: FeedbackRecord) {
        val push = analysed()
        val snippet = codeSnippet(repository.fileAt(record.revision, record.path).removeSuffix("\n").lines(), record.lines)
        val email = push.author.email
        when (val notification = notifications.value.notify(record, email, push.build.buildUrl, snippet)) {
            is Notification.Sent -> logger.info { "Sent Edict feedback ${record.id} of ${record.clusterId} to $email" }
            is Notification.Refused -> logger.warn {
                "$FEEDBACK_UNAVAILABLE: the Slack service refused feedback ${record.id} with HTTP ${notification.status}: " +
                    notification.detail
            }
            is Notification.Failed -> logger.warn(notification.cause) {
                "$FEEDBACK_UNAVAILABLE: the Slack service gave no answer to feedback ${record.id}"
            }
        }
    }

    /** Every record retires: the Slack service keeps the answers, which come back as its suggestions. */
    override fun outcomes(records: List<FeedbackRecord>, now: Instant): List<RecordOutcome> =
        records.map(RecordOutcome::Retired)

    private fun analysed(): SafePush = checkNotNull(safePush) { "The TeamCity build analyses no SafePush to give feedback on" }
}

/**
 * Finds on [space] the review of [repository] that a SafePush is of. Space rebases the commits it pushes, so no commit names the
 * review, but the review's Safe Merge names the Patronus build of the SafePush.
 */
internal class SpaceSafePushReviews(private val repository: ReviewRepository, private val space: CiHttpTransport) {
    private val reviewsRoot = "/projects/key:${urlPart(repository.owner)}/code-reviews"

    /**
     * The merge request that [author]'s SafePush [patronusId] is of: the first, among the author's recent reviews and then those
     * titled as the commit, whose Safe Merge runs the Patronus build. Without one, the SafePush itself stands for the review:
     * its id is the Patronus id, and its page the Patronus one, which no Space repository hosts.
     */
    fun review(author: SafePushAuthor, patronusId: String): ReviewReference {
        // Space usernames are the local parts of JetBrains addresses.
        val username = author.email.substringBefore('@')
        val authorReviews = reviews("author" to "username:$username", MAX_AUTHOR_REVIEWS) ?: emptyList<FoundReview>().also {
            logger.info { "Space knows no account $username; only reviews titled as the commit may be the SafePush's" }
        }
        val titledReviews = checkNotNull(reviews("text" to author.subject, MAX_TITLED_REVIEWS)) {
            "Space knows no ${repository.label()} to list the reviews of"
        }
        val review = (authorReviews + titledReviews).distinctBy { it.id }.firstOrNull { patronusId in safeMergeBuilds(it.id) }
        return if (review != null) repository.reference(review.id, spaceReviewUrl(repository, review.number))
        else repository.reference(patronusId, "https://patronus.labs.jb.gg/robot/$patronusId")
    }

    /**
     * The merge requests of [repository] that the search [filter] lists, in any state, the most recently updated first; null
     * when Space answers 404: it knows no such author, project or repository.
     */
    private fun reviews(filter: Pair<String, String>, limit: Int): List<FoundReview>? {
        val query = mapOf(
            filter,
            // Without a state, Space lists only open reviews; the Safe Merge may have merged it by now.
            "state" to "null",
            "repository" to repository.repository,
            "sort" to "LastUpdatedDesc",
            "\$top" to "$limit",
            "\$fields" to "data(review(id,number))",
        )
        val response = space.send("GET", reviewsRoot, query)
        if (response.status == 404) return null
        val body = response.body as? JsonObject
        check(response.status == 200 && body != null) { "Space answered the listing of reviews by ${filter.first} with HTTP ${response.status}" }
        return body.array("data").map { it.obj("review") }
            .filter { it.text("className") == "MergeRequestRecord" }
            .map { FoundReview(it.text("id"), it.number("number")) }
    }

    /** The Patronus builds that the Safe Merge of the merge request [reviewId] runs; none without a Safe Merge. */
    private fun safeMergeBuilds(reviewId: String): Set<String> {
        val response = space.send("GET", "$reviewsRoot/safe-merge", mapOf("mergeRequestId" to reviewId, "\$fields" to "checks(buildId)"))
        check(response.status == 200) { "Space answered the Safe Merge of review $reviewId with HTTP ${response.status}" }
        val body = response.body as? JsonObject
        check(body != null || response.body == JsonNull) { "Space answered the Safe Merge of review $reviewId with no object" }
        return body?.array("checks")?.mapTo(HashSet()) { it.text("buildId") }.orEmpty()
    }
}

/** A merge request [id] that Space lists, numbered [number] in its project. */
private data class FoundReview(val id: String, val number: Long)

/** What the Slack service did with a notification. */
internal sealed interface Notification {
    data object Sent : Notification

    /** The service refused it with [status], saying [detail]: the author has no Slack account, unsubscribed, or it failed. */
    data class Refused(val status: Int, val detail: String) : Notification

    /** The service gave no answer to tell: no connection, or one that is no JSON, such as a plain-text refusal of the token. */
    data class Failed(val cause: Exception) : Notification
}

/** The notifications of the Slack service, with `EDICT_NOTIFICATION_TOKEN` from the environment. */
internal fun slackFeedbackNotifications(): SlackFeedbackNotifications {
    val token = System.getenv("EDICT_NOTIFICATION_TOKEN")?.takeIf(String::isNotBlank)
        ?: error("Edict feedback on Space requires EDICT_NOTIFICATION_TOKEN to notify through the Slack service")
    return SlackFeedbackNotifications(DefaultCiHttpTransport(CiProviderId.SPACE, NOTIFICATION_SERVICE_URL, token))
}

/** Notifies authors of SafePush findings through the Slack service on [slack], which stores their answers. */
internal class SlackFeedbackNotifications(private val slack: CiHttpTransport) {
    /** Notifies [email] of [record], whose code [snippet] shows, found by the build at [buildUrl]. */
    fun notify(record: FeedbackRecord, email: String, buildUrl: String, snippet: CodeSnippet): Notification {
        val request = buildJsonObject {
            put("email", email)
            put("inspectionName", record.inspectionName)
            put("inspectionId", record.clusterId)
            put("inspectionDescription", record.inspectionDescription)
            put("filePath", record.path)
            // The service tells the lines of the snippet by its first one.
            put("startLine", snippet.startLine)
            put("commit", record.revision)
            put("problemDescription", record.message)
            put("buildUrl", buildUrl)
            put("codeSnippet", snippet.text)
            put("recordId", record.id)
            put("inspectionDigest", record.inspectionDigest)
        }
        val response = try {
            slack.send("POST", "/api/teamcity/notifications", body = request)
        }
        catch (e: IllegalStateException) {
            return Notification.Failed(e)
        }
        val answer = response.body as? JsonObject ?: JsonObject(emptyMap())
        return if (response.status in 200..299) Notification.Sent else Notification.Refused(response.status, answer.text("detail"))
    }
}

/** The [text] of the lines of a file from [startLine] on. */
internal data class CodeSnippet(val startLine: Int, val text: String)

/** The [lines] of a finding in the [file], with [SNIPPET_CONTEXT_LINES] lines around them that the file has. */
internal fun codeSnippet(file: List<String>, lines: EdictLineRange): CodeSnippet {
    val start = (lines.start - SNIPPET_CONTEXT_LINES).coerceAtLeast(1)
    val end = (lines.end + SNIPPET_CONTEXT_LINES).coerceAtMost(file.size)
    return CodeSnippet(start, file.subList(start - 1, end.coerceAtLeast(start - 1)).joinToString("\n"))
}
