package org.jetbrains.qodana.edict.integration

import java.io.PrintWriter
import java.io.StringWriter
import java.net.URI
import java.net.http.HttpClient
import java.net.http.HttpRequest
import java.net.http.HttpResponse
import java.nio.file.Files
import kotlin.test.assertContains
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue
import kotlinx.serialization.json.*
import org.jetbrains.qodana.edict.common.array
import org.jetbrains.qodana.edict.common.flag
import org.jetbrains.qodana.edict.common.json
import org.jetbrains.qodana.edict.common.number
import org.jetbrains.qodana.edict.common.obj
import org.jetbrains.qodana.edict.common.text
import org.jetbrains.qodana.edict.common.wireJson
import org.jetbrains.qodana.edict.integration.support.IntegrationTest
import org.jetbrains.qodana.edict.mcp.McpServer
import org.jetbrains.qodana.edict.model.Signal
import org.jetbrains.qodana.edict.model.Step
import org.jetbrains.qodana.edict.skills.Registry
import org.jetbrains.qodana.edict.skills.Skills
import org.jetbrains.qodana.edict.store.Store
import org.jetbrains.qodana.edict.store.validateInboxChanges
import org.jetbrains.qodana.edict.support.batch
import org.jetbrains.qodana.edict.support.launch
import org.junit.jupiter.api.Test

class McpIntegrationTest : IntegrationTest() {

    @Test
    fun `extract signals from one real commit through managed MCP lifecycle over HTTP`() {
        val repository = workspace.repository
        val skills = directory.resolve("skills")
        Skills.install(skills)
        Store(workspace.state).use { store ->
            val server = McpServer(store, logs = workspace.logs)
            server.serveHttp().use { transport ->
                val client = HttpClient.newHttpClient()
                var nextId = 0
                fun call(name: String, arguments: JsonObject = JsonObject(emptyMap())): JsonObject {
                    val request = buildJsonObject {
                        put("jsonrpc", "2.0"); put("id", ++nextId); put("method", "tools/call")
                        putJsonObject("params") { put("name", name); put("arguments", arguments) }
                    }
                    val response = client.send(
                        HttpRequest.newBuilder(URI(transport.url)).header("Content-Type", "application/json")
                            .POST(HttpRequest.BodyPublishers.ofString(wireJson.encodeToString(request))).build(),
                        HttpResponse.BodyHandlers.ofString()
                    )
                    assertEquals(200, response.statusCode())
                    val result = wireJson.parseToJsonElement(response.body()).jsonObject.getValue("result").jsonObject
                    assertEquals(false, result.flag("isError"), result.toString())
                    return result.getValue("structuredContent").jsonObject
                }

                val commit = repository.commit("HEAD")
                val created = call("edict_plan_create", buildJsonObject {
                    put("request", "Extract signals from the latest commit")
                    put(
                        "steps",
                        json.encodeToJsonElement(
                            listOf(
                                Step(
                                    "edict-batch-signal-analysis",
                                    "Inspect ${commit.workItemId}"
                                )
                            )
                        )
                    )
                })
                val manager = created.text("token")
                val batchId = created.obj("plan").array("tasks").single().text("id")
                fun launch(parent: String, task: String, skill: String, operations: List<String>): String {
                    val delegated = call("edict_delegate", buildJsonObject {
                        put("token", parent); put("taskId", task)
                        put("operations", json.encodeToJsonElement(operations)); put(
                        "scope",
                        json.encodeToJsonElement(if (operations.isEmpty()) emptyList() else listOf("inbox"))
                    )
                        put(
                            "prompt",
                            "\$managed-$skill\nRead ${skills.resolve(Registry.skillPath(skill))}. Inspect ${commit.workItemId}, ${commit.parentRevision} -> ${commit.commitRevision} in ${repository.root}."
                        )
                    })
                    val token = delegated.text("token")
                    val launchArguments = wireJson.parseToJsonElement(delegated.text("prompt").lineSequence().single { it.startsWith("{") }).jsonObject
                    assertEquals(setOf("token"), launchArguments.keys, "Launch instructions must match the token-only task-get schema")
                    assertEquals(token, launchArguments.text("token"))
                    val assignment = call("edict_task_get", launchArguments)
                    assertTrue(Files.readString(skills.resolve(assignment.text("skillPath"))).isNotBlank())
                    call(
                        "edict_task_start",
                        buildJsonObject { put("token", token); put("agentId", "scripted-$task"); put("skill", skill) })
                    return token
                }

                val batch = launch(manager, batchId, "edict-batch-signal-analysis", listOf("inbox.write"))
                val child = call(
                    "edict_task_add",
                    buildJsonObject {
                        put("token", batch); put("skill", "edict-signal-analysis"); put(
                        "title",
                        "Inspect exact before and after source"
                    )
                    })
                val leaf = launch(batch, child.text("id"), "edict-signal-analysis", emptyList())
                val signals = workspace.signals()
                call(
                    "edict_task_finish",
                    buildJsonObject {
                        put("token", leaf); put("status", "completed"); put(
                        "result",
                        json.encodeToString(signals)
                    )
                    })
                signals.forEach { signal ->
                    call(
                        "edict_state_write",
                        buildJsonObject {
                            put("token", batch); put("path", "inbox/${signal.id}.json"); put(
                            "content",
                            json.encodeToString(signal)
                        ); put("expectedHash", "")
                        })
                }
                call(
                    "edict_task_finish",
                    buildJsonObject {
                        put("token", batch); put("status", "completed"); put(
                        "result",
                        "Inspected ${commit.workItemId}; published ${signals.size} signals"
                    )
                    })
                val paths = store.list("inbox")
                assertEquals(2, paths.size)
                paths.forEach { repository.validateEvidence(json.decodeFromString<Signal>(store.read(it).content)) }
                assertEquals(2, validateInboxChanges(store, paths).validatedFiles.size)
                assertTrue(store.plan()!!.tasks.all { it.status == "completed" })
                val logs = Files.readString(workspace.logs.resolve("edict-mcp-system.log"))
                listOf(manager, batch, leaf).forEach { assertFalse(logs.contains(it)) }
                assertContains(logs, "[REDACTED]")
            }
        }
    }

    @Test
    fun `stdio is protocol only and survives invalid requests`() {
        Store(workspace.state).use { store ->
            val output = StringWriter()
            val input = """
                {"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26"}}
                {"jsonrpc":"2.0","method":"notifications/initialized"}
                bad json
                {"jsonrpc":"2.0","id":2,"method":"tools/list"}
                {"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"edict_plan_create","arguments":{}}}
                {"jsonrpc":"2.0","id":4,"method":"ping"}
            """.trimIndent()
            McpServer(store).serveStdio(input.reader().buffered(), PrintWriter(output, true))
            val lines = output.toString().lineSequence().filter(String::isNotBlank)
                .map { wireJson.parseToJsonElement(it).jsonObject }.toList()
            assertEquals(5, lines.size)
            assertEquals("edict-mcp", lines[0].obj("result").obj("serverInfo").text("name"))
            assertEquals(-32700, lines[1].obj("error").number("code"))
            assertEquals(19, lines[2].obj("result").array("tools").size)
            assertEquals(true, lines[3].obj("result").flag("isError"))
            assertTrue(lines.last().obj("result").isEmpty())
        }
    }
}
