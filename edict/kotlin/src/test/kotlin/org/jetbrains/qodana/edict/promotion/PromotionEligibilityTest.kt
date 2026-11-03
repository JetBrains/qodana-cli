// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.promotion

import org.jetbrains.qodana.edict.edictnext.EdictNextClusterStatus
import org.jetbrains.qodana.edict.edictnext.EdictNextSignal
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalSource
import org.jetbrains.qodana.edict.edictnext.EdictNextStoredCluster
import org.jetbrains.qodana.edict.promotion.support.COMMIT_REVISION
import org.jetbrains.qodana.edict.promotion.support.EvaluationFixture
import org.jetbrains.qodana.edict.promotion.support.PROMOTION_REPOSITORY
import org.jetbrains.qodana.edict.promotion.support.UnreferencedSource
import org.jetbrains.qodana.edict.promotion.support.clusterWithSource
import org.jetbrains.qodana.edict.promotion.support.fourFilesWithWeakAndNegativeSignals
import org.jetbrains.qodana.edict.promotion.support.oneFileAtEightRevisions
import org.jetbrains.qodana.edict.promotion.support.promotionRepository
import org.jetbrains.qodana.edict.promotion.support.referencedCluster
import org.jetbrains.qodana.edict.promotion.support.sameNameInFiveDirectories
import org.jetbrains.qodana.edict.promotion.support.signalId
import org.jetbrains.qodana.edict.promotion.support.store
import org.junit.jupiter.api.io.TempDir
import org.junit.jupiter.params.ParameterizedTest
import org.junit.jupiter.params.provider.EnumSource
import java.nio.file.Path
import kotlin.test.Test
import kotlin.test.assertEquals

internal class PromotionEligibilityTest {
  @TempDir
  lateinit var directory: Path

  private val repository by lazy { promotionRepository(directory) }

  @Test
  fun `a generated cluster counts its referenced strong positive source files`() {
    val cluster = repository.store(referencedCluster())

    assertEquals(
      PromotionEligibility(generated = true, strongPositiveSourceFiles = 5, referencedSourceFiles = 5),
      cluster.eligibility(),
    )
  }

  @Test
  fun `a cluster that is not generated is not eligible`() {
    val cluster = repository.store(referencedCluster().copy(status = EdictNextClusterStatus.Pending))

    assertEquals(
      PromotionEligibility(generated = false, strongPositiveSourceFiles = 5, referencedSourceFiles = 5),
      cluster.eligibility(),
    )
  }

  @Test
  fun `signals of one file at several revisions and with both labels count as one source file`() {
    val cluster = repository.store(oneFileAtEightRevisions())

    assertEquals(
      PromotionEligibility(generated = true, strongPositiveSourceFiles = 1, referencedSourceFiles = 1),
      cluster.eligibility(),
    )
  }

  @Test
  fun `equal file names in different directories are different source files`() {
    val cluster = repository.store(sameNameInFiveDirectories())

    assertEquals(
      PromotionEligibility(generated = true, strongPositiveSourceFiles = 5, referencedSourceFiles = 5),
      cluster.eligibility(),
    )
  }

  @Test
  fun `weak and negative signals do not count`() {
    val cluster = repository.store(fourFilesWithWeakAndNegativeSignals())

    assertEquals(
      PromotionEligibility(generated = true, strongPositiveSourceFiles = 4, referencedSourceFiles = 4),
      cluster.eligibility(),
    )
  }

  @Test
  fun `a commit signal without a URL is referenced in the CI repository`() {
    val cluster = repository.store(clusterWithSource(EdictNextSignalSource.FromCommit(COMMIT_REVISION)))

    assertEquals(
      PromotionEligibility(generated = true, strongPositiveSourceFiles = 5, referencedSourceFiles = 5),
      cluster.eligibility(),
    )
  }

  @Test
  fun `a signal whose example the inspection misses counts as observed but not as referenced`() {
    val cluster = repository.store(referencedCluster().copy(evaluation = EvaluationFixture(missed = setOf(signalId(0)))))

    assertEquals(
      PromotionEligibility(generated = true, strongPositiveSourceFiles = 5, referencedSourceFiles = 4),
      cluster.eligibility(),
    )
  }

  @Test
  fun `without an evaluation no signal is referenced`() {
    val cluster = repository.store(referencedCluster().copy(evaluation = null))

    assertEquals(
      PromotionEligibility(generated = true, strongPositiveSourceFiles = 5, referencedSourceFiles = 0),
      cluster.eligibility(),
    )
  }

  @Test
  fun `a signal without lines counts as observed but not as referenced`() {
    val cluster = referencedCluster().let { it.copy(signals = it.signals.dropLast(1) + it.signals.last().withoutLines()) }

    assertEquals(
      PromotionEligibility(generated = true, strongPositiveSourceFiles = 5, referencedSourceFiles = 4),
      repository.store(cluster).eligibility(),
    )
  }

  @ParameterizedTest
  @EnumSource(UnreferencedSource::class)
  fun `a signal source without a valid PR or commit diff URL counts as observed but not as referenced`(source: UnreferencedSource) {
    val cluster = repository.store(clusterWithSource(source.source))

    assertEquals(
      PromotionEligibility(generated = true, strongPositiveSourceFiles = 5, referencedSourceFiles = 4),
      cluster.eligibility(),
    )
  }

  /** Eligibility under the cluster's stored evaluation, with commit Signals linked in [PROMOTION_REPOSITORY]. */
  private fun EdictNextStoredCluster.eligibility(): PromotionEligibility =
    promotionEligibility(PROMOTION_REPOSITORY, repository.loadEvaluation(this))
}

private fun EdictNextSignal.withoutLines(): EdictNextSignal = copy(fileRevision = fileRevision.copy(expectedRanges = null))
