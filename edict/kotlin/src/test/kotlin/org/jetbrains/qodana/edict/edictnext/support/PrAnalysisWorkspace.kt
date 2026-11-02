// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.edictnext.support

import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put
import org.jetbrains.qodana.edict.ci.support.KOTLIN
import org.jetbrains.qodana.edict.common.EdictLayout
import org.jetbrains.qodana.edict.common.flag
import org.jetbrains.qodana.edict.common.obj
import org.jetbrains.qodana.edict.common.text
import org.jetbrains.qodana.edict.edictnext.EdictManagementService
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState
import org.jetbrains.qodana.edict.extraction.reviews.PrAnalysis
import org.jetbrains.qodana.edict.extraction.reviews.PrAnalysisDateRange
import org.jetbrains.qodana.edict.extraction.reviews.PrBatchSummary
import org.jetbrains.qodana.edict.extraction.reviews.PrItem
import org.jetbrains.qodana.edict.extraction.reviews.PrPublicationReceipt
import org.jetbrains.qodana.edict.extraction.reviews.PrReceipt
import org.jetbrains.qodana.edict.extraction.reviews.PrSignalDraft
import org.jetbrains.qodana.edict.extraction.reviews.RepositoryPrAnalysisCoverage
import org.jetbrains.qodana.edict.extraction.reviews.ReviewSelectionInput
import org.jetbrains.qodana.edict.edictnext.EdictNextSignal
import org.jetbrains.qodana.edict.support.FakeReviewApi
import org.jetbrains.qodana.edict.support.withTools
import org.jetbrains.qodana.edict.support.launch
import org.junit.jupiter.api.extension.AfterEachCallback
import org.junit.jupiter.api.extension.ExtensionContext
import java.nio.file.Files

/** The skill of the task that coordinates PR analysis and publishes its Signals. */
internal const val PR_ANALYSIS = "edict-pr-signal-analysis"

/** The skill of a task that coordinates the analysis of commits. */
private const val COMMIT_ANALYSIS = "edict-batch-signal-analysis"

/** The skill of the workers that analyze one chunk of a PR batch each. */
internal const val SIGNAL_ANALYSIS = "edict-signal-analysis"

/**
 * A PR analysis of JetBrains/kotlin on [TODAY]: fresh managed state whose plan has one running PR-analysis
 * [coordinator], the [reviews] it reads, and the [analysis] under test. Registered as an extension, it closes and
 * deletes the state after each test.
 */
internal class PrAnalysisWorkspace : AfterEachCallback {
  private val directory = Files.createTempDirectory("edict-pr-analysis")

  val store: EdictNextRepositoryState = EdictNextRepositoryState.open(directory.resolve("state"))
  val reviews = FakeReviewApi(KOTLIN)
  val analysis = PrAnalysis(store, reviews) { TODAY }

  private val plan = store.createPlan("Analyze reviews", listOf(EdictNextRepositoryState.Step(PR_ANALYSIS, "Analyze PRs")))

  /** The token of the running PR-analysis coordinator. */
  val coordinator: String = store.launch(plan.token, plan.plan.tasks.single().id, PR_ANALYSIS).token

  /** The token of the first analysis worker, delegated on first use. */
  val worker: String by lazy { analysisWorker() }

  /** The management tools over the same state and reviews, with their own PR analysis, as the MCP server serves them. */
  val tools: EdictManagementService by lazy {
    val layout = EdictLayout(directory, store.root)
    EdictManagementService(store, layout, ciReviews = reviews, prAnalysisToday = { TODAY }).withTools(layout)
  }

  /** The token of a new running analysis worker of the coordinator. */
  fun analysisWorker(): String {
    val task = store.addTask(coordinator, SIGNAL_ANALYSIS, "Analyze PR chunk")
    return store.launch(coordinator, task.id, SIGNAL_ANALYSIS).token
  }

  /** The token of a running analysis worker of a commit-analysis coordinator, which takes no part in the PR analysis. */
  fun commitAnalysisWorker(): String {
    val commits = store.addTask(plan.token, COMMIT_ANALYSIS, "Analyze commits")
    val commitCoordinator = store.launch(plan.token, commits.id, COMMIT_ANALYSIS).token
    val task = store.addTask(commitCoordinator, SIGNAL_ANALYSIS, "Analyze commit chunk")
    return store.launch(commitCoordinator, task.id, SIGNAL_ANALYSIS).token
  }

  /** Prepares the default batch of [CORRECTION] alone, which the provider has [files] of. */
  fun correctionBatch(files: Map<Pair<String, String>, String> = emptyMap()): PrBatchSummary {
    reviews.pullRequests += CORRECTION
    reviews.files += files
    return analysis.prepareBatch(coordinator, ReviewSelectionInput())
  }

  /** The PR number and thread ID of each work item of [batch], by chunk, as the worker reads them. */
  fun items(batch: PrBatchSummary): List<List<Pair<Int, String>>> = (0 until batch.availableChunkCount).map { chunk ->
    analysis.chunk(worker, batch.batchId, chunk).workItemIds.map { id ->
      analysis.get(worker, batch.batchId, id).let { it.pr.number to it.thread.threadId }
    }
  }

  /** The work item at [position] of chunk [chunkIndex] of [batch], as the worker reads it. */
  fun item(batch: PrBatchSummary, chunkIndex: Int = 0, position: Int = 0): PrItem =
    analysis.get(worker, batch.batchId, analysis.chunk(worker, batch.batchId, chunkIndex).workItemIds[position])

  /** Stores chunk [chunkIndex] of [batch] with every item inspected, none failed, and no Signal. */
  fun storeEmptyChunk(batch: PrBatchSummary, chunkIndex: Int = 0): PrReceipt {
    val ids = analysis.chunk(worker, batch.batchId, chunkIndex).workItemIds
    return analysis.store(worker, batch.batchId, chunkIndex, ids, emptyList(), emptyList(), emptyList())
  }

  /** Stores the first chunk of [batch], every item inspected and none failed, with [drafts]. */
  fun storeDrafts(batch: PrBatchSummary, vararg drafts: PrSignalDraft): PrReceipt {
    val ids = analysis.chunk(worker, batch.batchId, 0).workItemIds
    return analysis.store(worker, batch.batchId, 0, ids, emptyList(), emptyList(), emptyList(), drafts.toList())
  }

  /** Stores chunk [chunkIndex] of [batch], every item inspected and none failed, with the complete [signals]. */
  fun storeSignals(batch: PrBatchSummary, vararg signals: EdictNextSignal, chunkIndex: Int = 0): PrReceipt {
    val ids = analysis.chunk(worker, batch.batchId, chunkIndex).workItemIds
    return analysis.store(worker, batch.batchId, chunkIndex, ids, emptyList(), signals.toList(), emptyList())
  }

  /** Stores every chunk of [batch] empty, then publishes it. */
  fun publishEmpty(batch: PrBatchSummary): PrPublicationReceipt {
    repeat(batch.availableChunkCount) { storeEmptyChunk(batch, it) }
    return analysis.publishValidated(coordinator, batch.batchId)
  }

  /** Records the date [ranges] as analyzed. */
  fun coverDates(vararg ranges: PrAnalysisDateRange) {
    store.recordPrAnalysisCoverage(coordinator, RepositoryPrAnalysisCoverage(KOTLIN, ranges.toList()))
  }

  /** Records the pull requests [numbers] as analyzed. */
  fun coverNumbers(vararg numbers: Int) {
    store.recordPrAnalysisCoverage(coordinator, RepositoryPrAnalysisCoverage(KOTLIN, analyzedPrNumbers = numbers.toList()))
  }

  /**
   * Prepares the default batch through the tools and publishes it, after one worker stored each of the chunks
   * [storedChunks] and another worker failed each other chunk.
   */
  fun publishThroughTools(vararg storedChunks: Int) {
    val batch = callTool("edict_fetch_pr_batch", buildJsonObject { put("token", coordinator) }).obj("structuredContent")
    val batchId = batch.text("batchId")
    repeat(batch.text("availableChunkCount").toInt()) { chunk ->
      val chunkWorker = analysisWorker()
      if (chunk in storedChunks) {
        storeThroughTools(chunkWorker, batchId, chunk)
        store.finishTask(chunkWorker, "completed", "Stored chunk $chunk")
      }
      else {
        store.finishTask(chunkWorker, "failed", "Chunk $chunk failed after retry")
      }
    }
    callTool("edict_publish_validated_pr_signals", buildJsonObject { put("token", coordinator); put("batchId", batchId) })
  }

  private fun storeThroughTools(chunkWorker: String, batchId: String, chunk: Int) {
    val ids = callTool("edict_get_pr_analysis_chunk", buildJsonObject {
      put("token", chunkWorker)
      put("batchId", batchId)
      put("chunkIndex", chunk)
    }).obj("structuredContent").getValue("workItemIds").jsonArray.map { it.jsonPrimitive.content }
    callTool("edict_store_pr_signals", buildJsonObject {
      put("token", chunkWorker)
      put("batchId", batchId)
      put("chunkIndex", chunk)
      put("inspectedWorkItemIds", JsonArray(ids.map(::JsonPrimitive)))
      put("failedWorkItemIds", JsonArray(emptyList()))
      put("signals", JsonArray(emptyList()))
      put("problems", JsonArray(emptyList()))
    })
  }

  /** Calls the tool [name], failing the test's set-up when the tool fails. */
  private fun callTool(name: String, arguments: JsonObject): JsonObject =
    tools.call(name, arguments).also { check(it.flag("isError") != true) { "Set-up call of $name failed: $it" } }

  override fun afterEach(context: ExtensionContext) {
    store.close()
    directory.toFile().deleteRecursively()
  }
}

/** The result of a management tool that failed with [message]. */
internal fun toolError(message: String): JsonObject = buildJsonObject {
  put("content", JsonArray(listOf(buildJsonObject { put("type", "text"); put("text", message) })))
  put("isError", true)
}
