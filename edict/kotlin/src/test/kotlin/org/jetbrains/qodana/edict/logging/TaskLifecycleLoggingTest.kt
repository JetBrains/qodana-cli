package org.jetbrains.qodana.edict.logging

import kotlinx.serialization.json.*
import org.jetbrains.qodana.edict.common.EdictLayout
import org.jetbrains.qodana.edict.common.EdictConfiguration
import org.jetbrains.qodana.edict.common.flag
import org.jetbrains.qodana.edict.common.text
import org.jetbrains.qodana.edict.common.wireJson
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState
import org.jetbrains.qodana.edict.edictnext.EdictManagementService
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState.Delegation
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState.Step
import org.jetbrains.qodana.edict.support.EdictNextTestTools
import org.jetbrains.qodana.edict.support.edictNextToolset
import org.jetbrains.qodana.edict.support.launch
import org.jetbrains.qodana.edict.runtime.CodexPriceAnalysis
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
    fun `price tool prints the report to the CI stream only after all tasks are terminal`() {
        EdictNextRepositoryState.open(directory.resolve("state")).use { store ->
            val created = store.createPlan("Generate", listOf(Step("edict-next-cluster-generation", "Generate rule")))
            val output = StringWriter()
            val layout = EdictLayout(directory, store.root)
            val rendered = "Price report:\n  - Generate rule: $14.3129 (24,115,170 tokens)\nTotal price: $14.3129 (24,115,170 tokens)"
            val report = buildJsonObject {
                putJsonObject("totalPrice") { put("priceUsd", 14.3128844); put("totalTokens", 24_115_170) }
            }
            var analyses = 0
            val management = EdictManagementService(store, layout, taskOutput = PrintWriter(output), analyzePrice = { plan ->
                assertEquals(store.plan(), plan)
                analyses++
                CodexPriceAnalysis(report, rendered, 14.3128844, 24_115_170)
            })
            edictNextToolset(layout, management, configuration = EdictConfiguration(calculatePrice = true)).createServer()
            val arguments = buildJsonObject { put("token", created.token) }

            assertEquals(true, management.call("edict_calculate_price", arguments).flag("isError"))
            assertEquals(0, analyses)
            assertEquals("", output.toString())
            assertEquals("", Files.readString(layout.tasksLogPath))

            val worker = store.launch(created.token, created.plan.tasks.single().id, "edict-next-cluster-generation")
            assertEquals(true, management.call("edict_calculate_price", buildJsonObject { put("token", worker.token) }).flag("isError"))
            assertEquals(0, analyses)
            store.finishTask(worker.token, "completed", "Generated")

            val response = management.call("edict_calculate_price", arguments)
            assertEquals(false, response.flag("isError"))
            assertEquals(1, analyses)
            assertEquals(report, response.getValue("structuredContent"))
            assertEquals("$rendered\n", output.toString())
            assertEquals(output.toString(), Files.readString(layout.tasksLogPath))
            assertFalse(output.toString().contains(created.token))
        }
    }

    @Test
    fun `parallel task transitions have one ordered pair each and do not log rejected calls`() {
        EdictNextRepositoryState.open(directory.resolve("state")).use { store ->
            val created = store.createPlan("Generate", emptyList())
            val output = StringWriter()
            val layout = EdictLayout(directory, directory.resolve(".edict"))
            val server = EdictNextTestTools(store, layout = layout, taskOutput = PrintWriter(output))
            val workers = (1..15).map { index ->
                store.delegateChild(created.token, "edict-next-cluster-generation", "Generate $index")
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
            assertEquals(45, lines.size)
            workers.forEachIndexed { index, worker ->
                val prefix = "[-:${worker.taskId.take(8)}] Generate ${index + 1}"
                assertTrue(lines.indexOf("$prefix started") >= 0)
                val finished = lines.indexOfFirst { it.startsWith("$prefix finished") }
                assertTrue(finished > lines.indexOf("$prefix started"))
                assertEquals(
                    "$prefix finished [${if (index % 2 == 0) "completed" else "failed"}]",
                    lines[finished],
                )
                assertEquals("  Done", lines[finished + 1])
                assertFalse(output.toString().contains(worker.token))
            }
            assertEquals(output.toString(), Files.readString(layout.tasksLogPath))
        }
    }

    @Test
    fun `nested review inherits cluster and cancellation ends all active descendants once`() {
        EdictNextRepositoryState.open(directory.resolve("state")).use { store ->
            val created = store.createPlan("Generate", listOf(Step("edict-next-cluster-generation", "Generate rule")))
            val output = StringWriter()
            val server = EdictNextTestTools(store, taskOutput = PrintWriter(output))
            val cluster = store.delegate(created.token, created.plan.tasks.single().id,
                "\$edict-next-cluster-generation\nRead /skills/edict-next-cluster-generation/SKILL.md")
            store.readTask(cluster.token)
            server.ok("edict_task_start", start(cluster))
            val finished = store.delegateChild(cluster.token, "edict-next-generation-round", "Generation round")
            server.ok("edict_task_start", start(finished))
            server.ok("edict_task_finish", finish(finished))
            val review = store.delegateChild(cluster.token, "edict-next-code-example-overseer", "Reconcile\nexamples\tfor rule")
            server.ok("edict_task_start", start(review))
            val example = store.delegateChild(review.token, "edict-next-code-example", "Check example")
            server.ok("edict_task_start", start(example))
            server.ok("edict_task_cancel", buildJsonObject {
                put("token", created.token); put("taskId", cluster.taskId); put("result", "Worker lost")
            })
            val lines = output.toString().lines().filter { it.isNotEmpty() }
            assertEquals(12, lines.size)
            assertContains(lines, "[-:${review.taskId.take(8)}] Reconcile examples for rule started")
            assertContains(lines, "[-:${example.taskId.take(8)}] Check example finished [cancelled]")
            assertContains(lines, "[-:${cluster.taskId.take(8)}] Generate rule finished [cancelled]")
            assertEquals(3, lines.count { it == "  Worker lost" })
            assertEquals(
                1,
                lines.count { it == "[-:${finished.taskId.take(8)}] Generation round finished [completed]" },
            )
        }
    }

    @Test
    fun `lifecycle messages stay separate from tool responses`() {
        EdictNextRepositoryState.open(directory.resolve("state")).use { store ->
            val created = store.createPlan("Generate", listOf(Step("edict-next-cluster-generation", "Generate")))
            val task = created.plan.tasks.single()
            val worker = store.delegate(created.token, task.id, "\$edict-next-cluster-generation\nRead /skills/edict-next-cluster-generation/SKILL.md")
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
            assertEquals(
                    "[-:${task.id.take(8)}] Generate started\n" +
                    "[-:${task.id.take(8)}] Generate finished [completed]\n" +
                    "  Done\n",
                messages.toString(),
            )
        }
    }
}
