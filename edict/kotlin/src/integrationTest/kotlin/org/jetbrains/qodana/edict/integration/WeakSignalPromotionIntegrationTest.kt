// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.integration

import org.jetbrains.qodana.edict.edictnext.*
import org.jetbrains.qodana.edict.integration.support.IntegrationTest
import org.jetbrains.qodana.edict.integration.support.WeakSignalPromotionFixture as Fixture
import org.jetbrains.qodana.edict.integration.support.verifyManagedRun
import org.junit.jupiter.api.Test
import kotlin.test.*

class WeakSignalPromotionIntegrationTest : IntegrationTest() {
    override val fixtureRevision = Fixture.creationRevision
    override val fixtureProject = Fixture.project
    override val fixtureStatePath = "$fixtureProject/.edict"

    @Test
    fun `promote weak signals from a prepared generated inspection`() {
        val repository = EdictRepository(EdictRepositoryDirectory(workspace.state))
        val original = repository.loadCluster(Fixture.clusterId)
        val evaluation = assertNotNull(repository.loadEvaluation(original))
        val projectReview = assertNotNull(evaluation.projectReview)
        assertEquals(exampleSetDigest(original), evaluation.exampleSetDigest)
        assertTrue(repository.loadActiveWeakSignalPromotions().isEmpty())
        val reviews = Fixture.Reviews()

        workspace.withPromotionCodex(
            prompt = "Promote weak signals",
            configuration = Fixture.configuration,
            reviews = reviews,
        ) { store, runtime, _ ->
            val plan = assertNotNull(store.plan())
            assertEquals(listOf("edict-promote-weak-signal"), plan.tasks.map { it.skill })

            val request = reviews.requests.single()
            val changes = assertNotNull(request.sourceFix)
            assertEquals(Fixture.clusterId, request.clusterId)
            assertEquals("reviewer", request.reviewer)
            assertEquals("main", request.targetBranch)
            assertEquals(projectReview.projectRevision, changes.baseRevision)
            assertEquals(setOf(Fixture.sourcePath), changes.files.keys)
            val fixed = changes.files.getValue(Fixture.sourcePath)
            assertFalse("Thread.sleep" in fixed, "Remove the redundant delay")
            assertContains(fixed, "result.get()", message = "Keep waiting for the future")
            assertContains(request.description, projectReview.inspectionDescription)
            assertContains(request.description, "accept/merge this PR if you agree")
            assertContains(request.description, "## Validation")

            val cluster = repository.loadCluster(Fixture.clusterId)
            val promotion = repository.loadActiveWeakSignalPromotions()[Fixture.clusterId].orEmpty().single()
            assertEquals(Fixture.pullRequest, promotion.pullRequest)
            assertEquals(evaluation.inspectionHash, request.inspectionDigest)
            assertEquals(PromotionPrState.ON_REVIEW, promotion.state)
            assertEquals(original.manifest.status, cluster.manifest.status)
            assertEquals(original.signals, cluster.signals)
            assertTrue(cluster.manifest.promotions.isEmpty())
            assertTrue(repository.loadInboxSignals().isEmpty())
            assertTrue(reviews.checked.isEmpty())
            verifyManagedRun(workspace, runtime, plan)
        }
    }
}
