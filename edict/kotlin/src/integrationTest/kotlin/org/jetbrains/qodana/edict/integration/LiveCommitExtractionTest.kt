package org.jetbrains.qodana.edict.integration

import org.jetbrains.qodana.edict.*

import org.junit.jupiter.api.Test
import java.nio.file.Files
import kotlin.test.*

class LiveCommitExtractionTest : IntegrationTest() {
    @Test
    fun `managed skills extract signals from one Distillery commit with real native workers`() {
        workspace.withCodex("Extract signals from the latest commit.") { store, runtime, _ ->
            val plan = assertNotNull(store.plan())
            assertEquals(2, plan.tasks.size, "One batch and one evidence worker required")
            assertTrue(plan.tasks.all { it.status == "completed" && it.agentId.isNotBlank() })
            assertEquals(2, plan.tasks.map { it.agentId }.distinct().size)
            val batch = plan.tasks.single { it.skill == "edict-batch-signal-analysis" }
            val leaf = plan.tasks.single { it.skill == "edict-signal-analysis" }
            assertEquals(batch.id, leaf.parentId)
            assertContains(leaf.prompt, historyCommit)
            val signals = store.list("inbox").map { json.decodeFromString<Signal>(store.read(it).content) }
            assertEquals(2, signals.size, "One positive and one negative signal required")
            assertEquals(setOf(SignalLabel.POSITIVE, SignalLabel.NEGATIVE), signals.map { it.label }.toSet())
            signals.forEach {
                workspace.repository.validateEvidence(it)
                assertEquals(historyCommit, it.source.commitRevision)
                assertEquals(historyBefore, it.source.parentRevision)
                assertEquals(historyPath, it.fileRevision.path)
                val changedLine = if (it.label == SignalLabel.POSITIVE) 5 else 10
                assertTrue(it.fileRevision.expectedRanges.any { range -> changedLine in range.start..range.end })
            }
            val log = Files.readString(workspace.logs.resolve("edict-mcp-system.log"))
            // Missing reads and rejected candidate validation are recoverable; failed lifecycles are not.
            val lifecycle = Regex("] edict_(?:plan_create|delegate|task_[a-z]+) ")
            val rejected = log.lineSequence().filter { lifecycle.containsMatchIn(it) && it.contains("\"isError\":true") }.toList()
            assertTrue(rejected.isEmpty(), "A managed lifecycle call failed; inspect ${workspace.logs}")
            assertFalse(log.contains("] edict_task_cancel "))
            assertFalse(log.lineSequence().any { it.contains("] edict_task_finish ") && it.substringBefore(" => ").contains("\"status\":\"failed\"") })
            verifyRuntimeWorkers(runtime.home.resolve("sessions"), plan)
            verifyAgentLogs(workspace.logs, plan)
        }
    }
}
