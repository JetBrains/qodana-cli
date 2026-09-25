package org.jetbrains.qodana.edict.logging

import kotlinx.serialization.json.*
import org.jetbrains.qodana.edict.common.flag
import org.jetbrains.qodana.edict.common.wireJson
import org.jetbrains.qodana.edict.mcp.McpServer
import org.jetbrains.qodana.edict.model.Delegation
import org.jetbrains.qodana.edict.model.Step
import org.jetbrains.qodana.edict.store.Store
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

    private fun Store.delegateChild(parent: String, skill: String, title: String, scope: List<String> = emptyList()): Delegation {
        val task = addTask(parent, skill, title)
        return delegate(parent, task.id, emptyList(), scope, "\$$skill\nRead /skills/$skill/SKILL.md").also { readTask(it.token) }
    }

    private fun start(worker: Delegation) = buildJsonObject {
        put("token", worker.token); put("agentId", "agent-${worker.taskId}"); put("skill", worker.skill)
    }

    private fun finish(worker: Delegation, status: String = "completed") = buildJsonObject {
        put("token", worker.token); put("status", status); put("result", "Done")
    }

    private fun McpServer.ok(name: String, arguments: JsonObject) {
        assertEquals(false, call(name, arguments).flag("isError"))
    }

    @Test
    fun `parallel task transitions have one ordered pair each and do not log rejected calls`() {
        Store(directory.resolve("state")).use { store ->
            val created = store.createPlan("Generate", listOf(Step("edict-generation", "Generate")))
            val generation = store.launch(created.token, created.plan.tasks.single().id, "edict-generation", emptyList(), listOf("clusters"))
            val output = StringWriter()
            val logs = directory.resolve("logs")
            val server = McpServer(store, logs = logs, taskOutput = PrintWriter(output))
            val workers = (1..15).map { index ->
                store.delegateChild(generation.token, "edict-cluster-generation", "Generate $index", listOf("clusters/c$index"))
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
                val prefix = "[c${index + 1}:${worker.taskId}] Generate ${index + 1}"
                assertTrue(lines.indexOf("$prefix started") >= 0)
                assertTrue(lines.indexOf("$prefix finished") > lines.indexOf("$prefix started"))
                assertFalse(output.toString().contains(worker.token))
            }
            assertEquals(output.toString(), Files.readString(logs.resolve("edict-tasks.log")))
        }
    }

    @Test
    fun `nested review inherits cluster and cancellation ends all active descendants once`() {
        Store(directory.resolve("state")).use { store ->
            val created = store.createPlan("Generate", listOf(Step("edict-generation", "Generate")))
            val generation = store.launch(created.token, created.plan.tasks.single().id, "edict-generation", emptyList(), listOf("clusters"))
            val output = StringWriter()
            val server = McpServer(store, taskOutput = PrintWriter(output))
            val cluster = store.delegateChild(generation.token, "edict-cluster-generation", "Generate rule", listOf("clusters/chosen"))
            server.ok("edict_task_start", start(cluster))
            val finished = store.delegateChild(cluster.token, "edict-inspection-code-review", "Code review")
            server.ok("edict_task_start", start(finished))
            server.ok("edict_task_finish", finish(finished))
            val review = store.delegateChild(cluster.token, "edict-weak-signal-review", "Weak\nsignal\treview")
            server.ok("edict_task_start", start(review))
            val example = store.delegateChild(review.token, "edict-code-example", "Check example")
            server.ok("edict_task_start", start(example))
            server.ok("edict_task_cancel", buildJsonObject {
                put("token", created.token); put("taskId", generation.taskId); put("result", "Worker lost")
            })
            val lines = output.toString().lines().filter { it.isNotEmpty() }
            assertEquals(9, lines.size)
            assertContains(lines, "[chosen:${review.taskId}] Weak signal review started")
            assertContains(lines, "[chosen:${example.taskId}] Check example finished")
            assertContains(lines, "[chosen:${cluster.taskId}] Generate rule finished")
            assertContains(lines, "[-:${generation.taskId}] Generate finished")
            assertEquals(1, lines.count { it == "[chosen:${finished.taskId}] Code review finished" })
        }
    }

    @Test
    fun `stdio lifecycle messages stay out of JSON RPC stdout`() {
        Store(directory.resolve("state")).use { store ->
            val created = store.createPlan("Generate", listOf(Step("edict-generation", "Generate")))
            val task = created.plan.tasks.single()
            val worker = store.delegate(created.token, task.id, emptyList(), emptyList(), "\$edict-generation\nRead /skills/edict-generation/SKILL.md")
            store.readTask(worker.token)
            val messages = StringWriter()
            val protocol = StringWriter()
            val server = McpServer(store, taskOutput = PrintWriter(messages))
            val input = listOf("edict_task_start" to start(worker), "edict_task_finish" to finish(worker)).mapIndexed { index, (name, arguments) ->
                wireJson.encodeToString(buildJsonObject {
                    put("jsonrpc", "2.0"); put("id", index); put("method", "tools/call")
                    putJsonObject("params") { put("name", name); put("arguments", arguments) }
                })
            }.joinToString("\n")
            server.serveStdio(input.reader().buffered(), PrintWriter(protocol, true))
            val responses = protocol.toString().lines().filter { it.isNotEmpty() }.map { wireJson.parseToJsonElement(it).jsonObject }
            assertEquals(2, responses.size)
            responses.forEach { assertEquals(false, it.getValue("result").jsonObject.flag("isError")) }
            assertEquals("[-:${task.id}] Generate started\n[-:${task.id}] Generate finished\n", messages.toString())
        }
    }
}
