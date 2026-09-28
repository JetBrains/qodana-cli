package org.jetbrains.qodana.edict.edictnext

import io.modelcontextprotocol.kotlin.sdk.server.StdioServerTransport
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.runBlocking
import kotlinx.io.asSink
import kotlinx.io.asSource
import kotlinx.io.buffered
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.put
import org.jetbrains.qodana.edict.common.flag
import org.jetbrains.qodana.edict.common.obj
import org.jetbrains.qodana.edict.common.runProcess
import org.jetbrains.qodana.edict.common.text
import org.jetbrains.qodana.edict.common.wireJson
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
import kotlin.test.assertTrue

class EdictNextMcpToolsetTest {
  @TempDir
  lateinit var directory: Path

  @Test
  fun `management service tools are exposed by the Edict Next toolset`() = runBlocking {
    val movedTools = setOf(
      "edict_registry",
      "edict_plan_get",
      "edict_plan_create",
      "edict_task_add",
      "edict_delegate",
      "edict_task_get",
      "edict_task_start",
      "edict_task_finish",
      "edict_task_cancel",
    )
    EdictNextRepositoryState.open(directory.resolve("state")).use { store ->
      val management = EdictManagementService(store)
      val toolset = EdictNextMcpToolset("test-run", management)
      val server = toolset.createServer()
      assertTrue(server.tools.keys.containsAll(movedTools))
      val delegateSchema = server.tools.getValue("edict_delegate").tool.inputSchema
      assertEquals(setOf("token", "taskId", "prompt"), checkNotNull(delegateSchema.properties).keys)
      assertEquals(listOf("token", "taskId", "prompt"), delegateSchema.required)

      val output = ByteArrayOutputStream()
      val input = """
        {"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"unit","version":"1"}}}
        {"jsonrpc":"2.0","method":"notifications/initialized"}
        {"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"edict_plan_create","arguments":{"request":"Extract","steps":[{"skill":"edict-batch-signal-analysis","title":"Commit"}]}}}
        {"jsonrpc":"2.0","id":3,"method":"ping"}
      """.trimIndent() + "\n"
      val transport = StdioServerTransport(
        input = DelayedEofInputStream(ByteArrayInputStream(input.encodeToByteArray())).asSource().buffered(),
        output = output.asSink().buffered(),
      )
      val closed = CompletableDeferred<Unit>()
      transport.onClose { closed.complete(Unit) }
      server.createSession(transport)
      closed.await()
      val responses = output.toString(Charsets.UTF_8).lineSequence().filter(String::isNotBlank)
        .map { wireJson.parseToJsonElement(it).jsonObject }.toList()
      val creation = responses.single { it["id"].toString() == "2" }.obj("result")
      assertFalse(creation.flag("isError") == true)
      assertEquals("Extract", store.plan()?.request)
      val task = store.plan()!!.tasks.single()
      val token = creation.obj("structuredContent").text("token")
      val obsoleteArguments = management.call("edict_delegate", buildJsonObject {
        put("token", token)
        put("taskId", task.id)
        put("prompt", "\$edict-batch-signal-analysis\nRead /skills/edict-batch-signal-analysis/SKILL.md")
        put("operations", JsonArray(emptyList()))
      })
      assertTrue(obsoleteArguments.flag("isError") == true)
      val delegation = management.call("edict_delegate", buildJsonObject {
        put("token", token)
        put("taskId", task.id)
        put("prompt", "\$edict-batch-signal-analysis\nRead /skills/edict-batch-signal-analysis/SKILL.md")
      })
      assertFalse(delegation.flag("isError") == true)
      assertEquals("delegated", store.plan()!!.tasks.single().status)
    }
  }

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
      EdictNextRepositoryState.open(directory.resolve("state")).use { store ->
        val output = ByteArrayOutputStream()
        val input = """
          {"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"unit","version":"1"}}}
          {"jsonrpc":"2.0","method":"notifications/initialized"}
          {"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"edict_prepare_pipeline","arguments":{"worktreePath":"$repository"}}}
          {"jsonrpc":"2.0","id":3,"method":"ping"}
        """.trimIndent() + "\n"
        val transport = StdioServerTransport(
          input = DelayedEofInputStream(ByteArrayInputStream(input.encodeToByteArray())).asSource().buffered(),
          output = output.asSink().buffered(),
        )
        val closed = CompletableDeferred<Unit>()
        transport.onClose { closed.complete(Unit) }
        EdictNextMcpToolset(runId, EdictManagementService(store)).createServer().createSession(transport)
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
