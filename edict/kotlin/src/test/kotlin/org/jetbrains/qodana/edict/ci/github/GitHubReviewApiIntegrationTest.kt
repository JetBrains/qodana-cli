// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci.github

import org.jetbrains.qodana.edict.ci.CiProviderId
import org.jetbrains.qodana.edict.ci.DefaultCiHttpTransport
import org.jetbrains.qodana.edict.ci.MergedReview
import org.jetbrains.qodana.edict.ci.ReviewFetchResult
import org.jetbrains.qodana.edict.ci.ReviewSelection
import org.jetbrains.qodana.edict.ci.ReviewStatus
import org.jetbrains.qodana.edict.ci.support.ADDED_BY_917
import org.jetbrains.qodana.edict.ci.support.BEFORE_6007
import org.jetbrains.qodana.edict.ci.support.BEFORE_917
import org.jetbrains.qodana.edict.ci.support.BINARY_FILE_PATH
import org.jetbrains.qodana.edict.ci.support.DIRECTORY_PATH
import org.jetbrains.qodana.edict.ci.support.FIRST_TWO_MERGED_SELECTION
import org.jetbrains.qodana.edict.ci.support.KOTLIN
import org.jetbrains.qodana.edict.ci.support.KOTLIN_BOTS
import org.jetbrains.qodana.edict.ci.support.LARGE_FILE_PATH
import org.jetbrains.qodana.edict.ci.support.MERGE_6007
import org.jetbrains.qodana.edict.ci.support.MERGE_917
import org.jetbrains.qodana.edict.ci.support.MISSING_PULL_REQUEST
import org.jetbrains.qodana.edict.ci.support.MISSING_REVIEW
import org.jetbrains.qodana.edict.ci.support.MISSING_REVISION
import org.jetbrains.qodana.edict.ci.support.MISSING_TARGET_REQUEST
import org.jetbrains.qodana.edict.ci.support.NEW_FILE
import org.jetbrains.qodana.edict.ci.support.NEW_FILE_SOURCE_FIX_REQUEST
import org.jetbrains.qodana.edict.ci.support.PATH_6007
import org.jetbrains.qodana.edict.ci.support.PULL_REQUEST_1393
import org.jetbrains.qodana.edict.ci.support.PULL_REQUEST_5976
import org.jetbrains.qodana.edict.ci.support.PULL_REQUEST_6007
import org.jetbrains.qodana.edict.ci.support.PULL_REQUEST_7091
import org.jetbrains.qodana.edict.ci.support.PULL_REQUEST_7832
import org.jetbrains.qodana.edict.ci.support.PULL_REQUEST_917
import org.jetbrains.qodana.edict.ci.support.RELEASE_BRANCH
import org.jetbrains.qodana.edict.ci.support.RELEASE_HEAD
import org.jetbrains.qodana.edict.ci.support.SEVERAL_NUMBERS_SELECTION
import org.jetbrains.qodana.edict.ci.support.UNCHANGED_SOURCE_FIX_REQUEST
import org.jetbrains.qodana.edict.ci.support.TWO_DISCOVERY_PAGES_SELECTION
import org.jetbrains.qodana.edict.ci.support.WEEKEND_SELECTION
import org.jetbrains.qodana.edict.ci.support.kotlinRecording
import org.junit.jupiter.api.Tag
import org.junit.jupiter.api.condition.EnabledIfEnvironmentVariable
import java.nio.file.NoSuchFileException
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

/**
 * The recordings the mocked GitHub tests replay must still be true of the real JetBrains/kotlin: the client sends the
 * same reads and returns the same result. Recordings with scripted answers, such as every write path's, are only
 * replayed mocked: GitHub can't answer as the script has the repository.
 */
@Tag("integration")
@EnabledIfEnvironmentVariable(named = "GITHUB_TOKEN", matches = ".+")
internal class GitHubReviewApiIntegrationTest {
  private val github = DefaultCiHttpTransport(CiProviderId.GITHUB, "https://api.github.com", System.getenv("GITHUB_TOKEN"))


  @Test
  fun `a merged pull request has its threads, each with its replies`() {
    // PR 6007, merged: a thread of seven messages and one of two, interleaved in time.
    val recording = kotlinRecording("fetch-merged-6007")

    assertEquals(recording.result<ReviewFetchResult>(), recording.replay(github) { fetch(selectionOf(6007)) })
  }

  @Test
  fun `a comment on a whole file starts a thread without anchor lines`() {
    // PR 7832: its threads include a comment on a whole file and its reply.
    val recording = kotlinRecording("fetch-file-comments-7832")

    assertEquals(recording.result<ReviewFetchResult>(), recording.replay(github) { fetch(selectionOf(7832)) })
  }

  @Test
  fun `a pull request closed without merging is left out without a problem`() {
    // PR 5976, closed without merging.
    val recording = kotlinRecording("fetch-closed-5976")

    assertEquals(recording.result<ReviewFetchResult>(), recording.replay(github) { fetch(selectionOf(5976)) })
  }

  @Test
  fun `a missing pull request is a problem that names what GitHub doesn't have`() {
    // A pull request number GitHub doesn't have.
    val recording = kotlinRecording("fetch-missing-$MISSING_PULL_REQUEST")

    assertEquals(
      recording.result<ReviewFetchResult>(),
      recording.replay(github) { fetch(selectionOf(MISSING_PULL_REQUEST)) },
    )
  }

  @Test
  fun `pull requests are fetched in the requested order, past missing and unmerged ones`() {
    // SEVERAL_NUMBERS_SELECTION: three merged PRs in neither number order, a missing one and one closed unmerged.
    val recording = kotlinRecording("fetch-several-numbers")

    assertEquals(recording.result<ReviewFetchResult>(), recording.replay(github) { fetch(SEVERAL_NUMBERS_SELECTION) })
  }

  @Test
  fun `a review bot's thread is kept, its messages marked as from a bot`() {
    // PR 7185: a thread Copilot started that nobody answered, and a thread of people.
    val recording = kotlinRecording("fetch-bot-thread-7185")

    assertEquals(recording.result<ReviewFetchResult>(), recording.replay(github) { fetch(selectionOf(7185)) })
  }

  @Test
  fun `review comments on two pages make complete threads, each opened by the body of its review`() {
    // PR 6231: 131 review comments on two pages, and reviews with bodies.
    val recording = kotlinRecording("fetch-paged-comments-6231")

    assertEquals(recording.result<ReviewFetchResult>(), recording.replay(github) { fetch(selectionOf(6231)) })
  }

  // Only the result: the discovery pages list the most recently updated pull requests, which change every day.
  @Test
  fun `pull requests merged in a date range are discovered until one was updated before the range`() {
    // WEEKEND_SELECTION: discovery stops at the first PR updated before the weekend.
    val recording = kotlinRecording("fetch-merged-on-weekend")

    assertEquals(
      recording.result<ReviewFetchResult>(),
      GitHubReviewApi(KOTLIN, github, KOTLIN_BOTS).fetch(WEEKEND_SELECTION),
    )
  }

  // Only the result, for the same reason.
  @Test
  fun `discovery reads the next page while the first lists only pull requests updated after the range`() {
    // TWO_DISCOVERY_PAGES_SELECTION: the first page lists only pull requests updated after 2026-10-28.
    val recording = kotlinRecording("fetch-two-discovery-pages")

    assertEquals(
      recording.result<ReviewFetchResult>(),
      GitHubReviewApi(KOTLIN, github, KOTLIN_BOTS).fetch(TWO_DISCOVERY_PAGES_SELECTION),
    )
  }

  // Only the result, for the same reason; it also changes if a pull request merged in the range is updated.
  @Test
  fun `discovery stops at the maximum number of pull requests`() {
    // FIRST_TWO_MERGED_SELECTION: discovery stops after 9417, the second PR merged in the range.
    val recording = kotlinRecording("fetch-first-two-merged")

    assertEquals(
      recording.result<ReviewFetchResult>(),
      GitHubReviewApi(KOTLIN, github, KOTLIN_BOTS).fetch(FIRST_TWO_MERGED_SELECTION),
    )
  }

  @Test
  fun `a file is read as it is at a revision`() {
    // PATH_6007 as PR 6007 merged it.
    val recording = kotlinRecording("file-at-revision")

    assertEquals(recording.result<String>(), recording.replay(github) { file(MERGE_6007, PATH_6007) })
  }

  @Test
  fun `a file missing at a revision is reported missing by its path`() {
    // A path missing at MERGE_6007: GitHub answers 404 "Not Found".
    val recording = kotlinRecording("file-missing-path")

    val error = assertFailsWith<NoSuchFileException> {
      recording.replay(github) { file(MERGE_6007, "core/descriptors/Missing.kt") }
    }
    assertEquals("core/descriptors/Missing.kt", error.message)
  }

  @Test
  fun `a file at a revision GitHub doesn't have fails`() {
    // PATH_6007 at a revision GitHub doesn't have: a 404 that names the ref.
    val recording = kotlinRecording("file-unknown-revision")

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay(github) { file(MISSING_REVISION, PATH_6007) }
    }
    assertEquals("GitHub file read returned HTTP 404: No commit found for the ref $MISSING_REVISION", error.message)
  }

  @Test
  fun `a directory is not read as a file`() {
    // DIRECTORY_PATH, which GitHub lists instead of returning a file.
    val recording = kotlinRecording("file-directory")

    val error = assertFailsWith<IllegalStateException> {
      recording.replay(github) { file(RELEASE_HEAD, DIRECTORY_PATH) }
    }
    assertEquals("GitHub file read did not return an object", error.message)
  }

  @Test
  fun `a file GitHub serves without content is rejected`() {
    // LARGE_FILE_PATH, which GitHub returns without content.
    val recording = kotlinRecording("file-too-large")

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay(github) { file(RELEASE_HEAD, LARGE_FILE_PATH) }
    }
    assertEquals("GitHub did not return complete base64 file content", error.message)
  }

  @Test
  fun `a binary file is rejected`() {
    // BINARY_FILE_PATH, a PNG icon.
    val recording = kotlinRecording("file-binary")

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay(github) { file(RELEASE_HEAD, BINARY_FILE_PATH) }
    }
    assertEquals("Provider file content is invalid, binary or truncated", error.message)
  }

  @Test
  fun `a changed file is diffed between the revisions`() {
    // PATH_6007 before and after PR 6007's change.
    val recording = kotlinRecording("diff-of-merged-6007")

    assertEquals(
      recording.result<String>(),
      recording.replay(github) { diff(BEFORE_6007, MERGE_6007, PATH_6007, PATH_6007) },
    )
  }

  @Test
  fun `an added file is diffed from nothing`() {
    // ADDED_BY_917, which the merge of PR 917 added, so it is missing before.
    val recording = kotlinRecording("diff-of-added-file-917")

    assertEquals(
      recording.result<String>(),
      recording.replay(github) { diff(BEFORE_917, MERGE_917, ADDED_BY_917, ADDED_BY_917) },
    )
  }

  @Test
  fun `without a configured branch reviews target the default branch`() {
    // JetBrains/kotlin, whose default branch is master.
    val recording = kotlinRecording("default-branch")

    assertEquals(recording.result<String>(), recording.replay(github) { resolveTargetBranch(null) })
  }

  @Test
  fun `a pull request merged on GitHub is merged although GitHub closed it`() {
    // PR 6007, merged: GitHub reports it closed, with merged_at.
    val recording = kotlinRecording("review-merged-6007")

    assertEquals(recording.result<ReviewStatus>(), recording.replay(github) { review(PULL_REQUEST_6007) })
  }

  @Test
  fun `a pull request closed without merging is closed unmerged`() {
    // PR 5976, closed without merging.
    val recording = kotlinRecording("review-closed-5976")

    assertEquals(recording.result<ReviewStatus>(), recording.replay(github) { review(PULL_REQUEST_5976) })
  }

  @Test
  fun `an open pull request is open`() {
    // PR 1393, still open.
    val recording = kotlinRecording("review-open-1393")

    assertEquals(recording.result<ReviewStatus>(), recording.replay(github) { review(PULL_REQUEST_1393) })
  }

  @Test
  fun `a missing pull request fails`() {
    // A pull request number GitHub doesn't have.
    val recording = kotlinRecording("review-missing-$MISSING_PULL_REQUEST")

    val error = assertFailsWith<IllegalArgumentException> { recording.replay(github) { review(MISSING_REVIEW) } }
    assertEquals("GitHub pull-request read returned HTTP 404: Not Found", error.message)
  }

  @Test
  fun `evidence is the discussion, its reviews and their comments without the bots`() {
    // PR 6007's discussion: people, the deleted `ghost`, kotlin-safemerge[bot] and the user KotlinBuild.
    val recording = kotlinRecording("evidence-merged-6007")

    assertEquals(recording.result<ReviewStatus>(), recording.replay(github) { reviewWithEvidence(PULL_REQUEST_6007) })
  }

  @Test
  fun `a pull request closed without merging has its evidence`() {
    // PR 5976, closed without merging, with its discussion.
    val recording = kotlinRecording("evidence-closed-5976")

    assertEquals(recording.result<ReviewStatus>(), recording.replay(github) { reviewWithEvidence(PULL_REQUEST_5976) })
  }

  @Test
  fun `an open pull request has its evidence`() {
    // PR 1393, open, with its discussion.
    val recording = kotlinRecording("evidence-open-1393")

    assertEquals(recording.result<ReviewStatus>(), recording.replay(github) { reviewWithEvidence(PULL_REQUEST_1393) })
  }

  @Test
  fun `a single rebased commit is the merged change, from its parent`() {
    // PR 6007: one commit rebased onto its target, with the discussion of evidence-merged-6007.
    val recording = kotlinRecording("merged-review-6007")

    assertEquals(recording.result<MergedReview>(), recording.replay(github) { mergedReview(PULL_REQUEST_6007) })
  }

  @Test
  fun `a merge commit is the merged change, from its first parent`() {
    // PR 917: three commits merged by a merge commit.
    val recording = kotlinRecording("merged-review-merge-commit-917")

    assertEquals(recording.result<MergedReview>(), recording.replay(github) { mergedReview(PULL_REQUEST_917) })
  }

  @Test
  fun `a merged change lists the files of every page with their previous paths`() {
    // PR 7091: its merge commit lists 128 files on two pages and renames 7 of them.
    val recording = kotlinRecording("merged-review-paged-renames-7091")

    assertEquals(recording.result<MergedReview>(), recording.replay(github) { mergedReview(PULL_REQUEST_7091) })
  }

  @Test
  fun `several commits rebased onto the target are no merged change`() {
    // PR 7832: four commits rebased onto its target, so no single commit is the change.
    val recording = kotlinRecording("merged-review-rebased-7832")

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay(github) { mergedReview(PULL_REQUEST_7832) }
    }
    assertEquals("Cannot unambiguously identify the complete merged change for this single-parent, multi-commit PR", error.message)
  }

  @Test
  fun `a pull request closed without merging has no merged change`() {
    // PR 5976, closed without merging.
    val recording = kotlinRecording("merged-review-closed-5976")

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay(github) { mergedReview(PULL_REQUEST_5976) }
    }
    assertEquals("Review has not merged", error.message)
  }

  @Test
  fun `a missing target branch stops before creating the inspection branch`() {
    // MISSING_TARGET_REQUEST, against a branch the repository doesn't have.
    val recording = kotlinRecording("inspection-review-missing-target")

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay(github) { findOrCreateReview(MISSING_TARGET_REQUEST) }
    }
    assertEquals("GitHub target branch lookup returned HTTP 404: Not Found", error.message)
  }

  @Test
  fun `a fix that leaves a file as it is stops before any write`() {
    // UNCHANGED_SOURCE_FIX_REQUEST, which leaves `.bunch` as it is.
    val recording = kotlinRecording("source-fix-review-unchanged")

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay(github) { findOrCreateReview(UNCHANGED_SOURCE_FIX_REQUEST) }
    }
    assertEquals("Source fix for '.bunch' has no changes", error.message)
  }

  @Test
  fun `a fix that creates a file fails as a missing file`() {
    // NEW_FILE_SOURCE_FIX_REQUEST, which creates a file.
    val recording = kotlinRecording("source-fix-review-new-file")

    val error = assertFailsWith<NoSuchFileException> {
      recording.replay(github) { findOrCreateReview(NEW_FILE_SOURCE_FIX_REQUEST) }
    }
    assertEquals(NEW_FILE, error.message)
  }
}

private fun selectionOf(number: Int) = ReviewSelection(maxPrs = 1, prNumbers = listOf(number))
