// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict

import org.jetbrains.qodana.edict.common.GitRepository
import io.ktor.server.cio.CIO
import io.ktor.server.cio.CIOApplicationEngine
import io.ktor.server.engine.EmbeddedServer
import io.ktor.server.engine.embeddedServer
import io.modelcontextprotocol.kotlin.sdk.server.mcpStreamableHttp
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.runBlocking
import org.jetbrains.qodana.edict.ci.ReviewApi
import org.jetbrains.qodana.edict.common.EdictConfiguration
import org.jetbrains.qodana.edict.common.EdictLayout
import org.jetbrains.qodana.edict.edictnext.EdictManagementService
import org.jetbrains.qodana.edict.edictnext.EdictNextDistributionService
import org.jetbrains.qodana.edict.edictnext.EdictNextGenerationService
import org.jetbrains.qodana.edict.edictnext.EdictNextMcpToolset
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalPairs
import org.jetbrains.qodana.edict.edictnext.EdictRepository
import org.jetbrains.qodana.edict.edictnext.EdictRepositoryDirectory
import org.jetbrains.qodana.edict.edictnext.EdictSourceFileService
import org.jetbrains.qodana.edict.edictnext.IntellijMcpServerService
import org.jetbrains.qodana.edict.feedback.FeedbackSignals
import org.jetbrains.qodana.edict.feedback.FeedbackMcpToolset
import org.jetbrains.qodana.edict.feedback.FeedbackRecords
import org.jetbrains.qodana.edict.feedback.GitHubFeedbackApi
import org.jetbrains.qodana.edict.history.enrichment.EdictNextEnrichmentMcpToolset
import org.jetbrains.qodana.edict.history.enrichment.EdictNextEnrichmentService
import org.jetbrains.qodana.edict.promotion.PromotionService
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
    // TODO: we ara initialising a lot of things in Main and here. In my opinion, the initialisation code looks tangled and not structured at all. What do you think - should this be changed?
    /** Starts on [port]; the agent config names it, so a busy port fails instead of moving the server. 0 picks a free one. */
    fun start(
      layout: EdictLayout,
      port: Int,
      inspectionServer: IntellijMcpServerService,
      configuration: EdictConfiguration,
      ciReviews: ReviewApi?,
      promotionReviews: ReviewApi?,
      feedbackApi: GitHubFeedbackApi?,
    ): EdictServer {
      val store = EdictNextRepositoryState.open(layout.stateDirectory)
      try {
        // PR extraction records the Signals of one correction; distribution in the same run offers them together.
        val signalPairs = EdictNextSignalPairs()
        // The lock above created the state directory, so the repository can be opened now.
        val paths = EdictRepositoryDirectory(layout.stateDirectory)
        val repository = EdictRepository(paths)
        // TODO: a changed edict.promotion.url leaves active promotions in the earlier repository, which the promotion
        //  client cannot read: inspections on review, closed ones of generated clusters that checkReviews still reads, and
        //  weak-signal reviews in promotions/active.json. Their checks fail on every run; decide how to handle the change.
        val management = EdictManagementService(
          store,
          layout,
          ciReviews = ciReviews,
          defaultSignalCount = configuration.extraction.defaultSignalCount,
          signalPairs = signalPairs,
        )
        val toolset = EdictNextMcpToolset(
          layout, inspectionServer, management,
          EdictNextDistributionService(
            repository,
            layout.neighboursResponsePath,
            configuration.distribution.defaultSignalCount,
            pairs = signalPairs,
          ),
          EdictNextGenerationService(
            repository,
            inspectionServer,
            layout.root,
            configuration.generation.maxProjectAnalyses,
            configuration.generation.defaultGenerationCount,
          ),
          EdictNextEnrichmentMcpToolset(management, EdictNextEnrichmentService(repository, layout.root)),
          EdictSourceFileService(layout.root, ciReviews),
          configuration,
          PromotionService(repository, configuration, promotionReviews, store, executionBranch = {
            GitRepository(layout.root).currentBranch()
          }),
          feedbackApi?.let { FeedbackMcpToolset(management, FeedbackSignals(FeedbackRecords(paths), it, store)) },
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
