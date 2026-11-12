// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.promotion

import org.jetbrains.qodana.edict.common.DEFAULT_MIN_PROMOTION_SOURCE_FILES
import org.jetbrains.qodana.edict.ci.ReviewRepository
import org.jetbrains.qodana.edict.ci.support.ULTIMATE
import org.jetbrains.qodana.edict.core.EdictFileRevision
import org.jetbrains.qodana.edict.core.EdictLineRange
import org.jetbrains.qodana.edict.core.EdictProjectReview
import org.jetbrains.qodana.edict.core.EdictSignalSource
import org.jetbrains.qodana.edict.core.EdictStoredCluster
import org.jetbrains.qodana.edict.promotion.support.COMMIT_REVISION
import org.jetbrains.qodana.edict.promotion.support.EvaluationFixture
import org.jetbrains.qodana.edict.promotion.support.INSPECTION_DIGEST
import org.jetbrains.qodana.edict.promotion.support.PROMOTION_REPOSITORY
import org.jetbrains.qodana.edict.promotion.support.describedCluster
import org.jetbrains.qodana.edict.promotion.support.expectedText
import org.jetbrains.qodana.edict.promotion.support.markdownCluster
import org.jetbrains.qodana.edict.promotion.support.promotionRepository
import org.jetbrains.qodana.edict.promotion.support.referencedCluster
import org.jetbrains.qodana.edict.promotion.support.sharedFileCluster
import org.jetbrains.qodana.edict.promotion.support.signalId
import org.jetbrains.qodana.edict.promotion.support.store
import org.jetbrains.qodana.edict.promotion.support.withSources
import org.junit.jupiter.api.io.TempDir
import org.junit.jupiter.params.ParameterizedTest
import org.junit.jupiter.params.provider.CsvSource
import java.nio.file.Path
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

internal class PromotionDescriptionTest {
  @TempDir
  lateinit var directory: Path

  private val repository by lazy { promotionRepository(directory) }

  @Test
  fun `the description cites only satisfied, referenced strong positive signals`() {
    val cluster = repository.store(describedCluster())

    // Five signals; the first has a custom description, the fifth cites a commit by its short revision.
    assertEquals(expectedText("referenced-evidence-description.md"), cluster.description())
  }

  @Test
  fun `the description cites at most five source files`() {
    val cluster = repository.store(referencedCluster(files = 14))

    // The first five of 14 distinct source files, by signal id.
    assertEquals(expectedText("rule-description.md"), cluster.description())
  }

  @Test
  fun `the description escapes Markdown in signal descriptions`() {
    val cluster = repository.store(markdownCluster())

    // Markdown specials escaped in the first signal's description, which spans two lines.
    assertEquals(expectedText("markdown-description.md"), cluster.description())
  }

  @Test
  fun `the description cites each source file by its first referenced strong positive signal, whatever their order`() {
    val cluster = repository.store(sharedFileCluster())

    // Signals 02 to 06, whose files run Example2.kt to Example4.kt, then Example0.kt and Example1.kt.
    assertEquals(expectedText("one-per-file-description.md"), promotionDescription(cluster.copy(signals = cluster.signals.reversed())))
  }

  @ParameterizedTest
  @CsvSource(
    "0.8, 0.6, 'Estimated precision: **80.0%** · recall: **60.0%** · F1: **68.6%**'",
    "0.0, 0.0, 'Estimated precision: **0.0%** · recall: **0.0%** · F1: **0.0%**'",
  )
  fun `the description estimates precision, recall and their harmonic mean`(precision: Double, recall: Double, estimate: String) {
    val cluster = repository.store(referencedCluster().copy(evaluation = EvaluationFixture(precision, recall)))

    assertEquals(estimate, cluster.description().lines().single { it.startsWith("Estimated") })
  }

  @ParameterizedTest
  @CsvSource(
    "https://qodana.cloud/reports/latest, 'Last project analysis reported [73 findings](https://qodana.cloud/reports/latest); 20 were reviewed.'",
    "'javascript:alert(1)', 'Last project analysis reported 73 findings; 20 were reviewed.'",
  )
  fun `the description links the last project analysis only by a web report URL`(reportUrl: String, analysis: String) {
    val review = EdictProjectReview("a".repeat(40), 73, 20, 0, 0, 0, reportUrl = reportUrl)
    val cluster = repository.store(referencedCluster().copy(evaluation = EvaluationFixture(projectReview = review)))

    assertEquals(analysis, cluster.description().lines().single { it.startsWith("Last project analysis") })
  }

  @Test
  fun `a commit signal without a URL links the commit in the GitHub CI repository`() {
    val cluster = repository.store(referencedCluster().withSources(EdictSignalSource.FromCommit(COMMIT_REVISION)))

    assertEquals(
      "1. Fixed: \"positive 0\" in [d0d0caf](https://github.com/JetBrains/qodana-cli/commit/$COMMIT_REVISION).",
      cluster.description().lines().first { it.startsWith("1. ") },
    )
  }

  @Test
  fun `a finding users confirmed on GitHub is cited by its review comment`() {
    val comment = "https://github.com/JetBrains/qodana-cli/pull/7#discussion_r11"
    val source = EdictSignalSource.GitHubEvaluatedFinding("Rule", "Reports the rule.", "Breaks the rule", comment, emptyList(), INSPECTION_DIGEST)
    val cluster = repository.store(referencedCluster().withSources(source))

    assertEquals(
      "1. Confirmed: \"positive 0\" in [a review comment]($comment).",
      cluster.description().lines().first { it.startsWith("1. ") },
    )
  }

  @Test
  fun `a commit signal without a URL links the old lines of its commit in the Space CI repository`() {
    val cluster = repository.store(referencedCluster().withSources(EdictSignalSource.FromCommit(COMMIT_REVISION)))

    assertEquals(
      "1. Fixed: \"positive 0\" in [d0d0caf](https://jetbrains.team/p/ij/repositories/ultimate/revision/$COMMIT_REVISION" +
        "?file=Example0.kt&from-line=OLD:3&to-line=OLD:5).",
      promotionDescription(cluster, ULTIMATE).lines().first { it.startsWith("1. ") },
    )
  }

  @Test
  fun `a Space discussion is cited by its review page`() {
    val cluster = repository.store(referencedCluster().withSources(SPACE_REVIEW))

    assertEquals(
      "1. Fixed: \"positive 0\" in [PR #215081](https://jetbrains.team/p/ij/reviews/215081).",
      promotionDescription(cluster, ULTIMATE).lines().first { it.startsWith("1. ") },
    )
  }

  @Test
  fun `a Space discussion links the reviewed file at its revision and encoded path`() {
    val signal = referencedCluster().signals.first().copy(
      source = SPACE_REVIEW,
      fileRevision = EdictFileRevision("src/My File#1.kt", COMMIT_REVISION, listOf(EdictLineRange(13, 15))),
    )

    assertEquals(
      "https://jetbrains.team/p/ij/repositories/ultimate/files/$COMMIT_REVISION/src/My%20File%231.kt?line=13",
      signal.referenceUrl(ULTIMATE),
    )
  }

  @Test
  fun `a signal at a revision that is not a full SHA has no reference`() {
    val signal = referencedCluster().signals.first().let { it.copy(fileRevision = it.fileRevision.copy(revision = "master")) }

    assertEquals(null, signal.referenceUrl(PROMOTION_REPOSITORY))
  }

  @Test
  fun `a description needs an evaluation`() {
    val cluster = repository.store(referencedCluster().copy(evaluation = null))

    val error = assertFailsWith<IllegalArgumentException> { cluster.description() }
    assertEquals("Promotion requires a current evaluation matching the inspection and examples", error.message)
  }

  @Test
  fun `a description needs an evaluation of the current examples`() {
    val cluster = repository.store(referencedCluster().copy(evaluation = EvaluationFixture(exampleSetDigest = "stale")))

    val error = assertFailsWith<IllegalArgumentException> { cluster.description() }
    assertEquals("Promotion requires a current evaluation matching the inspection and examples", error.message)
  }

  @Test
  fun `a description needs five source files whose examples the inspection satisfies`() {
    val cluster = repository.store(referencedCluster().copy(evaluation = EvaluationFixture(missed = setOf(signalId(0)))))

    val error = assertFailsWith<IllegalArgumentException> { cluster.description() }
    assertEquals(
      "Promotion requires strong positive Signals from at least 5 distinct source files with satisfied examples and file/line URLs; found 4",
      error.message,
    )
  }

  /** The description of the stored inspection under the cluster's stored evaluation, in the CI repository [ci]. */
  private fun promotionDescription(cluster: EdictStoredCluster, ci: ReviewRepository = PROMOTION_REPOSITORY) =
    promotionDescription(cluster, INSPECTION_DIGEST, ci, repository.loadEvaluation(cluster), DEFAULT_MIN_PROMOTION_SOURCE_FILES)

  private fun EdictStoredCluster.description(): String = promotionDescription(this)
}

/** A Space review whose Signal links its discussion, as Space's chat does. */
private val SPACE_REVIEW = EdictSignalSource.FromPR(
  215081, "Reuse the run", listOf("Fix documentation"), "",
  "https://jetbrains.team/im/review/4RJJlz4NRWvS?message=F2A2J0W06tf&channel=42GEq23U7k8O",
)
