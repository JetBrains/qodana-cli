package org.jetbrains.qodana.edict.integration

import kotlinx.serialization.encodeToString
import org.jetbrains.qodana.edict.ci.CiProviderId
import org.jetbrains.qodana.edict.ci.ReviewRepository
import org.jetbrains.qodana.edict.support.FakeReviewApi
import org.jetbrains.qodana.edict.common.EdictConfiguration
import org.jetbrains.qodana.edict.common.PromotionConfiguration
import org.jetbrains.qodana.edict.core.EdictClusterManifest
import org.jetbrains.qodana.edict.core.EdictClusterStatus
import org.jetbrains.qodana.edict.core.EdictFileRevision
import org.jetbrains.qodana.edict.core.EdictExampleEvaluation
import org.jetbrains.qodana.edict.core.EdictLineRange
import org.jetbrains.qodana.edict.core.EdictEvaluation
import org.jetbrains.qodana.edict.core.exampleSetDigest
import org.jetbrains.qodana.edict.core.sha256Hex
import org.jetbrains.qodana.edict.core.EdictJson
import org.jetbrains.qodana.edict.core.EdictLanguage
import org.jetbrains.qodana.edict.core.EdictSignal
import org.jetbrains.qodana.edict.core.EdictSignalLabel
import org.jetbrains.qodana.edict.core.EdictSignalSource
import org.jetbrains.qodana.edict.core.EdictRepository
import org.jetbrains.qodana.edict.core.EdictRepositoryDirectory
import org.jetbrains.qodana.edict.core.PromotionPrState
import org.jetbrains.qodana.edict.integration.support.IntegrationTest
import org.jetbrains.qodana.edict.integration.support.verifyManagedRun
import org.junit.jupiter.api.Test
import kotlin.io.path.createDirectories
import kotlin.io.path.writeText
import kotlin.test.assertEquals
import kotlin.test.assertContains
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

class PromotionIntegrationTest : IntegrationTest() {
  @Test
  fun `managed promotion creates a PR and persists an on-review version`() {
    workspace.state.createDirectories()
    val repository = EdictRepository(EdictRepositoryDirectory(workspace.state))
    createGeneratedCluster(repository)
    val provider = FakeReviewApi(ReviewRepository(CiProviderId.GITHUB, "github.com", "JetBrains", "target"))

    workspace.withPromotionCodex(
      prompt = "Promote all generated inspections",
      configuration = EdictConfiguration(
        ci = ReviewRepository(CiProviderId.GITHUB, "github.com", "JetBrains", "target"),
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
      val request = provider.requests.single()
      assertEquals("inspections/integration-rule.inspection.kts", request.targetPath)
      val description = request.description
      assertContains(description, "## Description")
      assertContains(description, "## Evidence")
      repeat(5) { index -> assertContains(description, "https://github.com/source/project/pull/${index + 1}") }
      assertEquals(5, Regex("(?m)^\\d+\\. ").findAll(description).count())
      assertTrue("z-duplicate" !in description)
      val stored = repository.loadCluster("integration-rule").manifest.promotions.single()
      assertEquals(listOf(stored.pullRequest), provider.opened.values.toList())
      assertEquals(PromotionPrState.ON_REVIEW, stored.state)
      verifyManagedRun(workspace, runtime, plan)
    }
  }

  private fun createGeneratedCluster(repository: EdictRepository) {
    val cluster = repository.paths.clustersDirectory.resolve("integration-rule").createDirectories()
    cluster.resolve("signals").createDirectories()
    cluster.resolve("cluster.json").writeText(EdictJson.encodeToString(
      EdictClusterManifest("integration-rule", EdictLanguage.Kotlin, EdictClusterStatus.Generated),
    ))
    cluster.resolve("history.md").writeText("Created.\n")
    repeat(6) { index ->
      val signal = EdictSignal(
        "s-$index",
        fileRevision = EdictFileRevision("Example$index.kt", "a".repeat(40), listOf(EdictLineRange(1, 1))),
        source = EdictSignalSource.FromPR(
          index + 1, "Correction $index", listOf("Fix the problem"), "", "https://github.com/source/project/pull/${index + 1}",
        ),
        label = EdictSignalLabel.POSITIVE,
        description = "positive $index",
        syntheticExampleId = "example-$index",
      )
      cluster.resolve("signals/s-$index.json").writeText(EdictJson.encodeToString(signal))
      val duplicate = signal.copy(id = "z-duplicate-$index", fileRevision = signal.fileRevision.copy(revision = "another-revision"))
      cluster.resolve("signals/${duplicate.id}.json").writeText(EdictJson.encodeToString(duplicate))
    }
    repository.paths.inspectionsDirectory.createDirectories()
    repository.paths.inspectionPath("integration-rule").writeText("inspection")
    repository.loadCluster("integration-rule").directory.evaluationPath.writeText(EdictJson.encodeToString(EdictEvaluation(
      sha256Hex("inspection"), exampleSetDigest(repository.loadCluster("integration-rule")),
      (0..5).associate { "example-$it" to EdictExampleEvaluation(EdictSignalLabel.POSITIVE, true, 0) }, null, "Integration rule", "Detects integration problems.",
    )))
  }
}
