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
import org.jetbrains.qodana.edict.common.ExtractionConfiguration
import org.jetbrains.qodana.edict.common.array
import org.jetbrains.qodana.edict.common.flag
import org.jetbrains.qodana.edict.common.obj
import org.jetbrains.qodana.edict.common.runProcess
import org.jetbrains.qodana.edict.common.text
import org.jetbrains.qodana.edict.common.wireJson
import org.jetbrains.qodana.edict.ci.ReviewClient
import org.jetbrains.qodana.edict.history.enrichment.EdictNextEnrichmentMcpToolset
import org.jetbrains.qodana.edict.history.enrichment.EdictNextEnrichmentService
import org.jetbrains.qodana.edict.promotion.PromotionService
import org.jetbrains.qodana.edict.support.edictNextToolset
import org.jetbrains.qodana.edict.support.fixtureSignals
import org.jetbrains.qodana.edict.support.gitFixture
import org.jetbrains.qodana.edict.support.launch
import org.junit.jupiter.api.io.TempDir
import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream
import java.io.FilterInputStream
import java.io.InputStream
import java.net.URI
import java.nio.file.Files
import java.nio.file.Path
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
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
      "edict_task_cleanup_scratch",
      "edict_next_save_code_example",
      "edict_next_update_code_example_expected_range",
      "edict_next_assign_code_example",
      "edict_next_delete_code_example",
      "edict_next_save_candidate_inspection",
      "edict_next_append_cluster_history",
      "generate_psi_tree",
      "generate_inspection_kts_api",
      "generate_inspection_kts_examples",
      "edict_prepare_weak_signal_promotion",
      "edict_promote_weak_signal",
      "edict_check_weak_signal_promotion",
      "edict_record_weak_signal_promotion_signals",
      "edict_complete_weak_signal_promotion",
    )
    EdictNextRepositoryState.open(directory.resolve("state")).use { store ->
      val layout = EdictLayout(directory, directory.resolve(".edict"))
      val management = EdictManagementService(store, layout)
      val toolset = edictNextToolset(
        layout,
        management,
        configuration = EdictConfiguration(
          ci = EdictCIConfiguration("https://jetbrains.team/p/owner/repositories/repo"),
          extraction = ExtractionConfiguration(defaultSignalCount = 17),
        ),
      )
      val server = toolset.createServer()
      assertTrue(server.tools.keys.containsAll(movedTools))
      assertFalse("edict_calculate_price" in server.tools)
      assertTrue("edict_publish_signal" in server.tools)
      val regularSignal = server.tools.getValue("edict_publish_signal").tool.inputSchema.properties!!.getValue("signal")
      val weakSignals = server.tools.getValue("edict_record_weak_signal_promotion_signals").tool.inputSchema.properties!!.getValue("signals").jsonObject
      assertEquals(regularSignal, weakSignals.getValue("items"))
      assertFalse("processed" in regularSignal.jsonObject.getValue("properties").jsonObject)
      val clusterPublication = server.tools.getValue("edict_publish_signal_to_cluster").tool.inputSchema
      assertEquals(listOf("token", "clusterId", "signal"), clusterPublication.required)
      assertEquals(setOf("token", "clusterId", "signal"), checkNotNull(clusterPublication.properties).keys)
      val historyContext = server.tools.getValue("edict_next_get_history_context").tool.inputSchema
      assertEquals(listOf("token", "clusterId"), historyContext.required)
      assertEquals(setOf("token", "clusterId"), checkNotNull(historyContext.properties).keys)
      assertFalse("edict_next_record_history_analysis" in server.tools)
      assertTrue("edict_get_cluster_signals" in server.tools)
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
      assertEquals(signalSchema, checkNotNull(clusterPublication.properties).getValue("signal"))
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
      assertEquals(
        JsonPrimitive(4000),
        fromPrSchema.getValue("properties").jsonObject.getValue("diffPositiveToNegative").jsonObject.getValue("maxLength"),
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
        setOf("token", "startDate", "endDate", "prNumbers"),
        checkNotNull(fetchSchema.properties).keys,
      )
      assertEquals(listOf("token"), fetchSchema.required)
      val chunkSchema = server.tools.getValue("edict_get_pr_analysis_chunk").tool.inputSchema
      assertEquals(setOf("token", "batchId", "chunkIndex"), checkNotNull(chunkSchema.properties).keys)
      assertEquals(listOf("token", "batchId", "chunkIndex"), chunkSchema.required)
      val storePrSchema = server.tools.getValue("edict_store_pr_signals").tool.inputSchema
      assertEquals(
        setOf("token", "batchId", "chunkIndex", "inspectedWorkItemIds", "failedWorkItemIds", "signals", "signalDrafts", "problems"),
        checkNotNull(storePrSchema.properties).keys,
      )
      assertEquals(
        listOf("token", "batchId", "chunkIndex", "inspectedWorkItemIds", "failedWorkItemIds", "signals", "problems"),
        storePrSchema.required,
      )
      val draftDiffSchema = checkNotNull(storePrSchema.properties).getValue("signalDrafts").jsonObject
        .getValue("items").jsonObject.getValue("properties").jsonObject.getValue("diff").jsonObject
      assertTrue(JsonPrimitive("excerpt") in draftDiffSchema.getValue("required").jsonArray)
      assertEquals(JsonPrimitive(4000), draftDiffSchema.getValue("properties").jsonObject.getValue("excerpt").jsonObject.getValue("maxLength"))
      val contextSchema = server.tools.getValue("edict_context").tool.inputSchema
      assertEquals(setOf("token"), checkNotNull(contextSchema.properties).keys)
      assertEquals(listOf("token"), contextSchema.required)
      val distributionSchema = server.tools.getValue("edict_next_next_signal").tool.inputSchema
      assertTrue(distributionSchema.required.orEmpty().isEmpty())
      assertTrue(distributionSchema.properties.orEmpty().isEmpty())
      val cleanupSchema = server.tools.getValue("edict_task_cleanup_scratch").tool.inputSchema
      assertEquals(setOf("token", "taskId"), checkNotNull(cleanupSchema.properties).keys)
      assertEquals(listOf("token", "taskId"), cleanupSchema.required)

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
      val token = creation.obj("structuredContent").text("token")
      val contextOutput = ByteArrayOutputStream()
      val contextInput = """
        {"jsonrpc":"2.0","id":4,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"unit","version":"1"}}}
        {"jsonrpc":"2.0","method":"notifications/initialized"}
        {"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"edict_context","arguments":{"token":"$token"}}}
      """.trimIndent() + "\n"
      val contextTransport = StdioServerTransport(
        input = DelayedEofInputStream(ByteArrayInputStream(contextInput.encodeToByteArray())).asSource().buffered(),
        output = contextOutput.asSink().buffered(),
      )
      val contextClosed = CompletableDeferred<Unit>()
      contextTransport.onClose { contextClosed.complete(Unit) }
      server.createSession(contextTransport)
      contextClosed.await()
      val context = contextOutput.toString(Charsets.UTF_8).lineSequence().filter(String::isNotBlank)
        .map { wireJson.parseToJsonElement(it).jsonObject }
        .single { it["id"].toString() == "5" }.obj("result").obj("structuredContent")
      assertEquals("space", context.obj("reviewRepository").text("provider"))
      assertEquals("owner", context.obj("reviewRepository").text("owner"))
      assertEquals("repo", context.obj("reviewRepository").text("repo"))
      assertEquals(JsonPrimitive(17), context["defaultSignalCount"])
      assertEquals(layout.taskScratchDirectory(EdictLayout.MANAGER_SCRATCH_ID).toString(), context.text("scratchDirectory"))
      assertEquals("Extract", store.plan()?.request)
      val task = store.plan()!!.tasks.single()
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
      val childToken = delegation.obj("structuredContent").text("token")
      val assignment = management.call("edict_task_get", buildJsonObject { put("token", childToken) })
        .obj("structuredContent")
      val taskScratch = layout.taskScratchDirectory(task.id)
      assertEquals(taskScratch.toString(), assignment.text("scratchDirectory"))
      assertTrue(Files.isDirectory(taskScratch))
      Files.writeString(taskScratch.resolve("artifact"), "durable output already stored")
      val prematureCleanup = management.call("edict_task_cleanup_scratch", buildJsonObject {
        put("token", token)
        put("taskId", task.id)
      })
      assertTrue(prematureCleanup.flag("isError") == true)
      assertFalse(management.call("edict_task_start", buildJsonObject {
        put("token", childToken)
        put("agentId", "scratch-test-agent")
        put("skill", task.skill)
      }).flag("isError") == true)
      assertFalse(management.call("edict_task_finish", buildJsonObject {
        put("token", childToken)
        put("status", "completed")
        put("result", "Stored durable output")
      }).flag("isError") == true)
      val cleanup = management.call("edict_task_cleanup_scratch", buildJsonObject {
        put("token", token)
        put("taskId", task.id)
      })
      assertFalse(cleanup.flag("isError") == true)
      assertTrue(cleanup.obj("structuredContent").flag("removed") == true)
      assertFalse(Files.exists(taskScratch))
    }
  }

  @Test
  fun `cluster publication requires history capability and accepts the strict Signal model`() = runBlocking {
    val project = gitFixture(directory.resolve("project"))
    val layout = EdictLayout(project.root, directory.resolve("state"))
    EdictNextRepositoryState.open(layout.stateDirectory).use { store ->
      runProcess(store.root, listOf("git", "init", "--quiet"))
      val repository = EdictRepository(EdictRepositoryDirectory(store.root))
      val signals = fixtureSignals(project)
      Files.createDirectories(repository.paths.inboxDirectory)
      Files.writeString(repository.paths.inboxDirectory.resolve("${signals.first().id}.json"), EdictNextJson.encodeToString(signals.first()))
      repository.addSignalToCluster(signals.first().id, "string-equality")
      val inspection = IntellijMcpServerService(project.root)
      val generation = EdictNextGenerationService(repository, inspection, project.root)
      val plan = store.createPlan("Enrich the selected cluster", listOf(EdictNextRepositoryState.Step("edict-git-history-signal-enrichment", "Find independent corrections")))
      val worker = store.launch(plan.token, plan.plan.tasks.single().id, "edict-git-history-signal-enrichment")
      val signal = EdictNextJson.encodeToJsonElement(EdictNextSignal.serializer(), signals.last()).jsonObject
      val unsupportedSource = JsonObject(signal.getValue("source").jsonObject + ("parentRevision" to JsonPrimitive(project.resolve("HEAD^"))))
      fun publication(id: Int, token: String, document: JsonObject) = buildJsonObject {
        put("jsonrpc", "2.0")
        put("id", id)
        put("method", "tools/call")
        put("params", buildJsonObject {
          put("name", "edict_publish_signal_to_cluster")
          put("arguments", buildJsonObject {
            put("token", token)
            put("clusterId", "string-equality")
            put("signal", document)
          })
        })
      }.toString()
      fun historyCall(id: Int, token: String, tool: String = "edict_next_get_history_context") = buildJsonObject {
        put("jsonrpc", "2.0")
        put("id", id)
        put("method", "tools/call")
        put("params", buildJsonObject {
          put("name", tool)
          put("arguments", buildJsonObject {
            put("token", token)
            put("clusterId", "string-equality")
          })
        })
      }.toString()
      val input = listOf(
        """{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"unit","version":"1"}}}""",
        """{"jsonrpc":"2.0","method":"notifications/initialized"}""",
        publication(2, plan.token, signal),
        publication(3, worker.token, JsonObject(signal + ("source" to unsupportedSource))),
        publication(4, worker.token, signal),
        publication(5, worker.token, signal),
        historyCall(6, plan.token),
        historyCall(7, plan.token, "edict_get_cluster_signals"),
        historyCall(8, worker.token),
        historyCall(9, worker.token, "edict_get_cluster_signals"),
        historyCall(10, worker.token),
      ).joinToString("\n", postfix = "\n")
      val responsesReceived = CountDownLatch(1)
      val output = object : ByteArrayOutputStream() {
        @Synchronized
        override fun write(bytes: ByteArray, offset: Int, length: Int) {
          super.write(bytes, offset, length)
          val completeLines = toString(Charsets.UTF_8).substringBeforeLast('\n', "")
          val ids = completeLines.lineSequence().filter(String::isNotBlank)
            .map { wireJson.parseToJsonElement(it).jsonObject["id"].toString() }.toSet()
          if ((1..10).all { it.toString() in ids }) responsesReceived.countDown()
        }
      }
      val transport = StdioServerTransport(
        input = DelayedEofInputStream(ByteArrayInputStream(input.encodeToByteArray())) {
          check(responsesReceived.await(10, TimeUnit.SECONDS)) { "Timed out waiting for all history-tool responses" }
        }.asSource().buffered(),
        output = output.asSink().buffered(),
      )
      val closed = CompletableDeferred<Unit>()
      transport.onClose { closed.complete(Unit) }
      val management = EdictManagementService(store, layout)
      EdictNextMcpToolset(
        layout, inspection, management,
        EdictNextDistributionService(repository, layout.neighboursResponsePath), generation,
        EdictNextEnrichmentMcpToolset(management, EdictNextEnrichmentService(repository, project.root)),
        EdictSourceFileService(project.root, ReviewClient(), null),
        promotion = PromotionService({ repository }, EdictConfiguration()),
      ).createServer().createSession(transport)
      closed.await()
      val responses = output.toString(Charsets.UTF_8).lineSequence().filter(String::isNotBlank)
        .map { wireJson.parseToJsonElement(it).jsonObject }.associateBy { it["id"].toString() }
      assertTrue(responses.getValue("2").obj("result").flag("isError") == true)
      assertTrue(responses.getValue("3").obj("result").flag("isError") == true)
      for (id in listOf("4", "5")) assertFalse(responses.getValue(id).obj("result").flag("isError") == true)
      assertEquals(setOf(true, false), listOf("4", "5").map { responses.getValue(it).obj("result").obj("structuredContent").flag("created") }.toSet())
      assertEquals(signals.map { it.id }.toSet(), repository.clusterSignalIds("string-equality"))
      assertTrue(repository.loadInboxSignals().isEmpty())
      assertTrue(responses.getValue("6").obj("result").flag("isError") == true)
      for (id in listOf("7", "8", "9", "10")) assertFalse(responses.getValue(id).obj("result").flag("isError") == true)
      val context = responses.getValue("10").obj("result").obj("structuredContent")
      assertEquals("string-equality", context.text("clusterId"))
      assertFalse("signals" in context || "coverage" in context)
      assertEquals(project.resolve("HEAD"), context.obj("range").text("headRevision"))
      assertTrue(context.obj("range").getValue("commitRevisions").jsonArray.isNotEmpty())
      // Common evidence reads have no history capability or distribution/generation lifecycle prerequisite.
      for (id in listOf("7", "9")) {
        val evidence = responses.getValue(id).obj("result").obj("structuredContent")
        assertEquals("string-equality", evidence.text("clusterId"))
        assertTrue(evidence.array("signals").isNotEmpty())
      }
      val evidence = management.call("edict_get_cluster_signals", buildJsonObject { put("clusterId", "string-equality") })
        .obj("structuredContent")
      assertEquals(
        repository.loadCluster("string-equality").signals.map { it.id }.toSet(),
        evidence.array("signals").map { it.jsonObject.text("id") }.toSet(),
      )
      generation.selectGenerationClusters()
      assertTrue(generation.appendHistory("string-equality", "Generate from the accepted evidence").success)
    }
  }

  @Test
  fun `generator can correct only an expected range and cannot edit or reassign examples`() = runBlocking {
    val project = gitFixture(directory.resolve("project"))
    val layout = EdictLayout(project.root, directory.resolve("state"))
    EdictNextRepositoryState.open(layout.stateDirectory).use { store ->
      val repository = EdictRepository(EdictRepositoryDirectory(store.root))
      val signal = fixtureSignals(project).first()
      Files.createDirectories(repository.paths.inboxDirectory)
      Files.writeString(repository.paths.inboxDirectory.resolve("${signal.id}.json"), EdictNextJson.encodeToString(signal))
      repository.addSignalToCluster(signal.id, "string-equality")
      val example = EdictNextCodeExampleMetadata("public-helper", "Example.java", EdictNextSignalLabel.POSITIVE, listOf(EdictNextLineRange(2, 2)))
      val source = "class Example {\n}"
      repository.saveCodeExample("string-equality", example.id, example, source)
      repository.assignCodeExample("string-equality", signal.id, example.id)
      val before = repository.loadCluster("string-equality")
      val plan = store.createPlan("Generate", listOf(
        EdictNextRepositoryState.Step("edict-next-cluster-generation", "Generate string-equality"),
        EdictNextRepositoryState.Step("edict-git-history-signal-enrichment", "Find evidence"),
      ))
      val generator = store.launch(plan.token, plan.plan.tasks[0].id, "edict-next-cluster-generation")
      val unrelated = store.launch(plan.token, plan.plan.tasks[1].id, "edict-git-history-signal-enrichment")
      val tool = "edict_next_update_code_example_expected_range"
      val management = EdictManagementService(store, layout)
      val server = edictNextToolset(layout, management).createServer()
      val schema = server.tools.getValue(tool).tool.inputSchema
      assertEquals(setOf("token", "clusterId", "exampleId", "startLine", "endLine"), checkNotNull(schema.properties).keys)
      assertEquals(listOf("token", "clusterId", "exampleId", "startLine", "endLine"), schema.required)
      fun arguments(token: String) = buildJsonObject {
        put("token", token)
        put("clusterId", "string-equality")
        put("exampleId", example.id)
        put("startLine", 1)
        put("endLine", 1)
      }
      val base = arguments(generator.token)
      val denied = mutableListOf(
        tool to arguments(plan.token),
        tool to arguments(unrelated.token),
        tool to JsonObject(base - "endLine"),
        tool to JsonObject(base + ("startLine" to JsonPrimitive("not a line"))),
        "edict_next_save_code_example" to buildJsonObject {
          put("token", generator.token)
          put("clusterId", "string-equality")
          put("exampleId", example.id)
          put("metadataJson", EdictNextJson.encodeToString(example.copy(label = EdictNextSignalLabel.NEGATIVE)))
          put("sourceCode", "class Replaced {}")
        },
        "edict_next_assign_code_example" to buildJsonObject {
          put("token", generator.token)
          put("clusterId", "string-equality")
          put("signalId", signal.id)
          put("exampleId", example.id)
        },
        "edict_next_delete_code_example" to buildJsonObject {
          put("token", generator.token)
          put("clusterId", "string-equality")
          put("exampleId", example.id)
        },
      )
      for (field in listOf("sourceCode", "metadataJson", "label", "fileName", "signalId", "expectedRanges")) {
        denied += tool to JsonObject(base + (field to JsonPrimitive("must not change")))
      }
      val calls = denied + (tool to base)
      val input = listOf(
        """{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"unit","version":"1"}}}""",
        """{"jsonrpc":"2.0","method":"notifications/initialized"}""",
      ) + calls.mapIndexed { index, (name, args) ->
        buildJsonObject {
          put("jsonrpc", "2.0")
          put("id", index + 2)
          put("method", "tools/call")
          put("params", buildJsonObject { put("name", name); put("arguments", args) })
        }.toString()
      }
      val responsesReceived = CountDownLatch(1)
      val output = object : ByteArrayOutputStream() {
        @Synchronized
        override fun write(bytes: ByteArray, offset: Int, length: Int) {
          super.write(bytes, offset, length)
          val ids = toString(Charsets.UTF_8).substringBeforeLast('\n', "").lineSequence().filter(String::isNotBlank)
            .map { wireJson.parseToJsonElement(it).jsonObject["id"].toString() }.toSet()
          if ((1..calls.size + 1).all { it.toString() in ids }) responsesReceived.countDown()
        }
      }
      val transport = StdioServerTransport(
        input = DelayedEofInputStream(ByteArrayInputStream(input.joinToString("\n", postfix = "\n").encodeToByteArray())) {
          check(responsesReceived.await(10, TimeUnit.SECONDS)) { "Timed out waiting for range-correction responses" }
        }.asSource().buffered(),
        output = output.asSink().buffered(),
      )
      val closed = CompletableDeferred<Unit>()
      transport.onClose { closed.complete(Unit) }
      server.createSession(transport)
      closed.await()
      val responses = output.toString(Charsets.UTF_8).lineSequence().filter(String::isNotBlank)
        .map { wireJson.parseToJsonElement(it).jsonObject }.associateBy { it["id"].toString() }
      for (index in denied.indices) {
        assertTrue(responses.getValue((index + 2).toString()).obj("result").flag("isError") == true, "Denied call $index")
      }
      val result = responses.getValue((calls.size + 1).toString()).obj("result")
      assertFalse(result.flag("isError") == true)
      assertTrue(result.obj("structuredContent").flag("success") == true)
      val after = repository.loadCluster("string-equality")
      assertEquals(before.manifest, after.manifest)
      assertEquals(before.signals, after.signals)
      assertEquals(source, after.examples.single().code)
      assertEquals(example.copy(expectedRanges = listOf(EdictNextLineRange(1, 1))), after.examples.single().metadata)
    }
  }

  @Test
  fun `generation suggestion tool has an optional count and documents read only behavior`() {
    EdictNextRepositoryState.open(directory.resolve("selection-state")).use { store ->
      val layout = EdictLayout(directory, store.root)
      val server = edictNextToolset(layout, EdictManagementService(store, layout)).createServer()
      assertFalse("edict_next_get_generation_clusters" in server.tools)
      assertFalse("edict_next_validate_generation" in server.tools)
      val selection = server.tools.getValue("edict_next_select_generation_clusters").tool
      assertEquals(setOf("generationCount"), checkNotNull(selection.inputSchema.properties).keys)
      assertTrue(selection.inputSchema.required.isNullOrEmpty())
      assertContains(selection.description.orEmpty(), "Read-only")
      assertContains(selection.description.orEmpty(), "may choose other Pending clusters")
    }
  }

  @Test
  fun `price switch exposes the manager price tool`() {
    EdictNextRepositoryState.open(directory.resolve("price-state")).use { store ->
      val layout = EdictLayout(directory, store.root)
      val server = edictNextToolset(
        layout,
        EdictManagementService(store, layout),
        configuration = EdictConfiguration(calculatePrice = true),
      ).createServer()
      val schema = server.tools.getValue("edict_calculate_price").tool.inputSchema
      assertEquals(setOf("token"), checkNotNull(schema.properties).keys)
      assertEquals(listOf("token"), schema.required)
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

  @Test
  fun `weak-signal tools accept only their dedicated managed worker`() = runBlocking {
    val layout = EdictLayout(directory, directory.resolve("state"))
    EdictNextRepositoryState.open(layout.stateDirectory).use { store ->
      val manager = store.createPlan("Promote generated inspections", listOf(
        EdictNextRepositoryState.Step("edict-promote", "Normal promotion"),
        EdictNextRepositoryState.Step("edict-promote-weak-signal", "Source fixes"),
        EdictNextRepositoryState.Step("edict-check-promotion", "Inspection reviews"),
        EdictNextRepositoryState.Step("edict-check-weak-signal-promotion", "Source-fix reviews"),
      ))
      val normal = store.launch(manager.token, manager.plan.tasks[0].id, "edict-promote")
      val weak = store.launch(manager.token, manager.plan.tasks[1].id, "edict-promote-weak-signal")
      val normalChecker = store.launch(manager.token, manager.plan.tasks[2].id, "edict-check-promotion")
      val weakChecker = store.launch(manager.token, manager.plan.tasks[3].id, "edict-check-weak-signal-promotion")
      val output = ByteArrayOutputStream()
      val input = """
        {"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"unit","version":"1"}}}
        {"jsonrpc":"2.0","method":"notifications/initialized"}
        {"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"edict_prepare_weak_signal_promotion","arguments":{"token":"${weak.token}"}}}
        {"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"edict_prepare_weak_signal_promotion","arguments":{"token":"${normal.token}"}}}
        {"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"edict_promote_weak_signal","arguments":{"token":"${normal.token}","clusterId":"test","inspectionDigest":"unused","fixes":[],"validation":"none"}}}
        {"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"edict_check_weak_signal_promotion","arguments":{"token":"${weakChecker.token}"}}}
        {"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"edict_check_weak_signal_promotion","arguments":{"token":"${normalChecker.token}"}}}
        {"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"edict-check-promotion","arguments":{"token":"${weakChecker.token}"}}}
        {"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"edict_check_weak_signal_promotion","arguments":{"token":"${weak.token}"}}}
        {"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"edict-check-promotion","arguments":{"token":"${normalChecker.token}"}}}
        {"jsonrpc":"2.0","id":10,"method":"ping"}
        {"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"edict_record_weak_signal_promotion_signals","arguments":{"token":"${normalChecker.token}","clusterId":"test","inspectionDigest":"unused","pullRequest":{},"signals":[]}}}
        {"jsonrpc":"2.0","id":12,"method":"tools/call","params":{"name":"edict_record_weak_signal_promotion_signals","arguments":{"token":"${weak.token}","clusterId":"test","inspectionDigest":"unused","pullRequest":{},"signals":[]}}}
      """.trimIndent() + "\n"
      val transport = StdioServerTransport(
        input = DelayedEofInputStream(ByteArrayInputStream(input.encodeToByteArray())).asSource().buffered(),
        output = output.asSink().buffered(),
      )
      val closed = CompletableDeferred<Unit>()
      transport.onClose { closed.complete(Unit) }
      edictNextToolset(layout, EdictManagementService(store, layout)).createServer().createSession(transport)
      closed.await()
      val responses = output.toString(Charsets.UTF_8).lineSequence().filter(String::isNotBlank)
        .map { wireJson.parseToJsonElement(it).jsonObject }.toList()
      val selection = responses.single { it["id"].toString() == "2" }.obj("result")
      assertFalse(selection.flag("isError") == true)
      assertTrue(selection.obj("structuredContent").array("candidates").isEmpty())
      val check = responses.single { it["id"].toString() == "5" }.obj("result")
      assertFalse(check.flag("isError") == true)
      assertTrue(check.obj("structuredContent").keys.all { it in setOf("reviews", "failures") })
      assertTrue(check.obj("structuredContent").array("reviews").isEmpty())
      val normalCheck = responses.single { it["id"].toString() == "9" }.obj("result")
      assertFalse(normalCheck.flag("isError") == true)
      assertTrue(normalCheck.obj("structuredContent").keys.all { it in setOf("accepted", "undecided", "failures") })
      for (id in listOf("3", "4", "6", "7", "8", "11", "12")) {
        assertTrue(responses.single { it["id"].toString() == id }.obj("result").flag("isError") == true)
      }
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
          {"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"edict_next_calculate_neighbours_by_embeddings","arguments":{}}}
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
        assertContains(result.obj("structuredContent").text("summary"), "neighbours of 0 inbox Signal(s)")
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

  private class DelayedEofInputStream(
    input: InputStream,
    private val beforeEof: () -> Unit = { Thread.sleep(500) },
  ) : FilterInputStream(input) {
    private var delayed = false

    override fun read(): Int = delayAtEof(super.read())

    override fun read(bytes: ByteArray, offset: Int, length: Int): Int =
      delayAtEof(super.read(bytes, offset, length))

    private fun delayAtEof(result: Int): Int {
      if (result < 0 && !delayed) {
        delayed = true
        beforeEof()
      }
      return result
    }
  }
}

private object FixedEndpointLifecycle : IntellijMcpServerLifecycle {
  override suspend fun start(): URI = URI("http://127.0.0.1:1/mcp")
  override suspend fun stop() = Unit
}
