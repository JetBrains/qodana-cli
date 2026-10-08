package org.jetbrains.qodana.edict.edictnext

import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.encodeToJsonElement
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonArray
import kotlinx.serialization.json.JsonPrimitive
import org.jetbrains.qodana.edict.common.array
import org.jetbrains.qodana.edict.common.flag
import org.jetbrains.qodana.edict.common.EdictLayout
import org.jetbrains.qodana.edict.common.obj
import org.jetbrains.qodana.edict.common.text
import org.jetbrains.qodana.edict.common.wireJson
import org.jetbrains.qodana.edict.ci.CiProviderId
import org.jetbrains.qodana.edict.ci.PullRequest
import org.jetbrains.qodana.edict.ci.ReviewExtractionApi
import org.jetbrains.qodana.edict.ci.ReviewMessage
import org.jetbrains.qodana.edict.ci.ReviewRepository
import org.jetbrains.qodana.edict.ci.ReviewSelection
import org.jetbrains.qodana.edict.ci.ReviewThread
import org.jetbrains.qodana.edict.support.launch
import org.jetbrains.qodana.edict.support.edictNextToolset
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotEquals
import kotlin.test.assertTrue

class ConfiguredPrAnalysisTest {
  @TempDir
  lateinit var directory: Path

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
        put("maxPrs", 1)
        putJsonArray("prNumbers") { add(JsonPrimitive(7)) }
      })
      assertNotEquals(result.flag("isError"), true)
      assertEquals(configured, provider.selection?.repositoryRef)

      val override = management.call("edict_fetch_pr_batch", buildJsonObject {
        put("token", worker.token)
        put("provider", "space")
        put("owner", "attacker")
        put("repo", "other")
        put("maxPrs", 1)
        putJsonArray("prNumbers") { add(JsonPrimitive(7)) }
      })
      assertTrue(override.flag("isError") == true)
      assertEquals(configured, provider.selection?.repositoryRef)
    }
  }

  @Test
  fun `validated PR batch publishes cached models without resending them`() {
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
        anchorLine = 1,
        anchorEndLine = 1,
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
        put("maxPrs", 1)
        putJsonArray("prNumbers") { add(JsonPrimitive(7)) }
      })
      assertFalse(fetched.flag("isError") == true)
      val batchId = fetched.obj("structuredContent").text("batchId")
      val listed = management.call("edict_list_pr_analysis_items", buildJsonObject {
        put("token", worker.token)
        put("batchId", batchId)
        put("offset", 0)
        put("limit", 20)
      })
      val workItemId = listed.obj("structuredContent").array("items").single().text("workItemId")
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
      val validation = management.call("edict_validate_pr_signals", buildJsonObject {
        put("token", worker.token)
        put("batchId", batchId)
        put("inspectedWorkItemIds", JsonArray(listOf(JsonPrimitive(workItemId))))
        put("signals", wireJson.encodeToJsonElement(signals))
      })
      assertFalse(validation.flag("isError") == true)

      val publication = management.call("edict_publish_validated_pr_signals", buildJsonObject {
        put("token", worker.token)
        put("batchId", batchId)
      })
      assertFalse(publication.flag("isError") == true)
      val receipt = publication.obj("structuredContent")
      assertEquals(signals.map(EdictNextSignal::id), receipt.getValue("signalIds").jsonArray.map { it.jsonPrimitive.content })
      assertEquals(signals.map(EdictNextSignal::id), receipt.getValue("createdSignalIds").jsonArray.map { it.jsonPrimitive.content })
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
    }
  }

  private class CapturingReviewProvider(private val pullRequests: List<PullRequest> = emptyList()) : ReviewExtractionApi {
    var selection: ReviewSelection? = null
    override fun fetch(selection: ReviewSelection): List<PullRequest> {
      this.selection = selection
      return pullRequests
    }
    override fun file(repository: ReviewRepository, revision: String, path: String): String = error("unused")
    override fun diff(repository: ReviewRepository, before: String, after: String, beforePath: String, afterPath: String): String = error("unused")
  }
}
