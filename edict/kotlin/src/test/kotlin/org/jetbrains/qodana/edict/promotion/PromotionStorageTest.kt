// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.promotion

import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject
import org.jetbrains.qodana.edict.common.runProcess
import org.jetbrains.qodana.edict.core.EdictJson
import org.jetbrains.qodana.edict.core.validateRepositoryState
import org.jetbrains.qodana.edict.promotion.support.INSPECTION_DIGEST
import org.jetbrains.qodana.edict.promotion.support.PROMOTED_RULE
import org.jetbrains.qodana.edict.promotion.support.expectedText
import org.jetbrains.qodana.edict.promotion.support.promotionRepository
import org.jetbrains.qodana.edict.promotion.support.referencedCluster
import org.jetbrains.qodana.edict.promotion.support.store
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import kotlin.io.path.createDirectories
import kotlin.io.path.readText
import kotlin.io.path.writeText
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

internal class PromotionStorageTest {
  @TempDir
  lateinit var directory: Path

  private val repository by lazy { promotionRepository(directory) }

  @Test
  fun `legacy weak-signal reviews survive a cluster manifest update`() {
    val cluster = repository.store(referencedCluster())
    val path = cluster.directory.manifestPath
    val manifest = EdictJson.parseToJsonElement(path.readText()).jsonObject
    val records = JsonArray(listOf(EdictJson.parseToJsonElement(expectedText("legacy-weak-signal-promotion.json"))))
    path.writeText(EdictJson.encodeToString(JsonObject(manifest + ("weakSignalPromotions" to records))))

    repository.appendPromotion(cluster.id, INSPECTION_DIGEST, PROMOTED_RULE)

    val updated = repository.loadCluster(cluster.id).manifest
    assertEquals(listOf(PROMOTED_RULE), updated.promotions)
    assertEquals(records, updated.weakSignalPromotions)
  }

  @Test
  fun `state validation ignores legacy promotion files`() = runBlocking {
    val root = repository.paths.root
    runProcess(root, listOf("git", "init", "--quiet"))
    val record = expectedText("legacy-weak-signal-promotion.json")
    val active = root.resolve("promotions/active.json")
    val history = root.resolve("promotions/history/retired-cluster.json")
    val activeText = """{"activePromotions":{"retired-cluster":[$record]}}"""
    val historyText = "[$record]"
    history.parent.createDirectories()
    active.writeText(activeText)
    history.writeText(historyText)

    val state = repository.loadState()

    assertTrue(validateRepositoryState(repository, state).isEmpty())
    assertEquals(activeText, active.readText())
    assertEquals(historyText, history.readText())
  }
}
