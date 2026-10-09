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
import org.jetbrains.qodana.edict.common.EdictConfiguration
import org.jetbrains.qodana.edict.common.EdictLayout
import org.jetbrains.qodana.edict.promotion.PromotionReviewDecision
import org.jetbrains.qodana.edict.promotion.PromotionService
import kotlin.coroutines.cancellation.CancellationException
import kotlin.io.path.createDirectories

/** Registers the Edict Next tools on the standalone MCP Kotlin SDK server. */
internal class EdictNextMcpToolset(
  private val layout: EdictLayout,
  private val inspectionServer: IntellijMcpServerService,
  private val management: EdictManagementService,
  private val distribution: EdictNextDistributionService,
  private val generation: EdictNextGenerationService,
  private val sourceFiles: EdictSourceFileService,
  private val configuration: EdictConfiguration = EdictConfiguration(),
  private val promotion: PromotionService,
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
    management.registerTools(server, configuration.calculatePrice)
    registerPromotionTools(server)

    server.addTool(
      name = "edict_context",
      description = "Return this run's configured paths and CI review repository. Read-only.",
    ) {
      EdictRunContext(
        projectDirectory = layout.root.toString(),
        stateDirectory = layout.stateDirectory.toString(),
        scratchDirectory = layout.scratchDirectory.createDirectories().toString(),
        reviewRepository = configuration.ci?.reviewRepository,
      ).toToolResult()
    }

    server.addTool(
      name = "edict_file_at_ref",
      description = "Read a repository file at an exact commit with 1-based line numbers, from local Git or, when the " +
        "commit is not local, from the CI review repository. With anchor lines, return only them and radius lines around " +
        "them. Read-only.",
      inputSchema = toolSchema(
        required = listOf("path", "ref"),
        "path" to property("string", "File path relative to the repository root"),
        "ref" to property("string", "Full commit SHA"),
        "anchorStartLine" to property("integer", "Optional 1-based first line to focus on"),
        "anchorEndLine" to property("integer", "Optional 1-based last line to focus on; defaults to anchorStartLine"),
        "radius" to property("integer", "Context lines around the anchor (default ${EdictSourceFileService.DEFAULT_RADIUS})"),
      ),
    ) { request ->
      val anchor = request.optionalInt("anchorStartLine")?.let { EdictNextLineRange(it, request.optionalInt("anchorEndLine") ?: it) }
      val content = sourceFiles.fileAtRef(
        request.requireString("path"),
        request.requireString("ref"),
        anchor,
        request.optionalInt("radius") ?: EdictSourceFileService.DEFAULT_RADIUS,
      )
      CallToolResult(content = listOf(TextContent(content)))
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
      description = "Freeze and return the requested number of Pending clusters that generation must process, ranked " +
        "by descending positive-Signal count. Omit generationCount to use the configured default. Only eligible selected " +
        "clusters are returned.",
      inputSchema = toolSchema(
        required = emptyList(),
        "generationCount" to buildJsonObject {
          put("type", "integer")
          put("minimum", 1)
          put("description", "Optional number of clusters to generate; omitted uses edict.generation.defaultGenerationCount")
        },
      ),
    ) { request ->
      generation.getGenerationClusters(request.optionalInt("generationCount")).toToolResult()
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
      description = "Persist one complete synthetic example in the read-only managed state. Code-example, overseer, and weak-review workers must use this instead of filesystem writes.",
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
        "edict-next-weak-signal-review",
        "edict-next-inspection-code-review",
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
      description = "Delete one synthetic example from a frozen cluster and clear syntheticExampleId on every Signal that references it; those Signals then need a new example.",
      inputSchema = stringArguments(
        "token" to "Your delegated managed-task token",
        "clusterId" to "Frozen generation cluster id",
        "exampleId" to "Example id to delete",
      ),
    ) { request ->
      management.requireSkill(
        request.requireString("token"),
        "edict-next-code-example-overseer",
        "edict-next-weak-signal-review",
        "edict-next-inspection-code-review",
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
      description = "Detect conflicting Signals, then return SKIP when the predecessor passes every strong example; otherwise return GENERATE. Call before changing the cluster id or inspection candidate.",
      inputSchema = stringArguments("clusterId" to "Cluster id"),
    ) { request ->
      generation
        .getInspectionAction(request.requireString("clusterId"))
        .toToolResult()
    }

    server.addTool(
      name = "edict_next_save_candidate_inspection",
      description = """Store the complete candidate Inspection KTS of a frozen Pending cluster, then compile it, check its
        metadata against the cluster, and run every example. The candidate is stored even when it fails. Every strong
        example must pass before project analysis; weak-example results are reported for recall.""",
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
      name = "edict_next_rename_cluster",
      description = """Rename a frozen Pending cluster: its directory, cluster.json id, and candidate, including the
        candidate's `id = "<clusterId>"`. The predecessor keeps its id. Later calls use the new id.""",
      inputSchema = stringArguments(
        "token" to "Your delegated edict-next-cluster-generation task token",
        "clusterId" to "Frozen generation cluster id",
        "newClusterId" to "New lowercase kebab-case cluster id, unused by any cluster or inspection",
      ),
    ) { request ->
      management.requireSkill(request.requireString("token"), "edict-next-cluster-generation")
      generation.renameCluster(
        request.requireString("clusterId"),
        request.requireString("newClusterId"),
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
      name = "edict_next_get_new_inspection_results",
      description = """Run the current candidate, which must have passed validation when saved, over the analyzed project
        and create its weak-signal review manifest.
        Each cluster may run this a limited number of times per run; the response returns how many remain, and a call
        beyond the limit fails.""",
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
      name = "edict_next_record_evaluation",
      description = "Last step before finalising Generated: score the inspection that would be published (the analyzed candidate, or the predecessor on SKIP) on every strong and weak example and write clusters/<id>/evaluation.json (tp, fp, fn, precision, recall, inspection hash, per-example results). Writes nothing unless every strong example passes.",
      inputSchema = stringArguments(
        "token" to "Your delegated edict-next-cluster-generation task token",
        "clusterId" to "Pending frozen generation target cluster id",
      ),
    ) { request ->
      management.requireSkill(request.requireString("token"), "edict-next-cluster-generation")
      generation.recordEvaluation(request.requireString("clusterId")).toToolResult()
    }

    server.addTool(
      name = "edict_next_finalise_cluster",
      description = """End the processing of a frozen Pending cluster: record the reason in its history and move it to the
        requested status with that status's contract satisfied, or reject and leave the cluster unchanged.
        Generated: publishes the predecessor on SKIP, or the candidate that completed project analysis on GENERATE, when it
        passes every strong example and edict_next_record_evaluation scored exactly it on the current examples. Discontinued: removes the candidate and predecessor.
        Invalid: keeps the candidate and predecessor. Pending: only records the reason.""",
      inputSchema = toolSchema(
        listOf("token", "clusterId", "status", "reason"),
        "token" to property("string", "Your delegated edict-next-cluster-generation task token"),
        "clusterId" to property("string", "Pending frozen generation target cluster id"),
        "status" to buildJsonObject {
          put("type", "string")
          put("description", "Status to set")
          putJsonArray("enum") { EdictNextClusterStatus.entries.forEach { add(it.name) } }
        },
        "reason" to property("string", "Token-free decision recorded in the cluster history"),
      ),
    ) { request ->
      val clusterId = request.requireString("clusterId")
      try {
        management.requireSkill(request.requireString("token"), "edict-next-cluster-generation")
        val reason = request.requireString("reason")
        management.requireTokenFree(reason)
        val status = EdictNextClusterStatus.valueOf(request.requireString("status"))
        generation.finaliseCluster(clusterId, status, reason).toToolResult()
      }
      catch (e: CancellationException) {
        throw e
      }
      catch (e: Exception) {
        EdictNextFinaliseClusterResponse(
          success = false,
          summary = e.message ?: "Failed to finalise cluster '$clusterId'",
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

  private fun registerPromotionTools(server: Server) {
    server.addTool(
      name = "edict_promote_clusters",
      description = "Promote eligible Generated inspections with more than five strong Signals. Omit clusterIds to process every eligible cluster.",
      inputSchema = toolSchema(
        required = listOf("token"),
        "token" to property("string", "Your delegated edict-promote task token"),
        "clusterIds" to buildJsonObject {
          put("type", "array")
          put("description", "Optional cluster IDs to consider; omitted means every cluster")
          put("minItems", 1)
          put("uniqueItems", true)
          putJsonObject("items") { put("type", "string") }
        },
      ),
    ) { request ->
      try {
        management.requireSkill(request.requireString("token"), "edict-promote")
        val clusterIds = request.arguments?.get("clusterIds")?.let {
          EdictNextJson.decodeFromJsonElement<List<String>>(it)
        }
        promotion.promote(clusterIds).toToolResult()
      }
      catch (e: Exception) {
        EdictNextMutationResponse(false, e.message ?: "Inspection promotion failed").toToolResult(isError = true)
      }
    }

    server.addTool(
      name = "ecict-check-promotion",
      description = "Refresh ON_REVIEW promotion PRs, returning accepted promotions and closed-unmerged reviews awaiting a cluster decision.",
      inputSchema = toolSchema(
        required = listOf("token"),
        "token" to property("string", "Your delegated ecict-check-promotion task token"),
      ),
    ) { request ->
      try {
        management.requireSkill(request.requireString("token"), "ecict-check-promotion")
        val response = promotion.checkReviews()
        response.copy(
          undecided = response.undecided.map { it.copy(evidence = management.redact(it.evidence)) },
          failures = response.failures.map { it.copy(message = management.redact(it.message)) },
        ).toToolResult()
      }
      catch (e: Exception) {
        EdictNextMutationResponse(false, e.message ?: "Promotion review check failed").toToolResult(isError = true)
      }
    }

    val decisionObject = buildJsonObject {
      put("type", "object")
      putJsonObject("properties") {
        put("clusterId", property("string", "Cluster id"))
        put("promotionId", property("string", "Persisted promotion id"))
        put("decision", property("string", "MOVE_CLUSTER_TO_PENDING or DISCONTINUE_CLUSTER"))
        put("rationale", property("string", "Bounded decision rationale based on review evidence"))
      }
      putJsonArray("required") { listOf("clusterId", "promotionId", "decision", "rationale").forEach(::add) }
      put("additionalProperties", false)
    }
    server.addTool(
      name = "edict_promotion_decide_reviews",
      description = "Verify and resolve 1 to 10 closed reviews, atomically closing each promotion while updating cluster state and history.",
      inputSchema = toolSchema(
        required = listOf("token", "decisions"),
        "token" to property("string", "Your delegated edict-promotion-decision task token"),
        "decisions" to buildJsonObject {
          put("type", "array")
          put("minItems", 1)
          put("maxItems", 10)
          put("items", decisionObject)
        },
      ),
    ) { request ->
      try {
        management.requireSkill(request.requireString("token"), "edict-promotion-decision")
        val decisions = EdictNextJson.decodeFromJsonElement<List<PromotionReviewDecision>>(
          request.arguments?.get("decisions") ?: error("decisions is required"),
        )
        decisions.forEach { management.requireTokenFree(it.rationale) }
        val response = promotion.decideReviews(decisions)
        response.copy(
          failures = response.failures.map { it.copy(message = management.redact(it.message)) },
        ).toToolResult()
      }
      catch (e: Exception) {
        EdictNextMutationResponse(false, e.message ?: "Promotion review decision failed").toToolResult(isError = true)
      }
    }
  }

  /**
   * Forwards the IntelliJ Inspection KTS tools for authoring and quick single-file checks, so agents only need this
   * server. Running examples and the whole project stays with the managed tools, which validate and bound them.
   */
  private fun registerInspectionKtsTools(server: Server) {
    fun proxy(name: String, description: String, inputSchema: ToolSchema) {
      server.addTool(name = name, description = description, inputSchema = inputSchema) { request ->
        val arguments = request.arguments ?: JsonObject(emptyMap())
        inspectionServer.withClient { it.callTool(name, arguments) }.toProxiedToolResult()
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

private fun CallToolRequest.optionalInt(name: String): Int? =
  arguments?.get(name)?.jsonPrimitive?.let { it.intOrNull ?: error("'$name' must be an integer") }

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
