package org.jetbrains.qodana.edict.history.enrichment

import io.modelcontextprotocol.kotlin.sdk.server.Server
import io.modelcontextprotocol.kotlin.sdk.server.ServerOptions
import io.modelcontextprotocol.kotlin.sdk.types.Implementation
import io.modelcontextprotocol.kotlin.sdk.types.ServerCapabilities
import org.jetbrains.qodana.edict.common.EdictLayout
import org.jetbrains.qodana.edict.edictnext.EdictManagementService
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState
import org.jetbrains.qodana.edict.edictnext.EdictRepository
import org.jetbrains.qodana.edict.edictnext.EdictRepositoryDirectory
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import kotlin.test.Test
import kotlin.test.assertEquals

class EdictNextEnrichmentMcpToolsetTest {
  @TempDir
  lateinit var directory: Path

  @Test
  fun `enrichment tools register without generation distribution or IntelliJ services`() {
    EdictNextRepositoryState.open(directory.resolve("state")).use { store ->
      val management = EdictManagementService(store, EdictLayout(directory, store.root))
      val repository = EdictRepository(EdictRepositoryDirectory(store.root))
      val server = Server(
        serverInfo = Implementation("enrichment-test", "1.0.0"),
        options = ServerOptions(capabilities = ServerCapabilities(tools = ServerCapabilities.Tools(listChanged = false))),
        instructions = "Enrichment tool registration test",
      )
      management.registerTools(server)
      val managementTools = server.tools.keys.toSet()

      EdictNextEnrichmentMcpToolset(
        management, EdictNextEnrichmentService(repository, directory),
      ).registerTools(server)

      assertEquals(
        setOf("edict_next_get_history_context", "edict_publish_signal_to_cluster"),
        server.tools.keys - managementTools,
      )
      assertEquals(
        server.tools.getValue("edict_publish_signal").tool.inputSchema.properties?.get("signal"),
        server.tools.getValue("edict_publish_signal_to_cluster").tool.inputSchema.properties?.get("signal"),
      )
    }
  }
}
