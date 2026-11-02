package org.jetbrains.qodana.edict.support

import org.jetbrains.qodana.edict.edictnext.TokenRedactor
import kotlinx.serialization.json.JsonObject
import org.jetbrains.qodana.edict.common.EdictConfiguration
import org.jetbrains.qodana.edict.common.EdictLayout
import org.jetbrains.qodana.edict.edictnext.EdictManagementService
import org.jetbrains.qodana.edict.edictnext.EdictNextDistributionService
import org.jetbrains.qodana.edict.edictnext.EdictNextGenerationService
import org.jetbrains.qodana.edict.edictnext.EdictNextMcpToolset
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState
import org.jetbrains.qodana.edict.edictnext.EdictRepository
import org.jetbrains.qodana.edict.edictnext.EdictRepositoryDirectory
import org.jetbrains.qodana.edict.edictnext.EdictSourceFileService
import org.jetbrains.qodana.edict.edictnext.IntellijMcpServerService
import org.jetbrains.qodana.edict.history.enrichment.EdictNextEnrichmentMcpToolset
import org.jetbrains.qodana.edict.history.enrichment.EdictNextEnrichmentService
import org.jetbrains.qodana.edict.promotion.PromotionService
import java.io.PrintWriter
import kotlin.io.path.createDirectories

internal class EdictNextTestTools(
  store: EdictNextRepositoryState,
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
  also { edictNextToolset(layout, it).createServer() }

/** The toolset `EdictServer` wires for a run over [layout], creating the state directory its repository requires. */
internal fun edictNextToolset(
  layout: EdictLayout,
  management: EdictManagementService,
  inspectionServer: IntellijMcpServerService = IntellijMcpServerService(layout.root),
  configuration: EdictConfiguration = EdictConfiguration(),
  promotion: PromotionService? = null,
): EdictNextMcpToolset {
  val repository = EdictRepository(EdictRepositoryDirectory(layout.stateDirectory.createDirectories()))
  return EdictNextMcpToolset(
    layout, inspectionServer, management,
    EdictNextDistributionService(repository, layout.neighboursResponsePath, configuration.distribution.defaultSignalCount),
    EdictNextGenerationService(
      repository,
      inspectionServer,
      layout.root,
      configuration.generation.maxProjectAnalyses,
      configuration.generation.defaultGenerationCount,
    ),
    EdictNextEnrichmentMcpToolset(management, EdictNextEnrichmentService(repository, layout.root)),
    EdictSourceFileService(layout.root, null),
    configuration,
    promotion ?: PromotionService(repository, configuration, null, NoRedaction),
  )
}

/** Redacts nothing, for services whose test text holds no issued tokens. */
internal val NoRedaction = TokenRedactor { it }
