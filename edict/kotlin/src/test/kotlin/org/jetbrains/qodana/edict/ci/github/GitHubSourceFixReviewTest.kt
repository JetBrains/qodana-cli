// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci.github

import org.jetbrains.qodana.edict.ci.support.CREATED_REVIEW
import org.jetbrains.qodana.edict.ci.support.EXISTING_BRANCH
import org.jetbrains.qodana.edict.ci.support.NEW_FILE
import org.jetbrains.qodana.edict.ci.support.NEW_FILE_SOURCE_FIX_REQUEST
import org.jetbrains.qodana.edict.ci.support.RELEASE_BRANCH
import org.jetbrains.qodana.edict.ci.support.RELEASE_HEAD
import org.jetbrains.qodana.edict.ci.support.RELEASE_TREE
import org.jetbrains.qodana.edict.ci.support.SOURCE_FIX_BRANCH
import org.jetbrains.qodana.edict.ci.support.SOURCE_FIX_REQUEST
import org.jetbrains.qodana.edict.ci.support.UNCHANGED_SOURCE_FIX_REQUEST
import org.jetbrains.qodana.edict.ci.support.gitHubFailure
import org.jetbrains.qodana.edict.ci.support.kotlinRecording
import org.jetbrains.qodana.edict.ci.urlPart
import java.nio.file.NoSuchFileException
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

/**
 * Opening a source-fix review on GitHub: the run recorded against the frozen release branch of JetBrains/kotlin, and
 * variants of it with one step answered differently.
 */
internal class GitHubSourceFixReviewTest {
  @Test
  fun `all fixed files are committed at once to a new branch, keeping their modes`() {
    // SOURCE_FIX_REQUEST committed to a new branch at RELEASE_HEAD; scripted from the first write on.
    val recording = kotlinRecording("source-fix-review")

    assertEquals(CREATED_REVIEW, recording.replay { findOrCreateReview(SOURCE_FIX_REQUEST) })
  }

  @Test
  fun `a branch an earlier run left with the fixed files is reused`() {
    // source-fix-review, but an earlier run left the branch at FIXED_COMMIT with the fixes (scripted).
    val recording = kotlinRecording("source-fix-review-existing-branch")

    assertEquals(CREATED_REVIEW, recording.replay { findOrCreateReview(SOURCE_FIX_REQUEST) })
  }

  @Test
  fun `a branch with another fix is never overwritten`() {
    // source-fix-review, but the branch is at FIXED_COMMIT with another fix of FIXED_SCRIPT (scripted).
    val recording = kotlinRecording("source-fix-review-different-branch")

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { findOrCreateReview(SOURCE_FIX_REQUEST) }
    }
    assertEquals("GitHub source-fix branch already exists with different content", error.message)
  }

  @Test
  fun `a fix that leaves a file as it is stops before any write`() {
    // UNCHANGED_SOURCE_FIX_REQUEST, which leaves `.bunch` as it is.
    val recording = kotlinRecording("source-fix-review-unchanged")

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { findOrCreateReview(UNCHANGED_SOURCE_FIX_REQUEST) }
    }
    assertEquals("Source fix for '.bunch' has no changes", error.message)
  }

  // Pins current behaviour: a source fix can only change files the target revision has.
  @Test
  fun `a fix that creates a file fails as a missing file`() {
    // NEW_FILE_SOURCE_FIX_REQUEST, which creates a file.
    val recording = kotlinRecording("source-fix-review-new-file")

    val error = assertFailsWith<NoSuchFileException> {
      recording.replay { findOrCreateReview(NEW_FILE_SOURCE_FIX_REQUEST) }
    }
    assertEquals(NEW_FILE, error.message)
  }

  @Test
  fun `a failed branch lookup stops before any write`() {
    // source-fix-review, cut short by a failed read of the source-fix branch.
    val recording = kotlinRecording("source-fix-review")
      .failingAt("GET", "/heads/${urlPart(SOURCE_FIX_BRANCH)}", gitHubFailure(500, "Server Error"))

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { findOrCreateReview(SOURCE_FIX_REQUEST) }
    }
    assertEquals("GitHub branch lookup returned HTTP 500: Server Error", error.message)
  }

  @Test
  fun `a failed lookup of the base commit stops before any write`() {
    // source-fix-review, cut short by a failed read of the base commit.
    val recording = kotlinRecording("source-fix-review")
      .failingAt("GET", "/git/commits/$RELEASE_HEAD", gitHubFailure(500, "Server Error"))

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { findOrCreateReview(SOURCE_FIX_REQUEST) }
    }
    assertEquals("GitHub commit read returned HTTP 500: Server Error", error.message)
  }

  @Test
  fun `a failed lookup of the file modes stops before any write`() {
    // source-fix-review, cut short by a failed read of the base tree.
    val recording = kotlinRecording("source-fix-review")
      .failingAt("GET", "/git/trees/$RELEASE_TREE", gitHubFailure(500, "Server Error"))

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { findOrCreateReview(SOURCE_FIX_REQUEST) }
    }
    assertEquals("GitHub tree read returned HTTP 500: Server Error", error.message)
  }

  @Test
  fun `a branch creation GitHub refuses with 422 fails before opening the review`() {
    // source-fix-review, but GitHub refuses the branch with 422 "Reference already exists".
    val recording = kotlinRecording("source-fix-review").failingAt("POST", "/git/refs", EXISTING_BRANCH)

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { findOrCreateReview(SOURCE_FIX_REQUEST) }
    }
    assertEquals("GitHub branch creation returned HTTP 422: Reference already exists", error.message)
  }
}
