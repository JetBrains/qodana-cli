package org.jetbrains.qodana.edict.integration

import kotlin.test.assertContains
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import org.jetbrains.qodana.edict.integration.support.IntegrationTest
import org.jetbrains.qodana.edict.integration.support.historyBefore
import org.jetbrains.qodana.edict.integration.support.historyCommit
import org.jetbrains.qodana.edict.integration.support.historyPath
import org.jetbrains.qodana.edict.integration.support.signalFiles
import org.jetbrains.qodana.edict.integration.support.verifyCommitSignals
import org.jetbrains.qodana.edict.integration.support.verifyManagedRun
import org.junit.jupiter.api.Test

class LiveCommitExtractionTest : IntegrationTest() {
    @Test
    fun `managed skills extract signals from one Distillery commit with real native workers`() {
        workspace.withEdictNextCodex("Extract signal from last commit") { store, runtime, _ ->
            val plan = assertNotNull(store.plan())
            assertEquals(2, plan.tasks.size, "One batch and one evidence worker required")
            val batch = plan.tasks.single { it.skill == "edict-batch-signal-analysis" }
            val leaf = plan.tasks.single { it.skill == "edict-signal-analysis" }
            assertEquals(batch.id, leaf.parentId)
            assertContains(leaf.prompt, historyCommit)
            verifyCommitSignals(
                workspace.repository,
                signalFiles(workspace.state.resolve("inbox")),
                listOf(org.jetbrains.qodana.edict.integration.support.CommitExpectation(historyBefore, historyCommit, historyPath, 5, 10)),
            )
            verifyManagedRun(workspace, runtime, plan)
        }
    }
}
