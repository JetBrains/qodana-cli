// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.integration

import org.jetbrains.qodana.edict.integration.support.*
import org.junit.jupiter.api.Test
import kotlin.test.assertContains
import kotlin.test.assertEquals
import kotlin.test.assertNotNull

class LiveThreeCommitExtractionTest : IntegrationTest() {
    override val fixtureRevision = threeCommitHead
    override val fixtureProject = threeCommitProject

    @Test
    fun `managed skills extract one evidence pair from each of three Distillery commits`() {
        workspace.withCodex("Extract signals from the latest three commits.") { store, runtime, _ ->
            val expected = threeCommitExpectations()
            val plan = assertNotNull(store.plan())
            assertEquals(4, plan.tasks.size, "One batch and three independent evidence workers required")
            val batch = plan.tasks.single { it.skill == "edict-batch-signal-analysis" }
            assertEquals("", batch.parentId)
            val workers = plan.tasks.filter { it.skill == "edict-signal-analysis" }
            assertEquals(3, workers.size)
            workers.forEach { assertEquals(batch.id, it.parentId) }
            expected.forEach { commit ->
                val assigned = workers.filter { "commit-${commit.revision.take(16)}" in it.prompt }
                assertEquals(1, assigned.size, "Exactly one worker must be assigned ${commit.revision}")
                assertContains(assigned.single().prompt, commit.revision)
                assertContains(assigned.single().result, commit.revision)
            }
            verifyCommitSignals(workspace.repository, store.list("inbox").map(store::read), expected)
            verifyManagedRun(workspace, runtime, plan)
        }
    }
}
