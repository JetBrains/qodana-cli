// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci.github

import kotlinx.serialization.json.JsonObject
import org.jetbrains.qodana.edict.ci.CiHttpResponse
import org.jetbrains.qodana.edict.ci.support.CREATED_REVIEW
import org.jetbrains.qodana.edict.ci.support.EXISTING_REVIEW
import org.jetbrains.qodana.edict.ci.support.INSPECTION_BRANCH
import org.jetbrains.qodana.edict.ci.support.INSPECTION_PATH
import org.jetbrains.qodana.edict.ci.support.INSPECTION_REQUEST
import org.jetbrains.qodana.edict.ci.support.MISSING_TARGET_REQUEST
import org.jetbrains.qodana.edict.ci.support.NOT_COLLABORATOR
import org.jetbrains.qodana.edict.ci.support.PULL_REQUEST_EXISTS
import org.jetbrains.qodana.edict.ci.support.RELEASE_BRANCH
import org.jetbrains.qodana.edict.ci.support.RELEASE_HEAD
import org.jetbrains.qodana.edict.ci.support.RELEASE_PARENT
import org.jetbrains.qodana.edict.ci.support.answer
import org.jetbrains.qodana.edict.ci.support.gitHubFailure
import org.jetbrains.qodana.edict.ci.support.EXISTING_BRANCH
import org.jetbrains.qodana.edict.ci.support.kotlinRecording
import org.jetbrains.qodana.edict.ci.urlPart
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

/**
 * Opening an inspection review on GitHub: the run recorded against the frozen release branch of JetBrains/kotlin, and
 * variants of it with one step answered differently.
 */
internal class GitHubInspectionReviewTest {
  @Test
  fun `a review is opened from a new branch at the target head`() {
    // INSPECTION_REQUEST opened from a new branch at RELEASE_HEAD; scripted from the first write on.
    val recording = kotlinRecording("inspection-review")

    assertEquals(CREATED_REVIEW, recording.replay { findOrCreateReview(INSPECTION_REQUEST) })
  }

  @Test
  fun `an existing branch is reused`() {
    // inspection-review, but the branch exists already (scripted).
    val recording = kotlinRecording("inspection-review-existing-branch")

    assertEquals(CREATED_REVIEW, recording.replay { findOrCreateReview(INSPECTION_REQUEST) })
  }

  @Test
  fun `a branch creation GitHub refuses with 422 fails before committing`() {
    // inspection-review, but GitHub refuses the branch with 422 "Reference already exists".
    val recording = kotlinRecording("inspection-review").failingAt("POST", "/git/refs", EXISTING_BRANCH)

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { findOrCreateReview(INSPECTION_REQUEST) }
    }
    assertEquals("GitHub branch creation returned HTTP 422: Reference already exists", error.message)
  }

  @Test
  fun `a failed branch lookup stops before any write`() {
    // inspection-review, cut short by a failed read of the inspection branch.
    val recording = kotlinRecording("inspection-review")
      .failingAt("GET", "/heads/${urlPart(INSPECTION_BRANCH)}", gitHubFailure(500, "Server Error"))

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { findOrCreateReview(INSPECTION_REQUEST) }
    }
    assertEquals("GitHub branch lookup returned HTTP 500: Server Error", error.message)
  }

  @Test
  fun `a missing target branch stops before creating the branch`() {
    // MISSING_TARGET_REQUEST, against a branch the repository doesn't have.
    val recording = kotlinRecording("inspection-review-missing-target")

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { findOrCreateReview(MISSING_TARGET_REQUEST) }
    }
    assertEquals("GitHub target branch lookup returned HTTP 404: Not Found", error.message)
  }

  @Test
  fun `a target branch without a head stops before creating the branch`() {
    // inspection-review, but GitHub answers the target branch without its head, which it never documents.
    val recording = kotlinRecording("inspection-review")
      .failingAt("GET", "/heads/$RELEASE_BRANCH", answer(200, """{"object":{}}"""))

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { findOrCreateReview(INSPECTION_REQUEST) }
    }
    assertEquals("GitHub target branch has no head revision", error.message)
  }

  @Test
  fun `an inspection already on the branch is not committed again`() {
    // An interrupted run left the branch with this inspection committed and no review yet (scripted reads).
    val recording = kotlinRecording("inspection-review-same-inspection")

    assertEquals(CREATED_REVIEW, recording.replay { findOrCreateReview(INSPECTION_REQUEST) })
  }

  @Test
  fun `another inspection on the branch is replaced`() {
    // An earlier promotion left the branch holding another inspection, which the commit replaces by its blob.
    val recording = kotlinRecording("inspection-review-other-inspection")

    assertEquals(CREATED_REVIEW, recording.replay { findOrCreateReview(INSPECTION_REQUEST) })
  }

  @Test
  fun `a failed inspection lookup stops before committing`() {
    // inspection-review, cut short by a failed read of the inspection on the branch.
    val recording = kotlinRecording("inspection-review")
      .failingAt("GET", "/contents/$INSPECTION_PATH", gitHubFailure(500, "Server Error"))

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { findOrCreateReview(INSPECTION_REQUEST) }
    }
    assertEquals("GitHub target file lookup returned HTTP 500: Server Error", error.message)
  }

  @Test
  fun `a refused inspection commit stops before opening the review`() {
    // inspection-review, but GitHub refuses the commit because the branch moved meanwhile.
    val recording = kotlinRecording("inspection-review")
      .failingAt("PUT", "/contents/$INSPECTION_PATH", gitHubFailure(409, BRANCH_MOVED))

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { findOrCreateReview(INSPECTION_REQUEST) }
    }
    assertEquals("GitHub inspection commit returned HTTP 409: $BRANCH_MOVED", error.message)
  }

  @Test
  fun `an open review from the branch is reused and gets the reviewer`() {
    // An interrupted run committed the inspection and opened PR EXISTING_PULL_REQUEST, but requested no reviewer.
    val recording = kotlinRecording("inspection-review-open-review")

    assertEquals(EXISTING_REVIEW, recording.replay { findOrCreateReview(INSPECTION_REQUEST) })
  }

  @Test
  fun `a merged review from the branch is reused without a reviewer request`() {
    // PR EXISTING_PULL_REQUEST from the branch was merged and the branch deleted, so the branch is created again.
    val recording = kotlinRecording("inspection-review-merged-review")

    assertEquals(EXISTING_REVIEW, recording.replay { findOrCreateReview(INSPECTION_REQUEST) })
  }

  @Test
  fun `a review opened by its reviewer gets no reviewer request`() {
    // inspection-review, but the token runs as REVIEWER, so GitHub names REVIEWER as the PR's author.
    val recording = kotlinRecording("inspection-review-opened-by-reviewer")

    assertEquals(CREATED_REVIEW, recording.replay { findOrCreateReview(INSPECTION_REQUEST) })
  }

  @Test
  fun `the first of several reviews from the branch is reused`() {
    // inspection-review, but PRs EXISTING_PULL_REQUEST and OLDER_PULL_REQUEST are from the branch (scripted).
    val recording = kotlinRecording("inspection-review-several-reviews")

    assertEquals(EXISTING_REVIEW, recording.replay { findOrCreateReview(INSPECTION_REQUEST) })
  }

  @Test
  fun `a failed review lookup stops before opening a review`() {
    // inspection-review, cut short by a failed search for pull requests from the branch.
    val recording = kotlinRecording("inspection-review").failingAt("GET", "/pulls", gitHubFailure(500, "Server Error"))

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { findOrCreateReview(INSPECTION_REQUEST) }
    }
    assertEquals("GitHub pull-request lookup returned HTTP 500: Server Error", error.message)
  }

  // Pins current behaviour: a review opened meanwhile by another run is not looked up again.
  @Test
  fun `a review opened meanwhile by another run fails the creation`() {
    // inspection-review, but another run opened a pull request from the branch meanwhile.
    val recording = kotlinRecording("inspection-review").failingAt("POST", "/pulls", PULL_REQUEST_EXISTS)

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { findOrCreateReview(INSPECTION_REQUEST) }
    }
    assertEquals("GitHub pull-request creation returned HTTP 422: Validation Failed", error.message)
  }

  @Test
  fun `a created review without a number fails before requesting the reviewer`() {
    // inspection-review, but GitHub answers the creation without the pull request, which it never documents.
    val recording = kotlinRecording("inspection-review")
      .failingAt("POST", "/pulls", CiHttpResponse(201, JsonObject(emptyMap())))

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { findOrCreateReview(INSPECTION_REQUEST) }
    }
    assertEquals("GitHub pull request has no number", error.message)
  }

  @Test
  fun `a reviewer who is not a collaborator fails`() {
    // inspection-review, but GitHub refuses the reviewer with 422: not a collaborator.
    val recording = kotlinRecording("inspection-review")
      .failingAt("POST", "/requested_reviewers", gitHubFailure(422, NOT_COLLABORATOR))

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { findOrCreateReview(INSPECTION_REQUEST) }
    }
    assertEquals("GitHub reviewer assignment returned HTTP 422: $NOT_COLLABORATOR", error.message)
  }
}

/** GitHub's message refusing a commit to a branch that moved meanwhile. */
private const val BRANCH_MOVED = "is at $RELEASE_PARENT but expected $RELEASE_HEAD"
