// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.promotion

import org.jetbrains.qodana.edict.promotion.support.INSPECTION_DIGEST
import org.jetbrains.qodana.edict.promotion.support.describedCluster
import org.jetbrains.qodana.edict.promotion.support.expectedText
import org.jetbrains.qodana.edict.promotion.support.markdownCluster
import org.jetbrains.qodana.edict.promotion.support.promotionRepository
import org.jetbrains.qodana.edict.promotion.support.rangedCluster
import org.jetbrains.qodana.edict.promotion.support.referencedCluster
import org.jetbrains.qodana.edict.promotion.support.sharedFileCluster
import org.jetbrains.qodana.edict.promotion.support.store
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import kotlin.test.Test
import kotlin.test.assertEquals

internal class PromotionDescriptionTest {
  @TempDir
  lateinit var directory: Path

  private val repository by lazy { promotionRepository(directory) }

  @Test
  fun `the description cites only referenced strong positive signals`() {
    val cluster = repository.store(describedCluster())

    assertEquals(expectedText("referenced-evidence-description.md"), promotionDescription(cluster, TARGET_PATH, INSPECTION_DIGEST))
  }

  @Test
  fun `the description cites at most ten source files`() {
    val cluster = repository.store(referencedCluster(files = 14))

    assertEquals(expectedText("ten-of-fourteen-description.md"), promotionDescription(cluster, TARGET_PATH, INSPECTION_DIGEST))
  }

  @Test
  fun `the description escapes Markdown in signal descriptions and PR titles`() {
    val cluster = repository.store(markdownCluster())

    assertEquals(expectedText("markdown-description.md"), promotionDescription(cluster, TARGET_PATH, INSPECTION_DIGEST))
  }

  @Test
  fun `the description cites each source file by its first referenced strong positive signal, whatever their order`() {
    val cluster = repository.store(sharedFileCluster())

    assertEquals(
      expectedText("one-per-file-description.md"),
      promotionDescription(cluster.copy(signals = cluster.signals.reversed()), TARGET_PATH, INSPECTION_DIGEST),
    )
  }

  @Test
  fun `the description names a signal's lines and ranges`() {
    val cluster = repository.store(rangedCluster())

    assertEquals(expectedText("line-ranges-description.md"), promotionDescription(cluster, TARGET_PATH, INSPECTION_DIGEST))
  }
}

/** Where cluster `rule` is promoted to. */
private const val TARGET_PATH = "inspections/rule.inspection.kts"
