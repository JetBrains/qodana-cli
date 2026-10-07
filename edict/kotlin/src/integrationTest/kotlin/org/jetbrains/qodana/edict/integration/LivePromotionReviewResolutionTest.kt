// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.integration

import org.jetbrains.qodana.edict.ci.*
import org.jetbrains.qodana.edict.common.EdictCIConfiguration
import org.jetbrains.qodana.edict.common.EdictConfiguration
import org.jetbrains.qodana.edict.edictnext.*
import org.jetbrains.qodana.edict.integration.support.IntegrationTest
import org.jetbrains.qodana.edict.integration.support.verifyManagedRun
import org.junit.jupiter.api.Test
import java.time.Instant
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.atomic.AtomicInteger
import kotlin.io.path.createDirectories
import kotlin.io.path.exists
import kotlin.io.path.readText
import kotlin.io.path.writeText
import kotlin.test.*

class LivePromotionReviewResolutionTest : IntegrationTest() {
    override val fixtureProject: String = "."

    @Test
    fun `managed promotion checker resolves merged and rejected reviews with real native workers`() {
        workspace.state.createDirectories()
        val repository = EdictRepository(EdictRepositoryDirectory(workspace.state))
        val merged = createGeneratedCluster(repository, "merged-rule", "1")
        val revise = createGeneratedCluster(repository, "revise-rule", "2")
        val discontinue = createGeneratedCluster(repository, "discontinue-rule", "3")
        val reviews = ResolutionReviews(
            mapOf(
                merged.pullRequest.id to ReviewStatus(ReviewState.MERGED),
                revise.pullRequest.id to ReviewStatus(
                    ReviewState.CLOSED_UNMERGED,
                    "The rule is valuable, but this version is too broad. Narrow it and generate a revised inspection.",
                ),
                discontinue.pullRequest.id to ReviewStatus(
                    ReviewState.CLOSED_UNMERGED,
                    "The proposed rule is based on a false premise. Abandon it permanently; it must not be regenerated.",
                ),
            ),
        )

        workspace.withPromotionCodex(
            prompt = "Check all edict promotion reviews",
            configuration = EdictConfiguration(
                ci = EdictCIConfiguration("https://github.com/JetBrains/qodana-cli"),
            ),
            reviews = reviews,
        ) { store, runtime, result ->
            val plan = assertNotNull(store.plan())
            assertEquals(2, plan.tasks.size, "One checker and one bounded decision worker are required")
            val checker = plan.tasks.single { it.skill == "ecict-check-promotion" }
            val decision = plan.tasks.single { it.skill == "edict-promotion-decision" }
            assertEquals("", checker.parentId)
            assertEquals(checker.id, decision.parentId)
            assertEquals("completed", checker.status)
            assertEquals("completed", decision.status)
            assertTrue(result.isNotBlank())

            val acceptedCluster = repository.loadCluster("merged-rule")
            assertEquals(EdictNextClusterStatus.Generated, acceptedCluster.manifest.status)
            assertEquals(PromotionPrState.ACCEPTED, acceptedCluster.manifest.promotions.single().state)
            assertContains(acceptedCluster.historyPath.readText(), merged.pullRequest.url)

            val revisedCluster = repository.loadCluster("revise-rule")
            assertEquals(EdictNextClusterStatus.Pending, revisedCluster.manifest.status)
            assertEquals("revise-rule", revisedCluster.manifest.predecessorId)
            assertEquals(PromotionPrState.CLOSED, revisedCluster.manifest.promotions.single().state)
            assertTrue(repository.paths.inspectionPath("revise-rule").exists())
            assertContains(revisedCluster.historyPath.readText(), "moved cluster to Pending")

            val discontinuedCluster = repository.loadCluster("discontinue-rule")
            assertEquals(EdictNextClusterStatus.Discontinued, discontinuedCluster.manifest.status)
            assertEquals(PromotionPrState.CLOSED, discontinuedCluster.manifest.promotions.single().state)
            assertFalse(repository.paths.inspectionPath("discontinue-rule").exists())
            assertContains(discontinuedCluster.historyPath.readText(), "moved cluster to Discontinued")

            assertEquals(1, reviews.calls(merged.pullRequest.id))
            assertEquals(2, reviews.calls(revise.pullRequest.id))
            assertEquals(2, reviews.calls(discontinue.pullRequest.id))
            verifyManagedRun(workspace, runtime, plan)
        }
    }

    private fun createGeneratedCluster(
        repository: EdictRepository,
        clusterId: String,
        reviewId: String,
    ): InspectionPromotion {
        val content = "inspection for $clusterId"
        val digest = sha256Hex(content)
        val now = Instant.parse("2026-01-01T00:00:00Z").toString()
        val promotion = InspectionPromotion(
            id = "p-${reviewId.repeat(24)}",
            inspectionDigest = digest,
            targetBranch = "main",
            targetPath = "inspections/$clusterId.inspection.kts",
            pullRequest = ReviewReference(
                CiProviderId.GITHUB,
                "JetBrains",
                "qodana-cli",
                reviewId,
                "https://github.test/JetBrains/qodana-cli/pull/$reviewId",
            ),
            state = PromotionPrState.ON_REVIEW,
            createdAt = now,
            updatedAt = now,
        )
        val cluster = repository.paths.clustersDirectory.resolve(clusterId).createDirectories()
        cluster.resolve("signals").createDirectories()
        cluster.resolve("cluster.json").writeText(
            EdictNextJson.encodeToString(
                EdictNextClusterManifest(
                    clusterId,
                    EdictNextLanguage.Kotlin,
                    EdictNextClusterStatus.Generated,
                    promotions = listOf(promotion),
                ),
            )
        )
        cluster.resolve("history.md").writeText("Created.\n")
        repository.paths.inspectionsDirectory.createDirectories()
        repository.paths.inspectionPath(clusterId).writeText(content)
        return promotion
    }

    private class ResolutionReviews(
        private val reviews: Map<String, ReviewStatus>,
    ) : ReviewManagementApi {
        private val calls = ConcurrentHashMap<String, AtomicInteger>()

        override fun resolveTargetBranch(repository: ReviewRepository, configured: String?): String =
            error("Review creation is outside this scenario")

        override fun ensureReview(request: CreateReviewRequest): CreatedReview =
            error("Review creation is outside this scenario")

        override fun review(reference: ReviewReference): ReviewStatus {
            calls.computeIfAbsent(reference.id) { AtomicInteger() }.incrementAndGet()
            return reviews.getValue(reference.id)
        }

        fun calls(reviewId: String): Int = calls[reviewId]?.get() ?: 0
    }
}
