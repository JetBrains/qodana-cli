package org.jetbrains.qodana.edict.integration

import java.net.URI
import java.net.http.HttpClient
import java.net.http.HttpRequest
import java.net.http.HttpResponse
import java.nio.file.Files
import java.nio.file.Path
import java.util.concurrent.TimeUnit
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonObject
import org.jetbrains.qodana.edict.common.array
import org.jetbrains.qodana.edict.common.obj
import org.jetbrains.qodana.edict.common.text
import org.jetbrains.qodana.edict.common.wireJson
import org.jetbrains.qodana.edict.common.EdictLayout
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState
import org.jetbrains.qodana.edict.integration.support.IntegrationTest
import org.jetbrains.qodana.edict.integration.support.inspection.InspectionLifecycleFixture
import org.junit.jupiter.api.Test

class CliTest : IntegrationTest() {

    @Test
    fun `installed Kotlin application parses and validates forwarded Qodana YAML`() {
        val config = directory.resolve("partial-qodana.yaml")
        Files.writeString(config, "edict:\n  ci:\n    url:\n")
        val stderr = directory.resolve("config-stderr")
        val process = ProcessBuilder(
            System.getProperty("edict.executable"),
            "mcp",
            "--qodana-yaml",
            config.toString(),
        )
            .apply {
                environment()["EDICT_OPTS"] = "-D${EdictLayout.LOG_DIRECTORY_PROPERTY}=${workspace.layout.logDirectory}"
            }
            .directory(workspace.project.toFile())
            .redirectOutput(directory.resolve("config-stdout").toFile())
            .redirectError(stderr.toFile())
            .start()
        process.outputStream.close()
        assertTrue(process.waitFor(30, TimeUnit.SECONDS), "CLI did not reject partial Qodana YAML")
        assertEquals(1, process.exitValue())
        assertTrue(Files.readString(stderr).contains("edict.ci.url is required"))
    }

    @Test
    fun `installed application serves Edict Next over HTTP at the installed port and releases state lock on exit`() {
        val executable = System.getProperty("edict.executable")
        val stderr = directory.resolve("stderr")
        val state = workspace.state
        val codexHome = Files.createDirectories(directory.resolve("codex-home"))
        val project = workspace.project.toRealPath().toString()
        Files.writeString(codexHome.resolve("config.toml"), "[projects.${JsonPrimitive(project)}]\ntrust_level = \"trusted\"\n")
        // The fixture keeps logs outside the checkout, which must stay unchanged.
        val logs = "-D${EdictLayout.LOG_DIRECTORY_PROPERTY}=${workspace.layout.logDirectory}"
        val install = ProcessBuilder(executable, "install").directory(workspace.project.toFile())
            .apply { environment()["CODEX_HOME"] = codexHome.toString(); environment()["EDICT_OPTS"] = logs }
            .redirectOutput(directory.resolve("install.stdout").toFile()).redirectError(stderr.toFile()).start()
        assertTrue(install.waitFor(30, TimeUnit.SECONDS) && install.exitValue() == 0, Files.readString(directory.resolve("install.stdout")))
        InspectionLifecycleFixture(directory).use { inspection ->
            val process = ProcessBuilder(
                executable,
                "mcp",
                "--state-dir",
                state.toString(),
                "--qodana-executable",
                inspection.qodanaExecutable.toString(),
            )
                .apply { environment()["EDICT_OPTS"] = logs }
                .directory(workspace.project.toFile()).redirectOutput(directory.resolve("stdout").toFile())
                .redirectError(stderr.toFile())
                .start()
            try {
                val url = awaitListening(process, directory.resolve("stdout"))
                val configured = Files.readString(workspace.project.resolve(".codex/config.toml"))
                assertTrue("url = \"$url\"" in configured, "Server URL $url is not the installed one")
                val client = HttpClient.newHttpClient()
                val initialize = client.send(
                    mcpRequest(url, null, """{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"integration","version":"1"}}}"""),
                    HttpResponse.BodyHandlers.ofString(),
                )
                val session = initialize.headers().firstValue("mcp-session-id").orElseThrow()
                assertEquals("edict-mcp", initialize.message().obj("result").obj("serverInfo").text("name"))
                client.send(mcpRequest(url, session, """{"jsonrpc":"2.0","method":"notifications/initialized"}"""), HttpResponse.BodyHandlers.discarding())
                val tools = client.send(mcpRequest(url, session, """{"jsonrpc":"2.0","id":2,"method":"tools/list"}"""), HttpResponse.BodyHandlers.ofString())
                    .message().obj("result").array("tools").map { it.text("name") }.toSet()
                assertTrue("edict_context" in tools)
                assertTrue("edict_delegate" in tools)
                assertTrue("edict_next_prepare_pipeline" in tools)
                assertTrue("edict_next_validate_generation" in tools)
                assertTrue("generate_inspection_kts_api" in tools)
            } finally {
                process.destroy()
                assertTrue(process.waitFor(30, TimeUnit.SECONDS), "CLI did not terminate")
            }
            EdictNextRepositoryState.open(state).use { assertNull(it.plan()) }
        }
    }
}

private fun awaitListening(process: Process, stdout: Path): String {
    val deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(30)
    while (System.nanoTime() < deadline) {
        Files.readAllLines(stdout).firstNotNullOfOrNull { it.substringAfter("edict-mcp listening at ", "").ifEmpty { null } }
            ?.let { return it }
        check(process.isAlive) { "CLI exited before listening: ${Files.readString(stdout)}" }
        Thread.sleep(100)
    }
    error("CLI did not start listening: ${Files.readString(stdout)}")
}

private fun mcpRequest(url: String, session: String?, body: String): HttpRequest = HttpRequest.newBuilder(URI(url))
    .header("Content-Type", "application/json")
    .header("Accept", "application/json, text/event-stream")
    .apply { session?.let { header("mcp-session-id", it) } }
    .POST(HttpRequest.BodyPublishers.ofString(body))
    .build()

// Streamable HTTP may answer with a single server-sent event instead of a plain JSON body.
private fun HttpResponse<String>.message(): JsonObject =
    wireJson.parseToJsonElement(body().lineSequence().map { it.removePrefix("data:").trim() }.first { it.startsWith("{") }).jsonObject
