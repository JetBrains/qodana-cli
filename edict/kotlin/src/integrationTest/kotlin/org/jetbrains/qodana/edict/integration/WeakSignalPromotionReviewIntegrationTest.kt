// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.integration

import org.jetbrains.qodana.edict.ci.ReviewState
import org.jetbrains.qodana.edict.ci.ReviewStatus
import org.jetbrains.qodana.edict.edictnext.*
import org.jetbrains.qodana.edict.integration.support.IntegrationTest
import org.jetbrains.qodana.edict.integration.support.WeakSignalPromotionFixture as Fixture
import org.jetbrains.qodana.edict.integration.support.verifyManagedRun
import org.junit.jupiter.api.Test
import kotlin.io.path.readText
import kotlin.test.*

class WeakSignalPromotionReviewIntegrationTest : IntegrationTest() {
    override val fixtureRevision = Fixture.reviewRevision
    override val fixtureProject = Fixture.project
    override val fixtureStatePath = "$fixtureProject/.edict"

    @Test
    fun `check promotion reviews and add actual merged evidence to the originating cluster`() {
        val repository = EdictRepository(EdictRepositoryDirectory(workspace.state))
        val original = repository.loadCluster(Fixture.clusterId)
        val previous = repository.loadActiveWeakSignalPromotions()[Fixture.clusterId].orEmpty().single()
        assertEquals(PromotionPrState.ON_REVIEW, previous.state)
        val inspection = repository.paths.inspectionPath(Fixture.clusterId).readText()
        val feedback = "Removing the redundant delay is correct: get still waits for the future. Accepted."
        val reviews = Fixture.Reviews(ReviewStatus(ReviewState.MERGED, feedback))

        workspace.withPromotionCodex(
            prompt = "Check weak signal promotion reviews",
            configuration = Fixture.configuration,
            reviews = reviews,
        ) { store, runtime, _ ->
            val plan = assertNotNull(store.plan())
            assertEquals(listOf("edict-check-weak-signal-promotion"), plan.tasks.map { it.skill })
            assertTrue(reviews.checked.isNotEmpty())
            assertTrue(reviews.checked.all { it == Fixture.pullRequest })
            assertTrue(reviews.requests.isEmpty(), "Checking must not create another PR")

            val cluster = repository.loadCluster(Fixture.clusterId)
            val checked = repository.loadWeakSignalPromotionHistory(Fixture.clusterId).single()
            assertTrue(repository.loadActiveWeakSignalPromotions().isEmpty())
            assertEquals(PromotionPrState.ACCEPTED, checked.state)
            assertEquals(previous, checked.copy(state = previous.state))
            assertEquals(original.manifest.status, cluster.manifest.status)
            val added = cluster.signals.filter { it.id !in original.signalIds }
            val signal = added.single() // The merged fix deletes the delay; no absent-code negative signal.
            assertEquals(EdictNextSignalLabel.POSITIVE, signal.label)
            assertEquals(Fixture.creationRevision, signal.fileRevision.revision)
            assertEquals(Fixture.sourcePath, signal.fileRevision.path)
            assertEquals(listOf(EdictNextLineRange(8, 8)), signal.fileRevision.expectedRanges)
            assertEquals(EdictNextSignalStrength.STRONG, signal.strength)
            assertContains(cluster.manifest.processedSignalIds, signal.id)
            assertEquals(original.signals.single().syntheticExampleId, signal.syntheticExampleId)
            assertEquals(exampleSetDigest(original), exampleSetDigest(cluster), "Covered evidence must preserve the evaluated example set")
            assertEquals(feedback, (signal.source as EdictNextSignalSource.FromPR).discussionMessages.single())
            assertFalse("reviewEvidence" in cluster.directory.manifestPath.readText())
            assertEquals(inspection, repository.paths.inspectionPath(Fixture.clusterId).readText())
            assertTrue(cluster.manifest.promotions.isEmpty())
            assertTrue(repository.loadInboxSignals().isEmpty(), "Feedback belongs directly to the cluster")
            verifyManagedRun(workspace, runtime, plan)
        }
    }
}
