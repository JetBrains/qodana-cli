package org.jetbrains.qodana.edict.edictnext

import com.intellij.ml.llm.qodana.agents.edictnext.EdictNextJson
import com.intellij.ml.llm.qodana.agents.edictnext.EdictNextMarkGeneratedResponse
import com.intellij.ml.llm.qodana.agents.edictnext.EdictNextSignalValidationResponse
import io.modelcontextprotocol.kotlin.sdk.server.Server
import io.modelcontextprotocol.kotlin.sdk.server.ServerOptions
import io.modelcontextprotocol.kotlin.sdk.types.CallToolRequest
import io.modelcontextprotocol.kotlin.sdk.types.CallToolResult
import io.modelcontextprotocol.kotlin.sdk.types.Implementation
import io.modelcontextprotocol.kotlin.sdk.types.ServerCapabilities
import io.modelcontextprotocol.kotlin.sdk.types.TextContent
import io.modelcontextprotocol.kotlin.sdk.types.ToolSchema
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.encodeToJsonElement
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonObject
import kotlin.coroutines.cancellation.CancellationException

/** Registers the Edict Next tools on the standalone MCP Kotlin SDK server. */
internal class EdictNextMcpToolset {
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
  ).also(::registerTools)

  private fun registerTools(server: Server) {
    server.addTool(
      name = "edict_next_prepare_pipeline",
      description = "Snapshot the complete Edict repository and prepare neighbours for up to 100 alphabetical inbox Signals. Call once.",
      inputSchema = stringArguments(
        "worktreePath" to "Absolute path to the agent-created Edict repository worktree",
      ),
    ) { request ->
      EdictNextDistributionService.getInstance(sessionId)
        .preparePipeline(request.requireString("worktreePath"))
        .toToolResult()
    }

    server.addTool(
      name = "edict_next_next_signal",
      description = "Return the next alphabetical inbox Signal and its nearest cluster or inbox candidates. Call until it returns STOP_DISTRIBUTION.",
    ) {
      EdictNextDistributionService.getInstance(sessionId).nextSignal().toToolResult()
    }

    server.addTool(
      name = "edict_next_get_distribution_context",
      description = "Return distribution evidence for one Signal or every member of a cluster. Cluster retrieval records evidence access for the current Signal.",
      inputSchema = stringArguments(
        "kind" to "Context kind: 'signal' or 'cluster'",
        "id" to "Signal or cluster id",
      ),
    ) { request ->
      EdictNextDistributionService.getInstance(sessionId)
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
        EdictNextDistributionService.getInstance(sessionId)
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
      EdictNextDistributionService.getInstance(sessionId).validateDistribution().toToolResult()
    }

    server.addTool(
      name = "edict_next_get_generation_clusters",
      description = "Freeze and return the Pending clusters that generation must process",
    ) {
      EdictNextGenerationService.getInstance(sessionId).getGenerationClusters().toToolResult()
    }

    server.addTool(
      name = "edict_next_validate_code_example",
      description = "Validate one stored code example structurally: metadata, parsing, and target ranges. Whether the example is semantically correct stays the agent's decision.",
      inputSchema = stringArguments(
        "clusterId" to "Cluster id",
        "exampleId" to "Code example id inside the cluster",
      ),
    ) { request ->
      EdictNextGenerationService.getInstance(sessionId)
        .validateCodeExample(request.requireString("clusterId"), request.requireString("exampleId"))
        .toToolResult()
    }

    server.addTool(
      name = "edict_next_validate_cluster_examples",
      description = "Validate that every cluster Signal has one structurally valid focused code example with a matching label.",
      inputSchema = stringArguments("clusterId" to "Cluster id"),
    ) { request ->
      EdictNextGenerationService.getInstance(sessionId)
        .validateClusterExamples(request.requireString("clusterId"))
        .toToolResult()
    }

    server.addTool(
      name = "edict_next_get_inspection_action",
      description = "Detect conflicting Signals, then return SKIP when the predecessor meets the required 85% example accuracy; otherwise return GENERATE. Call before changing the cluster id or inspection candidate.",
      inputSchema = stringArguments("clusterId" to "Cluster id"),
    ) { request ->
      EdictNextGenerationService.getInstance(sessionId)
        .getInspectionAction(request.requireString("clusterId"))
        .toToolResult()
    }

    server.addTool(
      name = "edict_next_validate_inspection",
      description = "Compile an inspection candidate, require its KTS metadata to match the cluster, and measure its label accuracy across the evidence. Returns achieved accuracy, reported negative examples, and uncovered positive examples.",
      inputSchema = stringArguments(
        "clusterId" to "Cluster id; the candidate is inspections/<clusterId>.candidate.kts",
      ),
    ) { request ->
      EdictNextGenerationService.getInstance(sessionId)
        .validateInspection(request.requireString("clusterId"))
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
        EdictNextGenerationService.getInstance(sessionId).markGenerated(clusterId).toToolResult()
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
      EdictNextGenerationService.getInstance(sessionId).validateGeneration().toToolResult()
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
