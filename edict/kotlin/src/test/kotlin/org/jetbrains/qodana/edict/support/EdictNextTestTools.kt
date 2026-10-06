package org.jetbrains.qodana.edict.support

import kotlinx.serialization.json.JsonObject
import org.jetbrains.qodana.edict.common.EdictLayout
import org.jetbrains.qodana.edict.edictnext.EdictManagementService
import org.jetbrains.qodana.edict.edictnext.EdictNextDistributionService
import org.jetbrains.qodana.edict.edictnext.EdictNextGenerationService
import org.jetbrains.qodana.edict.edictnext.EdictNextMcpToolset
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState
import org.jetbrains.qodana.edict.edictnext.EdictRepository
import org.jetbrains.qodana.edict.edictnext.EdictRepositoryDirectory
import org.jetbrains.qodana.edict.edictnext.IntellijMcpServerService
import java.io.PrintWriter
import kotlin.io.path.createDirectories

internal class EdictNextTestTools(
  store: EdictNextRepositoryState,
  // Logs land next to the state, never inside it.
  layout: EdictLayout = EdictLayout(store.root.parent, store.root),
  taskOutput: PrintWriter = PrintWriter(System.err, true),
) {
  private val management = EdictManagementService(store, layout, taskOutput = taskOutput)
  init {
    // Registering the toolset fills the management tool table that call() uses; the IDE never starts.
    edictNextToolset(layout, management).createServer()
  }

  fun call(name: String, arguments: JsonObject): JsonObject = management.call(name, arguments)
}

/** The toolset `EdictServer` wires for a run over [layout], creating the state directory its repository requires. */
internal fun edictNextToolset(
  layout: EdictLayout,
  management: EdictManagementService,
  inspectionServer: IntellijMcpServerService = IntellijMcpServerService(layout.root),
): EdictNextMcpToolset {
  val repository = EdictRepository(EdictRepositoryDirectory(layout.stateDirectory.createDirectories()))
  return EdictNextMcpToolset(
    layout, inspectionServer, management,
    EdictNextDistributionService(repository, layout.neighboursResponsePath),
    EdictNextGenerationService(repository, inspectionServer, layout.root),
  )
}
