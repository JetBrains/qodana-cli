package org.jetbrains.qodana.edict.edictnext

import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonArray
import kotlinx.serialization.json.JsonPrimitive
import org.jetbrains.qodana.edict.common.flag
import org.jetbrains.qodana.edict.common.EdictLayout
import org.jetbrains.qodana.edict.ci.CiProviderId
import org.jetbrains.qodana.edict.ci.PullRequest
import org.jetbrains.qodana.edict.ci.ReviewExtractionApi
import org.jetbrains.qodana.edict.ci.ReviewRepository
import org.jetbrains.qodana.edict.ci.ReviewSelection
import org.jetbrains.qodana.edict.support.launch
import org.jetbrains.qodana.edict.support.edictNextToolset
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import kotlin.test.Test
import kotlin.test.assertEquals
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

  private class CapturingReviewProvider : ReviewExtractionApi {
    var selection: ReviewSelection? = null
    override fun fetch(selection: ReviewSelection): List<PullRequest> {
      this.selection = selection
      return emptyList()
    }
    override fun file(repository: ReviewRepository, revision: String, path: String): String = error("unused")
    override fun diff(repository: ReviewRepository, before: String, after: String, beforePath: String, afterPath: String): String = error("unused")
  }
}
