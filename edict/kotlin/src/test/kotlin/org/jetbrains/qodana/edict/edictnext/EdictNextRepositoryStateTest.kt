package org.jetbrains.qodana.edict.edictnext

import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState.Delegation
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState.Step
import org.jetbrains.qodana.edict.extraction.reviews.PrAnalysisDateRange
import org.jetbrains.qodana.edict.extraction.reviews.RepositoryPrAnalysisCoverage
import org.jetbrains.qodana.edict.ci.CiProviderId
import org.jetbrains.qodana.edict.ci.ReviewRepository
import org.jetbrains.qodana.edict.support.batch
import org.jetbrains.qodana.edict.support.fixtureSignals
import org.jetbrains.qodana.edict.support.gitFixture
import org.jetbrains.qodana.edict.support.launch
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Files
import java.nio.file.Path
import kotlin.test.assertEquals
import kotlin.test.assertFails
import kotlin.test.assertFalse
import kotlin.test.assertTrue
import kotlin.reflect.KClass
import kotlin.reflect.KMutableProperty
import kotlin.reflect.KType
import kotlin.reflect.full.memberProperties
import kotlin.reflect.typeOf

class EdictNextRepositoryStateTest {
  @TempDir
  lateinit var directory: Path

  @Test
  fun `historical coordinators can publish validated commit signals`() {
    val repository = gitFixture(directory.resolve("source"))
    val signal = fixtureSignals(repository).first()
    listOf("edict-git-history-signal-analysis", "edict-retrospective-signal-analysis").forEach { skill ->
      EdictNextRepositoryState.open(directory.resolve(skill)).use { state ->
        val plan = state.createPlan("Find historical evidence", listOf(Step(skill, "Search")))
        val worker = state.launch(plan.token, plan.plan.tasks.single().id, skill)
        assertTrue(state.publishSignal(worker.token, signal, repository::validateEvidence).created)
        assertEquals(signal, state.inboxSignal(signal.id))
        assertTrue(Files.exists(directory.resolve(skill).resolve("inbox/${signal.id}.json")))
      }
    }
  }

  @Test
  fun `PR analysis coverage merges ranges and PR numbers and survives restart`() {
    val skill = "edict-pr-signal-analysis"
    val steps = listOf(Step(skill, "Reviews"))
    val repository = ReviewRepository(CiProviderId.GITHUB, "jetbrains", "qodana")
    var taskId = ""
    val expected = RepositoryPrAnalysisCoverage(
      repository,
      analyzedDateRanges = listOf(PrAnalysisDateRange("2026-01-01", "2026-01-05")),
      analyzedPrNumbers = listOf(2, 3, 9),
    )
    EdictNextRepositoryState.open(directory).use { state ->
      val plan = state.createPlan("Reviews", steps)
      taskId = plan.plan.tasks.single().id
      val worker = state.launch(plan.token, taskId, skill)
      assertEquals(RepositoryPrAnalysisCoverage(repository), state.getPrAnalysisCoverage(worker.token, repository))
      state.recordPrAnalysisCoverage(
        worker.token,
        RepositoryPrAnalysisCoverage(
          repository,
          listOf(PrAnalysisDateRange("2026-01-01", "2026-01-03")),
          listOf(9, 3, 9),
        ),
      )
      assertEquals(
        expected,
        state.recordPrAnalysisCoverage(
          worker.token,
          RepositoryPrAnalysisCoverage(
            repository,
            listOf(PrAnalysisDateRange("2026-01-04", "2026-01-05")),
            listOf(2),
          ),
        ),
      )
      assertTrue(Files.exists(directory.resolve("extraction/pr-analysis-coverage.json")))
    }
    EdictNextRepositoryState.open(directory).use { state ->
      val resumed = state.createPlan("Reviews", steps)
      val worker = state.launch(resumed.token, taskId, skill)
      assertEquals(expected, state.getPrAnalysisCoverage(worker.token, repository))
    }
  }

  @Test
  fun `plan permits only one PR analysis coordinator`() {
    EdictNextRepositoryState.open(directory).use { state ->
      val skill = "edict-pr-signal-analysis"
      assertFails {
        state.createPlan("Reviews", listOf(Step(skill, "First"), Step(skill, "Second")))
      }
      val plan = state.createPlan("Reviews", listOf(Step(skill, "Only")))
      assertFails { state.addTask(plan.token, skill, "Another") }
    }
  }

  @Test
  fun `signal publication derives storage from the model and is idempotent`() {
    EdictNextRepositoryState.open(directory).use { state ->
      val (_, batch) = state.batch()
      val key = "repository:commit:item:negative"
      val signal = EdictNextSignal(
        id = stableSignalId(key),
        idempotencyKey = key,
        fileRevision = EdictNextFileRevision("src/Example.kt", "a".repeat(40), listOf(EdictNextLineRange(1, 1))),
        source = EdictNextSignalSource.FromCommit("b".repeat(40)),
        label = EdictNextSignalLabel.NEGATIVE,
        description = "Example finding",
        provenance = EdictNextSignalProvenance("commit-item"),
      )

      assertTrue(state.publishSignal(batch.token, signal).created)
      assertFalse(state.publishSignal(batch.token, signal).created)
      assertEquals(signal, state.inboxSignal(signal.id))
      assertTrue(Files.exists(directory.resolve("inbox/${signal.id}.json")))
      assertFails { state.publishSignal(batch.token, signal.copy(description = "Conflicting finding")) }
    }
  }

  @Test
  fun `generation reviews are not limited per cluster worker`() {
    val steps = listOf(Step("edict-next-generation", "Generate"))
    var generationId = ""
    var clusterId = ""
    val reviewSkills = listOf(
      "edict-next-inspection-shallow-review",
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
        val title = if (attempt == 0) "Initial review" else "Review after repair $attempt"
        val review = state.addTask(worker.token, skill, title)
        val reviewer = state.launch(worker.token, review.id, skill)
        state.finishTask(reviewer.token, "completed", "Reviewed")
      }
    }
    EdictNextRepositoryState.open(directory).use { state ->
      val resumed = state.createPlan("Generate", steps)
      val generation = state.launch(resumed.token, generationId, "edict-next-generation")
      val worker = state.launch(generation.token, clusterId, "edict-next-cluster-generation")
      // Project analysis bounds the expensive stage; the cheap shallow review repeats as often as a cluster needs it.
      for (skill in reviewSkills) state.addTask(worker.token, skill, "Fifth review attempt")
      assertEquals(
        5,
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
      state.startTask(delegation.token, "worker", delegation.skill)
      assertFails { state.startTask(delegation.token, "worker", delegation.skill) }
      assertFails { state.createPlan("Again", listOf(Step("edict-next-run", "Run"))) }
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
      assertTrue(state.plan()!!.tasks.all { it.status == "failed" })
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

  @Test
  fun `plan is deeply immutable so plan() can share it without copying`() {
    val leaves = setOf<KClass<*>>(String::class, Int::class, Long::class, Boolean::class)
    val collections = setOf<KClass<*>>(List::class, Set::class, Map::class)
    val violations = mutableListOf<String>()
    val visited = mutableSetOf<KClass<*>>()

    fun check(type: KType, path: String) {
      val klass = type.classifier as? KClass<*> ?: return violations.plusAssign("$path: unsupported type $type")
      when {
        klass in leaves -> Unit
        klass in collections -> {
          // List and MutableList share a runtime class; only the declared type tells them apart.
          if (type.toString().startsWith("kotlin.collections.Mutable")) violations += "$path: mutable collection $type"
          type.arguments.forEachIndexed { index, argument ->
            argument.type?.let { check(it, "$path[$index]") } ?: violations.plusAssign("$path: star projection")
          }
        }
        klass.isData -> if (visited.add(klass)) {
          for (property in klass.memberProperties) {
            if (property is KMutableProperty<*>) violations += "$path.${property.name}: var"
            check(property.returnType, "$path.${property.name}")
          }
        }
        else -> violations += "$path: $type is not a known immutable type"
      }
    }

    check(typeOf<EdictNextRepositoryState.Plan>(), "Plan")
    assertTrue(violations.isEmpty(), "plan() returns the shared Plan, so it must stay immutable:\n" + violations.joinToString("\n"))
  }
}
