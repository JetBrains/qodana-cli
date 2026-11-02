// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci.support

import org.jetbrains.qodana.edict.ci.ReviewReference
import java.security.MessageDigest

/** The requests the Space client sends to [ULTIMATE] and Space's answers, for scripting what the real repository lacks. */
internal val ULTIMATE_SPACE = SpaceExchanges(ULTIMATE)

/**
 * Calls of the Space client on states IJ/ultimate doesn't have, such as a branch an earlier run left: the recorded
 * calls with the steps that state changes answered by script.
 */
internal object UltimateScripts {
  private val space = ULTIMATE_SPACE

  /** The inspection branch exists at a commit that already holds the inspection. */
  fun inspectionAlreadyOnBranch() = scripted(
    space.branchRead(ULTIMATE_INSPECTION_BRANCH) answeredWith space.branch(ULTIMATE_INSPECTION_BRANCH, INSPECTION_COMMIT),
    *fileRead(INSPECTION_COMMIT, ULTIMATE_INSPECTION_PATH, ULTIMATE_INSPECTION_CONTENT),
    *reviewOpened(),
  )

  /** The inspection branch exists at the target head without the inspection: a run stopped before committing. */
  fun inspectionBranchLeftAtTarget() = scripted(
    space.branchRead(ULTIMATE_INSPECTION_BRANCH) answeredWith space.branch(ULTIMATE_INSPECTION_BRANCH, RELEASE_191_HEAD),
    space.fileList(RELEASE_191_HEAD, ULTIMATE_INSPECTION_PATH) answeredWith space.noFiles(),
    *withoutBranchCreation(inspectionReviewOpened),
  )

  /** The inspection branch exists at another commit, which holds another inspection. */
  fun inspectionBranchWithOtherContent() = scripted(
    space.branchRead(ULTIMATE_INSPECTION_BRANCH) answeredWith space.branch(ULTIMATE_INSPECTION_BRANCH, INSPECTION_COMMIT),
    *fileRead(INSPECTION_COMMIT, ULTIMATE_INSPECTION_PATH, "val previousInspection = 1\n"),
    space.branchRead(RELEASE_191) answeredWith space.branch(RELEASE_191, RELEASE_191_HEAD),
  )

  /** The inspection branch exists, but Space names no commit for it. */
  fun inspectionBranchWithoutRevision() = scripted(
    space.branchRead(ULTIMATE_INSPECTION_BRANCH) answeredWith space.branch(ULTIMATE_INSPECTION_BRANCH, ""),
  )

  /** An open review from the inspection branch exists, listed after a review of the same branch into another target. */
  fun inspectionReviewAlreadyOpen() = scripted(
    *inspectionCommitted(),
    space.reviewSearch(ULTIMATE_INSPECTION_BRANCH, RELEASE_191, "Opened") answeredWith space.reviews(
      space.review("1OtHeR0TaRgT", 239998, ULTIMATE_INSPECTION_BRANCH, "master"),
      space.review(ULTIMATE_EXISTING_REVIEW.id, EXISTING_REVIEW_NUMBER, "refs/heads/$ULTIMATE_INSPECTION_BRANCH", RELEASE_191),
    ),
    space.reviewerAssignment(ULTIMATE_EXISTING_REVIEW.id, ULTIMATE_REVIEWER) answeredWith spaceEmpty(),
  )

  /** A closed review from the inspection branch exists, and no open one. */
  fun inspectionReviewClosedBefore() = scripted(
    *inspectionCommitted(),
    space.reviewSearch(ULTIMATE_INSPECTION_BRANCH, RELEASE_191, "Opened") answeredWith space.reviews(),
    space.reviewSearch(ULTIMATE_INSPECTION_BRANCH, RELEASE_191, "Closed") answeredWith space.reviews(
      space.review(ULTIMATE_EXISTING_REVIEW.id, EXISTING_REVIEW_NUMBER, ULTIMATE_INSPECTION_BRANCH, "refs/heads/$RELEASE_191"),
    ),
    space.reviewerAssignment(ULTIMATE_EXISTING_REVIEW.id, ULTIMATE_REVIEWER) answeredWith spaceEmpty(),
  )

  /** The reviewer is no profile id, so Space answers 404, and is then assigned by username. */
  fun inspectionReviewForUsername() = scripted(
    *inspectionReviewOpened.exchanges.dropLast(1).toTypedArray(),
    space.reviewerAssignment(ULTIMATE_NEW_REVIEW_ID, ULTIMATE_REVIEWER) answeredWith PROFILE_NOT_FOUND,
    space.reviewerAssignment(ULTIMATE_NEW_REVIEW_ID, "username:$ULTIMATE_REVIEWER") answeredWith spaceEmpty(),
  )

  /** The reviewer is neither a profile id nor a username. */
  fun inspectionReviewForUnknownReviewer() = scripted(
    *inspectionReviewOpened.exchanges.dropLast(1).toTypedArray(),
    space.reviewerAssignment(ULTIMATE_NEW_REVIEW_ID, ULTIMATE_REVIEWER) answeredWith PROFILE_NOT_FOUND,
    space.reviewerAssignment(ULTIMATE_NEW_REVIEW_ID, "username:$ULTIMATE_REVIEWER") answeredWith PROFILE_NOT_FOUND,
  )

  /** The reviewer, configured as a `username:` selector, doesn't exist. */
  fun inspectionReviewForUnknownUsername() = scripted(
    *inspectionReviewOpened.exchanges.dropLast(1).toTypedArray(),
    space.reviewerAssignment(ULTIMATE_NEW_REVIEW_ID, "username:$ULTIMATE_REVIEWER") answeredWith PROFILE_NOT_FOUND,
  )

  /** The source-fix branch exists at the base without the fix: a run stopped before committing. */
  fun sourceFixBranchLeftAtBase() = scripted(
    space.branchRead(ULTIMATE_SOURCE_FIX_BRANCH) answeredWith space.branch(ULTIMATE_SOURCE_FIX_BRANCH, RELEASE_191_HEAD),
    *withoutBranchCreation(sourceFixReviewOpened),
  )

  /** The source-fix branch exists at a commit with the fix, from which a review was opened. */
  fun sourceFixAlreadyCommitted() = scripted(
    space.branchRead(ULTIMATE_SOURCE_FIX_BRANCH) answeredWith space.branch(ULTIMATE_SOURCE_FIX_BRANCH, SOURCE_FIX_COMMIT),
    *ULTIMATE_SOURCE_FIX.files.entries.flatMap { (path, content) -> fileRead(SOURCE_FIX_COMMIT, path, content).asList() }.toTypedArray(),
    space.reviewSearch(ULTIMATE_SOURCE_FIX_BRANCH, RELEASE_191, "Opened") answeredWith space.reviews(
      space.review(ULTIMATE_EXISTING_REVIEW.id, EXISTING_REVIEW_NUMBER, ULTIMATE_SOURCE_FIX_BRANCH, RELEASE_191),
    ),
    space.reviewerAssignment(ULTIMATE_EXISTING_REVIEW.id, ULTIMATE_REVIEWER) answeredWith spaceEmpty(),
  )

  /** The source-fix branch exists at another commit, whose [ULTIMATE_BUILD_TXT] isn't the fixed one. */
  fun sourceFixBranchWithOtherContent() = scripted(
    space.branchRead(ULTIMATE_SOURCE_FIX_BRANCH) answeredWith space.branch(ULTIMATE_SOURCE_FIX_BRANCH, SOURCE_FIX_COMMIT),
    *fileRead(SOURCE_FIX_COMMIT, ULTIMATE_BUILD_TXT, ULTIMATE_BUILD_TXT_AT_191),
  )

  // ULTIMATE_INSPECTION_REQUEST opened from a new branch at the real head of RELEASE_191; writes scripted.
  private val inspectionReviewOpened = ultimateRecording("inspection-review-opened")

  // ULTIMATE_SOURCE_FIX_REQUEST opened from a new branch at the real head of RELEASE_191; writes scripted.
  private val sourceFixReviewOpened = ultimateRecording("source-fix-review-opened")

  private val openReviewSearch = space.reviewSearch(ULTIMATE_INSPECTION_BRANCH, RELEASE_191, "Opened")

  /** The recorded call up to the inspection commit. */
  private fun inspectionCommitted(): Array<Exchange> =
    inspectionReviewOpened.exchanges.takeWhile { it.request != openReviewSearch }.toTypedArray()

  /** The recorded call from the review search on: no review exists, so a new one is opened. */
  private fun reviewOpened(): Array<Exchange> =
    inspectionReviewOpened.exchanges.dropWhile { it.request != openReviewSearch }.toTypedArray()

  /** The reads of [path] at [revision], where it holds [content], stored as the Git blob of that content. */
  private fun fileRead(revision: String, path: String, content: String): Array<Exchange> {
    val blob = gitBlob(content)
    return arrayOf(
      space.fileList(revision, path) answeredWith space.files(path, blob),
      space.fileContent(blob) answeredWith space.content(content),
    )
  }

  /** The exchanges of [recording] after its first, a read that found no request branch, without creating the branch. */
  private fun withoutBranchCreation(recording: Recording): Array<Exchange> =
    recording.exchanges.drop(1).filterNot { it.request.endpoint.endsWith("/head") }.toTypedArray()

  private fun scripted(vararg exchanges: Exchange) = ultimateScripted(*exchanges)
}

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
  return MessageDigest.getInstance("SHA-1").digest("blob ${bytes.size}\u0000".encodeToByteArray() + bytes)
    .joinToString("") { "%02x".format(it) }
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

