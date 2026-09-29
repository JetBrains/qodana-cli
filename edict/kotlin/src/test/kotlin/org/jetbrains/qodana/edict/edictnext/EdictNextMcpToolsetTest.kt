package org.jetbrains.qodana.edict.edictnext

import io.modelcontextprotocol.kotlin.sdk.server.StdioServerTransport
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.runBlocking
import kotlinx.io.asSink
import kotlinx.io.asSource
import kotlinx.io.buffered
import kotlinx.serialization.json.jsonObject
import org.jetbrains.qodana.edict.common.flag
import org.jetbrains.qodana.edict.common.obj
import org.jetbrains.qodana.edict.common.runProcess
import org.jetbrains.qodana.edict.common.text
import org.jetbrains.qodana.edict.common.wireJson
import org.jetbrains.qodana.edict.mcp.McpServer
import org.jetbrains.qodana.edict.store.Store
import org.junit.jupiter.api.io.TempDir
import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream
import java.io.FilterInputStream
import java.io.InputStream
import java.nio.file.Files
import java.nio.file.Path
import kotlin.test.Test
import kotlin.test.assertContains
import kotlin.test.assertEquals
import kotlin.test.assertFalse

class EdictNextMcpToolsetTest {
  @TempDir
  lateinit var directory: Path

  @Test
  fun `tool handlers use the Edict run id rather than the MCP connection id`() = runBlocking {
    val repository = Files.createDirectory(directory.resolve("repository"))
    runProcess(repository, listOf("git", "init", "--quiet", "--initial-branch=main"))
    runProcess(repository, listOf("git", "config", "user.email", "edict-test@localhost"))
    runProcess(repository, listOf("git", "config", "user.name", "Edict Test"))
    Files.writeString(repository.resolve("README.md"), "# Empty Edict repository\n")
    runProcess(repository, listOf("git", "add", "README.md"))
    runProcess(repository, listOf("git", "commit", "--quiet", "-m", "Initialize"))

    val runId = "loaded-edict-run"
    val inspection = IntellijMcpServerService(
      projectPath = repository,
      commandRunner = CommandRunner { command ->
        if ("start" in command) CommandResult(0, """{"status":"ready","url":"http://127.0.0.1:1/mcp"}""")
        else CommandResult(0, "{}")
      },
      clientFactory = InspectionKtsClientFactory { EmptyInspectionClient() },
    )
    val context = EdictSessionContext.getInstance(runId)
    context.load(
      workspace = EdictNextWorkspace.forRun(directory.resolve("logs"), runId),
      sourceRepository = repository,
      analyzedProject = repository,
      qodanaExecutable = "/qodana",
      inspectionServer = inspection,
    )
    try {
      Store(directory.resolve("state")).use { store ->
        val output = ByteArrayOutputStream()
        val input = """
          {"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"unit","version":"1"}}}
          {"jsonrpc":"2.0","method":"notifications/initialized"}
          {"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"edict_next_prepare_pipeline","arguments":{"worktreePath":"$repository"}}}
          {"jsonrpc":"2.0","id":3,"method":"ping"}
        """.trimIndent() + "\n"
        val transport = StdioServerTransport(
          input = DelayedEofInputStream(ByteArrayInputStream(input.encodeToByteArray())).asSource().buffered(),
          output = output.asSink().buffered(),
        )
        val closed = CompletableDeferred<Unit>()
        transport.onClose { closed.complete(Unit) }
        EdictNextMcpToolset(runId, McpServer(store)).createServer().createSession(transport)
        closed.await()

        val responses = output.toString(Charsets.UTF_8).lineSequence().filter(String::isNotBlank)
          .map { wireJson.parseToJsonElement(it).jsonObject }.toList()
        assertEquals(3, responses.size)
        val result = responses.single { it["id"].toString() == "2" }.obj("result")
        assertFalse(result.flag("isError") == true)
        assertContains(result.obj("structuredContent").text("summary"), "0 Signal(s) selected")
      }
    } finally {
      context.unload()
    }
  }

  private class EmptyInspectionClient : InspectionKtsClient {
    override suspend fun compile(code: String): InspectionKtsCompileResult = error("not used")
    override suspend fun runExamples(
      code: String,
      examples: List<InspectionKtsExampleRequest>,
    ): InspectionKtsBatchRunResult = error("not used")
    override suspend fun analyzeProject(code: String): InspectionKtsProjectRunResult = error("not used")
    override fun close() = Unit
  }

  private class DelayedEofInputStream(input: InputStream) : FilterInputStream(input) {
    private var delayed = false

    override fun read(): Int = delayAtEof(super.read())

    override fun read(bytes: ByteArray, offset: Int, length: Int): Int =
      delayAtEof(super.read(bytes, offset, length))

    private fun delayAtEof(result: Int): Int {
      if (result < 0 && !delayed) {
        delayed = true
        Thread.sleep(500)
      }
      return result
    }
  }
}
