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

    // Five signals; the first has a custom description, the fifth cites a commit diff with lines 12-15.
    assertEquals(expectedText("referenced-evidence-description.md"), promotionDescription(cluster, TARGET_PATH, INSPECTION_DIGEST))
  }

  @Test
  fun `the description cites at most ten source files`() {
    val cluster = repository.store(referencedCluster(files = 14))

    // Ten signals listed out of 14 distinct source files.
    assertEquals(expectedText("ten-of-fourteen-description.md"), promotionDescription(cluster, TARGET_PATH, INSPECTION_DIGEST))
  }

  @Test
  fun `the description escapes Markdown in signal descriptions and PR titles`() {
    val cluster = repository.store(markdownCluster())

    // Markdown specials escaped: in the first signal description and in the second pull request title.
    assertEquals(expectedText("markdown-description.md"), promotionDescription(cluster, TARGET_PATH, INSPECTION_DIGEST))
  }

  @Test
  fun `the description cites each source file by its first referenced strong positive signal, whatever their order`() {
    val cluster = repository.store(sharedFileCluster())

    // Signals 02 to 06, whose files run Example2.kt to Example4.kt, then Example0.kt and Example1.kt.
    assertEquals(
      expectedText("one-per-file-description.md"),
      promotionDescription(cluster.copy(signals = cluster.signals.reversed()), TARGET_PATH, INSPECTION_DIGEST),
    )
  }

  @Test
  fun `the description names a signal's lines and ranges`() {
    val cluster = repository.store(rangedCluster())

    // Evidence line ranges: "3" for the first signal, "7-9, 12" for the second, none for the others.
    assertEquals(expectedText("line-ranges-description.md"), promotionDescription(cluster, TARGET_PATH, INSPECTION_DIGEST))
  }
}

/** Where cluster `rule` is promoted to. */
private const val TARGET_PATH = "inspections/rule.inspection.kts"
