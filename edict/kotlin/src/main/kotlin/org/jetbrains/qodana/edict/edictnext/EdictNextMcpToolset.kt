package org.jetbrains.qodana.edict.edictnext

import io.modelcontextprotocol.kotlin.sdk.server.Server
import io.modelcontextprotocol.kotlin.sdk.server.ServerOptions
import io.modelcontextprotocol.kotlin.sdk.types.CallToolRequest
import io.modelcontextprotocol.kotlin.sdk.types.CallToolResult
import io.modelcontextprotocol.kotlin.sdk.types.Implementation
import io.modelcontextprotocol.kotlin.sdk.types.ServerCapabilities
import io.modelcontextprotocol.kotlin.sdk.types.TextContent
import io.modelcontextprotocol.kotlin.sdk.types.ToolSchema
import kotlinx.serialization.json.*
import org.jetbrains.qodana.edict.common.EdictLayout
import kotlin.coroutines.cancellation.CancellationException
import kotlin.io.path.createDirectories

/** Registers the Edict Next tools on the standalone MCP Kotlin SDK server. */
internal class EdictNextMcpToolset(
  private val layout: EdictLayout,
  private val inspectionServer: IntellijMcpServerService,
  private val management: EdictManagementService,
  private val distribution: EdictNextDistributionService,
  private val generation: EdictNextGenerationService,
) {
  fun createServer(): Server = Server(
    serverInfo = Implementation(
      name = SERVER_NAME,
      version = "1.0.0",
    ),
    options = ServerOptions(
      capabilities = ServerCapabilities(
        tools = ServerCapabilities.Tools(listChanged = false),
      ),
    ),
    instructions = EdictManagementService.INSTRUCTIONS,
  ).also(::registerTools)

  private fun registerTools(server: Server) {
    management.registerTools(server)

    server.addTool(
      name = "edict_context",
      description = "Return the absolute paths of this run: inspected project, read-only state root, and private scratch root. Read-only.",
    ) {
      EdictRunContext(
        projectDirectory = layout.root.toString(),
        stateDirectory = layout.stateDirectory.toString(),
        scratchDirectory = layout.scratchDirectory.createDirectories().toString(),
      ).toToolResult()
    }

    server.addTool(
      name = "edict_next_prepare_pipeline",
      description = "Snapshot the state repository and prepare neighbours for up to 100 alphabetical inbox Signals. Call once.",
    ) {
      distribution.preparePipeline().toToolResult()
    }

    server.addTool(
      name = "edict_next_next_signal",
      description = "Return the next alphabetical inbox Signal and its nearest cluster or inbox candidates. Call until it returns STOP_DISTRIBUTION.",
    ) {
      distribution.nextSignal().toToolResult()
    }

    server.addTool(
      name = "edict_next_get_distribution_context",
      description = "Return distribution evidence for one Signal or every member of a cluster. Cluster retrieval records evidence access for the current Signal.",
      inputSchema = stringArguments(
        "kind" to "Context kind: 'signal' or 'cluster'",
        "id" to "Signal or cluster id",
      ),
    ) { request ->
      distribution
        .getContext(request.requireString("kind"), request.requireString("id"))
        .toToolResult()
    }

    server.addTool(
      name = "edict_next_add_signal_to_cluster",
      description = "Add a prepared Signal to an existing cluster or create a new Pending cluster with the supplied id. Existing-cluster assignment requires its context to have been loaded for the current Signal. This is the only distribution mutation.",
      inputSchema = stringArguments(
        "signalId" to "ID returned by edict_next_next_signal",
        "clusterId" to "Existing or new kebab-case cluster id",
      ),
    ) { request ->
      val signalId = request.requireString("signalId")
      try {
        distribution
          .addSignalToCluster(signalId, request.requireString("clusterId"))
          .toToolResult()
      }
      catch (e: Exception) {
        EdictNextSignalValidationResponse(
          signalId = signalId,
          summary = e.message ?: "The ${e.javaClass} error occurred",
          added = false,
        ).toToolResult(isError = true)
      }
    }

    server.addTool(
      name = "edict_next_validate_distribution",
      description = "Sanity-check distribution file changes and require every returned Signal to be assigned",
    ) {
      distribution.validateDistribution().toToolResult()
    }

    server.addTool(
      name = "edict_next_get_generation_clusters",
      description = "Freeze and return the Pending clusters that generation must process",
    ) {
      generation.getGenerationClusters().toToolResult()
    }

    server.addTool(
      name = "edict_next_validate_code_example",
      description = "Validate one stored code example structurally: metadata, parsing, and target ranges. Whether the example is semantically correct stays the agent's decision.",
      inputSchema = stringArguments(
        "clusterId" to "Cluster id",
        "exampleId" to "Code example id inside the cluster",
      ),
    ) { request ->
      generation
        .validateCodeExample(request.requireString("clusterId"), request.requireString("exampleId"))
        .toToolResult()
    }

    server.addTool(
      name = "edict_next_save_code_example",
      description = "Persist one complete synthetic example in the read-only managed state. Code-example, overseer, and review workers must use this instead of filesystem writes.",
      inputSchema = stringArguments(
        "token" to "Your delegated managed-task token",
        "clusterId" to "Frozen generation cluster id",
        "exampleId" to "New or existing lowercase kebab-case example id",
        "metadataJson" to "Complete metadata JSON with id, fileName, label, and expectedRanges",
        "sourceCode" to "Complete self-contained Java or Kotlin source",
      ),
    ) { request ->
      val token = request.requireString("token")
      management.requireSkill(
        token,
        "edict-next-code-example",
        "edict-next-code-example-overseer",
        "edict-next-inspection-code-review",
        "edict-next-weak-signal-review",
      )
      management.requireTokenFree(request.requireString("metadataJson"))
      management.requireTokenFree(request.requireString("sourceCode"))
      generation.saveCodeExample(
        request.requireString("clusterId"),
        request.requireString("exampleId"),
        request.requireString("metadataJson"),
        request.requireString("sourceCode"),
      ).toToolResult()
    }

    server.addTool(
      name = "edict_next_assign_code_example",
      description = "Assign a structurally valid stored example to one persisted cluster Signal. Only syntheticExampleId is changed.",
      inputSchema = stringArguments(
        "token" to "Your delegated managed-task token",
        "clusterId" to "Frozen generation cluster id",
        "signalId" to "Persisted Signal id in the cluster",
        "exampleId" to "Validated example id in the cluster",
      ),
    ) { request ->
      management.requireSkill(
        request.requireString("token"),
        "edict-next-code-example",
        "edict-next-code-example-overseer",
      )
      generation.assignCodeExample(
        request.requireString("clusterId"),
        request.requireString("signalId"),
        request.requireString("exampleId"),
      ).toToolResult()
    }

    server.addTool(
      name = "edict_next_delete_code_example",
      description = "Delete one unassigned synthetic example from a frozen cluster.",
      inputSchema = stringArguments(
        "token" to "Your delegated managed-task token",
        "clusterId" to "Frozen generation cluster id",
        "exampleId" to "Unassigned example id to delete",
      ),
    ) { request ->
      management.requireSkill(
        request.requireString("token"),
        "edict-next-code-example-overseer",
        "edict-next-weak-signal-review",
      )
      generation.deleteCodeExample(
        request.requireString("clusterId"),
        request.requireString("exampleId"),
      ).toToolResult()
    }

    server.addTool(
      name = "edict_next_validate_cluster_examples",
      description = "Validate that every cluster Signal has one structurally valid focused code example with a matching label.",
      inputSchema = stringArguments("clusterId" to "Cluster id"),
    ) { request ->
      generation
        .validateClusterExamples(request.requireString("clusterId"))
        .toToolResult()
    }

    server.addTool(
      name = "edict_next_get_inspection_action",
      description = "Detect conflicting Signals, then return SKIP when the predecessor meets the required 85% example accuracy; otherwise return GENERATE. Call before changing the cluster id or inspection candidate.",
      inputSchema = stringArguments("clusterId" to "Cluster id"),
    ) { request ->
      generation
        .getInspectionAction(request.requireString("clusterId"))
        .toToolResult()
    }

    server.addTool(
      name = "edict_next_save_candidate_inspection",
      description = "Persist the complete candidate Inspection KTS for a frozen Pending cluster in read-only managed state.",
      inputSchema = stringArguments(
        "token" to "Your delegated edict-next-cluster-generation task token",
        "clusterId" to "Frozen generation cluster id",
        "inspectionKtsCode" to "Complete candidate Inspection KTS source",
      ),
    ) { request ->
      management.requireSkill(request.requireString("token"), "edict-next-cluster-generation")
      management.requireTokenFree(request.requireString("inspectionKtsCode"))
      generation.saveCandidateInspection(
        request.requireString("clusterId"),
        request.requireString("inspectionKtsCode"),
      ).toToolResult()
    }

    server.addTool(
      name = "edict_next_append_cluster_history",
      description = "Append one token-free operational decision to a frozen Pending cluster history in managed state.",
      inputSchema = stringArguments(
        "token" to "Your delegated edict-next-cluster-generation task token",
        "clusterId" to "Frozen generation cluster id",
        "entry" to "Concise operational decision or known problem",
      ),
    ) { request ->
      management.requireSkill(request.requireString("token"), "edict-next-cluster-generation")
      management.requireTokenFree(request.requireString("entry"))
      generation.appendHistory(
        request.requireString("clusterId"),
        request.requireString("entry"),
      ).toToolResult()
    }

    server.addTool(
      name = "edict_next_validate_inspection",
      description = """Compile an inspection candidate, require its KTS metadata to match the cluster, and measure its label accuracy across the evidence. 
        Returns achieved accuracy, reported negative examples, and uncovered positive examples.""",
      inputSchema = stringArguments(
        "clusterId" to "Cluster id; the candidate is inspections/<clusterId>.candidate.kts",
      ),
    ) { request ->
      generation
        .validateInspection(request.requireString("clusterId"))
        .toToolResult()
    }

    server.addTool(
      name = "edict_next_get_new_inspection_results",
      description = "Run the validated candidate over the analyzed project and create its weak-signal review manifest.",
      inputSchema = stringArguments(
        "clusterId" to "Cluster id; the candidate is inspections/<clusterId>.candidate.kts",
        "privateScratchDirectory" to "Absolute private directory for review manifests and outputs",
      ),
    ) { request ->
      generation
        .getNewInspectionResults(
          request.requireString("clusterId"),
          request.requireString("privateScratchDirectory"),
        )
        .toToolResult()
    }

    server.addTool(
      name = "edict_next_mark_generated",
      description = "Validate the current selected inspection and apply the cluster's Generated transition in one guarded operation.",
      inputSchema = stringArguments(
        "clusterId" to "Pending frozen generation target cluster id",
      ),
    ) { request ->
      val clusterId = request.requireString("clusterId")
      try {
        generation.markGenerated(clusterId).toToolResult()
      }
      catch (e: CancellationException) {
        throw e
      }
      catch (e: Exception) {
        EdictNextMarkGeneratedResponse(
          success = false,
          summary = e.message ?: "Failed to mark cluster '$clusterId' Generated",
        ).toToolResult(isError = true)
      }
    }

    server.addTool(
      name = "edict_next_validate_generation",
      description = "Read and validate generation changes against the frozen targets without modifying the repository.",
    ) {
      generation.validateGeneration().toToolResult()
    }

    registerInspectionKtsTools(server)
  }

  /** Forwards every IntelliJ Inspection KTS tool so agents only need this server, as when the IDE served every tool. */
  private fun registerInspectionKtsTools(server: Server) {
    fun proxy(name: String, description: String, inputSchema: ToolSchema, wholeProject: Boolean = false) {
      server.addTool(name = name, description = description, inputSchema = inputSchema) { request ->
        val arguments = request.arguments ?: JsonObject(emptyMap())
        // Whole-project runs share the IDE's opened project with the pipeline's own analyses, so they queue behind them.
        val result = if (wholeProject) inspectionServer.waitForAnalysis { it.callTool(name, arguments) }
        else inspectionServer.withClient { it.callTool(name, arguments) }
        result.toProxiedToolResult()
      }
    }
    val inspectionKtsCode = "inspectionKtsCode" to property("string", "The complete inspection.kts script")

    proxy(
      name = "generate_psi_tree",
      description = "Parse a Java or Kotlin code snippet with the IntelliJ PSI parser and return its PSI tree.",
      inputSchema = toolSchema(
        required = listOf("code", "language"),
        "code" to property("string", "Source code snippet to parse"),
        "language" to property("string", "Programming language: 'Java' or 'Kotlin'"),
      ),
    )
    proxy(
      name = "generate_inspection_kts_examples",
      description = "Return example inspection.kts scripts to use as templates for writing new inspections.",
      inputSchema = toolSchema(
        required = emptyList(),
        "language" to property("string", "Target language for examples: 'Java', 'Kotlin', or 'Any' (default)"),
        "includeAdditionalExamples" to property("boolean", "If true, includes additional curated examples besides templates"),
      ),
    )
    proxy(
      name = "generate_inspection_kts_api",
      description = "Return the inspection.kts API (PSI classes and helpers) available for the given language.",
      inputSchema = toolSchema(
        required = listOf("language"),
        "language" to property("string", "Target language: 'Java' or 'Kotlin'"),
        "wrapInTags" to property("boolean", "If true, wraps the API content in <API> and <api.kt> tags"),
      ),
    )
    proxy(
      name = "compile_inspection_kts",
      description = "Compile an inspection.kts script and return its inspection metadata without executing it.",
      inputSchema = toolSchema(required = listOf("inspectionKtsCode"), inspectionKtsCode),
    )
    proxy(
      name = "run_inspection_kts",
      description = "Compile an inspection.kts script and run it against one file of the inspected project, " +
        "optionally replacing its content. Returns compilation errors or the found problems.",
      inputSchema = toolSchema(
        required = listOf("inspectionKtsCode", "contextPath"),
        inspectionKtsCode,
        "contextPath" to property("string", "Path of the target file relative to the inspected project, e.g. 'src/my/Example.kt'"),
        "targetFileContent" to property("string", "Content to analyze instead of the file on disk; the file must exist when omitted"),
      ),
    )
    proxy(
      name = "run_inspection_kts_examples",
      description = "Compile an inspection.kts script once and run it on the target source file of each supplied " +
        "example project. Returns per-example problems or execution errors.",
      inputSchema = toolSchema(
        required = listOf("inspectionKtsCode", "examples"),
        inspectionKtsCode,
        "examples" to buildJsonObject {
          put("type", "array")
          put("description", "Example projects and the source file to inspect in each")
          putJsonObject("items") {
            put("type", "object")
            putJsonObject("properties") {
              put("id", property("string", "Caller-chosen example id echoed in the result"))
              put("projectPath", property("string", "Absolute path of the example project directory"))
              put("targetFilePath", property("string", "Source file path relative to projectPath"))
            }
            putJsonArray("required") { add("id"); add("projectPath"); add("targetFilePath") }
          }
        },
      ),
    )
    proxy(
      name = "run_inspection_kts_project",
      description = "Compile an inspection.kts script and run it on every Java and Kotlin file of the inspected " +
        "project. Waits for other whole-project analyses; can take many minutes.",
      inputSchema = toolSchema(required = listOf("inspectionKtsCode"), inspectionKtsCode),
      wholeProject = true,
    )
  }

  companion object {
    /** The server's name in the MCP handshake, also the name agents register it under. */
    const val SERVER_NAME = "edict-mcp"
  }
}

private fun stringArguments(vararg arguments: Pair<String, String>): ToolSchema = ToolSchema(
  properties = buildJsonObject {
    arguments.forEach { (name, description) ->
      putJsonObject(name) {
        put("type", "string")
        put("description", description)
      }
    }
  },
  required = arguments.map(Pair<String, String>::first),
)

private fun property(type: String, description: String): JsonObject = buildJsonObject {
  put("type", type)
  put("description", description)
}

private fun toolSchema(required: List<String>, vararg properties: Pair<String, JsonObject>): ToolSchema = ToolSchema(
  properties = JsonObject(properties.toMap()),
  required = required,
)

private fun CallToolRequest.requireString(name: String): String =
  arguments?.get(name)?.jsonPrimitive?.content
  ?: error("'$name' is required")

private inline fun <reified T> T.toToolResult(isError: Boolean = false): CallToolResult {
  val element = EdictNextJson.encodeToJsonElement(this)
  return CallToolResult(
    content = listOf(TextContent(EdictNextJson.encodeToString(this))),
    isError = isError.takeIf { it },
    structuredContent = element as? JsonObject,
  )
}

private fun JsonObject.toProxiedToolResult(): CallToolResult = CallToolResult(
  content = (this["content"] as? JsonArray).orEmpty().mapNotNull { item ->
    val block = item as? JsonObject ?: return@mapNotNull null
    if (block["type"]?.jsonPrimitive?.content == "text") {
      TextContent(block["text"]?.jsonPrimitive?.content.orEmpty())
    }
    else null
  },
  isError = this["isError"]?.jsonPrimitive?.content?.toBooleanStrictOrNull(),
  structuredContent = this["structuredContent"] as? JsonObject,
)
