// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.integration.support

import org.jetbrains.qodana.edict.ci.*
import org.jetbrains.qodana.edict.common.EdictCIConfiguration
import org.jetbrains.qodana.edict.common.EdictConfiguration
import org.jetbrains.qodana.edict.common.PromotionConfiguration
import java.util.concurrent.CopyOnWriteArrayList

internal object WeakSignalPromotionFixture {
    const val creationRevision = "97f52e37ebd84ba1c2a77b419a0dde672a886cbb"
    const val reviewRevision = "285607cf907ca99328b66ae0f223fcfa297d5ed4"
    const val project = "testPromoteWeakSignals"
    const val clusterId = "avoid-delay-before-future-get"
    const val sourcePath = "$project/src/main/java/com/mycompany/app/FutureWaiter.java"
    val pullRequest = ReviewReference(CiProviderId.GITHUB, "owner", "repo", "42", "https://github.test/owner/repo/pull/42")
    val configuration = EdictConfiguration(
        ci = EdictCIConfiguration("https://github.com/owner/repo"),
        promotion = PromotionConfiguration("reviewer", "main"),
    )

    class Reviews(private val status: ReviewStatus = ReviewStatus(ReviewState.OPEN)) : ReviewManagementApi, ReviewExtractionApi {
        val requests = CopyOnWriteArrayList<CreateReviewRequest>()
        val checked = CopyOnWriteArrayList<ReviewReference>()

        override fun resolveTargetBranch(repository: ReviewRepository, configured: String?): String = configured ?: "main"

        override fun ensureReview(request: CreateReviewRequest): CreatedReview {
            requests += request
            return CreatedReview(pullRequest.id, pullRequest.url)
        }

        override fun review(reference: ReviewReference): ReviewStatus = error("Weak-signal PRs require reviewer evidence")

        override fun reviewWithEvidence(reference: ReviewReference): ReviewStatus {
            checked += reference
            return status
        }

        override fun mergedReview(reference: ReviewReference): MergedReview {
            check(status.state == ReviewState.MERGED)
            return MergedReview("Remove redundant delay before Future.get", listOf(status.evidence), creationRevision, "c".repeat(40),
                listOf(ReviewFileChange(sourcePath, sourcePath)))
        }

        override fun fetch(selection: ReviewSelection): ReviewFetchResult = error("Not an extraction fixture")
        override fun file(repository: ReviewRepository, revision: String, path: String): String {
            require(path == sourcePath && revision in listOf(creationRevision, "c".repeat(40)))
            return if (revision == creationRevision) before else before.replace("        Thread.sleep(100);\n", "")
        }
        override fun diff(repository: ReviewRepository, before: String, after: String, beforePath: String, afterPath: String, context: Int): String =
            "--- a/$sourcePath\n+++ b/$sourcePath\n@@ -6,6 +6,5 @@\n" +
                " public final class FutureWaiter {\n" +
                "     public String await(CompletableFuture<String> result) throws InterruptedException, ExecutionException {\n" +
                "-        Thread.sleep(100);\n" +
                "         return result.get();\n     }\n }\n"

        private val before = """
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
    }
}
