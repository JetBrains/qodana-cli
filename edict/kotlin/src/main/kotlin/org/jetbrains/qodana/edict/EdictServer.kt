// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict

import io.ktor.server.cio.CIO
import io.ktor.server.cio.CIOApplicationEngine
import io.ktor.server.engine.EmbeddedServer
import io.ktor.server.engine.embeddedServer
import io.modelcontextprotocol.kotlin.sdk.server.mcpStreamableHttp
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.runBlocking
import org.jetbrains.qodana.edict.common.EdictConfiguration
import org.jetbrains.qodana.edict.common.EdictLayout
import org.jetbrains.qodana.edict.edictnext.EdictManagementService
import org.jetbrains.qodana.edict.edictnext.EdictNextDistributionService
import org.jetbrains.qodana.edict.edictnext.EdictNextGenerationService
import org.jetbrains.qodana.edict.edictnext.EdictNextMcpToolset
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState
import org.jetbrains.qodana.edict.edictnext.EdictRepository
import org.jetbrains.qodana.edict.edictnext.EdictRepositoryDirectory
import org.jetbrains.qodana.edict.edictnext.IntellijMcpServerService
import org.jetbrains.qodana.edict.extraction.reviews.ReviewClient
import org.jetbrains.qodana.edict.extraction.reviews.ReviewProvider
import java.util.concurrent.atomic.AtomicBoolean

/**
 * One Edict MCP server process serving one run over Streamable HTTP on loopback. It holds the state lock for its whole
 * life and owns everything it starts: [close] stops HTTP, then the IDE, then releases the state, once.
 */
internal class EdictServer private constructor(
  val store: EdictNextRepositoryState,
  val management: EdictManagementService,
  private val inspectionServer: IntellijMcpServerService,
  private val engine: EmbeddedServer<CIOApplicationEngine, CIOApplicationEngine.Configuration>,
  val url: String,
) : AutoCloseable {
  private val closing = AtomicBoolean()
  private val closed = CompletableDeferred<Unit>()

  /** Blocks until [close], for example from a shutdown hook. */
  fun await(): Unit = runBlocking { closed.await() }

  override fun close() {
    if (!closing.compareAndSet(false, true)) return
    val failures = listOf({ engine.stop(1_000, 5_000) }, { runBlocking { inspectionServer.stop() } }, store::close)
      .mapNotNull { runCatching(it).exceptionOrNull() }
    closed.complete(Unit)
    failures.firstOrNull()?.let { failure -> failures.drop(1).forEach(failure::addSuppressed); throw failure }
  }

  companion object {
    // TODO: look into signalRepository != signal rpository
    /** Starts on [port]; the agent config names it, so a busy port fails instead of moving the server. 0 picks a free one. */
    fun start(
      layout: EdictLayout,
      port: Int,
      inspectionServer: IntellijMcpServerService,
      reviewProvider: ReviewProvider = ReviewClient(),
      configuration: EdictConfiguration = EdictConfiguration(),
    ): EdictServer {
      val store = EdictNextRepositoryState.open(layout.stateDirectory)
      try {
        val management = EdictManagementService(store, layout, reviewProvider = reviewProvider)
        // The lock above created the state directory, so the repository can be opened now.
        val repository = EdictRepository(EdictRepositoryDirectory(layout.stateDirectory))
        val toolset = EdictNextMcpToolset(
          layout, inspectionServer, management,
          EdictNextDistributionService(repository, layout.neighboursResponsePath),
          EdictNextGenerationService(repository, inspectionServer, layout.root),
          configuration,
        )
        val engine = embeddedServer(CIO, host = "127.0.0.1", port = port) {
          mcpStreamableHttp { toolset.createServer() }
        }.start(wait = false)
        val resolvedPort = runBlocking { engine.engine.resolvedConnectors() }.single().port
        return EdictServer(store, management, inspectionServer, engine, "http://127.0.0.1:$resolvedPort/mcp")
      }
      catch (e: Throwable) {
        store.close()
        throw e
      }
    }
  }
}
