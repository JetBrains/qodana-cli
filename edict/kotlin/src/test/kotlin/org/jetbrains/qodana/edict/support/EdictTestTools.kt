package org.jetbrains.qodana.edict.support

import org.jetbrains.qodana.edict.core.TokenRedactor
import kotlinx.serialization.json.JsonObject
import org.jetbrains.qodana.edict.common.EdictConfiguration
import org.jetbrains.qodana.edict.common.EdictLayout
import org.jetbrains.qodana.edict.core.EdictManagementService
import org.jetbrains.qodana.edict.core.EdictDistributionService
import org.jetbrains.qodana.edict.core.EdictGenerationService
import org.jetbrains.qodana.edict.core.EdictMcpToolset
import org.jetbrains.qodana.edict.core.EdictRepositoryState
import org.jetbrains.qodana.edict.core.EdictRepository
import org.jetbrains.qodana.edict.core.EdictRepositoryDirectory
import org.jetbrains.qodana.edict.core.EdictSourceFileService
import org.jetbrains.qodana.edict.core.IntellijMcpServerService
import org.jetbrains.qodana.edict.history.enrichment.EdictEnrichmentMcpToolset
import org.jetbrains.qodana.edict.history.enrichment.EdictEnrichmentService
import org.jetbrains.qodana.edict.promotion.PromotionService
import java.io.PrintWriter
import kotlin.io.path.createDirectories

internal class EdictTestTools(
  store: EdictRepositoryState,
  // Logs land next to the state, never inside it.
  layout: EdictLayout = EdictLayout(store.root.parent, store.root),
  taskOutput: PrintWriter = PrintWriter(System.err, true),
) {
  private val management = EdictManagementService(store, layout, taskOutput).withTools(layout)

  fun call(name: String, arguments: JsonObject): JsonObject = management.call(name, arguments)
}

/**
 * This service with its tool table filled, as the MCP server over [layout] serves it: registering the toolset fills the
 * table that `call` uses, and the IDE never starts.
 */
internal fun EdictManagementService.withTools(layout: EdictLayout): EdictManagementService =
  also { edictToolset(layout, it).createServer() }

/** The toolset `EdictServer` wires for a run over [layout], creating the state directory its repository requires. */
internal fun edictToolset(
  layout: EdictLayout,
  management: EdictManagementService,
  inspectionServer: IntellijMcpServerService = IntellijMcpServerService(layout.root),
  configuration: EdictConfiguration = EdictConfiguration(),
  promotion: PromotionService? = null,
): EdictMcpToolset {
  val repository = EdictRepository(EdictRepositoryDirectory(layout.stateDirectory.createDirectories()))
  return EdictMcpToolset(
    layout, inspectionServer, management,
    EdictDistributionService(repository, layout.neighboursResponsePath, configuration.distribution.defaultSignalCount),
    EdictGenerationService(
      repository,
      inspectionServer,
      layout.root,
      configuration.generation.maxProjectAnalyses,
      configuration.generation.defaultGenerationCount,
    ),
    EdictEnrichmentMcpToolset(management, EdictEnrichmentService(repository, layout.root)),
    EdictSourceFileService(layout.root, null),
    configuration,
    promotion ?: PromotionService(repository, configuration, null, NoRedaction),
    feedback = null,
  )
}

/** Redacts nothing, for services whose test text holds no issued tokens. */
internal val NoRedaction = TokenRedactor { it }
