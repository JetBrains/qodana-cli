package org.jetbrains.qodana.edict.store

import org.jetbrains.qodana.edict.common.json
import org.jetbrains.qodana.edict.model.Delegation
import org.jetbrains.qodana.edict.model.Step
import org.jetbrains.qodana.edict.skills.Registry
import org.jetbrains.qodana.edict.support.batch
import org.jetbrains.qodana.edict.support.fixtureSignals
import org.jetbrains.qodana.edict.support.gitFixture
import org.jetbrains.qodana.edict.support.launch
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Files
import java.nio.file.Path
import java.util.concurrent.Executors
import kotlin.test.assertEquals
import kotlin.test.assertFails
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class StoreTest {
    @TempDir
    lateinit var directory: Path

    @Test
    fun `extraction workers cannot replace source evidence with submitted feedback`() {
        val original = fixtureSignals(gitFixture(directory.resolve("source"))).first()
        val signal = original.copy(source = org.jetbrains.qodana.edict.model.SignalSource("SubmittedFeedback", "",
            message = "Feedback", url = "benchmark/Rule/specification.json"))
        Store(directory.resolve("state")).use { store ->
            val (_, worker) = store.batch()
            assertFails { store.write(worker.token, "inbox/${signal.id}.json", json.encodeToString(signal), "") }
            assertTrue(store.list("inbox").isEmpty())
        }
    }

    @Test
    fun `worker must fetch assignment and declare matching skill before start`() {
        Store(directory).use { store ->
            val plan = store.createPlan("Extract", listOf(Step("edict-batch-signal-analysis", "Commit")))
            val d = store.delegate(
                plan.token,
                plan.plan.tasks.single().id,
                listOf("inbox.write"),
                listOf("inbox"),
                "\$edict-batch-signal-analysis\nRead /skills/edict-batch-signal-analysis/SKILL.md. Analyze HEAD."
            )
            assertFails { store.startTask(d.token, "worker", d.skill) }
            assertFails { store.addTask(d.token, "edict-signal-analysis", "Inspect") }
            assertEquals(d.taskId, store.readTask(d.token).taskId)
            assertFails { store.startTask(d.token, "worker", "edict_manager") }
            store.startTask(d.token, "worker", d.skill)
            assertFails { store.startTask(d.token, "worker", d.skill) }
            assertFails { store.createPlan("Again", listOf(Step("edict-run", "Run"))) }
        }
    }

    @Test
    fun `capabilities enforce call graph scope and revocation`() {
        Store(directory).use { store ->
            val (manager, batch) = store.batch()
            val child = store.addTask(batch.token, "edict-signal-analysis", "Inspect evidence")
            assertFails {
                store.delegate(
                    batch.token,
                    child.id,
                    listOf("inbox.write"),
                    listOf("inbox"),
                    "\$edict-signal-analysis\nInspect"
                )
            }
            assertFails {
                store.delegate(
                    batch.token,
                    child.id,
                    emptyList(),
                    listOf("clusters"),
                    "\$edict-signal-analysis\nInspect"
                )
            }
            assertFails {
                store.delegate(
                    batch.token,
                    child.id,
                    emptyList(),
                    emptyList(),
                    "\$edict-signal-analysis\n${manager.token}"
                )
            }
            val leaf = store.launch(batch.token, child.id, child.skill, emptyList(), emptyList())
            listOf(manager.token, leaf.token, "forged").forEach { token ->
                assertFails {
                    store.write(
                        token,
                        "inbox/s-fake.json",
                        "{}",
                        ""
                    )
                }
            }
            assertFails { store.addTask(leaf.token, "edict-generation", "Escalate") }
            assertFails { store.finishTask(batch.token, "completed", "Done") }
            store.cancelTask(manager.token, batch.taskId, "Worker lost")
            assertFails { store.readTask(leaf.token) }
            assertFails { store.finishTask(batch.token, "completed", "Done") }
            assertTrue(store.plan()!!.tasks.all { it.status == "failed" })
            val retry = store.launch(manager.token, batch.taskId, batch.skill, listOf("inbox.write"), listOf("inbox"))
            val retriedLeaf = store.launch(retry.token, child.id, child.skill, emptyList(), emptyList())
            store.finishTask(retriedLeaf.token, "completed", "Inspected")
            store.finishTask(retry.token, "completed", "Done")
            assertTrue(store.plan()!!.tasks.all { it.status == "completed" })
        }
    }

    @Test
    fun `state lock recovery and fresh capabilities preserve completed work`() {
        val steps = listOf(Step("edict-batch-signal-analysis", "Commit"))
        lateinit var old: Delegation
        lateinit var id: String
        Store(directory).use { store ->
            assertFails { Store(directory).close() }
            val manager = store.createPlan("Extract", steps)
            id = manager.plan.id
            old = store.launch(
                manager.token,
                manager.plan.tasks.single().id,
                steps.single().skill,
                listOf("inbox.write"),
                listOf("inbox")
            )
            val task = store.addTask(old.token, "edict-signal-analysis", "Evidence")
            val leaf = store.launch(old.token, task.id, task.skill, emptyList(), emptyList())
            store.finishTask(leaf.token, "completed", "Verified")
        }
        Store(directory).use { store ->
            assertFails { store.readTask(old.token) }
            assertFails { store.createPlan("Different", steps) }
            val resumed = store.createPlan("Extract", steps)
            assertEquals(id, resumed.plan.id)
            assertEquals(listOf("pending", "completed"), resumed.plan.tasks.map { it.status })
            val batch = store.launch(resumed.token, old.taskId, old.skill, listOf("inbox.write"), listOf("inbox"))
            store.finishTask(batch.token, "completed", "Resumed")
            assertFalse(Files.readString(directory.resolve("plans/$id.json")).contains(batch.token))
        }
    }

    @Test
    fun `writes reject stale hashes malformed evidence and escaping paths without changes`() {
        val signal = fixtureSignals(gitFixture(directory.resolve("source"))).first()
        Store(directory.resolve("state")).use { store ->
            val (_, batch) = store.batch()
            val name = "inbox/${signal.id}.json"
            val content = json.encodeToString(signal)
            val file = store.write(batch.token, name, content, "")
            assertFails { store.write(batch.token, name, content, "") }
            assertFails { store.write(batch.token, name, "{}", file.hash) }
            assertEquals(file, store.read(name))
            assertFails {
                store.write(
                    batch.token,
                    name,
                    json.encodeToString(signal.copy(description = batch.token)),
                    file.hash
                )
            }
            listOf(
                "../escape",
                "/tmp/escape",
                "inbox/../escape",
                "inbox//a.json",
                "inbox/a.json.",
                "inbox/a.json ",
                "inbox\\a.json",
                "inbox/C:a.json",
                "plans/new.json"
            ).forEach {
                assertFails("Accepted $it") { store.write(batch.token, it, content, "") }
            }
            Files.createSymbolicLink(store.root.resolve("inbox/link.json"), directory.resolve("outside"))
            assertFails { store.read("inbox/link.json") }
            assertFails { store.list("inbox") }
        }
    }

    @Test
    fun `parallel replacements permit only one writer for a hash`() {
        val signal = fixtureSignals(gitFixture(directory.resolve("source"))).first()
        Store(directory.resolve("state")).use { store ->
            val (_, batch) = store.batch()
            val path = "inbox/${signal.id}.json"
            val old = store.write(batch.token, path, json.encodeToString(signal), "")
            val pool = Executors.newFixedThreadPool(4)
            try {
                val results = (1..4).map { i ->
                    pool.submit<Boolean> {
                        runCatching {
                            store.write(
                                batch.token,
                                path,
                                json.encodeToString(signal.copy(description = "Correction $i")),
                                old.hash
                            )
                        }.isSuccess
                    }
                }.map { it.get() }
                assertEquals(1, results.count { it })
            } finally {
                pool.shutdownNow()
            }
        }
    }

    @Test
    fun `distribution and generation preserve evidence and constrain example writes`() {
        val signal = fixtureSignals(gitFixture(directory.resolve("source"))).first()
        Store(directory.resolve("state")).use { store ->
            val created = store.createPlan(
                "Extract distribute generate",
                listOf(
                    Step("edict-batch-signal-analysis", "Extract"),
                    Step("edict-distribution", "Distribute"),
                    Step("edict-generation", "Generate")
                )
            )
            val batch = store.launch(
                created.token,
                created.plan.tasks[0].id,
                "edict-batch-signal-analysis",
                listOf("inbox.write"),
                listOf("inbox")
            )
            val inbox = store.write(batch.token, "inbox/${signal.id}.json", json.encodeToString(signal), "")
            store.finishTask(batch.token, "completed", "Extracted")
            val distribution = store.launch(
                created.token,
                created.plan.tasks[1].id,
                "edict-distribution",
                Registry["edict-distribution"].operations,
                listOf(inbox.path, "clusters/c-equality")
            )
            val cluster = "clusters/c-equality"
            store.write(
                distribution.token,
                "$cluster/description.json",
                "{\"id\":\"c-equality\",\"status\":\"Pending\"}",
                ""
            )
            val moved = store.write(distribution.token, "$cluster/signals/${signal.id}.json", inbox.content, "")
            assertEquals(inbox.hash, moved.hash)
            assertFails { store.delete(distribution.token, inbox.path, "") }
            store.delete(distribution.token, inbox.path, inbox.hash)
            store.finishTask(distribution.token, "completed", "Distributed")
            val generation = store.launch(
                created.token,
                created.plan.tasks[2].id,
                "edict-generation",
                Registry["edict-generation"].operations,
                listOf(cluster, "inspections")
            )
            val clusterTask = store.addTask(generation.token, "edict-cluster-generation", "Generate cluster")
            val worker = store.launch(
                generation.token,
                clusterTask.id,
                clusterTask.skill,
                Registry[clusterTask.skill].operations,
                listOf(cluster, "inspections")
            )
            val exampleTask = store.addTask(worker.token, "edict-code-example", "Example")
            val example = store.launch(
                worker.token,
                exampleTask.id,
                exampleTask.skill,
                Registry[exampleTask.skill].operations,
                listOf(cluster)
            )
            assertFails {
                store.write(
                    example.token,
                    moved.path,
                    json.encodeToString(signal.copy(description = "Tampered", syntheticExampleId = "e-1")),
                    moved.hash
                )
            }
            val linked = store.write(
                example.token,
                moved.path,
                json.encodeToString(signal.copy(syntheticExampleId = "e-1")),
                moved.hash
            )
            assertFails { store.delete(example.token, linked.path, linked.hash) }
            store.write(example.token, "$cluster/synthetic-examples/e-1/project/Example.java", "class Example {}", "")
            store.finishTask(example.token, "completed", "Example assigned")
            store.finishTask(worker.token, "completed", "Prepared")
            store.finishTask(generation.token, "completed", "Done")
        }
    }
}
