package org.jetbrains.qodana.edict.integration

import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.runBlocking
import kotlinx.io.asSink
import kotlinx.io.asSource
import kotlinx.io.buffered
import kotlin.test.assertContains
import kotlin.test.assertEquals
import kotlin.test.assertTrue
import kotlinx.serialization.json.*
import org.jetbrains.qodana.edict.common.array
import org.jetbrains.qodana.edict.common.flag
import org.jetbrains.qodana.edict.common.obj
import org.jetbrains.qodana.edict.common.text
import org.jetbrains.qodana.edict.common.wireJson
import org.jetbrains.qodana.edict.integration.support.IntegrationTest
import org.jetbrains.qodana.edict.edictnext.EdictManagementService
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState
import org.jetbrains.qodana.edict.support.edictNextToolset
import org.junit.jupiter.api.Test
import io.modelcontextprotocol.kotlin.sdk.server.StdioServerTransport

class McpIntegrationTest : IntegrationTest() {

    @Test
    fun `Edict Next stdio combines managed delegation and generation tools`() {
        EdictNextRepositoryState.open(workspace.state).use { store ->
            val output = ByteArrayOutputStream()
            val input = """
                {"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"integration","version":"1"}}}
                {"jsonrpc":"2.0","method":"notifications/initialized"}
                {"jsonrpc":"2.0","id":2,"method":"tools/list"}
                {"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"edict_plan_create","arguments":{}}}
                {"jsonrpc":"2.0","id":4,"method":"ping"}
            """.trimIndent() + "\n"
            val server = edictNextToolset(workspace.layout, EdictManagementService(store, workspace.layout)).createServer()
            val transport = StdioServerTransport(
                input = ByteArrayInputStream(input.encodeToByteArray()).asSource().buffered(),
                output = output.asSink().buffered(),
            )
            runBlocking {
                val closed = CompletableDeferred<Unit>()
                transport.onClose { closed.complete(Unit) }
                server.createSession(transport)
                closed.await()
            }
            val lines = output.toString(Charsets.UTF_8).lineSequence().filter(String::isNotBlank)
                .map { wireJson.parseToJsonElement(it).jsonObject }.toList()
            assertEquals(4, lines.size)
            assertEquals("edict-mcp", lines[0].obj("result").obj("serverInfo").text("name"))
            val tools = lines[1].obj("result").array("tools").map { it.text("name") }.toSet()
            assertContains(tools, "edict_delegate")
            assertContains(tools, "edict_next_prepare_pipeline")
            assertContains(tools, "edict_next_get_new_inspection_results")
            assertContains(tools, "generate_inspection_kts_api")
            assertContains(tools, "run_inspection_kts")
            assertContains(tools, "compile_inspection_kts")
            assertContains(tools, "run_inspection_kts_examples")
            assertContains(tools, "run_inspection_kts_project")
            assertContains(tools, "edict_promote_clusters")
            assertContains(tools, "ecict-check-promotion")
            assertContains(tools, "edict_promotion_decide_reviews")
            assertEquals(true, lines[2].obj("result").flag("isError"))
            assertTrue(lines.last().obj("result").isEmpty())
        }
    }
}
