// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.promotion

import org.jetbrains.qodana.edict.edictnext.EdictNextClusterStatus
import org.jetbrains.qodana.edict.promotion.support.UnreferencedSource
import org.jetbrains.qodana.edict.promotion.support.clusterWithSource
import org.jetbrains.qodana.edict.promotion.support.fourFilesWithWeakAndNegativeSignals
import org.jetbrains.qodana.edict.promotion.support.oneFileAtEightRevisions
import org.jetbrains.qodana.edict.promotion.support.promotionRepository
import org.jetbrains.qodana.edict.promotion.support.referencedCluster
import org.jetbrains.qodana.edict.promotion.support.sameNameInFiveDirectories
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
      cluster.promotionEligibility(),
    )
  }

  @Test
  fun `a cluster that is not generated is not eligible`() {
    val cluster = repository.store(referencedCluster().copy(status = EdictNextClusterStatus.Pending))

    assertEquals(
      PromotionEligibility(generated = false, strongPositiveSourceFiles = 5, referencedSourceFiles = 5),
      cluster.promotionEligibility(),
    )
  }

  @Test
  fun `signals of one file at several revisions and with both labels count as one source file`() {
    val cluster = repository.store(oneFileAtEightRevisions())

    assertEquals(
      PromotionEligibility(generated = true, strongPositiveSourceFiles = 1, referencedSourceFiles = 1),
      cluster.promotionEligibility(),
    )
  }

  @Test
  fun `equal file names in different directories are different source files`() {
    val cluster = repository.store(sameNameInFiveDirectories())

    assertEquals(
      PromotionEligibility(generated = true, strongPositiveSourceFiles = 5, referencedSourceFiles = 5),
      cluster.promotionEligibility(),
    )
  }

  @Test
  fun `weak and negative signals do not count`() {
    val cluster = repository.store(fourFilesWithWeakAndNegativeSignals())

    assertEquals(
      PromotionEligibility(generated = true, strongPositiveSourceFiles = 4, referencedSourceFiles = 4),
      cluster.promotionEligibility(),
    )
  }

  @ParameterizedTest
  @EnumSource(UnreferencedSource::class)
  fun `a signal source without a valid PR or commit diff URL counts as observed but not as referenced`(source: UnreferencedSource) {
    val cluster = repository.store(clusterWithSource(source.source))

    assertEquals(
      PromotionEligibility(generated = true, strongPositiveSourceFiles = 5, referencedSourceFiles = 4),
      cluster.promotionEligibility(),
    )
  }
}
