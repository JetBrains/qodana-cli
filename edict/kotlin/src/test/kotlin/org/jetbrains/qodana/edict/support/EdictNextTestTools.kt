package org.jetbrains.qodana.edict.support

import kotlinx.serialization.json.JsonObject
import org.jetbrains.qodana.edict.edictnext.EdictManagementService
import org.jetbrains.qodana.edict.edictnext.EdictNextMcpToolset
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState
import java.io.PrintWriter
import java.nio.file.Path

internal class EdictNextTestTools(
  store: EdictNextRepositoryState,
  logs: Path? = null,
  taskOutput: PrintWriter = PrintWriter(System.err, true),
) {
  private val management = EdictManagementService(store, logs = logs, taskOutput = taskOutput)
  private val toolset = EdictNextMcpToolset(runId = "unit-test", management = management)
    .also { it.createServer() }

  fun call(name: String, arguments: JsonObject): JsonObject = management.call(name, arguments)
}
