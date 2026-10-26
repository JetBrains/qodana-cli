package org.jetbrains.qodana.edict.edictnext

import kotlinx.coroutines.runBlocking
import org.jetbrains.qodana.edict.common.runProcess
import org.junit.jupiter.api.io.TempDir
import java.nio.ByteBuffer
import java.nio.ByteOrder
import java.nio.file.Files
import java.nio.file.Path
import kotlin.io.path.createDirectories
import kotlin.io.path.exists
import kotlin.io.path.readText
import kotlin.io.path.writeText
import kotlin.test.Test
import kotlin.test.assertContains
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

class EdictNextDistributionServiceTest {
  @TempDir
  lateinit var directory: Path

  @Test
  fun `default distribution batch can exceed the default extraction target`() = runBlocking {
    val repository = inboxRepository(101)
    val distribution = EdictNextDistributionService(repository, directory.resolve("neighbours.json"))
    assertDistributedCount(repository, distribution, expectedCount = 101)
  }

  @Test
  fun `configured distribution limit controls the batch size`() = runBlocking {
    for (expected in listOf(1, 2, 3)) {
      val repository = inboxRepository(4, "state-$expected")
      val distribution = EdictNextDistributionService(repository, directory.resolve("neighbours-$expected.json"), expected)
      assertDistributedCount(repository, distribution, expectedCount = expected)
    }
  }

  @Test
  fun `each task assigns its share of the batch and hands off to the next task`() = runBlocking {
    val repository = inboxRepository(5)
    val distribution = EdictNextDistributionService(repository, directory.resolve("neighbours.json"), taskSignalCount = 2)
    distribution.calculateNeighbours()
    val actions = mutableListOf<String>()
    while (true) {
      val prepared = distribution.nextSignal()
      if (prepared.nextAction == EdictNextNextAction.STOP_DISTRIBUTION) break
      if (prepared.nextAction == EdictNextNextAction.HAND_OFF) {
        actions += "hand off: ${prepared.summary}"
        assertContains(distribution.calculateNeighbours().summary, "already calculated")
        continue
      }
      distribution.addSignalToCluster(assertNotNull(prepared.signalId), "rule-${actions.size}")
      actions += "assign"
    }

    assertEquals(
      listOf("assign", "assign", "hand off", "assign", "assign", "hand off", "assign"),
      actions.map { it.substringBefore(":") },
    )
    assertContains(actions[2], "3 selected Signal(s) remain")
    assertTrue(repository.loadState().inboxSignals.isEmpty())
  }

  @Test
  fun `nearest small clusters come with their members and count as loaded context`() = runBlocking {
    val repository = inboxRepository(40)
    val distribution = EdictNextDistributionService(repository, directory.resolve("neighbours.json"), taskSignalCount = 40)
    distribution.calculateNeighbours()
    val first = distribution.nextSignal()
    val inboxCandidate = first.candidates.filterIsInstance<EdictNextSignalCandidate.SignalCandidate>().first()
    assertEquals("Same inspection evidence", inboxCandidate.description)
    distribution.addSignalToCluster(assertNotNull(first.signalId), "rule")
    var included = 0
    while (true) {
      val prepared = distribution.nextSignal()
      val cluster = prepared.candidates.filterIsInstance<EdictNextSignalCandidate.Cluster>().single()
      if (cluster.signals == null) {
        // A cluster grown past the size limit needs an explicit context call before joining.
        val unloaded = assertFailsWith<IllegalStateException> { distribution.addSignalToCluster(assertNotNull(prepared.signalId), "rule") }
        assertContains(unloaded.message.orEmpty(), "edict_next_get_distribution_context")
        assertEquals(included + 1, cluster.memberCount)
        break
      }
      assertEquals(cluster.memberCount, cluster.signals.size)
      // Joining an included cluster needs no context call.
      distribution.addSignalToCluster(assertNotNull(prepared.signalId), "rule")
      included++
    }
    assertTrue(included >= 2, "included $included")
  }

  private fun inboxRepository(count: Int, name: String = "state"): EdictRepository {
    val state = directory.resolve(name).createDirectories()
    runProcess(state, listOf("git", "init", "--quiet"))
    val repository = EdictRepository(EdictRepositoryDirectory(state))
    val inbox = repository.paths.inboxDirectory.createDirectories()
    repeat(count) { index ->
      val key = SIGNAL_KEY.replace("pr:42:", "pr:${index + 1}:")
      val id = stableSignalId(key)
      inbox.resolve("$id.json").writeText(EdictNextJson.encodeToString(signal(id, "Same inspection evidence", "batch", key)))
    }
    // Use a cached normalized vector so batch tests do not download or load the embedding model.
    val vector = ByteBuffer.allocate(EdictNextGteEmbedder.DIMENSION * Float.SIZE_BYTES).order(ByteOrder.LITTLE_ENDIAN)
    vector.putFloat(1f)
    Files.write(
      repository.paths.gteEmbeddingCacheDirectory.createDirectories().resolve("${sha256Hex("Same inspection evidence")}.f32"),
      vector.array(),
    )
    return repository
  }

  private suspend fun assertDistributedCount(
    repository: EdictRepository,
    distribution: EdictNextDistributionService,
    expectedCount: Int,
  ) {
    val initialIds = repository.loadState().inboxSignals.map { it.id }.sorted()
    distribution.calculateNeighbours()
    repeat(expectedCount) { index ->
      val prepared = distribution.nextSignal().takeUnless { it.nextAction == EdictNextNextAction.HAND_OFF }
        ?: distribution.nextSignal()
      assertEquals(initialIds[index], prepared.signalId)
      distribution.addSignalToCluster(assertNotNull(prepared.signalId), "rule-$index")
    }
    assertEquals(EdictNextNextAction.STOP_DISTRIBUTION, distribution.nextSignal().nextAction)
    assertEquals(initialIds.drop(expectedCount), repository.loadState().inboxSignals.map { it.id }.sorted())
  }

  @Test
  fun `neighbour calculation replaces a re-published clustered Signal and resets generation`() = runBlocking {
    val state = directory.resolve("state").createDirectories()
    runProcess(state, listOf("git", "init", "--quiet"))
    val repository = EdictRepository(EdictRepositoryDirectory(state))
    val clusterId = "generated-rule"
    val signalId = stableSignalId(SIGNAL_KEY)
    val cluster = state.resolve("clusters/$clusterId").createDirectories()
    cluster.resolve("signals").createDirectories()
    cluster.resolve("cluster.json").writeText(
      EdictNextJson.encodeToString(
        EdictNextClusterManifest(clusterId, EdictNextLanguage.Kotlin, EdictNextClusterStatus.Generated),
      ),
    )
    cluster.resolve("history.md").writeText("Generated from the original Signal.\n")
    val original = signal(signalId, "Original model", "old-batch")
    cluster.resolve("signals/$signalId.json").writeText(EdictNextJson.encodeToString(original))
    val retainedId = stableSignalId(UNIQUE_SIGNAL_KEY)
    cluster.resolve("signals/$retainedId.json").writeText(
      EdictNextJson.encodeToString(signal(retainedId, "Retained model", "old-batch", UNIQUE_SIGNAL_KEY)),
    )
    repository.paths.inspectionPath(clusterId).also {
      it.parent.createDirectories()
      it.writeText("// generated inspection")
    }
    val replacement = signal(signalId, "Improved re-published model", "new-batch")
    repository.paths.inboxDirectory.createDirectories()
      .resolve("$signalId.json").writeText(EdictNextJson.encodeToString(replacement))

    val distribution = EdictNextDistributionService(repository, directory.resolve("neighbours.json"))
    val prepared = distribution.calculateNeighbours()

    assertContains(prepared.summary, "neighbours of 0 inbox Signal(s)")
    assertContains(prepared.summary, "1 re-published Signal(s) replaced")
    val current = repository.loadState()
    val currentCluster = current.clusters.single()
    assertEquals(replacement, currentCluster.signals.single { it.id == signalId })
    assertEquals(setOf(retainedId), currentCluster.manifest.processedSignalIds)
    assertEquals(EdictNextClusterStatus.Pending, currentCluster.manifest.status)
    assertEquals(clusterId, currentCluster.manifest.predecessorId)
    assertFalse(repository.paths.inboxDirectory.resolve("$signalId.json").exists())
    assertContains(currentCluster.historyPath.readText(), "Replaced Signal `$signalId`")
    assertTrue(repository.paths.inspectionPath(clusterId).exists())
    assertTrue(validateRepositoryState(repository, current).isEmpty())
  }

  @Test
  fun `ambiguous ownership rejects all replacements without changing the repository`() = runBlocking {
    val state = directory.resolve("state").createDirectories()
    runProcess(state, listOf("git", "init", "--quiet"))
    val repository = EdictRepository(EdictRepositoryDirectory(state))
    val uniqueId = stableSignalId(UNIQUE_SIGNAL_KEY)
    val ambiguousId = stableSignalId(AMBIGUOUS_SIGNAL_KEY)
    writePendingCluster(state, "unique-owner", signal(uniqueId, "Original unique model", "old", UNIQUE_SIGNAL_KEY))
    writePendingCluster(state, "ambiguous-a", signal(ambiguousId, "Original ambiguous model", "old", AMBIGUOUS_SIGNAL_KEY))
    writePendingCluster(state, "ambiguous-b", signal(ambiguousId, "Original ambiguous model", "old", AMBIGUOUS_SIGNAL_KEY))
    repository.paths.inboxDirectory.createDirectories().also { inbox ->
      inbox.resolve("$uniqueId.json").writeText(
        EdictNextJson.encodeToString(signal(uniqueId, "Replacement unique model", "new", UNIQUE_SIGNAL_KEY)),
      )
      inbox.resolve("$ambiguousId.json").writeText(
        EdictNextJson.encodeToString(signal(ambiguousId, "Replacement ambiguous model", "new", AMBIGUOUS_SIGNAL_KEY)),
      )
    }

    val distribution = EdictNextDistributionService(repository, directory.resolve("neighbours.json"))
    val error = assertFailsWith<IllegalStateException> { distribution.calculateNeighbours() }

    assertContains(error.message.orEmpty(), "Signal occurs 3 times")
    assertEquals("Original unique model", repository.loadCluster("unique-owner").signals.single().description)
    assertTrue(repository.paths.inboxDirectory.resolve("$uniqueId.json").exists())
    assertTrue(repository.paths.inboxDirectory.resolve("$ambiguousId.json").exists())
  }

  private fun writePendingCluster(state: Path, clusterId: String, signal: EdictNextSignal) {
    val cluster = state.resolve("clusters/$clusterId").createDirectories()
    cluster.resolve("signals").createDirectories()
    cluster.resolve("cluster.json").writeText(
      EdictNextJson.encodeToString(
        EdictNextClusterManifest(clusterId, EdictNextLanguage.Kotlin, EdictNextClusterStatus.Pending),
      ),
    )
    cluster.resolve("signals/${signal.id}.json").writeText(EdictNextJson.encodeToString(signal))
  }

  private fun signal(
    id: String,
    description: String,
    batchId: String,
    idempotencyKey: String = SIGNAL_KEY,
  ): EdictNextSignal = EdictNextSignal(
    id = id,
    idempotencyKey = idempotencyKey,
    fileRevision = EdictNextFileRevision("Example.kt", "a".repeat(40), listOf(EdictNextLineRange(1, 1))),
    source = EdictNextSignalSource.FromPR(
      prNumber = 42,
      title = "Improve example",
      discussionMessages = listOf("Please improve this"),
      diffPositiveToNegative = "--- a/Example.kt\n+++ b/Example.kt\n@@ -1 +1 @@\n-old\n+new\n",
      url = "https://example.invalid/review/42",
    ),
    label = EdictNextSignalLabel.POSITIVE,
    description = description,
    provenance = EdictNextSignalProvenance("work-item", batchId),
  )

  private companion object {
    val SIGNAL_KEY = "space:ij/ultimate:pr:42:work-item:0:POSITIVE:Example.kt:${"a".repeat(40)}:1-1"
    val UNIQUE_SIGNAL_KEY = "space:ij/ultimate:pr:43:work-item:0:POSITIVE:Example.kt:${"a".repeat(40)}:1-1"
    val AMBIGUOUS_SIGNAL_KEY = "space:ij/ultimate:pr:44:work-item:0:POSITIVE:Example.kt:${"a".repeat(40)}:1-1"
  }
}
