// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.integration.support

import org.jetbrains.qodana.edict.ci.*
import org.jetbrains.qodana.edict.common.EdictConfiguration
import org.jetbrains.qodana.edict.common.PromotionConfiguration
import org.jetbrains.qodana.edict.support.FakeReviewApi

internal object WeakSignalPromotionFixture {
    const val creationRevision = "97f52e37ebd84ba1c2a77b419a0dde672a886cbb"
    const val reviewRevision = "285607cf907ca99328b66ae0f223fcfa297d5ed4"
    const val project = "testPromoteWeakSignals"
    const val clusterId = "avoid-delay-before-future-get"
    const val sourcePath = "$project/src/main/java/com/mycompany/app/FutureWaiter.java"

    /** The promotion stored at [reviewRevision]. */
    val pullRequest = ReviewReference(CiProviderId.GITHUB, "owner", "repo", "42", "https://github.com/owner/repo/pull/42")
    val configuration = EdictConfiguration(
        ci = ReviewRepository(CiProviderId.GITHUB, "github.com", "owner", "repo"),
        promotion = PromotionConfiguration("reviewer", "main"),
    )

    /** The promotion repository, where [pullRequest] has [status] and, once merged, removes the delay from [sourcePath]. */
    fun reviews(status: ReviewStatus = ReviewStatus(ReviewState.OPEN)) =
        FakeReviewApi(ReviewRepository(CiProviderId.GITHUB, "github.com", "owner", "repo")).apply {
            statuses[pullRequest] = status
            merges[pullRequest] = MergedReview(
                "Remove redundant delay before Future.get", listOf(status.evidence), creationRevision, MERGED_REVISION,
                listOf(ReviewFileChange(sourcePath, sourcePath)),
            )
            files[creationRevision to sourcePath] = SOURCE
            files[MERGED_REVISION to sourcePath] = SOURCE.replace("        Thread.sleep(100);\n", "")
        }
}

private val MERGED_REVISION = "c".repeat(40)

private val SOURCE = """
    package com.mycompany.app;

    import java.util.concurrent.CompletableFuture;
    import java.util.concurrent.ExecutionException;

    public final class FutureWaiter {
        public String await(CompletableFuture<String> result) throws InterruptedException, ExecutionException {
            Thread.sleep(100);
            return result.get();
        }
    }
""".trimIndent() + "\n"
