package org.jetbrains.qodana.edict.integration

import kotlinx.serialization.encodeToString
import org.jetbrains.qodana.edict.ci.CreateReviewRequest
import org.jetbrains.qodana.edict.ci.CreatedReview
import org.jetbrains.qodana.edict.ci.ReviewManagementApi
import org.jetbrains.qodana.edict.ci.ReviewReference
import org.jetbrains.qodana.edict.ci.ReviewRepository
import org.jetbrains.qodana.edict.ci.ReviewState
import org.jetbrains.qodana.edict.ci.ReviewStatus
import org.jetbrains.qodana.edict.common.EdictCIConfiguration
import org.jetbrains.qodana.edict.common.EdictConfiguration
import org.jetbrains.qodana.edict.common.PromotionConfiguration
import org.jetbrains.qodana.edict.edictnext.EdictNextClusterManifest
import org.jetbrains.qodana.edict.edictnext.EdictNextClusterStatus
import org.jetbrains.qodana.edict.edictnext.EdictNextFileRevision
import org.jetbrains.qodana.edict.edictnext.EdictNextJson
import org.jetbrains.qodana.edict.edictnext.EdictNextLanguage
import org.jetbrains.qodana.edict.edictnext.EdictNextSignal
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalLabel
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalSource
import org.jetbrains.qodana.edict.edictnext.EdictRepository
import org.jetbrains.qodana.edict.edictnext.EdictRepositoryDirectory
import org.jetbrains.qodana.edict.edictnext.PromotionPrState
import org.jetbrains.qodana.edict.integration.support.IntegrationTest
import org.jetbrains.qodana.edict.integration.support.verifyManagedRun
import org.junit.jupiter.api.Test
import kotlin.io.path.createDirectories
import kotlin.io.path.writeText
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

class PromotionIntegrationTest : IntegrationTest() {
  @Test
  fun `managed promotion creates a PR and persists an on-review version`() {
    workspace.state.createDirectories()
    val repository = EdictRepository(EdictRepositoryDirectory(workspace.state))
    createGeneratedCluster(repository)
    val provider = IntegrationPromotionProvider()

    workspace.withPromotionCodex(
      prompt = "Promote all generated inspections",
      configuration = EdictConfiguration(
        ci = EdictCIConfiguration("https://github.com/JetBrains/target"),
        promotion = PromotionConfiguration("reviewer"),
      ),
      reviews = provider,
    ) { store, runtime, result ->
      val plan = assertNotNull(store.plan())
      assertEquals(1, plan.tasks.size)
      val promotion = plan.tasks.single()
      assertEquals("edict-promote", promotion.skill)
      assertEquals("completed", promotion.status)
      assertTrue(result.isNotBlank())
      assertEquals("inspections/integration-rule.inspection.kts", provider.request?.targetPath)
      val stored = repository.loadCluster("integration-rule").manifest.promotions.single()
      assertEquals("42", stored.pullRequest.id)
      assertEquals(PromotionPrState.ON_REVIEW, stored.state)
      verifyManagedRun(workspace, runtime, plan)
    }
  }

  private fun createGeneratedCluster(repository: EdictRepository) {
    val cluster = repository.paths.clustersDirectory.resolve("integration-rule").createDirectories()
    cluster.resolve("signals").createDirectories()
    cluster.resolve("cluster.json").writeText(EdictNextJson.encodeToString(
      EdictNextClusterManifest("integration-rule", EdictNextLanguage.Kotlin, EdictNextClusterStatus.Generated),
    ))
    cluster.resolve("history.md").writeText("Created.\n")
    repeat(6) { index ->
      cluster.resolve("signals/s-$index.json").writeText(EdictNextJson.encodeToString(
        EdictNextSignal(
          "s-$index",
          fileRevision = EdictNextFileRevision("Example.kt", "revision"),
          source = EdictNextSignalSource.Generated(),
          label = EdictNextSignalLabel.POSITIVE,
          description = "positive $index",
        ),
      ))
    }
    repository.paths.inspectionsDirectory.createDirectories()
    repository.paths.inspectionPath("integration-rule").writeText("inspection")
  }

  private class IntegrationPromotionProvider : ReviewManagementApi {
    var request: CreateReviewRequest? = null
    override fun resolveTargetBranch(repository: ReviewRepository, configured: String?): String = configured ?: "main"
    override fun ensureReview(request: CreateReviewRequest): CreatedReview {
      this.request = request
      return CreatedReview("42", "https://github.test/pr/42")
    }
    override fun review(reference: ReviewReference) = ReviewStatus(ReviewState.OPEN)
  }
}
