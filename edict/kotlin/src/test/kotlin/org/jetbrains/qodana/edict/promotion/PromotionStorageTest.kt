// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.promotion

import org.jetbrains.qodana.edict.edictnext.EdictNextClusterManifest
import org.jetbrains.qodana.edict.edictnext.EdictNextJson
import org.jetbrains.qodana.edict.edictnext.WeakSignalPromotion
import org.jetbrains.qodana.edict.promotion.support.SOURCE_FIX_ACCEPTED
import org.jetbrains.qodana.edict.promotion.support.activeReviewsText
import org.jetbrains.qodana.edict.promotion.support.expectedText
import org.jetbrains.qodana.edict.promotion.support.promotionRepository
import org.jetbrains.qodana.edict.promotion.support.review
import org.jetbrains.qodana.edict.promotion.support.store
import org.jetbrains.qodana.edict.promotion.support.storeLegacyActiveReview
import org.jetbrains.qodana.edict.promotion.support.weakCluster
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import kotlin.io.path.readText
import kotlin.test.Test
import kotlin.test.assertEquals

internal class PromotionStorageTest {
  @TempDir
  lateinit var directory: Path

  private val repository by lazy { promotionRepository(directory) }

  @Test
  fun `a weak-signal promotion is stored as its review and state`() {
    assertEquals(
      EdictNextJson.parseToJsonElement(expectedText("weak-signal-promotion.json")),
      EdictNextJson.encodeToJsonElement(WeakSignalPromotion.serializer(), SOURCE_FIX_ACCEPTED),
    )
  }

  @Test
  fun `legacy weak-signal reviews in a cluster manifest migrate to the active state`() {
    repository.store(weakCluster())
    repository.storeLegacyActiveReview()

    repository.loadActiveWeakSignalPromotions()

    assertEquals(activeReviewsText(SOURCE_FIX_ACCEPTED), repository.paths.activeWeakSignalPromotionsPath.readText())
  }

  @Test
  fun `migrating legacy weak-signal reviews keeps the rest of the cluster manifest`() {
    val cluster = repository.store(weakCluster())
    repository.storeLegacyActiveReview()

    repository.loadActiveWeakSignalPromotions()

    assertEquals(
      EdictNextJson.encodeToJsonElement(EdictNextClusterManifest.serializer(), cluster.manifest),
      EdictNextJson.parseToJsonElement(cluster.directory.manifestPath.readText()),
    )
  }
}
