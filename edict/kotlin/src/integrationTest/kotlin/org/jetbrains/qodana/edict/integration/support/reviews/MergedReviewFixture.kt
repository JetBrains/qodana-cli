// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.integration.support.reviews

import org.jetbrains.qodana.edict.ci.CiProviderId
import org.jetbrains.qodana.edict.ci.PullRequest
import org.jetbrains.qodana.edict.ci.ReviewMessage
import org.jetbrains.qodana.edict.ci.ReviewRepository
import org.jetbrains.qodana.edict.ci.ReviewThread
import org.jetbrains.qodana.edict.common.GitRepository
import org.jetbrains.qodana.edict.integration.support.historyBefore
import org.jetbrains.qodana.edict.integration.support.historyCommit
import org.jetbrains.qodana.edict.integration.support.historyPath
import org.jetbrains.qodana.edict.support.FakeReviewApi
import java.time.LocalDate
import java.time.ZoneOffset
import kotlin.test.assertTrue

/**
 * Pull request 7 of `owner/repo` on [provider], merged yesterday, with one review comment on the Distillery change
 * [historyBefore]..[historyCommit]: the reviews the Live tests extract Signals from. The provider clients themselves are
 * tested against recordings of the real repositories.
 */
internal class MergedReviewFixture(provider: CiProviderId, repository: GitRepository) {
    val title = "Replace Thread.sleep with explicit worker synchronization"
    val message =
        "Please replace Thread.sleep with an explicit worker completion signal. The final commit uses CountDownLatch.await instead."
    val source = listOf(historyBefore, historyCommit).associateWith { repository.fileAt(it, historyPath) }

    /** The URL of the review comment, as the provider links it. */
    val discussionUrl = when (provider) {
        CiProviderId.GITHUB -> "https://github.com/owner/repo/pull/7#discussion_r10"
        CiProviderId.SPACE -> "https://jetbrains.team/im/review/review-id?message=root&channel=feed"
    }

    val client = FakeReviewApi(
        when (provider) {
            CiProviderId.GITHUB -> ReviewRepository(CiProviderId.GITHUB, "github.com", "owner", "repo")
            CiProviderId.SPACE -> ReviewRepository(CiProviderId.SPACE, "jetbrains.team", "owner", "repo")
        },
    ).apply {
        val url = when (provider) {
            CiProviderId.GITHUB -> "https://github.com/owner/repo/pull/7"
            CiProviderId.SPACE -> "https://jetbrains.team/p/owner/reviews/7"
        }
        val mergedAt = LocalDate.now(ZoneOffset.UTC).minusDays(1).atTime(12, 0).toInstant(ZoneOffset.UTC).toEpochMilli()
        val comment = ReviewMessage("reviewer", message, "2026-09-20T12:00:00Z")
        val threadId = when (provider) {
            CiProviderId.GITHUB -> "github-7-10"
            CiProviderId.SPACE -> "space-7-root"
        }
        val thread = ReviewThread(threadId, discussionUrl, historyPath, historyBefore, 5, 5, listOf(comment))
        pullRequests += PullRequest(7, url, title, "Correct worker synchronization", historyBefore, historyCommit, mergedAt, listOf(thread))
        source.forEach { (revision, text) -> files[revision to historyPath] = text }
    }

    /** Asserts that the run fetched reviews; the agents choose the selection, so its dates are outside the test's control. */
    fun verifyRequests() {
        assertTrue(client.selections.isNotEmpty(), "The run must fetch the configured reviews")
    }
}
