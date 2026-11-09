// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci.support

import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import org.jetbrains.qodana.edict.ci.ReviewApi
import org.jetbrains.qodana.edict.ci.ReviewReference
import org.jetbrains.qodana.edict.common.obj
import org.jetbrains.qodana.edict.common.text
import java.security.MessageDigest
import java.util.HexFormat

/** The requests the Space client sends to [ULTIMATE] and Space's answers, for scripting what the real repository lacks. */
internal val ULTIMATE_SPACE = SpaceExchanges(ULTIMATE)

/**
 * Calls of the Space client on states IJ/ultimate doesn't have, such as a branch an earlier run left: the recorded
 * calls with the steps that state changes answered by script.
 */
internal object UltimateScripts {
  private val space = ULTIMATE_SPACE

  /** The inspection branch exists at a commit that already holds the inspection. */
  fun inspectionAlreadyOnBranch() = ultimateScripted(
    space.branchRead(INSPECTION_BRANCH) answeredWith space.branch(INSPECTION_BRANCH, INSPECTION_COMMIT),
    *fileRead(INSPECTION_COMMIT, INSPECTION_PATH, INSPECTION_CONTENT),
    *reviewOpened(),
  )

  /** The inspection branch exists at the target head without the inspection: a run stopped before committing. */
  fun inspectionBranchLeftAtTarget() = ultimateScripted(
    space.branchRead(INSPECTION_BRANCH) answeredWith space.branch(INSPECTION_BRANCH, RELEASE_191_HEAD),
    space.fileList(RELEASE_191_HEAD, INSPECTION_PATH) answeredWith space.noFiles(),
    *withoutBranchCreation(inspectionReviewOpened),
  )

  /** The inspection branch exists at another commit, which holds another inspection. */
  fun inspectionBranchWithOtherContent() = ultimateScripted(
    space.branchRead(INSPECTION_BRANCH) answeredWith space.branch(INSPECTION_BRANCH, INSPECTION_COMMIT),
    *fileRead(INSPECTION_COMMIT, INSPECTION_PATH, "val previousInspection = 1\n"),
    space.branchRead(RELEASE_191) answeredWith space.branch(RELEASE_191, RELEASE_191_HEAD),
  )

  /** The inspection branch exists, but Space names no commit for it. */
  fun inspectionBranchWithoutRevision() = ultimateScripted(
    space.branchRead(INSPECTION_BRANCH) answeredWith space.branch(INSPECTION_BRANCH, ""),
  )

  /** An open review from the inspection branch exists, listed after a review of the same branch into another target. */
  fun inspectionReviewAlreadyOpen() = ultimateScripted(
    *inspectionCommitted(),
    space.reviewSearch(INSPECTION_BRANCH, RELEASE_191, "Opened") answeredWith space.reviews(
      space.review("1OtHeR0TaRgT", 239998, INSPECTION_BRANCH, "master"),
      space.review(ULTIMATE_EXISTING_REVIEW.id, EXISTING_REVIEW_NUMBER, "refs/heads/$INSPECTION_BRANCH", RELEASE_191),
    ),
    space.reviewerAssignment(ULTIMATE_EXISTING_REVIEW.id, ULTIMATE_REVIEWER) answeredWith spaceEmpty(),
  )

  /** A closed review from the inspection branch exists, and no open one; a closed review takes no reviewer. */
  fun inspectionReviewClosedBefore() = ultimateScripted(
    *inspectionCommitted(),
    space.reviewSearch(INSPECTION_BRANCH, RELEASE_191, "Opened") answeredWith space.reviews(),
    space.reviewSearch(INSPECTION_BRANCH, RELEASE_191, "Closed") answeredWith space.reviews(
      space.review(ULTIMATE_EXISTING_REVIEW.id, EXISTING_REVIEW_NUMBER, INSPECTION_BRANCH, "refs/heads/$RELEASE_191", "Closed"),
    ),
  )

  /** The reviewer is no profile id, so Space answers 404, and is then assigned by username. */
  fun inspectionReviewForUsername() = ultimateScripted(
    *inspectionReviewOpened.until(newReviewerAssignment),
    space.reviewerAssignment(ULTIMATE_NEW_REVIEW_ID, ULTIMATE_REVIEWER) answeredWith PROFILE_NOT_FOUND,
    space.reviewerAssignment(ULTIMATE_NEW_REVIEW_ID, "username:$ULTIMATE_REVIEWER") answeredWith spaceEmpty(),
  )

  /** The reviewer is neither a profile id nor a username. */
  fun inspectionReviewForUnknownReviewer() = ultimateScripted(
    *inspectionReviewOpened.until(newReviewerAssignment),
    space.reviewerAssignment(ULTIMATE_NEW_REVIEW_ID, ULTIMATE_REVIEWER) answeredWith PROFILE_NOT_FOUND,
    space.reviewerAssignment(ULTIMATE_NEW_REVIEW_ID, "username:$ULTIMATE_REVIEWER") answeredWith PROFILE_NOT_FOUND,
  )

  /** The reviewer, configured as a `username:` selector, doesn't exist. */
  fun inspectionReviewForUnknownUsername() = ultimateScripted(
    *inspectionReviewOpened.until(newReviewerAssignment),
    space.reviewerAssignment(ULTIMATE_NEW_REVIEW_ID, "username:$ULTIMATE_REVIEWER") answeredWith PROFILE_NOT_FOUND,
  )

  /** The source-fix branch exists at the base without the fix: a run stopped before committing. */
  fun sourceFixBranchLeftAtBase() = ultimateScripted(
    space.branchRead(ULTIMATE_SOURCE_FIX_BRANCH) answeredWith space.branch(ULTIMATE_SOURCE_FIX_BRANCH, RELEASE_191_HEAD),
    *withoutBranchCreation(sourceFixReviewOpened),
  )

  /** The source-fix branch exists at a commit with the fix, from which a review was opened. */
  fun sourceFixAlreadyCommitted() = ultimateScripted(
    space.branchRead(ULTIMATE_SOURCE_FIX_BRANCH) answeredWith space.branch(ULTIMATE_SOURCE_FIX_BRANCH, SOURCE_FIX_COMMIT),
    *ULTIMATE_SOURCE_FIX.files.entries.flatMap { (path, content) -> fileRead(SOURCE_FIX_COMMIT, path, content).asList() }.toTypedArray(),
    space.reviewSearch(ULTIMATE_SOURCE_FIX_BRANCH, RELEASE_191, "Opened") answeredWith space.reviews(
      space.review(ULTIMATE_EXISTING_REVIEW.id, EXISTING_REVIEW_NUMBER, ULTIMATE_SOURCE_FIX_BRANCH, RELEASE_191),
    ),
    space.reviewerAssignment(ULTIMATE_EXISTING_REVIEW.id, ULTIMATE_REVIEWER) answeredWith spaceEmpty(),
  )

  /** The source-fix branch exists at another commit, whose [ULTIMATE_BUILD_TXT] isn't the fixed one. */
  fun sourceFixBranchWithOtherContent() = ultimateScripted(
    space.branchRead(ULTIMATE_SOURCE_FIX_BRANCH) answeredWith space.branch(ULTIMATE_SOURCE_FIX_BRANCH, SOURCE_FIX_COMMIT),
    *fileRead(SOURCE_FIX_COMMIT, ULTIMATE_BUILD_TXT, ULTIMATE_BUILD_TXT_AT_191),
  )

  /** Review 222772 with its discussion's one message repeated 51 times, in a page of 50 and a page of 1. */
  fun discussionInTwoPages() = oneDiscussion.answering("GET", "/chats/messages") { page ->
    val message = page.response.body.arrayAt("messages").single().jsonObject
    val copies = (1..51).map { message.with("id" to JsonPrimitive("${message.text("id")}-$it")) }
    val next = page.request.copy(query = page.request.query + ("startFromDate" to message.obj("created").text("iso")))
    listOf(
      page.withBody(page.response.body.with("messages" to JsonArray(copies.take(50)), "nextStartFromDate" to message.obj("created"))),
      next answeredWith page.response.copy(body = page.response.body.with("messages" to JsonArray(copies.drop(50)))),
    )
  }

  /** Review 222772 with its discussion anchored on its file, without lines. */
  fun wholeFileDiscussion() = oneDiscussion.answering("GET", "/sync-batch") { feed ->
    val data = feed.response.body.arrayAt("data").map { wrapper ->
      val message = wrapper.jsonObject.obj("chatMessage")
      val details = message.obj("details")
      val discussion = details.obj("codeDiscussion")
      if (discussion.isEmpty()) wrapper
      else wrapper.with("chatMessage" to message.with("details" to details.with(
        "codeDiscussion" to discussion.with("anchor" to JsonObject(discussion.obj("anchor") - "line" - "oldLine")),
      )))
    }
    listOf(feed.withBody(feed.response.body.with("data" to JsonArray(data))))
  }

  /** Review 222772 with Space saying its discussion's history is limited. */
  fun limitedDiscussionHistory() = oneDiscussion.answering("GET", "/chats/messages") { page ->
    listOf(page.withBody(page.response.body.with("orgLimitReached" to JsonPrimitive(true))))
  }

  /** Review 222772 with its feed saying more follow, under the cursor `0` it was read with. */
  fun stalledFeed() = oneDiscussion.answering("GET", "/sync-batch") { feed ->
    listOf(feed.withBody(feed.response.body.with("hasMore" to JsonPrimitive(true), "etag" to JsonPrimitive("0"))))
  }

  /** Review 222772 with the first element of its feed null. */
  fun malformedFeedElement() = oneDiscussion.answering("GET", "/sync-batch") { feed ->
    val data = feed.response.body.arrayAt("data")
    listOf(feed.withBody(feed.response.body.with("data" to JsonArray(listOf(JsonNull) + data.drop(1)))))
  }

  /** The first two reviews merged on 2026-11-01, with null, a reviewless element and a repeat first on the page. */
  fun malformedDiscovery() = firstTwoMerged.answering("GET", "/code-reviews") { page ->
    val data = page.response.body.arrayAt("data")
    listOf(page.withBody(page.response.body.with("data" to JsonArray(listOf(JsonNull, JsonObject(emptyMap()), data[0]) + data))))
  }

  /** ULTIMATE_OPEN_REVIEW with its feed page omitting `hasMore`. */
  fun incompleteFeed() = ultimateRecording("evidence-open").answering("GET", "/sync-batch") { feed ->
    listOf(feed.withBody(JsonObject(feed.response.body.jsonObject - "hasMore")))
  }

  // Review 222772: one code discussion with one person's message, among its author's events.
  private val oneDiscussion = ultimateRecording("fetch-one-discussion-222772")

  // The first two reviews merged on 2026-11-01, the second one on discovery page two.
  private val firstTwoMerged = ultimateRecording("fetch-first-two-merged-on-2026-11-01")

  // ULTIMATE_INSPECTION_REQUEST opened from a new branch at the real head of RELEASE_191; scripted from the first write on.
  private val inspectionReviewOpened = ultimateRecording("inspection-review-opened")

  // ULTIMATE_SOURCE_FIX_REQUEST opened from a new branch at the real head of RELEASE_191; scripted from the first write on.
  private val sourceFixReviewOpened = ultimateRecording("source-fix-review-opened")

  private val openReviewSearch = space.reviewSearch(INSPECTION_BRANCH, RELEASE_191, "Opened")

  private val newReviewerAssignment = space.reviewerAssignment(ULTIMATE_NEW_REVIEW_ID, ULTIMATE_REVIEWER)

  /** The recorded call up to the inspection commit. */
  private fun inspectionCommitted(): Array<Exchange> = inspectionReviewOpened.until(openReviewSearch)

  /** The recorded call from the review search on: no review exists, so a new one is opened. */
  private fun reviewOpened(): Array<Exchange> =
    inspectionReviewOpened.exchanges.drop(inspectionReviewOpened.until(openReviewSearch).size).toTypedArray()

  /** The reads of [path] at [revision], where it holds [content], stored as the Git blob of that content. */
  private fun fileRead(revision: String, path: String, content: String): Array<Exchange> {
    val blob = gitBlob(content)
    return arrayOf(
      space.fileList(revision, path) answeredWith space.files(path, blob),
      space.fileContent(blob) answeredWith space.content(content),
    )
  }

  /** The exchanges of [recording] after its first, a read that found no request branch, without creating the branch. */
  private fun withoutBranchCreation(recording: Recording<ReviewApi>): Array<Exchange> =
    recording.exchanges.drop(1).filterNot { it.request.endpoint.endsWith("/head") }.toTypedArray()
}

/** The array under [key] in this object. */
private fun JsonElement.arrayAt(key: String): JsonArray = jsonObject.getValue(key).jsonArray

/** A review already opened from a request's branch, found instead of opening another. */
internal val ULTIMATE_EXISTING_REVIEW: ReviewReference =
  ULTIMATE.reference("4ExIsT1nGrV0", "https://jetbrains.team/p/ij/reviews/239999")
private const val EXISTING_REVIEW_NUMBER = 239999

/** Made-up commits on the request branches. */
private const val INSPECTION_COMMIT = "4d1c7e0ba5e0f1e5a9d3c2b1a0f9e8d7c6b5a4f3"
private const val SOURCE_FIX_COMMIT = "5f1e0d9c8b7a69584736251403f2e1d0c9b8a796"

/** The id Git gives a blob of [content]: the SHA-1 hex of its header and bytes. */
private fun gitBlob(content: String): String {
  val bytes = content.encodeToByteArray()
  val digest = MessageDigest.getInstance("SHA-1").digest("blob ${bytes.size}\u0000".encodeToByteArray() + bytes)
  return HexFormat.of().formatHex(digest)
}

/** The exchanges of this recording before [request], which it must send. */
private fun Recording<*>.until(request: ProviderRequest): Array<Exchange> {
  val index = exchanges.indexOfFirst { it.request == request }
  require(index >= 0) { "The recording sends no $request" }
  return exchanges.take(index).toTypedArray()
}

/** Space's errors, as the cases need them. */
internal val INTERNAL_ERROR = spaceError(500, "internal-error", "Internal Server Error")
internal val WRITE_DENIED = spaceError(403, "permission-denied", "Permission is not granted: VcsRepository.Write")
internal val REVIEW_DENIED = spaceError(403, "permission-denied", "Permission is not granted: CodeReview.Edit")
internal val BRANCH_CONFLICT = spaceError(409, "conflict", "Branch was updated")

/** A commit Space reports as failed in a successful response. */
internal val COMMIT_FAILED = ULTIMATE_SPACE.commitResult(success = false, message = "Branch changed concurrently")

/** Space's answer to a participant that is no profile: the client then tries the value as a username. */
private val PROFILE_NOT_FOUND = spaceError(404, "not-found", "Profile not found")

