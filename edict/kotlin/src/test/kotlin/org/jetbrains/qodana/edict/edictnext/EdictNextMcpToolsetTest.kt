package org.jetbrains.qodana.edict.edictnext

import io.modelcontextprotocol.kotlin.sdk.server.StdioServerTransport
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.runBlocking
import kotlinx.io.asSink
import kotlinx.io.asSource
import kotlinx.io.buffered
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.addJsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonArray
import org.jetbrains.qodana.edict.common.array
import org.jetbrains.qodana.edict.common.flag
import org.jetbrains.qodana.edict.common.obj
import org.jetbrains.qodana.edict.common.runProcess
import org.jetbrains.qodana.edict.common.text
import org.jetbrains.qodana.edict.common.wireJson
import org.jetbrains.qodana.edict.reviews.ReviewClient
import org.junit.jupiter.api.io.TempDir
import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream
import java.io.FilterInputStream
import java.io.InputStream
import java.net.URI
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
      "edict_next_save_code_example",
      "edict_next_assign_code_example",
      "edict_next_delete_code_example",
      "edict_next_save_candidate_inspection",
      "edict_next_append_cluster_history",
      "generate_psi_tree",
      "generate_inspection_kts_api",
      "generate_inspection_kts_examples",
    )
    EdictNextRepositoryState.open(directory.resolve("state")).use { store ->
      val management = EdictManagementService(store)
      val toolset = EdictNextMcpToolset("test-run", management)
      val server = toolset.createServer()
      assertTrue(server.tools.keys.containsAll(movedTools))
      assertTrue("edict_publish_signal" in server.tools)
      assertFalse("edict_state_write" in server.tools)
      assertTrue(server.tools.keys.intersect(INSPECTION_KTS_UPSTREAM_TOOL_NAMES) == INSPECTION_KTS_AGENT_TOOL_NAMES)
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
  fun `toolset exposes every tool agents used from the single IDE server`() {
    EdictNextRepositoryState.open(directory.resolve("state")).use { store ->
      val server = EdictNextMcpToolset("test-run", EdictManagementService(store, extensions = listOf(EdictPrAnalysisService(store, ReviewClient()))))
        .createServer()
      assertEquals(
        setOf(
          "edict_registry", "edict_plan_get", "edict_plan_create", "edict_task_add", "edict_delegate",
          "edict_task_get", "edict_task_start", "edict_task_finish", "edict_task_cancel", "edict_state_write",
          "edict_prepare_pr_analysis", "edict_list_pr_analysis_items", "edict_get_pr_analysis_item",
          "edict_validate_pr_signals", "edict_pr_file_at_ref", "edict_pr_file_diff",
          "edict_next_prepare_pipeline", "edict_next_next_signal", "edict_next_get_distribution_context",
          "edict_next_add_signal_to_cluster", "edict_next_validate_distribution", "edict_next_get_generation_clusters",
          "edict_next_validate_code_example", "edict_next_validate_cluster_examples", "edict_next_get_inspection_action",
          "edict_next_validate_inspection", "edict_next_get_new_inspection_results", "edict_next_mark_generated",
          "edict_next_validate_generation", "edict_next_save_code_example", "edict_next_assign_code_example",
          "edict_next_delete_code_example", "edict_next_save_candidate_inspection", "edict_next_append_cluster_history",
          "generate_psi_tree", "generate_inspection_kts_examples", "generate_inspection_kts_api",
          "compile_inspection_kts", "run_inspection_kts", "run_inspection_kts_examples", "run_inspection_kts_project",
        ),
        server.tools.keys,
      )
      assertEquals(listOf("code", "language"), server.tools.getValue("generate_psi_tree").tool.inputSchema.required)
    }
  }

  @Test
  fun `inspection kts authoring tools are forwarded to the IDE verbatim`() = runBlocking {
    val runId = "proxy-edict-run"
    val client = ProxyInspectionClient()
    val context = EdictSessionContext.getInstance(runId)
    context.load(
      workspace = EdictNextWorkspace.forRun(directory.resolve("logs"), runId),
      sourceRepository = gitRepository(),
      analyzedProject = gitRepository(),
      qodanaExecutable = "/qodana",
      inspectionServer = IntellijMcpServerService(
        projectPath = directory,
        clientFactory = InspectionKtsClientFactory { client },
        serverLifecycle = FixedEndpointLifecycle,
      ),
    )
    try {
      EdictNextRepositoryState.open(directory.resolve("state")).use { store ->
        val output = ByteArrayOutputStream()
        val input = """
          {"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"unit","version":"1"}}}
          {"jsonrpc":"2.0","method":"notifications/initialized"}
          {"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"generate_psi_tree","arguments":{"code":"class A","language":"Kotlin"}}}
          {"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"generate_inspection_kts_api","arguments":{"language":"Cobol"}}}
          {"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"run_inspection_kts_project","arguments":{"inspectionKtsCode":"listOf()"}}}
          {"jsonrpc":"2.0","id":5,"method":"ping"}
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
        val psiTree = responses.single { it["id"].toString() == "2" }.obj("result")
        assertFalse(psiTree.flag("isError") == true)
        assertEquals("tree of class A", psiTree.getValue("content").jsonArray.single().jsonObject.text("text"))
        val api = responses.single { it["id"].toString() == "3" }.obj("result")
        assertTrue(api.flag("isError") == true)
        assertEquals(
          listOf(
            "generate_psi_tree" to buildJsonObject { put("code", "class A"); put("language", "Kotlin") },
            "generate_inspection_kts_api" to buildJsonObject { put("language", "Cobol") },
            "run_inspection_kts_project" to buildJsonObject { put("inspectionKtsCode", "listOf()") },
          ),
          client.calls,
        )
      }
    } finally {
      context.unload()
    }
  }

  private fun gitRepository(): Path {
    val repository = directory.resolve("repository")
    if (Files.exists(repository)) return repository
    Files.createDirectory(repository)
    runProcess(repository, listOf("git", "init", "--quiet", "--initial-branch=main"))
    runProcess(repository, listOf("git", "config", "user.email", "edict-test@localhost"))
    runProcess(repository, listOf("git", "config", "user.name", "Edict Test"))
    Files.writeString(repository.resolve("README.md"), "# Empty Edict repository\n")
    runProcess(repository, listOf("git", "add", "README.md"))
    runProcess(repository, listOf("git", "commit", "--quiet", "-m", "Initialize"))
    return repository
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
      clientFactory = InspectionKtsClientFactory { EmptyInspectionClient() },
      serverLifecycle = FixedEndpointLifecycle,
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
          {"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"edict_next_prepare_pipeline","arguments":{"worktreePath":"$repository"}}}
          {"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"generate_inspection_kts_api","arguments":{"language":"Java","projectPath":"/ignored"}}}
          {"jsonrpc":"2.0","id":4,"method":"ping"}
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
        assertEquals(4, responses.size)
        val result = responses.single { it["id"].toString() == "2" }.obj("result")
        assertFalse(result.flag("isError") == true)
        assertContains(result.obj("structuredContent").text("summary"), "0 Signal(s) selected")
        val api = responses.single { it["id"].toString() == "3" }.obj("result")
        assertFalse(api.flag("isError") == true)
        assertEquals("inspection api", api.array("content").single().text("text"))
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
    override suspend fun callTool(name: String, arguments: JsonObject): JsonObject = buildJsonObject {
      put("isError", false)
      put("content", JsonArray(listOf(buildJsonObject {
        put("type", "text")
        put("text", "inspection api")
      })))
    }
    override fun close() = Unit
  }

  private class ProxyInspectionClient : InspectionKtsClient {
    val calls = mutableListOf<Pair<String, JsonObject>>()
    override suspend fun compile(code: String): InspectionKtsCompileResult = error("not used")
    override suspend fun runExamples(
      code: String,
      examples: List<InspectionKtsExampleRequest>,
    ): InspectionKtsBatchRunResult = error("not used")
    override suspend fun analyzeProject(code: String): InspectionKtsProjectRunResult = error("not used")
    override suspend fun callTool(name: String, arguments: JsonObject): JsonObject {
      calls += name to arguments
      return if (name == "generate_psi_tree") {
        buildJsonObject { putJsonArray("content") { addJsonObject { put("type", "text"); put("text", "tree of class A") } } }
      }
      else {
        buildJsonObject {
          putJsonArray("content") { addJsonObject { put("type", "text"); put("text", "Unsupported language") } }
          put("isError", true)
        }
      }
    }
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

private object FixedEndpointLifecycle : IntellijMcpServerLifecycle {
  override suspend fun start(): URI = URI("http://127.0.0.1:1/mcp")
  override suspend fun stop() = Unit
}
