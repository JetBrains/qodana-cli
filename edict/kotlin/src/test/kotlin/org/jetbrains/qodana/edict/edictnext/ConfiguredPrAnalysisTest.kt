package org.jetbrains.qodana.edict.edictnext

import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.encodeToJsonElement
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonArray
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.decodeFromJsonElement
import org.jetbrains.qodana.edict.common.array
import org.jetbrains.qodana.edict.common.flag
import org.jetbrains.qodana.edict.common.EdictLayout
import org.jetbrains.qodana.edict.common.obj
import org.jetbrains.qodana.edict.common.text
import org.jetbrains.qodana.edict.common.wireJson
import org.jetbrains.qodana.edict.ci.CiProviderId
import org.jetbrains.qodana.edict.ci.PullRequest
import org.jetbrains.qodana.edict.ci.ReviewExtractionApi
import org.jetbrains.qodana.edict.ci.ReviewFetchResult
import org.jetbrains.qodana.edict.ci.ReviewMessage
import org.jetbrains.qodana.edict.ci.ReviewRepository
import org.jetbrains.qodana.edict.ci.ReviewSelection
import org.jetbrains.qodana.edict.ci.ReviewThread
import org.jetbrains.qodana.edict.extraction.reviews.PrAnalysis
import org.jetbrains.qodana.edict.extraction.reviews.PrAnalysisDateRange
import org.jetbrains.qodana.edict.extraction.reviews.PrSignalDiff
import org.jetbrains.qodana.edict.extraction.reviews.PrSignalDraft
import org.jetbrains.qodana.edict.extraction.reviews.ReviewSelectionInput
import org.jetbrains.qodana.edict.support.launch
import org.jetbrains.qodana.edict.support.edictNextToolset
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import java.time.LocalDate
import java.time.ZoneOffset
import kotlin.test.Test
import kotlin.test.assertContains
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotEquals
import kotlin.test.assertFails
import kotlin.test.assertTrue

class ConfiguredPrAnalysisTest {
  @TempDir
  lateinit var directory: Path

  @Test
  fun `daily extraction returns one complete date and advances after publication`() {
    val configured = ReviewRepository(CiProviderId.GITHUB, "JetBrains", "qodana-cli")
    val today = LocalDate.of(2026, 10, 9)
    fun pr(number: Int, date: LocalDate, relevant: Boolean): PullRequest = PullRequest(
      number = number,
      url = "https://example.test/review/$number",
      title = "Review $number",
      body = "",
      baseRevision = "a".repeat(40),
      headRevision = "b".repeat(40),
      closeTimestamp = date.atTime(12, 0).toInstant(ZoneOffset.UTC).toEpochMilli(),
      threads = if (!relevant) emptyList() else listOf(ReviewThread(
        threadId = "thread-$number",
        reviewDiscussionUrl = "https://example.test/review/$number#discussion",
        filePath = "src/Example$number.kt",
        originalCommitSha = "a".repeat(40),
        anchorLine = 1,
        anchorEndLine = 1,
        messages = listOf(ReviewMessage("reviewer", "Fix this", "${date}T12:00:00Z")),
      )),
    )
    val provider = DateReviewProvider(listOf(
      pr(9, today, relevant = true),
      pr(8, today.minusDays(1), relevant = true),
      pr(7, today.minusDays(2), relevant = false),
      pr(6, today.minusDays(3), relevant = true),
    ))
    EdictNextRepositoryState.open(directory.resolve("state")).use { store ->
      val plan = store.createPlan(
        "Daily analysis",
        listOf(EdictNextRepositoryState.Step("edict-pr-signal-analysis", "Analyze full UTC dates")),
      )
      val worker = store.launch(plan.token, plan.plan.tasks.single().id, "edict-pr-signal-analysis")
      val layout = EdictLayout(directory, store.root)
      val management = EdictManagementService(
        store,
        layout,
        reviewProvider = provider,
        reviewRepository = configured,
        prAnalysisToday = { today },
      )
      edictNextToolset(layout, management).createServer()

      val fetched = management.call("edict_fetch_pr_batch", buildJsonObject { put("token", worker.token) })
      assertFalse(fetched.flag("isError") == true)
      val summary = fetched.obj("structuredContent")
      assertEquals(setOf("batchId", "availableChunkCount"), summary.keys)
      assertEquals(1, summary.getValue("availableChunkCount").jsonPrimitive.content.toInt())
      assertTrue(provider.selections.all { LocalDate.parse(it.endDate).isBefore(today) })
      assertTrue(store.getPrAnalysisCoverage(worker.token, configured).analyzedDateRanges.isEmpty())

      val batchId = summary.text("batchId")
      val analysis = launchAnalysisWorker(store, worker.token)
      val chunkResult = management.call("edict_get_pr_analysis_chunk", buildJsonObject {
        put("token", analysis.token)
        put("batchId", batchId)
        put("chunkIndex", 0)
      })
      assertFalse(chunkResult.flag("isError") == true, chunkResult.toString())
      val listed = chunkResult.obj("structuredContent")
      val workItemIds = listed.getValue("workItemIds").jsonArray.map { it.jsonPrimitive.content }
      assertEquals(1, workItemIds.size, listed.toString())
      val validation = management.call("edict_store_pr_signals", buildJsonObject {
        put("token", analysis.token)
        put("batchId", batchId)
        put("chunkIndex", 0)
        put("inspectedWorkItemIds", JsonArray(workItemIds.map(::JsonPrimitive)))
        put("failedWorkItemIds", JsonArray(emptyList()))
        put("signals", JsonArray(emptyList()))
        put("problems", JsonArray(emptyList()))
      })
      assertFalse(validation.flag("isError") == true)
      val publication = management.call("edict_publish_validated_pr_signals", buildJsonObject {
        put("token", worker.token)
        put("batchId", batchId)
      })
      assertFalse(publication.flag("isError") == true)
      val receipt = publication.obj("structuredContent")
      assertTrue(receipt.flag("coverageUpdated") == true)
      assertEquals(
        listOf(PrAnalysisDateRange("2026-10-08", "2026-10-08")),
        wireJson.decodeFromJsonElement<List<PrAnalysisDateRange>>(receipt.getValue("analyzedDateRanges")),
      )

      val coverage = store.getPrAnalysisCoverage(worker.token, configured)
      assertEquals(listOf(PrAnalysisDateRange("2026-10-08", "2026-10-08")), coverage.analyzedDateRanges)
      assertTrue(coverage.analyzedPrNumbers.isEmpty())

      val next = management.call("edict_fetch_pr_batch", buildJsonObject { put("token", worker.token) })
        .obj("structuredContent")
      assertEquals(0, next.getValue("availableChunkCount").jsonPrimitive.content.toInt())
      val nextPublication = management.call("edict_publish_validated_pr_signals", buildJsonObject {
        put("token", worker.token)
        put("batchId", next.text("batchId"))
      }).obj("structuredContent")
      assertEquals(
        listOf(PrAnalysisDateRange("2026-10-07", "2026-10-07")),
        wireJson.decodeFromJsonElement<List<PrAnalysisDateRange>>(nextPublication.getValue("analyzedDateRanges")),
      )
    }
  }

  @Test
  fun `explicit date selection ignores daily target and records the complete date`() {
    val configured = ReviewRepository(CiProviderId.GITHUB, "JetBrains", "qodana-cli")
    val today = LocalDate.of(2026, 10, 9)
    val selectedDate = today.minusDays(1)
    val provider = DateReviewProvider((1..3).map { number ->
      PullRequest(
        number = number,
        url = "https://example.test/review/$number",
        title = "Review $number",
        body = "",
        baseRevision = "a".repeat(40),
        headRevision = "b".repeat(40),
        closeTimestamp = selectedDate.atTime(12, number).toInstant(ZoneOffset.UTC).toEpochMilli(),
        threads = listOf(ReviewThread(
          threadId = "thread-$number",
          reviewDiscussionUrl = "https://example.test/review/$number#discussion",
          filePath = "src/Example$number.kt",
          originalCommitSha = "a".repeat(40),
          anchorLine = 1,
          anchorEndLine = 1,
          messages = listOf(ReviewMessage("reviewer", "Fix this", "${selectedDate}T12:00:00Z")),
        )),
      )
    })
    EdictNextRepositoryState.open(directory.resolve("state")).use { store ->
      val plan = store.createPlan(
        "Date analysis",
        listOf(EdictNextRepositoryState.Step("edict-pr-signal-analysis", "Analyze a complete UTC date")),
      )
      val worker = store.launch(plan.token, plan.plan.tasks.single().id, "edict-pr-signal-analysis")
      val layout = EdictLayout(directory, store.root)
      val management = EdictManagementService(
        store,
        layout,
        reviewProvider = provider,
        reviewRepository = configured,
        prAnalysisToday = { today },
      )
      edictNextToolset(layout, management).createServer()

      val fetched = management.call("edict_fetch_pr_batch", buildJsonObject {
        put("token", worker.token)
        put("startDate", selectedDate.toString())
        put("endDate", selectedDate.toString())
      })
      assertFalse(fetched.flag("isError") == true)
      val summary = fetched.obj("structuredContent")
      assertEquals(1, summary.getValue("availableChunkCount").jsonPrimitive.content.toInt())

      val batchId = summary.text("batchId")
      val analysis = launchAnalysisWorker(store, worker.token)
      val chunkResult = management.call("edict_get_pr_analysis_chunk", buildJsonObject {
        put("token", analysis.token)
        put("batchId", batchId)
        put("chunkIndex", 0)
      })
      assertFalse(chunkResult.flag("isError") == true, chunkResult.toString())
      val listed = chunkResult.obj("structuredContent")
      val workItemIds = listed.getValue("workItemIds").jsonArray.map { it.jsonPrimitive.content }
      val validation = management.call("edict_store_pr_signals", buildJsonObject {
        put("token", analysis.token)
        put("batchId", batchId)
        put("chunkIndex", 0)
        put("inspectedWorkItemIds", JsonArray(workItemIds.map(::JsonPrimitive)))
        put("failedWorkItemIds", JsonArray(emptyList()))
        put("signals", JsonArray(emptyList()))
        put("problems", JsonArray(emptyList()))
      })
      assertFalse(validation.flag("isError") == true)
      val publication = management.call("edict_publish_validated_pr_signals", buildJsonObject {
        put("token", worker.token)
        put("batchId", batchId)
      })
      assertFalse(publication.flag("isError") == true)
      val receipt = publication.obj("structuredContent")
      assertEquals(3, receipt.getValue("selectedPrCount").jsonPrimitive.content.toInt())
      assertTrue(receipt.flag("coverageUpdated") == true)
      assertEquals(
        listOf(PrAnalysisDateRange(selectedDate.toString(), selectedDate.toString())),
        wireJson.decodeFromJsonElement<List<PrAnalysisDateRange>>(receipt.getValue("analyzedDateRanges")),
      )
      assertEquals(
        listOf(PrAnalysisDateRange(selectedDate.toString(), selectedDate.toString())),
        store.getPrAnalysisCoverage(worker.token, configured).analyzedDateRanges,
      )
    }
  }

  @Test
  fun `provider element problems are returned without recording incomplete date coverage`() {
    val configured = ReviewRepository(CiProviderId.SPACE, "project", "repo")
    val today = LocalDate.of(2026, 10, 9)
    val selectedDate = today.minusDays(1)
    val problem = "Space review discovery page 1 data[3]: expected object"
    val provider = DateReviewProvider(emptyList(), listOf(problem))
    EdictNextRepositoryState.open(directory.resolve("state")).use { store ->
      val plan = store.createPlan(
        "Date analysis",
        listOf(EdictNextRepositoryState.Step("edict-pr-signal-analysis", "Analyze a complete UTC date")),
      )
      val worker = store.launch(plan.token, plan.plan.tasks.single().id, "edict-pr-signal-analysis")
      val layout = EdictLayout(directory, store.root)
      val management = EdictManagementService(
        store,
        layout,
        reviewProvider = provider,
        reviewRepository = configured,
        prAnalysisToday = { today },
      )
      edictNextToolset(layout, management).createServer()

      val fetched = management.call("edict_fetch_pr_batch", buildJsonObject {
        put("token", worker.token)
        put("startDate", selectedDate.toString())
        put("endDate", selectedDate.toString())
      })
      assertFalse(fetched.flag("isError") == true)
      val summary = fetched.obj("structuredContent")
      assertEquals(setOf("batchId", "availableChunkCount"), summary.keys)
      assertEquals(0, summary.getValue("availableChunkCount").jsonPrimitive.content.toInt())
      val publication = management.call("edict_publish_validated_pr_signals", buildJsonObject {
        put("token", worker.token)
        put("batchId", summary.text("batchId"))
      })
      assertFalse(publication.flag("isError") == true)
      val receipt = publication.obj("structuredContent")
      assertEquals(listOf(problem), receipt.getValue("problems").jsonArray.map { it.jsonPrimitive.content })
      assertFalse(receipt.flag("coverageUpdated") == true)
      assertTrue(store.getPrAnalysisCoverage(worker.token, configured).analyzedDateRanges.isEmpty())
    }
  }

  @Test
  fun `publication records coverage when successful work items are majority despite failed chunk`() {
    val configured = ReviewRepository(CiProviderId.GITHUB, "JetBrains", "qodana-cli")
    val today = LocalDate.of(2026, 10, 9)
    val selectedDate = today.minusDays(1)
    val provider = DateReviewProvider(datedPullRequests(9, selectedDate))
    EdictNextRepositoryState.open(directory.resolve("state")).use { store ->
      val plan = store.createPlan(
        "Partial date analysis",
        listOf(EdictNextRepositoryState.Step("edict-pr-signal-analysis", "Analyze a complete UTC date")),
      )
      val coordinator = store.launch(plan.token, plan.plan.tasks.single().id, "edict-pr-signal-analysis")
      val layout = EdictLayout(directory, store.root)
      val management = EdictManagementService(
        store,
        layout,
        reviewProvider = provider,
        reviewRepository = configured,
        prAnalysisToday = { today },
      )
      edictNextToolset(layout, management).createServer()

      val summary = management.call("edict_fetch_pr_batch", buildJsonObject {
        put("token", coordinator.token)
      }).obj("structuredContent")
      assertEquals(2, summary.getValue("availableChunkCount").jsonPrimitive.content.toInt())
      val batchId = summary.text("batchId")
      val analysis = launchAnalysisWorker(store, coordinator.token)
      val chunk = management.call("edict_get_pr_analysis_chunk", buildJsonObject {
        put("token", analysis.token)
        put("batchId", batchId)
        put("chunkIndex", 0)
      }).obj("structuredContent")
      val workItemIds = chunk.getValue("workItemIds").jsonArray.map { it.jsonPrimitive.content }
      assertEquals(8, workItemIds.size)
      assertFalse(management.call("edict_store_pr_signals", buildJsonObject {
        put("token", analysis.token)
        put("batchId", batchId)
        put("chunkIndex", 0)
        put("inspectedWorkItemIds", JsonArray(workItemIds.map(::JsonPrimitive)))
        put("failedWorkItemIds", JsonArray(emptyList()))
        put("signals", JsonArray(emptyList()))
        put("problems", JsonArray(emptyList()))
      }).flag("isError") == true)
      store.finishTask(analysis.token, "completed", "Stored chunk 0")
      val failedAnalysis = launchAnalysisWorker(store, coordinator.token)
      store.finishTask(failedAnalysis.token, "failed", "Chunk 1 failed after retry")

      val publication = management.call("edict_publish_validated_pr_signals", buildJsonObject {
        put("token", coordinator.token)
        put("batchId", batchId)
      })
      assertFalse(publication.flag("isError") == true)
      val receipt = publication.obj("structuredContent")
      assertEquals(1, receipt.getValue("storedChunkCount").jsonPrimitive.content.toInt())
      assertEquals(listOf(1), receipt.getValue("failedChunkIndexes").jsonArray.map { it.jsonPrimitive.content.toInt() })
      assertEquals(8, receipt.getValue("successfulWorkItemCount").jsonPrimitive.content.toInt())
      assertEquals(1, receipt.getValue("failedWorkItemCount").jsonPrimitive.content.toInt())
      assertTrue(receipt.flag("coverageUpdated") == true)
      assertEquals(
        listOf(PrAnalysisDateRange(selectedDate.toString(), selectedDate.toString())),
        store.getPrAnalysisCoverage(coordinator.token, configured).analyzedDateRanges,
      )
      val completion = management.call("edict_task_finish", buildJsonObject {
        put("token", coordinator.token)
        put("status", "completed")
        put("result", "Published partial batch; one of two chunks failed")
      })
      assertFalse(completion.flag("isError") == true, completion.toString())
      assertEquals("failed", store.plan()!!.tasks.single { it.id == failedAnalysis.taskId }.status)
    }
  }

  @Test
  fun `publication withholds coverage when exactly half of work items succeed`() {
    val configured = ReviewRepository(CiProviderId.GITHUB, "JetBrains", "qodana-cli")
    val today = LocalDate.of(2026, 10, 9)
    val selectedDate = today.minusDays(1)
    EdictNextRepositoryState.open(directory.resolve("state")).use { store ->
      val plan = store.createPlan(
        "Half-complete date analysis",
        listOf(EdictNextRepositoryState.Step("edict-pr-signal-analysis", "Analyze a complete UTC date")),
      )
      val coordinator = store.launch(plan.token, plan.plan.tasks.single().id, "edict-pr-signal-analysis")
      val analysis = PrAnalysis(store, DateReviewProvider(datedPullRequests(16, selectedDate))) { today }
      val batch = analysis.prepareBatch(coordinator.token, configured, ReviewSelectionInput())
      assertEquals(2, batch.availableChunkCount)
      val worker = launchAnalysisWorker(store, coordinator.token)
      val chunk = analysis.chunk(worker.token, batch.batchId, 0)
      analysis.store(
        worker.token,
        batch.batchId,
        0,
        chunk.workItemIds,
        emptyList(),
        emptyList(),
        emptyList(),
      )

      val receipt = analysis.publishValidated(coordinator.token, batch.batchId)
      assertEquals(8, receipt.successfulWorkItemCount)
      assertEquals(8, receipt.failedWorkItemCount)
      assertFalse(receipt.coverageUpdated)
      assertTrue(store.getPrAnalysisCoverage(coordinator.token, configured).analyzedDateRanges.isEmpty())
    }
  }

  @Test
  fun `a stored Signal draft gets its PR metadata and diff from the work item`() {
    val configured = ReviewRepository(CiProviderId.GITHUB, "JetBrains", "qodana-cli")
    val today = LocalDate.of(2026, 10, 9)
    val diff = """
      diff --git a/src/Example1.kt b/src/Example1.kt
      --- a/src/Example1.kt
      +++ b/src/Example1.kt
      @@ -1,3 +1,3 @@
       fun a() {
      -  bad()
      +  good()
       }
    """.trimIndent() + "\n"
    EdictNextRepositoryState.open(directory.resolve("state")).use { store ->
      val plan = store.createPlan(
        "Draft storage",
        listOf(EdictNextRepositoryState.Step("edict-pr-signal-analysis", "Analyze a complete UTC date")),
      )
      val coordinator = store.launch(plan.token, plan.plan.tasks.single().id, "edict-pr-signal-analysis")
      val pullRequests = datedPullRequests(1, today.minusDays(1))
      val analysis = PrAnalysis(store, DateReviewProvider(pullRequests, diff = diff)) { today }
      val batch = analysis.prepareBatch(coordinator.token, configured, ReviewSelectionInput())
      val worker = launchAnalysisWorker(store, coordinator.token)
      val item = analysis.chunk(worker.token, batch.batchId, 0).workItemIds.single()
      val key = "$item|0|POSITIVE|src/Example1.kt|${"a".repeat(40)}|2:2"
      fun draft(before: String) = PrSignalDraft(
        workItemId = item,
        fileRevision = EdictNextFileRevision("src/Example1.kt", "a".repeat(40), listOf(EdictNextLineRange(2, 2))),
        label = EdictNextSignalLabel.POSITIVE,
        description = "Calls bad() where good() is required.",
        diff = PrSignalDiff(before, "b".repeat(40), "src/Example1.kt", "src/Example1.kt"),
      )

      val rejected = analysis.store(worker.token, batch.batchId, 0, listOf(item), emptyList(), emptyList(), emptyList(), listOf(draft("c".repeat(40))))
      assertEquals(0, rejected.signalCount)
      assertContains(rejected.failures.single().message, "base/comment revision to head")

      val receipt = analysis.store(worker.token, batch.batchId, 0, listOf(item), emptyList(), emptyList(), emptyList(), listOf(draft("a".repeat(40))))
      assertEquals(1, receipt.signalCount, receipt.failures.toString())
      analysis.publishValidated(coordinator.token, batch.batchId)
      val source = checkNotNull(store.inboxSignal(stableSignalId(key))).source as EdictNextSignalSource.FromPR
      val pr = pullRequests.single()
      assertEquals(pr.number, source.prNumber)
      assertEquals(pr.title, source.title)
      assertEquals(pr.threads.single().reviewDiscussionUrl, source.url)
      assertEquals(listOf("Fix this"), source.discussionMessages)
      assertEquals(diff, source.diffPositiveToNegative)
    }
  }

  @Test
  fun `a draft stores the changes near its lines from a large diff and fails when they cannot fit`() {
    val configured = ReviewRepository(CiProviderId.GITHUB, "JetBrains", "qodana-cli")
    val today = LocalDate.of(2026, 10, 9)
    val header = "--- a/src/Example1.kt\n+++ b/src/Example1.kt\n"
    val context = { line: Int -> " context $line ${"x".repeat(40)}" }
    val canonical = header + "@@ -1,250 +1,250 @@\n" + (1..250).joinToString("\n", postfix = "\n") {
      when (it) {
        100 -> "-bad()\n+good()"
        in 200..250 -> "-removed $it ${"y".repeat(80)}\n+added $it ${"z".repeat(80)}"
        else -> context(it)
      }
    }
    val excerpt = header + "@@ -97,7 +97,7 @@\n" +
      ((97..99).map(context) + listOf("-bad()", "+good()") + (101..103).map(context)).joinToString("") { "$it\n" }
    assertTrue(canonical.length > 4000)
    EdictNextRepositoryState.open(directory.resolve("state")).use { store ->
      val plan = store.createPlan("Bounded evidence", listOf(EdictNextRepositoryState.Step("edict-pr-signal-analysis", "Analyze date")))
      val coordinator = store.launch(plan.token, plan.plan.tasks.single().id, "edict-pr-signal-analysis")
      val analysis = PrAnalysis(store, DateReviewProvider(datedPullRequests(1, today.minusDays(1)), diff = canonical)) { today }
      val batch = analysis.prepareBatch(coordinator.token, configured, ReviewSelectionInput())
      val worker = launchAnalysisWorker(store, coordinator.token)
      val item = analysis.chunk(worker.token, batch.batchId, 0).workItemIds.single()
      fun draft(range: EdictNextLineRange, label: EdictNextSignalLabel = EdictNextSignalLabel.POSITIVE) = PrSignalDraft(
        item,
        EdictNextFileRevision("src/Example1.kt", (if (label == EdictNextSignalLabel.POSITIVE) "a" else "b").repeat(40), listOf(range)),
        label,
        "Use the correct call",
        diff = PrSignalDiff("a".repeat(40), "b".repeat(40), "src/Example1.kt", "src/Example1.kt"),
      )
      val receipt = analysis.store(
        worker.token, batch.batchId, 0, listOf(item), emptyList(), emptyList(), emptyList(),
        listOf(
          draft(EdictNextLineRange(100, 100)),
          draft(EdictNextLineRange(100, 100), EdictNextSignalLabel.NEGATIVE),
          draft(EdictNextLineRange(200, 250)),
        ),
      )
      assertEquals(2, receipt.signalCount, receipt.failures.toString())
      assertContains(receipt.failures.single().message, "do not fit 4000 characters")
      analysis.publishValidated(coordinator.token, batch.batchId)
      val signals = receipt.signalIds.map { checkNotNull(store.inboxSignal(it)) }
      signals.forEach { assertEquals(excerpt, (it.source as EdictNextSignalSource.FromPR).diffPositiveToNegative) }
    }
  }

  @Test
  fun `coordinator cannot complete when more than half of chunks fail`() {
    val configured = ReviewRepository(CiProviderId.GITHUB, "JetBrains", "qodana-cli")
    val today = LocalDate.of(2026, 10, 9)
    EdictNextRepositoryState.open(directory.resolve("state")).use { store ->
      val plan = store.createPlan(
        "Failed date analysis",
        listOf(EdictNextRepositoryState.Step("edict-pr-signal-analysis", "Analyze a complete UTC date")),
      )
      val coordinator = store.launch(plan.token, plan.plan.tasks.single().id, "edict-pr-signal-analysis")
      val analysis = PrAnalysis(store, DateReviewProvider(datedPullRequests(9, today.minusDays(1)))) { today }
      val batch = analysis.prepareBatch(coordinator.token, configured, ReviewSelectionInput())
      assertEquals(2, batch.availableChunkCount)
      analysis.publishValidated(coordinator.token, batch.batchId)

      val failure = assertFails { analysis.complete(coordinator.token) }
      assertTrue(failure.message.orEmpty().contains("More than half"), failure.message)
    }
  }

  @Test
  fun `PR extraction uses configured repository and rejects MCP identity overrides`() {
    val configured = ReviewRepository(CiProviderId.GITHUB, "JetBrains", "qodana-cli")
    val provider = CapturingReviewProvider()
    EdictNextRepositoryState.open(directory.resolve("state")).use { store ->
      val plan = store.createPlan(
        "Analyze reviews",
        listOf(EdictNextRepositoryState.Step("edict-pr-signal-analysis", "Analyze configured repository")),
      )
      val worker = store.launch(plan.token, plan.plan.tasks.single().id, "edict-pr-signal-analysis")
      val layout = EdictLayout(directory, store.root)
      val management = EdictManagementService(store, layout, reviewProvider = provider, reviewRepository = configured)
      edictNextToolset(layout, management).createServer()
      val result = management.call("edict_fetch_pr_batch", buildJsonObject {
        put("token", worker.token)
        putJsonArray("prNumbers") { add(JsonPrimitive(7)) }
      })
      assertNotEquals(result.flag("isError"), true)
      assertEquals(configured, provider.selection?.repositoryRef)

      val override = management.call("edict_fetch_pr_batch", buildJsonObject {
        put("token", worker.token)
        put("provider", "space")
        put("owner", "attacker")
        put("repo", "other")
        putJsonArray("prNumbers") { add(JsonPrimitive(7)) }
      })
      assertTrue(override.flag("isError") == true)
      assertEquals(configured, provider.selection?.repositoryRef)
    }
  }

  @Test
  fun `stored PR batch publishes valid models and reports rejected models`() {
    val configured = ReviewRepository(CiProviderId.GITHUB, "JetBrains", "qodana-cli")
    val baseRevision = "a".repeat(40)
    val headRevision = "b".repeat(40)
    val message = "Use the corrected form."
    val discussionUrl = "https://example.test/review/7#discussion"
    val provider = CapturingReviewProvider(listOf(PullRequest(
      number = 7,
      url = "https://example.test/review/7",
      title = "Correct the call",
      body = "",
      baseRevision = baseRevision,
      headRevision = headRevision,
      closeTimestamp = 1,
      threads = listOf(ReviewThread(
        threadId = "thread-7",
        reviewDiscussionUrl = discussionUrl,
        filePath = "src/Example.kt",
        originalCommitSha = baseRevision,
        messages = listOf(ReviewMessage("reviewer", message, "2026-10-08T00:00:00Z")),
      )),
    )))
    EdictNextRepositoryState.open(directory.resolve("state")).use { store ->
      val plan = store.createPlan(
        "Analyze reviews",
        listOf(EdictNextRepositoryState.Step("edict-pr-signal-analysis", "Analyze configured repository")),
      )
      val worker = store.launch(plan.token, plan.plan.tasks.single().id, "edict-pr-signal-analysis")
      val layout = EdictLayout(directory, store.root)
      val management = EdictManagementService(store, layout, reviewProvider = provider, reviewRepository = configured)
      edictNextToolset(layout, management).createServer()

      val fetched = management.call("edict_fetch_pr_batch", buildJsonObject {
        put("token", worker.token)
        putJsonArray("prNumbers") { add(JsonPrimitive(7)) }
      })
      assertFalse(fetched.flag("isError") == true)
      val batchId = fetched.obj("structuredContent").text("batchId")
      val analysis = launchAnalysisWorker(store, worker.token)
      val listed = management.call("edict_get_pr_analysis_chunk", buildJsonObject {
        put("token", analysis.token)
        put("batchId", batchId)
        put("chunkIndex", 0)
      })
      assertFalse(listed.flag("isError") == true, listed.toString())
      val workItemId = listed.obj("structuredContent").getValue("workItemIds").jsonArray.single().jsonPrimitive.content
      val diff = """
        --- a/src/Example.kt
        +++ b/src/Example.kt
        @@ -1 +1 @@
        -bad()
        +good()
      """.trimIndent()
      val source = EdictNextSignalSource.FromPR(
        prNumber = 7,
        title = "Correct the call",
        discussionMessages = listOf(message),
        diffPositiveToNegative = diff,
        url = discussionUrl,
      )
      fun signal(label: EdictNextSignalLabel): EdictNextSignal {
        val key = "github:JetBrains/qodana-cli:pr:7:$workItemId:$label"
        return EdictNextSignal(
          id = stableSignalId(key),
          idempotencyKey = key,
          fileRevision = EdictNextFileRevision(
            path = "src/Example.kt",
            revision = if (label == EdictNextSignalLabel.POSITIVE) baseRevision else headRevision,
            expectedRanges = listOf(EdictNextLineRange(1, 1)),
          ),
          source = source,
          label = label,
          description = if (label == EdictNextSignalLabel.POSITIVE) "The old call is incorrect" else "The corrected call is used",
          provenance = EdictNextSignalProvenance(workItemId, batchId),
        )
      }
      val signals = listOf(signal(EdictNextSignalLabel.POSITIVE), signal(EdictNextSignalLabel.NEGATIVE))
      val invalidKey = "github:JetBrains/qodana-cli:pr:7:$workItemId:invalid"
      val invalid = signal(EdictNextSignalLabel.POSITIVE).copy(
        id = stableSignalId(invalidKey),
        idempotencyKey = invalidKey,
        description = "",
      )
      val stored = management.call("edict_store_pr_signals", buildJsonObject {
        put("token", analysis.token)
        put("batchId", batchId)
        put("chunkIndex", 0)
        put("inspectedWorkItemIds", JsonArray(listOf(JsonPrimitive(workItemId))))
        put("failedWorkItemIds", JsonArray(emptyList()))
        put("signals", wireJson.encodeToJsonElement(signals + invalid))
        put("problems", JsonArray(emptyList()))
      })
      assertFalse(stored.flag("isError") == true)
      val storeReceipt = stored.obj("structuredContent")
      assertEquals(signals.size, storeReceipt.getValue("signalCount").jsonPrimitive.content.toInt(), storeReceipt.toString())
      assertEquals(
        signals.map(EdictNextSignal::id),
        storeReceipt.getValue("signalIds").jsonArray.map { it.jsonPrimitive.content },
      )
      val failure = storeReceipt.array("failures").single().jsonObject
      assertEquals(2, failure.getValue("signalIndex").jsonPrimitive.content.toInt())
      assertEquals(invalid.id, failure.getValue("signalId").jsonPrimitive.content)
      assertEquals(workItemId, failure.getValue("workItemId").jsonPrimitive.content)
      assertTrue(failure.getValue("message").jsonPrimitive.content.contains("description"))

      val workerDirectPublication = management.call("edict_publish_signal", buildJsonObject {
        put("token", analysis.token)
        put("signal", wireJson.encodeToJsonElement(signals.first()))
      })
      assertTrue(workerDirectPublication.flag("isError") == true)
      val workerBatchPublication = management.call("edict_publish_validated_pr_signals", buildJsonObject {
        put("token", analysis.token)
        put("batchId", batchId)
      })
      assertTrue(workerBatchPublication.flag("isError") == true)

      val previousModel = signals.first().copy(description = "Previously published description")
      assertTrue(store.publishSignal(worker.token, previousModel).created)

      val publication = management.call("edict_publish_validated_pr_signals", buildJsonObject {
        put("token", worker.token)
        put("batchId", batchId)
      })
      assertFalse(publication.flag("isError") == true)
      val receipt = publication.obj("structuredContent")
      assertEquals(signals.map(EdictNextSignal::id), receipt.getValue("signalIds").jsonArray.map { it.jsonPrimitive.content })
      assertEquals(signals.drop(1).map(EdictNextSignal::id), receipt.getValue("createdSignalIds").jsonArray.map { it.jsonPrimitive.content })
      assertFalse(receipt.flag("coverageUpdated") == true)
      assertTrue(store.getPrAnalysisCoverage(worker.token, configured).analyzedPrNumbers.isEmpty())
      assertEquals(signals, signals.map { store.inboxSignal(it.id) })

      val retry = management.call("edict_publish_validated_pr_signals", buildJsonObject {
        put("token", worker.token)
        put("batchId", batchId)
      })
      assertFalse(retry.flag("isError") == true)
      assertTrue(retry.obj("structuredContent").getValue("createdSignalIds").jsonArray.isEmpty())

      val directPublication = management.call("edict_publish_signal", buildJsonObject {
        put("token", worker.token)
        put("signal", wireJson.encodeToJsonElement(signals.first()))
      })
      assertTrue(directPublication.flag("isError") == true)

      val failureMessage = failure.getValue("message").jsonPrimitive.content
      val taskResult = "Published ${signals.size} Signals; rejected ${invalid.id}: $failureMessage"
      assertFalse(management.call("edict_task_finish", buildJsonObject {
        put("token", analysis.token)
        put("status", "completed")
        put("result", "Stored chunk 0")
      }).flag("isError") == true)
      val completion = management.call("edict_task_finish", buildJsonObject {
        put("token", worker.token)
        put("status", "completed")
        put("result", taskResult)
      })
      assertFalse(completion.flag("isError") == true)
      assertEquals(
        taskResult,
        checkNotNull(store.plan()).tasks.single { it.skill == "edict-pr-signal-analysis" }.result,
      )
    }
  }

  @Test
  fun `daily extraction returns an empty batch after review history is exhausted`() {
    val configured = ReviewRepository(CiProviderId.GITHUB, "JetBrains", "qodana-cli")
    val provider = DateReviewProvider(emptyList())
    EdictNextRepositoryState.open(directory.resolve("state")).use { store ->
      val plan = store.createPlan(
        "Daily analysis",
        listOf(EdictNextRepositoryState.Step("edict-pr-signal-analysis", "Analyze full UTC dates")),
      )
      val worker = store.launch(plan.token, plan.plan.tasks.single().id, "edict-pr-signal-analysis")
      val layout = EdictLayout(directory, store.root)
      val management = EdictManagementService(
        store,
        layout,
        reviewProvider = provider,
        reviewRepository = configured,
        prAnalysisToday = { LocalDate.of(1970, 1, 1) },
      )
      edictNextToolset(layout, management).createServer()

      val summary = management.call("edict_fetch_pr_batch", buildJsonObject { put("token", worker.token) })
        .obj("structuredContent")
      assertEquals(0, summary.getValue("availableChunkCount").jsonPrimitive.content.toInt())
      assertTrue(provider.selections.isEmpty())

      val batchId = summary.text("batchId")
      val publication = management.call("edict_publish_validated_pr_signals", buildJsonObject {
        put("token", worker.token)
        put("batchId", batchId)
      })
      assertFalse(publication.flag("isError") == true)
      val receipt = publication.obj("structuredContent")
      assertEquals(0, receipt.getValue("selectedPrCount").jsonPrimitive.content.toInt())
      assertTrue(receipt["analyzedDateRanges"]?.jsonArray.orEmpty().isEmpty())
      assertFalse(management.call("edict_task_finish", buildJsonObject {
        put("token", worker.token)
        put("status", "completed")
        put("result", "Review history exhausted without Signals")
      }).flag("isError") == true)
    }
  }

  private fun launchAnalysisWorker(
    store: EdictNextRepositoryState,
    coordinatorToken: String,
  ): EdictNextRepositoryState.Delegation {
    val task = store.addTask(coordinatorToken, "edict-signal-analysis", "Analyze PR chunk")
    return store.launch(coordinatorToken, task.id, "edict-signal-analysis")
  }

  private fun datedPullRequests(count: Int, date: LocalDate): List<PullRequest> = (1..count).map { number ->
    PullRequest(
      number = number,
      url = "https://example.test/review/$number",
      title = "Review $number",
      body = "",
      baseRevision = "a".repeat(40),
      headRevision = "b".repeat(40),
      closeTimestamp = date.atTime(12, number).toInstant(ZoneOffset.UTC).toEpochMilli(),
      threads = listOf(ReviewThread(
        threadId = "thread-$number",
        reviewDiscussionUrl = "https://example.test/review/$number#discussion",
        filePath = "src/Example$number.kt",
        originalCommitSha = "a".repeat(40),
        messages = listOf(ReviewMessage("reviewer", "Fix this", "${date}T12:00:00Z")),
      )),
    )
  }

  private class CapturingReviewProvider(private val pullRequests: List<PullRequest> = emptyList()) : ReviewExtractionApi {
    var selection: ReviewSelection? = null
    override fun fetch(selection: ReviewSelection): ReviewFetchResult {
      this.selection = selection
      return ReviewFetchResult(pullRequests)
    }
    override fun file(repository: ReviewRepository, revision: String, path: String): String = error("unused")
    override fun diff(repository: ReviewRepository, before: String, after: String, beforePath: String, afterPath: String, context: Int): String = error("unused")
  }

  private class DateReviewProvider(
    private val pullRequests: List<PullRequest>,
    private val problems: List<String> = emptyList(),
    private val diff: String? = null,
  ) : ReviewExtractionApi {
    val selections = mutableListOf<ReviewSelection>()

    override fun fetch(selection: ReviewSelection): ReviewFetchResult {
      selections += selection
      return ReviewFetchResult(
        pullRequests.filter { selection.containsDate(it.closeTimestamp) }
          .sortedByDescending(PullRequest::closeTimestamp)
          .take(selection.maxPrs),
        problems,
      )
    }

    override fun file(repository: ReviewRepository, revision: String, path: String): String = error("unused")

    override fun diff(
      repository: ReviewRepository,
      before: String,
      after: String,
      beforePath: String,
      afterPath: String,
      context: Int,
    ): String = diff ?: error("unused")
  }
}
