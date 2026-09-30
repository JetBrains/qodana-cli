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
import kotlin.coroutines.cancellation.CancellationException

/** Registers the Edict Next tools on the standalone MCP Kotlin SDK server. */
internal class EdictNextMcpToolset(
  private val runId: String,
  private val management: EdictManagementService,
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
      name = "edict_prepare_pipeline",
      description = "Snapshot the server-configured managed Edict repository and prepare neighbours for up to 100 alphabetical inbox Signals. Call once with no arguments.",
    ) {
      EdictNextDistributionService.getInstance(runId)
        .preparePipeline()
        .toToolResult()
    }

    server.addTool(
      name = "edict_next_signal",
      description = "Return the next alphabetical inbox Signal and its nearest cluster or inbox candidates. Call until it returns STOP_DISTRIBUTION.",
    ) {
      EdictNextDistributionService.getInstance(runId).nextSignal().toToolResult()
    }

    server.addTool(
      name = "edict_next_get_distribution_context",
      description = "Return distribution evidence for one Signal or every member of a cluster. Cluster retrieval records evidence access for the current Signal.",
      inputSchema = stringArguments(
        "kind" to "Context kind: 'signal' or 'cluster'",
        "id" to "Signal or cluster id",
      ),
    ) { request ->
      EdictNextDistributionService.getInstance(runId)
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
        EdictNextDistributionService.getInstance(runId)
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
      EdictNextDistributionService.getInstance(runId).validateDistribution().toToolResult()
    }

    server.addTool(
      name = "edict_next_get_generation_clusters",
      description = "Freeze and return the Pending clusters that generation must process",
    ) {
      EdictNextGenerationService.getInstance(runId).getGenerationClusters().toToolResult()
    }

    server.addTool(
      name = "edict_next_validate_code_example",
      description = "Validate one stored code example structurally: metadata, parsing, and target ranges. Whether the example is semantically correct stays the agent's decision.",
      inputSchema = stringArguments(
        "clusterId" to "Cluster id",
        "exampleId" to "Code example id inside the cluster",
      ),
    ) { request ->
      EdictNextGenerationService.getInstance(runId)
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
      EdictNextGenerationService.getInstance(runId).saveCodeExample(
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
      EdictNextGenerationService.getInstance(runId).assignCodeExample(
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
      EdictNextGenerationService.getInstance(runId).deleteCodeExample(
        request.requireString("clusterId"),
        request.requireString("exampleId"),
      ).toToolResult()
    }

    server.addTool(
      name = "edict_next_validate_cluster_examples",
      description = "Validate that every cluster Signal has one structurally valid focused code example with a matching label.",
      inputSchema = stringArguments("clusterId" to "Cluster id"),
    ) { request ->
      EdictNextGenerationService.getInstance(runId)
        .validateClusterExamples(request.requireString("clusterId"))
        .toToolResult()
    }

    server.addTool(
      name = "edict_next_get_inspection_action",
      description = "Detect conflicting Signals, then return SKIP when the predecessor meets the required 85% example accuracy; otherwise return GENERATE. Call before changing the cluster id or inspection candidate.",
      inputSchema = stringArguments("clusterId" to "Cluster id"),
    ) { request ->
      EdictNextGenerationService.getInstance(runId)
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
      EdictNextGenerationService.getInstance(runId).saveCandidateInspection(
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
      EdictNextGenerationService.getInstance(runId).appendHistory(
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
      EdictNextGenerationService.getInstance(runId)
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
      EdictNextGenerationService.getInstance(runId)
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
        EdictNextGenerationService.getInstance(runId).markGenerated(clusterId).toToolResult()
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
      EdictNextGenerationService.getInstance(runId).validateGeneration().toToolResult()
    }
  }

  private companion object {
    const val SERVER_NAME = "edict-mcp-next"
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
