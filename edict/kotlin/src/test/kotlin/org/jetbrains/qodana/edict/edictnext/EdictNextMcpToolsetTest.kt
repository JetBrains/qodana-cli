package org.jetbrains.qodana.edict.edictnext

import io.modelcontextprotocol.kotlin.sdk.server.StdioServerTransport
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.runBlocking
import kotlinx.io.asSink
import kotlinx.io.asSource
import kotlinx.io.buffered
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.addJsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonArray
import org.jetbrains.qodana.edict.common.EdictLayout
import org.jetbrains.qodana.edict.common.EdictCIConfiguration
import org.jetbrains.qodana.edict.common.EdictConfiguration
import org.jetbrains.qodana.edict.common.array
import org.jetbrains.qodana.edict.common.flag
import org.jetbrains.qodana.edict.common.obj
import org.jetbrains.qodana.edict.common.runProcess
import org.jetbrains.qodana.edict.common.text
import org.jetbrains.qodana.edict.common.wireJson
import org.jetbrains.qodana.edict.support.edictNextToolset
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
      val layout = EdictLayout(directory, directory.resolve(".edict"))
      val management = EdictManagementService(store, layout)
      val toolset = edictNextToolset(
        layout,
        management,
        configuration = EdictConfiguration(ci = EdictCIConfiguration("https://jetbrains.team/p/owner/repositories/repo")),
      )
      val server = toolset.createServer()
      assertTrue(server.tools.keys.containsAll(movedTools))
      assertTrue("edict_publish_signal" in server.tools)
      assertTrue("edict_publish_validated_pr_signals" in server.tools)
      assertTrue("edict_get_pr_analysis_coverage" in server.tools)
      assertTrue("edict_record_pr_analysis_coverage" in server.tools)
      assertFalse("edict_state_write" in server.tools)
      val delegateSchema = server.tools.getValue("edict_delegate").tool.inputSchema
      assertEquals(setOf("token", "taskId", "prompt"), checkNotNull(delegateSchema.properties).keys)
      assertEquals(listOf("token", "taskId", "prompt"), delegateSchema.required)
      val signalSchema = checkNotNull(
        server.tools.getValue("edict_publish_signal").tool.inputSchema.properties,
      ).getValue("signal").jsonObject
      assertEquals(
        setOf(
          "id", "idempotencyKey", "fileRevision", "source", "label", "description", "strength",
          "syntheticExampleId", "provenance",
        ),
        signalSchema.getValue("properties").jsonObject.keys,
      )
      assertEquals(
        listOf("id", "fileRevision", "source", "label", "description"),
        signalSchema.getValue("required").jsonArray.map { (it as JsonPrimitive).content },
      )
      val sourceVariants = signalSchema.getValue("properties").jsonObject.getValue("source").jsonObject
        .getValue("oneOf").jsonArray.map { it.jsonObject }
      assertEquals(4, sourceVariants.size)
      val fromPrSchema = sourceVariants.single {
        it.getValue("properties").jsonObject.getValue("type").jsonObject.getValue("enum").jsonArray
          .single() == JsonPrimitive("FromPR")
      }
      assertEquals(
        listOf("type", "prNumber", "title", "discussionMessages", "diffPositiveToNegative", "url"),
        fromPrSchema.getValue("required").jsonArray.map { (it as JsonPrimitive).content },
      )
      val publishPrSchema = server.tools.getValue("edict_publish_validated_pr_signals").tool.inputSchema
      assertEquals(setOf("token", "batchId"), checkNotNull(publishPrSchema.properties).keys)
      assertEquals(listOf("token", "batchId"), publishPrSchema.required)
      val coverageSchema = checkNotNull(
        server.tools.getValue("edict_record_pr_analysis_coverage").tool.inputSchema.properties,
      )
      assertEquals(
        setOf("token", "analyzedDateRanges", "analyzedPrNumbers"),
        coverageSchema.keys,
      )
      val fetchSchema = server.tools.getValue("edict_fetch_pr_batch").tool.inputSchema
      assertEquals(
        setOf("token", "startDate", "endDate", "maxPrs", "prNumbers"),
        checkNotNull(fetchSchema.properties).keys,
      )
      assertEquals(listOf("token", "maxPrs"), fetchSchema.required)

      val output = ByteArrayOutputStream()
      val input = """
        {"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"unit","version":"1"}}}
        {"jsonrpc":"2.0","method":"notifications/initialized"}
        {"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"edict_plan_create","arguments":{"request":"Extract","steps":[{"skill":"edict-batch-signal-analysis","title":"Commit"}]}}}
        {"jsonrpc":"2.0","id":3,"method":"ping"}
        {"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"edict_context","arguments":{}}}
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
      val context = responses.single { it["id"].toString() == "4" }.obj("result").obj("structuredContent")
      assertEquals("space", context.obj("reviewRepository").text("provider"))
      assertEquals("owner", context.obj("reviewRepository").text("owner"))
      assertEquals("repo", context.obj("reviewRepository").text("repo"))
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
      val layout = EdictLayout(directory, directory.resolve(".edict"))
      val server = edictNextToolset(layout, EdictManagementService(store, layout)).createServer()
      assertEquals(
        setOf(
          "edict_registry", "edict_plan_get", "edict_plan_create", "edict_task_add", "edict_delegate",
          "edict_task_get", "edict_task_start", "edict_task_finish", "edict_task_cancel", "edict_publish_signal",
          "edict_publish_validated_pr_signals",
          "edict_fetch_pr_batch", "edict_list_pr_analysis_items", "edict_get_pr_analysis_item",
          "edict_validate_pr_signals", "edict_pr_file_at_ref", "edict_pr_file_diff",
          "edict_get_pr_analysis_coverage", "edict_record_pr_analysis_coverage",
          "edict_context", "edict_file_at_ref", "edict_next_prepare_pipeline", "edict_next_next_signal", "edict_next_get_distribution_context",
          "edict_next_add_signal_to_cluster", "edict_next_validate_distribution", "edict_next_get_generation_clusters",
          "edict_next_validate_code_example", "edict_next_validate_cluster_examples", "edict_next_get_inspection_action",
          "edict_next_get_new_inspection_results", "edict_next_finalise_cluster",
          "edict_next_validate_generation", "edict_next_save_code_example", "edict_next_assign_code_example",
          "edict_next_delete_code_example", "edict_next_save_candidate_inspection", "edict_next_rename_cluster",
          "edict_next_append_cluster_history",
          "edict_promote_clusters", "ecict-check-promotion", "edict_promotion_decide_reviews",
          "generate_psi_tree", "generate_inspection_kts_examples", "generate_inspection_kts_api",
          "compile_inspection_kts", "run_inspection_kts",
        ),
        server.tools.keys,
      )
      val promoteSchema = server.tools.getValue("edict_promote_clusters").tool.inputSchema
      assertEquals(setOf("token", "clusterIds"), checkNotNull(promoteSchema.properties).keys)
      assertEquals(listOf("token"), promoteSchema.required)
      assertEquals(listOf("code", "language"), server.tools.getValue("generate_psi_tree").tool.inputSchema.required)
    }
  }

  @Test
  fun `inspection kts authoring tools are forwarded to the IDE verbatim`() = runBlocking {
    val client = ProxyInspectionClient()
    val inspection = IntellijMcpServerService(
      projectPath = directory,
      clientFactory = InspectionKtsClientFactory { client },
      serverLifecycle = FixedEndpointLifecycle,
    )
    val layout = EdictLayout(gitRepository(), gitRepository())
    try {
      EdictNextRepositoryState.open(directory.resolve("state")).use { store ->
        val output = ByteArrayOutputStream()
        val input = """
          {"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"unit","version":"1"}}}
          {"jsonrpc":"2.0","method":"notifications/initialized"}
          {"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"generate_psi_tree","arguments":{"code":"class A","language":"Kotlin"}}}
          {"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"generate_inspection_kts_api","arguments":{"language":"Cobol"}}}
          {"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"run_inspection_kts","arguments":{"inspectionKtsCode":"listOf()","contextPath":"A.kt"}}}
          {"jsonrpc":"2.0","id":5,"method":"ping"}
        """.trimIndent() + "\n"
        val transport = StdioServerTransport(
          input = DelayedEofInputStream(ByteArrayInputStream(input.encodeToByteArray())).asSource().buffered(),
          output = output.asSink().buffered(),
        )
        val closed = CompletableDeferred<Unit>()
        transport.onClose { closed.complete(Unit) }
        edictNextToolset(layout, EdictManagementService(store, layout), inspection).createServer().createSession(transport)
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
            "run_inspection_kts" to buildJsonObject { put("inspectionKtsCode", "listOf()"); put("contextPath", "A.kt") },
          ),
          client.calls,
        )
      }
    } finally {
      inspection.stop()
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
  fun `pipeline tools work on the run's state repository`() = runBlocking {
    val repository = Files.createDirectory(directory.resolve("repository"))
    runProcess(repository, listOf("git", "init", "--quiet", "--initial-branch=main"))
    runProcess(repository, listOf("git", "config", "user.email", "edict-test@localhost"))
    runProcess(repository, listOf("git", "config", "user.name", "Edict Test"))
    Files.writeString(repository.resolve("README.md"), "# Empty Edict repository\n")
    runProcess(repository, listOf("git", "add", "README.md"))
    runProcess(repository, listOf("git", "commit", "--quiet", "-m", "Initialize"))

    val inspection = IntellijMcpServerService(
      projectPath = repository,
      clientFactory = InspectionKtsClientFactory { EmptyInspectionClient() },
      serverLifecycle = FixedEndpointLifecycle,
    )
    val layout = EdictLayout(repository, repository, directory.resolve("log"))
    try {
      EdictNextRepositoryState.open(directory.resolve("state")).use { store ->
        val output = ByteArrayOutputStream()
        val input = """
          {"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"unit","version":"1"}}}
          {"jsonrpc":"2.0","method":"notifications/initialized"}
          {"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"edict_next_prepare_pipeline","arguments":{}}}
          {"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"generate_inspection_kts_api","arguments":{"language":"Java","projectPath":"/ignored"}}}
          {"jsonrpc":"2.0","id":4,"method":"ping"}
        """.trimIndent() + "\n"
        val transport = StdioServerTransport(
          input = DelayedEofInputStream(ByteArrayInputStream(input.encodeToByteArray())).asSource().buffered(),
          output = output.asSink().buffered(),
        )
        val closed = CompletableDeferred<Unit>()
        transport.onClose { closed.complete(Unit) }
        edictNextToolset(layout, EdictManagementService(store, layout), inspection).createServer().createSession(transport)
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
      inspection.stop()
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
