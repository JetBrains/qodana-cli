// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci.space

import org.jetbrains.qodana.edict.ci.support.BRANCH_CONFLICT
import org.jetbrains.qodana.edict.ci.support.COMMIT_FAILED
import org.jetbrains.qodana.edict.ci.support.INTERNAL_ERROR
import org.jetbrains.qodana.edict.ci.support.RELEASE_191
import org.jetbrains.qodana.edict.ci.support.REVIEW_DENIED
import org.jetbrains.qodana.edict.ci.support.ULTIMATE_EXISTING_REVIEW
import org.jetbrains.qodana.edict.ci.support.ULTIMATE_INSPECTION_BRANCH
import org.jetbrains.qodana.edict.ci.support.ULTIMATE_INSPECTION_REQUEST
import org.jetbrains.qodana.edict.ci.support.ULTIMATE_NEW_REVIEW
import org.jetbrains.qodana.edict.ci.support.ULTIMATE_NEW_REVIEW_ID
import org.jetbrains.qodana.edict.ci.support.ULTIMATE_NEW_REVIEW_NUMBER
import org.jetbrains.qodana.edict.ci.support.ULTIMATE_REVIEWER
import org.jetbrains.qodana.edict.ci.support.ULTIMATE_SPACE
import org.jetbrains.qodana.edict.ci.support.UltimateScripts
import org.jetbrains.qodana.edict.ci.support.WRITE_DENIED
import org.jetbrains.qodana.edict.ci.support.answer
import org.jetbrains.qodana.edict.ci.support.spaceEmpty
import org.jetbrains.qodana.edict.ci.support.ultimateRecording
import org.jetbrains.qodana.edict.ci.urlPart
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

/**
 * Opening an inspection review on IJ/ultimate: the regular run reads the real repository with its writes scripted;
 * each other test answers one step differently.
 */
internal class SpaceInspectionReviewTest {
  @Test
  fun `a review is opened from a new branch at the target head`() {
    // ULTIMATE_INSPECTION_REQUEST opened from a new branch at the real head of RELEASE_191; writes scripted.
    val recording = ultimateRecording("inspection-review-opened")

    assertEquals(ULTIMATE_NEW_REVIEW, recording.replay { findOrCreateReview(ULTIMATE_INSPECTION_REQUEST) })
  }

  @Test
  fun `a failed branch lookup stops before any write`() {
    // inspection-review-opened, cut short by a failed read of the inspection branch.
    val recording = ultimateRecording("inspection-review-opened")
      .failingAt("GET", "/branches/${urlPart("refs/heads/$ULTIMATE_INSPECTION_BRANCH")}", INTERNAL_ERROR)

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { findOrCreateReview(ULTIMATE_INSPECTION_REQUEST) }
    }
    assertEquals("Space branch lookup returned HTTP 500: Internal Server Error", error.message)
  }

  @Test
  fun `a branch without a revision stops before any write`() {
    // The inspection branch exists, but Space names no commit for it.
    val recording = UltimateScripts.inspectionBranchWithoutRevision()

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { findOrCreateReview(ULTIMATE_INSPECTION_REQUEST) }
    }
    assertEquals("Space inspection branch has no revision", error.message)
  }

  @Test
  fun `an inspection already on the branch is not committed again`() {
    // The inspection branch exists at a commit that already holds the inspection.
    val recording = UltimateScripts.inspectionAlreadyOnBranch()

    assertEquals(ULTIMATE_NEW_REVIEW, recording.replay { findOrCreateReview(ULTIMATE_INSPECTION_REQUEST) })
  }

  @Test
  fun `a branch left at the target head is committed to without creating it again`() {
    // The inspection branch exists at the target head without the inspection: a run stopped before committing.
    val recording = UltimateScripts.inspectionBranchLeftAtTarget()

    assertEquals(ULTIMATE_NEW_REVIEW, recording.replay { findOrCreateReview(ULTIMATE_INSPECTION_REQUEST) })
  }

  @Test
  fun `a branch with another inspection is never overwritten`() {
    // The inspection branch exists at another commit, which holds another inspection.
    val recording = UltimateScripts.inspectionBranchWithOtherContent()

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { findOrCreateReview(ULTIMATE_INSPECTION_REQUEST) }
    }
    assertEquals("Space inspection branch already exists with different content", error.message)
  }

  @Test
  fun `a missing target branch stops before creating the branch`() {
    // inspection-review-opened, but the target branch doesn't exist.
    val recording = ultimateRecording("inspection-review-opened")
      .failingAt("GET", "/branches/${urlPart("refs/heads/$RELEASE_191")}", spaceEmpty())

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { findOrCreateReview(ULTIMATE_INSPECTION_REQUEST) }
    }
    assertEquals("Space target branch does not exist", error.message)
  }

  @Test
  fun `a target branch without a revision stops before creating the branch`() {
    // inspection-review-opened, but Space names no revision for the target branch.
    val recording = ultimateRecording("inspection-review-opened")
      .failingAt("GET", "/branches/${urlPart("refs/heads/$RELEASE_191")}", ULTIMATE_SPACE.branch(RELEASE_191, ""))

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { findOrCreateReview(ULTIMATE_INSPECTION_REQUEST) }
    }
    assertEquals("Space target branch has no revision", error.message)
  }

  @Test
  fun `a refused branch creation stops before committing, with Space's reason`() {
    // inspection-review-opened, but Space denies creating the branch.
    val recording = ultimateRecording("inspection-review-opened").failingAt("POST", "/head", WRITE_DENIED)

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { findOrCreateReview(ULTIMATE_INSPECTION_REQUEST) }
    }
    assertEquals("Space inspection branch creation returned HTTP 403: Permission is not granted: VcsRepository.Write", error.message)
  }

  @Test
  fun `a refused commit stops before looking for the review`() {
    // inspection-review-opened, but Space refuses the commit because the branch was updated.
    val recording = ultimateRecording("inspection-review-opened").failingAt("POST", "/commit", BRANCH_CONFLICT)

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { findOrCreateReview(ULTIMATE_INSPECTION_REQUEST) }
    }
    assertEquals("Space API returned HTTP 409: Branch was updated", error.message)
  }

  @Test
  fun `a commit Space reports failed in a successful response stops before looking for the review`() {
    // inspection-review-opened, but Space reports the commit as failed in a successful response.
    val recording = ultimateRecording("inspection-review-opened").failingAt("POST", "/commit", COMMIT_FAILED)

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { findOrCreateReview(ULTIMATE_INSPECTION_REQUEST) }
    }
    assertEquals("Space inspection commit failed: Branch changed concurrently", error.message)
  }

  @Test
  fun `a failed review lookup stops before opening a review`() {
    // inspection-review-opened, cut short by a failed search for reviews from the branch.
    val recording = ultimateRecording("inspection-review-opened").failingAt("GET", "/code-reviews", INTERNAL_ERROR)

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { findOrCreateReview(ULTIMATE_INSPECTION_REQUEST) }
    }
    assertEquals("Space API returned HTTP 500: Internal Server Error", error.message)
  }

  @Test
  fun `a review lookup without data stops before opening a review`() {
    // inspection-review-opened, but Space answers the review search without data.
    val recording = ultimateRecording("inspection-review-opened").failingAt("GET", "/code-reviews", answer(200, "{}"))

    val error = assertFailsWith<IllegalStateException> {
      recording.replay { findOrCreateReview(ULTIMATE_INSPECTION_REQUEST) }
    }
    assertEquals("Space review lookup returned no data array", error.message)
  }

  @Test
  fun `an open review from the branch into the target is reused`() {
    // An open review from the inspection branch, listed after a review of the branch into another target.
    val recording = UltimateScripts.inspectionReviewAlreadyOpen()

    assertEquals(ULTIMATE_EXISTING_REVIEW, recording.replay { findOrCreateReview(ULTIMATE_INSPECTION_REQUEST) })
  }

  @Test
  fun `a closed review from the branch is reused`() {
    // A closed review from the inspection branch, and no open one.
    val recording = UltimateScripts.inspectionReviewClosedBefore()

    assertEquals(ULTIMATE_EXISTING_REVIEW, recording.replay { findOrCreateReview(ULTIMATE_INSPECTION_REQUEST) })
  }

  @Test
  fun `a refused review creation fails with Space's reason`() {
    // inspection-review-opened, but Space denies opening the review.
    val recording = ultimateRecording("inspection-review-opened").failingAt("POST", "/merge-requests", REVIEW_DENIED)

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { findOrCreateReview(ULTIMATE_INSPECTION_REQUEST) }
    }
    assertEquals("Space API returned HTTP 403: Permission is not granted: CodeReview.Edit", error.message)
  }

  @Test
  fun `a created review without an id fails before assigning the reviewer`() {
    // inspection-review-opened, but Space answers the new review without its id.
    val recording = ultimateRecording("inspection-review-opened")
      .failingAt("POST", "/merge-requests", answer(200, """{"number":$ULTIMATE_NEW_REVIEW_NUMBER}"""))

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { findOrCreateReview(ULTIMATE_INSPECTION_REQUEST) }
    }
    assertEquals("Space code review has no id", error.message)
  }

  @Test
  fun `a created review without a number fails before assigning the reviewer`() {
    // inspection-review-opened, but Space answers the new review without its number.
    val recording = ultimateRecording("inspection-review-opened")
      .failingAt("POST", "/merge-requests", answer(200, """{"id":"$ULTIMATE_NEW_REVIEW_ID"}"""))

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { findOrCreateReview(ULTIMATE_INSPECTION_REQUEST) }
    }
    assertEquals("Space code review has no number", error.message)
  }

  @Test
  fun `a reviewer who is no profile id is assigned by username`() {
    // The reviewer is no profile id, so Space answers 404, and is then assigned by username.
    val recording = UltimateScripts.inspectionReviewForUsername()

    assertEquals(ULTIMATE_NEW_REVIEW, recording.replay { findOrCreateReview(ULTIMATE_INSPECTION_REQUEST) })
  }

  @Test
  fun `a reviewer who is neither a profile id nor a username fails`() {
    // The reviewer is neither a profile id nor a username.
    val recording = UltimateScripts.inspectionReviewForUnknownReviewer()

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { findOrCreateReview(ULTIMATE_INSPECTION_REQUEST) }
    }
    assertEquals("Space reviewer assignment returned HTTP 404: Profile not found", error.message)
  }

  @Test
  fun `a reviewer selector that names nobody is not tried as a username`() {
    // The reviewer, configured as a `username:` selector, doesn't exist.
    val recording = UltimateScripts.inspectionReviewForUnknownUsername()

    val request = ULTIMATE_INSPECTION_REQUEST.copy(reviewer = "username:$ULTIMATE_REVIEWER")
    val error = assertFailsWith<IllegalArgumentException> { recording.replay { findOrCreateReview(request) } }
    assertEquals("Space reviewer assignment returned HTTP 404: Profile not found", error.message)
  }

  @Test
  fun `a refused reviewer assignment fails`() {
    // inspection-review-opened, but Space denies assigning the reviewer.
    val recording = ultimateRecording("inspection-review-opened")
      .failingAt("POST", "/participants/reviewer", REVIEW_DENIED)

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { findOrCreateReview(ULTIMATE_INSPECTION_REQUEST) }
    }
    assertEquals("Space reviewer assignment returned HTTP 403: Permission is not granted: CodeReview.Edit", error.message)
  }
}
