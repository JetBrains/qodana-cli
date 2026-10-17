package org.jetbrains.qodana.edict.history.enrichment

import io.modelcontextprotocol.kotlin.sdk.server.Server
import kotlinx.serialization.json.*
import org.jetbrains.qodana.edict.common.wireJson
import org.jetbrains.qodana.edict.edictnext.EdictManagementService
import org.jetbrains.qodana.edict.edictnext.EdictNextSignal
import org.jetbrains.qodana.edict.edictnext.property
import org.jetbrains.qodana.edict.edictnext.requireString
import org.jetbrains.qodana.edict.edictnext.stringArguments
import org.jetbrains.qodana.edict.edictnext.toToolResult
import org.jetbrains.qodana.edict.edictnext.toolSchema

/** Historical enrichment tools, independent of distribution and inspection-generation tool registration. */
internal class EdictNextEnrichmentMcpToolset(
  private val management: EdictManagementService,
  private val enrichment: EdictNextEnrichmentService,
) {
  /** Management tools must be registered first: they provide the shared Signal input schema. */
  fun registerTools(server: Server) {
    server.addTool(
      name = "edict_next_get_history_context",
      description = "Return the bounded Git-history range for one Pending cluster: frozen HEAD, date cutoff and exact " +
        "eligible commit revisions. Uses the smaller window of the latest $HISTORY_ANALYSIS_COMMIT_DEPTH commits and " +
        "the last $HISTORY_ANALYSIS_DAY_DEPTH days. Fetch Signals separately with edict_get_cluster_signals. " +
        "Does not select generation targets or persist analysis coverage.",
      inputSchema = stringArguments(
        "token" to "Your managed history-enrichment task token",
        "clusterId" to "Subject Pending cluster ID",
      ),
    ) { request ->
      management.requireSkill(request.requireString("token"), "edict-git-history-signal-enrichment")
      enrichment.getHistoryContext(request.requireString("clusterId")).toToolResult()
    }

    server.addTool(
      name = "edict_publish_signal_to_cluster",
      description = "Validate and publish one STRONG historical FromCommit Signal directly into its Pending " +
        "cluster. Only the history-enrichment agent may call this. " +
        "Identical retries succeed; conflicting IDs are rejected. Never writes to the inbox.",
      inputSchema = toolSchema(
        required = listOf("token", "clusterId", "signal"),
        "token" to property("string", "Your managed history-enrichment task token"),
        "clusterId" to property("string", "Pending cluster being enriched"),
        "signal" to checkNotNull(server.tools.getValue("edict_publish_signal").tool.inputSchema.properties)
          .getValue("signal").jsonObject,
      ),
    ) { request ->
      management.requireSkill(request.requireString("token"), "edict-git-history-signal-enrichment")
      val document = request.arguments?.get("signal") ?: error("Missing signal")
      management.requireTokenFree(document.toString())
      enrichment.publishClusterSignal(
        request.requireString("clusterId"),
        wireJson.decodeFromJsonElement<EdictNextSignal>(document),
      ).toToolResult()
    }
  }
}
