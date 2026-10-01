package org.jetbrains.qodana.edict.integration

import java.nio.file.Files
import java.util.concurrent.TimeUnit
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlinx.serialization.json.jsonObject
import org.jetbrains.qodana.edict.common.array
import org.jetbrains.qodana.edict.common.obj
import org.jetbrains.qodana.edict.common.text
import org.jetbrains.qodana.edict.common.wireJson
import org.jetbrains.qodana.edict.integration.support.IntegrationTest
import org.jetbrains.qodana.edict.integration.support.inspection.InspectionLifecycleFixture
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState
import org.junit.jupiter.api.Test

class CliTest : IntegrationTest() {

    @Test
    fun `installed application serves Edict Next stdio and releases state lock on EOF`() {
        val stdout = directory.resolve("stdout")
        val stderr = directory.resolve("stderr")
        val state = workspace.state
        InspectionLifecycleFixture(directory).use { inspection ->
            val process = ProcessBuilder(
                System.getProperty("edict.executable"),
                "edict-mcp-next",
                "--project-dir",
                workspace.project.toString(),
                "--state-dir",
                state.toString(),
                "--log-dir",
                directory.resolve("log").toString(),
                "--qodana-executable",
                inspection.qodanaExecutable.toString(),
            )
                .directory(workspace.project.toFile()).redirectOutput(stdout.toFile()).redirectError(stderr.toFile())
                .start()
            try {
                process.outputStream.bufferedWriter().use {
                    it.appendLine("""{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"integration","version":"1"}}}""")
                    it.appendLine("""{"jsonrpc":"2.0","method":"notifications/initialized"}""")
                    it.appendLine("""{"jsonrpc":"2.0","id":2,"method":"tools/list"}""")
                }
                assertTrue(process.waitFor(30, TimeUnit.SECONDS), "CLI did not terminate on EOF")
                assertEquals(0, process.exitValue(), Files.readString(stderr))
                val responses = Files.readAllLines(stdout).map { wireJson.parseToJsonElement(it).jsonObject }
                assertEquals(2, responses.size)
                assertEquals("edict-mcp-next", responses.first().obj("result").obj("serverInfo").text("name"))
                val tools = responses.last().obj("result").array("tools").map { it.text("name") }.toSet()
                assertTrue("edict_delegate" in tools)
                assertTrue("edict_prepare_pipeline" in tools)
                assertTrue("edict_next_validate_generation" in tools)
                assertTrue("generate_inspection_kts_api" in tools)
                EdictNextRepositoryState.open(state).use { assertNull(it.plan()) }
            } finally {
                if (process.isAlive) process.destroyForcibly()
            }
        }
    }
}
