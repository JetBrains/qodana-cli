package org.jetbrains.qodana.edict.logging

import kotlinx.serialization.json.*
import org.jetbrains.qodana.edict.common.flag
import org.jetbrains.qodana.edict.common.text
import org.jetbrains.qodana.edict.common.wireJson
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState.Delegation
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState.Step
import org.jetbrains.qodana.edict.support.EdictNextTestTools
import org.jetbrains.qodana.edict.support.launch
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import java.io.PrintWriter
import java.io.StringWriter
import java.nio.file.Files
import java.nio.file.Path
import java.util.concurrent.Executors
import kotlin.test.*

class TaskLifecycleLoggingTest {
    @TempDir
    lateinit var directory: Path

    private fun EdictNextRepositoryState.delegateChild(parent: String, skill: String, title: String): Delegation {
        val task = addTask(parent, skill, title)
        return delegate(parent, task.id, "\$$skill\nRead /skills/$skill/SKILL.md").also { readTask(it.token) }
    }

    private fun start(worker: Delegation) = buildJsonObject {
        put("token", worker.token); put("agentId", "agent-${worker.taskId}"); put("skill", worker.skill)
    }

    private fun finish(worker: Delegation, status: String = "completed") = buildJsonObject {
        put("token", worker.token); put("status", status); put("result", "Done")
    }

    private fun EdictNextTestTools.ok(name: String, arguments: JsonObject) {
        assertEquals(false, call(name, arguments).flag("isError"))
    }

    @Test
    fun `parallel task transitions have one ordered pair each and do not log rejected calls`() {
        EdictNextRepositoryState.open(directory.resolve("state")).use { store ->
            val created = store.createPlan("Generate", listOf(Step("edict-next-generation", "Generate")))
            val generation = store.launch(created.token, created.plan.tasks.single().id, "edict-next-generation")
            val output = StringWriter()
            val logs = directory.resolve("logs")
            val server = EdictNextTestTools(store, logs = logs, taskOutput = PrintWriter(output))
            val workers = (1..15).map { index ->
                store.delegateChild(generation.token, "edict-next-cluster-generation", "Generate $index")
            }
            Executors.newFixedThreadPool(15).use { executor ->
                workers.mapIndexed { index, worker -> executor.submit {
                    server.ok("edict_task_start", start(worker))
                    assertEquals(true, server.call("edict_task_start", start(worker)).flag("isError"))
                    server.ok("edict_task_finish", finish(worker, if (index % 2 == 0) "completed" else "failed"))
                    assertEquals(true, server.call("edict_task_finish", finish(worker)).flag("isError"))
                } }.forEach { it.get() }
            }
            val lines = output.toString().lines().filter { it.isNotEmpty() }
            assertEquals(30, lines.size)
            workers.forEachIndexed { index, worker ->
                val prefix = "[-:${worker.taskId}] Generate ${index + 1}"
                assertTrue(lines.indexOf("$prefix started") >= 0)
                assertTrue(lines.indexOf("$prefix finished") > lines.indexOf("$prefix started"))
                assertFalse(output.toString().contains(worker.token))
            }
            assertEquals(output.toString(), Files.readString(logs.resolve("edict-tasks.log")))
        }
    }

    @Test
    fun `nested review inherits cluster and cancellation ends all active descendants once`() {
        EdictNextRepositoryState.open(directory.resolve("state")).use { store ->
            val created = store.createPlan("Generate", listOf(Step("edict-next-generation", "Generate")))
            val generation = store.launch(created.token, created.plan.tasks.single().id, "edict-next-generation")
            val output = StringWriter()
            val server = EdictNextTestTools(store, taskOutput = PrintWriter(output))
            val cluster = store.delegateChild(generation.token, "edict-next-cluster-generation", "Generate rule")
            server.ok("edict_task_start", start(cluster))
            val finished = store.delegateChild(cluster.token, "edict-next-inspection-code-review", "Code review")
            server.ok("edict_task_start", start(finished))
            server.ok("edict_task_finish", finish(finished))
            val review = store.delegateChild(cluster.token, "edict-next-weak-signal-review", "Weak\nsignal\treview")
            server.ok("edict_task_start", start(review))
            val example = store.delegateChild(review.token, "edict-next-code-example", "Check example")
            server.ok("edict_task_start", start(example))
            server.ok("edict_task_cancel", buildJsonObject {
                put("token", created.token); put("taskId", generation.taskId); put("result", "Worker lost")
            })
            val lines = output.toString().lines().filter { it.isNotEmpty() }
            assertEquals(9, lines.size)
            assertContains(lines, "[-:${review.taskId}] Weak signal review started")
            assertContains(lines, "[-:${example.taskId}] Check example finished")
            assertContains(lines, "[-:${cluster.taskId}] Generate rule finished")
            assertContains(lines, "[-:${generation.taskId}] Generate finished")
            assertEquals(1, lines.count { it == "[-:${finished.taskId}] Code review finished" })
        }
    }

    @Test
    fun `lifecycle messages stay separate from tool responses`() {
        EdictNextRepositoryState.open(directory.resolve("state")).use { store ->
            val created = store.createPlan("Generate", listOf(Step("edict-next-generation", "Generate")))
            val task = created.plan.tasks.single()
            val worker = store.delegate(created.token, task.id, "\$edict-next-generation\nRead /skills/edict-next-generation/SKILL.md")
            store.readTask(worker.token)
            val messages = StringWriter()
            val protocol = StringWriter()
            val server = EdictNextTestTools(store, taskOutput = PrintWriter(messages))
            val input = listOf("edict_task_start" to start(worker), "edict_task_finish" to finish(worker)).mapIndexed { index, (name, arguments) ->
                wireJson.encodeToString(buildJsonObject {
                    put("jsonrpc", "2.0"); put("id", index); put("method", "tools/call")
                    putJsonObject("params") { put("name", name); put("arguments", arguments) }
                })
            }.joinToString("\n")
            input.lineSequence().filter(String::isNotBlank).forEach { request ->
                val message = wireJson.parseToJsonElement(request).jsonObject
                val params = message.getValue("params").jsonObject
                val response = server.call(params.text("name"), params.getValue("arguments").jsonObject)
                protocol.appendLine(wireJson.encodeToString(response))
            }
            val responses = protocol.toString().lines().filter { it.isNotEmpty() }.map { wireJson.parseToJsonElement(it).jsonObject }
            assertEquals(2, responses.size)
            responses.forEach { assertEquals(false, it.flag("isError")) }
            assertEquals("[-:${task.id}] Generate started\n[-:${task.id}] Generate finished\n", messages.toString())
        }
    }
}
