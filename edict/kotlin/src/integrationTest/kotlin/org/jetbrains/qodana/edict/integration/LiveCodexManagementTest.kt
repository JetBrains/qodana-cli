package org.jetbrains.qodana.edict.integration

import org.jetbrains.qodana.edict.integration.support.IntegrationTest
import org.jetbrains.qodana.edict.integration.support.verifyAgentLogs
import org.jetbrains.qodana.edict.integration.support.verifyRuntimeWorkers
import org.junit.jupiter.api.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

class LiveCodexManagementTest : IntegrationTest() {
    override val fixtureProject: String = "."

    @Test
    fun `real manager delegates empty distribution to one managed worker`() {
        val state = workspace.state.toAbsolutePath().normalize()
        workspace.withEdictNextCodex(
            """
            Use the edict_manager skill and its managed protocol.
            Create exactly one top-level edict-next-distribution task.
            Delegate it to a fresh native subagent and complete the managed lifecycle.
            The Edict worktree is $state and is intentionally empty: verify that it has no Signals to distribute.
            The inspected IntelliJ project is ${workspace.project.toAbsolutePath().normalize()}.
            Do not create additional tasks or edit repository files.
            """.trimIndent(),
        ) { repositoryState, runtime, result ->
            val plan = assertNotNull(repositoryState.plan())
            assertEquals(1, plan.tasks.size, "The manager must create one bounded distribution task")
            val task = plan.tasks.single()
            assertEquals("edict-next-distribution", task.skill)
            assertEquals("completed", task.status)
            assertTrue(task.agentId.isNotBlank(), "Distribution must run in a native subagent")
            assertTrue(task.result.isNotBlank(), "The worker must persist its outcome")
            assertTrue(result.isNotBlank(), "The manager must return a final response")
            verifyRuntimeWorkers(runtime.home.resolve("sessions"), plan)
            verifyAgentLogs(workspace.logs, plan)
            val price = runtime.writePriceReport(plan)
            assertTrue(price.topStages.single().inclusivePrice.totalTokens > 0, "Worker receipt must be attributed")
            assertEquals(0L, price.unattributedPrice.totalTokens, "Every managed session must be attributed")
            assertTrue(price.totalPrice.priceUsd > 0.0, "Official OpenAI rates must produce a USD total")
        }
    }
}
