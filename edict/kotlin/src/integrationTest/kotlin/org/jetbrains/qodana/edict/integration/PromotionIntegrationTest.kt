package org.jetbrains.qodana.edict.integration

import kotlinx.serialization.encodeToString
import org.jetbrains.qodana.edict.ci.CiProviderId
import org.jetbrains.qodana.edict.ci.ReviewRepository
import org.jetbrains.qodana.edict.support.FakeReviewApi
import org.jetbrains.qodana.edict.common.EdictConfiguration
import org.jetbrains.qodana.edict.common.PromotionConfiguration
import org.jetbrains.qodana.edict.edictnext.EdictNextClusterManifest
import org.jetbrains.qodana.edict.edictnext.EdictNextClusterStatus
import org.jetbrains.qodana.edict.edictnext.EdictNextFileRevision
import org.jetbrains.qodana.edict.edictnext.EdictNextExampleEvaluation
import org.jetbrains.qodana.edict.edictnext.EdictNextLineRange
import org.jetbrains.qodana.edict.edictnext.EdictNextEvaluation
import org.jetbrains.qodana.edict.edictnext.EdictNextInspectionAction
import org.jetbrains.qodana.edict.edictnext.exampleSetDigest
import org.jetbrains.qodana.edict.edictnext.sha256Hex
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
import kotlin.test.assertContains
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

class PromotionIntegrationTest : IntegrationTest() {
  @Test
  fun `managed promotion creates a PR and persists an on-review version`() {
    workspace.state.createDirectories()
    val repository = EdictRepository(EdictRepositoryDirectory(workspace.state))
    createGeneratedCluster(repository)
    val provider = FakeReviewApi(ReviewRepository(CiProviderId.GITHUB, "github.test", "JetBrains", "target"))

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
      repeat(5) { index -> assertContains(description, "https://github.test/source/project/pull/${index + 1}") }
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
    cluster.resolve("cluster.json").writeText(EdictNextJson.encodeToString(
      EdictNextClusterManifest("integration-rule", EdictNextLanguage.Kotlin, EdictNextClusterStatus.Generated),
    ))
    cluster.resolve("history.md").writeText("Created.\n")
    repeat(6) { index ->
      val signal = EdictNextSignal(
        "s-$index",
        fileRevision = EdictNextFileRevision("Example$index.kt", "a".repeat(40), listOf(EdictNextLineRange(1, 1))),
        source = EdictNextSignalSource.FromPR(
          index + 1, "Correction $index", listOf("Fix the problem"), "", "https://github.test/source/project/pull/${index + 1}",
        ),
        label = EdictNextSignalLabel.POSITIVE,
        description = "positive $index",
        syntheticExampleId = "example-$index",
      )
      cluster.resolve("signals/s-$index.json").writeText(EdictNextJson.encodeToString(signal))
      val duplicate = signal.copy(id = "z-duplicate-$index", fileRevision = signal.fileRevision.copy(revision = "another-revision"))
      cluster.resolve("signals/${duplicate.id}.json").writeText(EdictNextJson.encodeToString(duplicate))
    }
    repository.paths.inspectionsDirectory.createDirectories()
    repository.paths.inspectionPath("integration-rule").writeText("inspection")
    repository.loadCluster("integration-rule").directory.evaluationPath.writeText(EdictNextJson.encodeToString(EdictNextEvaluation(
      sha256Hex("inspection"), exampleSetDigest(repository.loadCluster("integration-rule")),
      EdictNextInspectionAction.GENERATE, 6, 0, 0, 1.0, 1.0, (0..5).associate { "example-$it" to EdictNextExampleEvaluation(EdictNextSignalLabel.POSITIVE, true, true, 0, 1) }, null, "Integration rule", "Detects integration problems.",
    )))
  }
}
