package org.jetbrains.qodana.edict.edictnext

import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState.Delegation
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState.Step
import org.jetbrains.qodana.edict.support.batch
import org.jetbrains.qodana.edict.support.launch
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Files
import java.nio.file.Path
import kotlin.test.assertEquals
import kotlin.test.assertFails
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class EdictNextRepositoryStateTest {
  @TempDir
  lateinit var directory: Path

  @Test
  fun `review budget survives restart and is independent for each cluster and review stage`() {
    val steps = listOf(Step("edict-next-generation", "Generate"))
    var generationId = ""
    var clusterId = ""
    val reviewSkills = listOf(
      "edict-next-inspection-code-review",
      "edict-next-weak-signal-review",
    )
    EdictNextRepositoryState.open(directory).use { state ->
      val plan = state.createPlan("Generate", steps)
      generationId = plan.plan.tasks.single().id
      val generation = state.launch(plan.token, generationId, "edict-next-generation")
      val cluster = state.addTask(generation.token, "edict-next-cluster-generation", "First cluster")
      clusterId = cluster.id
      val worker = state.launch(generation.token, cluster.id, cluster.skill)
      for (skill in reviewSkills) repeat(4) { attempt ->
        val title = if (attempt == 0) "Initial review" else "Review after repair $attempt/3"
        val review = state.addTask(worker.token, skill, title)
        val reviewer = state.launch(worker.token, review.id, skill)
        state.finishTask(reviewer.token, "completed", "Reviewed")
      }
    }
    EdictNextRepositoryState.open(directory).use { state ->
      val resumed = state.createPlan("Generate", steps)
      val generation = state.launch(resumed.token, generationId, "edict-next-generation")
      val worker = state.launch(generation.token, clusterId, "edict-next-cluster-generation")
      for (skill in reviewSkills) {
        assertTrue(
          assertFails { state.addTask(worker.token, skill, "Fifth review attempt") }
            .message.orEmpty().contains("Three review repair iterations"),
        )
      }
      repeat(4) { state.addTask(worker.token, "edict-next-code-example-overseer", "Example $it") }
      val second = state.addTask(generation.token, "edict-next-cluster-generation", "Second cluster")
      val sibling = state.launch(generation.token, second.id, second.skill)
      for (skill in reviewSkills) state.addTask(sibling.token, skill, "First review")
      assertEquals(
        4,
        state.plan()!!.tasks.count { it.parentId == clusterId && it.skill == "edict-next-inspection-code-review" },
      )
    }
  }

  @Test
  fun `worker must fetch assignment and declare matching skill before start`() {
    EdictNextRepositoryState.open(directory).use { state ->
      val plan = state.createPlan("Extract", listOf(Step("edict-batch-signal-analysis", "Commit")))
      val delegation = state.delegate(
        plan.token,
        plan.plan.tasks.single().id,
        "\$edict-batch-signal-analysis\nRead /skills/edict-batch-signal-analysis/SKILL.md. Analyze HEAD.",
      )
      assertFails { state.startTask(delegation.token, "worker", delegation.skill) }
      assertFails { state.addTask(delegation.token, "edict-signal-analysis", "Inspect") }
      assertEquals(delegation.taskId, state.readTask(delegation.token).taskId)
      assertFails { state.startTask(delegation.token, "worker", "edict_manager") }
      val started = state.startTask(delegation.token, "worker", delegation.skill)
      assertEquals(delegation.taskId, started.taskId)
      assertEquals("running", started.status)
      assertEquals(state.plan()!!.revision, started.planRevision)
      assertFails { state.startTask(delegation.token, "worker", delegation.skill) }
      assertFails { state.createPlan("Again", listOf(Step("edict-next-run", "Run"))) }
    }
  }

  @Test
  fun `plan delta returns only tasks changed after the requested revision`() {
    EdictNextRepositoryState.open(directory).use { state ->
      val created = state.createPlan("Extract", listOf(Step("edict-batch-signal-analysis", "Commit")))
      val initialRevision = created.plan.revision
      assertEquals(created.plan.tasks.map { it.id }, state.planDelta(0).tasks.map { it.id })
      assertTrue(state.planDelta(initialRevision).tasks.isEmpty())

      val batch = state.launch(created.token, created.plan.tasks.single().id, "edict-batch-signal-analysis")
      val afterStart = state.planDelta(initialRevision)
      assertEquals(listOf(batch.taskId), afterStart.tasks.map { it.id })
      assertEquals("running", afterStart.tasks.single().status)
      assertTrue(afterStart.revision > initialRevision)

      val child = state.addTask(batch.token, "edict-signal-analysis", "Inspect")
      val afterChild = state.planDelta(afterStart.revision)
      assertEquals(listOf(child.id), afterChild.tasks.map { it.id })
    }
  }

  @Test
  fun `capabilities enforce call graph and revocation`() {
    EdictNextRepositoryState.open(directory).use { state ->
      val (manager, batch) = state.batch()
      val child = state.addTask(batch.token, "edict-signal-analysis", "Inspect evidence")
      assertFails {
        state.delegate(batch.token, child.id, "\$edict-signal-analysis\n${manager.token}")
      }
      val leaf = state.launch(batch.token, child.id, child.skill)
      assertFails { state.addTask(leaf.token, "edict-next-generation", "Escalate") }
      assertFails { state.finishTask(batch.token, "completed", "Done") }
      state.cancelTask(manager.token, batch.taskId, "Worker lost")
      assertFails { state.readTask(leaf.token) }
      assertFails { state.finishTask(batch.token, "completed", "Done") }
      val failed = state.plan()!!.tasks
      assertTrue(failed.all { it.status == "failed" })
      assertEquals("Worker lost", failed.single { it.id == batch.taskId }.result)
      assertEquals(batch.taskId, failed.single { it.id == child.id }.blockedByTaskId)
      assertTrue(failed.single { it.id == child.id }.result.isEmpty())
      val retry = state.launch(manager.token, batch.taskId, batch.skill)
      val retriedLeaf = state.launch(retry.token, child.id, child.skill)
      state.finishTask(retriedLeaf.token, "completed", "Inspected")
      state.finishTask(retry.token, "completed", "Done")
      assertTrue(state.plan()!!.tasks.all { it.status == "completed" })
    }
  }

  @Test
  fun `state lock recovery and fresh capabilities preserve completed work`() {
    val steps = listOf(Step("edict-batch-signal-analysis", "Commit"))
    lateinit var old: Delegation
    lateinit var id: String
    EdictNextRepositoryState.open(directory).use { state ->
      assertFails { EdictNextRepositoryState.open(directory).close() }
      val manager = state.createPlan("Extract", steps)
      id = manager.plan.id
      old = state.launch(manager.token, manager.plan.tasks.single().id, steps.single().skill)
      val task = state.addTask(old.token, "edict-signal-analysis", "Evidence")
      val leaf = state.launch(old.token, task.id, task.skill)
      state.finishTask(leaf.token, "completed", "Verified")
    }
    EdictNextRepositoryState.open(directory).use { state ->
      assertFails { state.readTask(old.token) }
      assertFails { state.createPlan("Different", steps) }
      val resumed = state.createPlan("Extract", steps)
      assertEquals(id, resumed.plan.id)
      assertEquals(listOf("pending", "completed"), resumed.plan.tasks.map { it.status })
      val batch = state.launch(resumed.token, old.taskId, old.skill)
      state.finishTask(batch.token, "completed", "Resumed")
      assertFalse(Files.readString(directory.resolve("plans/$id.json")).contains(batch.token))
    }
  }
}
