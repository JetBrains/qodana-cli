package org.jetbrains.qodana.edict.edictnext

import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonObject
import org.jetbrains.qodana.edict.common.runProcess
import org.jetbrains.qodana.edict.common.GitRepository
import org.jetbrains.qodana.edict.history.enrichment.EdictNextEnrichmentService
import org.jetbrains.qodana.edict.support.fixtureSignals
import org.jetbrains.qodana.edict.support.gitFixture
import org.junit.jupiter.api.io.TempDir
import java.net.URI
import java.nio.file.Files
import java.nio.file.Path
import kotlin.io.path.createDirectories
import kotlin.io.path.createParentDirectories
import kotlin.io.path.exists
import kotlin.io.path.isRegularFile
import kotlin.io.path.readText
import kotlin.io.path.writeText
import kotlin.test.Test
import kotlin.test.assertContains
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class EdictNextGenerationServiceTest {
  @TempDir
  lateinit var directory: Path

  private val analyses = mutableListOf<String>()
  private lateinit var enrichment: EdictNextEnrichmentService

  @Test
  fun `enrichment remains available after restart and generation rename`() = runBlocking<Unit> {
    val (generation, inspection) = generation("second" to (EdictNextSignalStrength.STRONG to EdictNextSignalLabel.POSITIVE))
    val repository = EdictRepository(EdictRepositoryDirectory(directory.resolve("state")))
    val project = GitRepository(directory.resolve("project"))
    try {
      val context = enrichment.getHistoryContext("busy-wait")
      assertEquals("busy-wait", context.clusterId)
      assertEquals(project.resolve("HEAD"), context.range.headRevision)
      val accepted = fixtureSignals(project).first()
      assertTrue(enrichment.publishClusterSignal("busy-wait", accepted).created)
      assertFalse(enrichment.publishClusterSignal("busy-wait", accepted).created)
      assertEquals(context.range.commitRevisions, enrichment.getHistoryContext("second").range.commitRevisions)
      assertFalse(repository.loadCluster("busy-wait").candidateInspectionPath.exists())
      assertFalse(repository.loadCluster("busy-wait").directory.evaluationPath.exists())
      assertTrue(generation.appendHistory("second", "Manager chose this cluster without selection").success)
      assertEquals(context.range.commitRevisions, enrichment.getHistoryContext("busy-wait").range.commitRevisions)

      val restartedEnrichment = EdictNextEnrichmentService(EdictRepository(repository.paths), project.root)
      val restarted = EdictNextGenerationService(EdictRepository(repository.paths), inspection, project.root)
      assertEquals(context.range.commitRevisions, restartedEnrichment.getHistoryContext("busy-wait").range.commitRevisions)
      restarted.renameCluster("busy-wait", "renamed-cluster")
      assertEquals(setOf("s-busy-wait", accepted.id), repository.clusterSignalIds("renamed-cluster"))
      assertEquals(context.range.commitRevisions, restartedEnrichment.getHistoryContext("renamed-cluster").range.commitRevisions)
      assertTrue(restartedEnrichment.publishClusterSignal("renamed-cluster", fixtureSignals(project).last()).created)
      assertTrue(restarted.appendHistory("renamed-cluster", "Continue with the latest evidence").success)
      assertTrue(analyses.isEmpty())
    } finally {
      inspection.stop()
    }
  }

  @Test
  fun `saving validates the candidate and only a passing one is analyzed`() = withGeneration { generation, repository ->
    val scratch = directory.resolve("scratch").toString()
    assertFailsWith<IllegalStateException> { generation.getNewInspectionResults("busy-wait", scratch) }

    val failing = generation.saveCandidateInspection("busy-wait", "// busy-wait")
    // Stored although it fails, so the state always holds the latest attempt.
    assertEquals("// busy-wait", repository.loadCluster("busy-wait").candidateInspectionPath.readText())
    assertFalse(failing.overallSuccess)
    assertEquals(EdictNextNextAction.REPAIR_INSPECTION, failing.nextAction)
    val unvalidated = assertFailsWith<IllegalStateException> { generation.getNewInspectionResults("busy-wait", scratch) }
    assertContains(unvalidated.message.orEmpty(), "has not passed validation")

    val passing = generation.saveCandidateInspection("busy-wait", "// busy-wait report")
    assertTrue(passing.overallSuccess)
    assertEquals(EdictNextNextAction.REVIEW_INSPECTION, passing.nextAction)
    generation.getNewInspectionResults("busy-wait", scratch)
    assertEquals(listOf("busy-wait"), analyses)
  }

  @Test
  fun `each cluster runs a limited number of project analyses`() = withGeneration { generation, _ ->
    val scratch = directory.resolve("scratch").toString()
    generation.saveCandidateInspection("busy-wait", "// busy-wait report")
    generation.saveCandidateInspection("empty-catch", "// empty-catch report")

    assertEquals(1, generation.getNewInspectionResults("busy-wait", scratch).remainingProjectAnalyses)
    assertEquals(0, generation.getNewInspectionResults("busy-wait", scratch).remainingProjectAnalyses)
    generation.selectGenerationClusters()
    val error = assertFailsWith<IllegalStateException> { generation.getNewInspectionResults("busy-wait", scratch) }
    assertContains(error.message.orEmpty(), "used all 2 project analyses")
    // Another cluster keeps its own count, and a refused call starts no analysis.
    assertEquals(1, generation.getNewInspectionResults("empty-catch", scratch).remainingProjectAnalyses)
    assertEquals(listOf("busy-wait", "busy-wait", "empty-catch"), analyses)
  }

  @Test
  fun `clusters without a strong positive Signal are not selected`() = runBlocking<Unit> {
    val (generation, inspection) = generation(
      "weak-positive" to (EdictNextSignalStrength.WEAK to EdictNextSignalLabel.POSITIVE),
      "strong-negative" to (EdictNextSignalStrength.STRONG to EdictNextSignalLabel.NEGATIVE),
    )
    try {
      val response = generation.selectGenerationClusters()
      assertEquals(listOf("busy-wait"), response.clusters.map { it.clusterId })
      // Suggestions are not an allowlist. Acceptance still requires strong positive evidence.
      assertFalse(generation.saveCandidateInspection("weak-positive", "// weak-positive report").overallSuccess)
    }
    finally {
      inspection.stop()
    }
  }

  @Test
  fun `requested generation count overrides default and ranks only strong positive Signals`() = runBlocking<Unit> {
    val strongPositive = EdictNextSignalStrength.STRONG to EdictNextSignalLabel.POSITIVE
    val strongNegative = EdictNextSignalStrength.STRONG to EdictNextSignalLabel.NEGATIVE
    val (generation, inspection) = generation(
      "most" to strongPositive,
      "tie-b" to strongPositive,
      "tie-a" to strongPositive,
      "negative-heavy" to strongNegative,
      defaultGenerationCount = 1,
    )
    writeSignal("most", "s-most-2", EdictNextSignalStrength.STRONG, EdictNextSignalLabel.POSITIVE)
    writeSignal("most", "s-most-3", EdictNextSignalStrength.STRONG, EdictNextSignalLabel.POSITIVE)
    writeSignal("tie-a", "s-tie-a-2", EdictNextSignalStrength.STRONG, EdictNextSignalLabel.POSITIVE)
    writeSignal("tie-b", "s-tie-b-2", EdictNextSignalStrength.STRONG, EdictNextSignalLabel.POSITIVE)
    writeSignal("tie-b", "s-tie-b-3", EdictNextSignalStrength.STRONG, EdictNextSignalLabel.NEGATIVE)
    writeSignal("tie-b", "s-tie-b-4", EdictNextSignalStrength.STRONG, EdictNextSignalLabel.NEGATIVE)
    writeSignal("negative-heavy", "s-negative-heavy-2", EdictNextSignalStrength.STRONG, EdictNextSignalLabel.NEGATIVE)
    writeSignal("negative-heavy", "s-negative-heavy-3", EdictNextSignalStrength.STRONG, EdictNextSignalLabel.NEGATIVE)
    repeat(5) { writeSignal("busy-wait", "s-weak-$it", EdictNextSignalStrength.WEAK, EdictNextSignalLabel.POSITIVE) }

    try {
      val response = generation.selectGenerationClusters(generationCount = 2)
      assertEquals(listOf("most", "tie-a"), response.clusters.map { it.clusterId })
      assertTrue(generation.saveCandidateInspection("tie-b", "// tie-b report").overallSuccess)
    }
    finally {
      inspection.stop()
    }
  }

  @Test
  fun `configured generation count is used when request omits it`() = runBlocking<Unit> {
    val strongPositive = EdictNextSignalStrength.STRONG to EdictNextSignalLabel.POSITIVE
    val (generation, inspection) = generation(
      "second" to strongPositive,
      defaultGenerationCount = 1,
    )
    writeSignal("second", "s-second-2", EdictNextSignalStrength.STRONG, EdictNextSignalLabel.POSITIVE)

    try {
      val response = generation.selectGenerationClusters()
      assertEquals(listOf("second"), response.clusters.map { it.clusterId })
      val error = assertFailsWith<IllegalArgumentException> { generation.selectGenerationClusters(generationCount = 0) }
      assertContains(error.message.orEmpty(), "generationCount must be at least 1")
    }
    finally {
      inspection.stop()
    }
  }

  @Test
  fun `selection ranks distinct unprocessed strong positive source files`() = runBlocking<Unit> {
    val strongPositive = EdictNextSignalStrength.STRONG to EdictNextSignalLabel.POSITIVE
    val (generation, inspection) = generation("a-repeated" to strongPositive, "z-diverse" to strongPositive)
    val repository = EdictRepository(EdictRepositoryDirectory(directory.resolve("state")))
    val repeated = repository.loadCluster("a-repeated").signals.single().fileRevision.path
    repeat(8) { index ->
      writeSignal("a-repeated", "s-repeat-$index", strongPositive.first, strongPositive.second, repeated, "revision-$index")
    }
    writeSignal("z-diverse", "s-diverse", strongPositive.first, strongPositive.second)
    writeSignal("a-repeated", "s-negative", EdictNextSignalStrength.STRONG, EdictNextSignalLabel.NEGATIVE)
    writeSignal("a-repeated", "s-weak", EdictNextSignalStrength.WEAK, EdictNextSignalLabel.POSITIVE)
    writeSignal("a-repeated", "s-processed", strongPositive.first, strongPositive.second)
    val cluster = repository.loadCluster("a-repeated")
    cluster.directory.manifestPath.writeText(EdictNextJson.encodeToString(cluster.manifest.copy(processedSignalIds = setOf("s-processed"))))

    try {
      assertEquals(listOf("z-diverse", "a-repeated", "busy-wait"), generation.selectGenerationClusters().clusters.map { it.clusterId })
    } finally {
      inspection.stop()
    }
  }

  @Test
  fun `Pending clusters with only processed positive evidence remain eligible at lowest priority`() = runBlocking<Unit> {
    val (generation, inspection) = generation("fresh" to (EdictNextSignalStrength.STRONG to EdictNextSignalLabel.POSITIVE))
    val repository = EdictRepository(EdictRepositoryDirectory(directory.resolve("state")))
    val cluster = repository.loadCluster("busy-wait")
    cluster.directory.manifestPath.writeText(
      EdictNextJson.encodeToString(cluster.manifest.copy(processedSignalIds = cluster.signalIds)),
    )
    try {
      assertEquals(listOf("fresh", "busy-wait"), generation.selectGenerationClusters().clusters.map { it.clusterId })
    }
    finally {
      inspection.stop()
    }
  }

  @Test
  fun `successful generation and reuse persist processed Signals without discarding validation evidence`() = runBlocking<Unit> {
    val strongPositive = EdictNextSignalStrength.STRONG to EdictNextSignalLabel.POSITIVE
    val (generation, inspection) = generation("fresh" to strongPositive, "processed" to strongPositive)
    val repository = EdictRepository(EdictRepositoryDirectory(directory.resolve("state")))
    try {
      repeat(5) { index ->
        val id = "s-processed-$index"
        writeSignal("processed", id, strongPositive.first, strongPositive.second)
        repository.assignCodeExample("processed", id, "processed-example")
      }
      writeSignal("fresh", "s-fresh-2", strongPositive.first, strongPositive.second)
      generation.getInspectionAction("processed")
      generation.saveCandidateInspection("processed", "// processed report")
      generation.getNewInspectionResults("processed", directory.resolve("scratch").toString())
      assertTrue(generation.recordEvaluation("processed").success)
      assertTrue(generation.finaliseCluster("processed", EdictNextClusterStatus.Generated, "Accepted").success)
      val processedIds = repository.clusterSignalIds("processed")
      assertEquals(processedIds, repository.loadCluster("processed").manifest.processedSignalIds)

      val newSignal = repository.loadCluster("processed").signals.first().copy(id = "s-new")
      repository.paths.inboxDirectory.createDirectories().resolve("${newSignal.id}.json")
        .writeText(EdictNextJson.encodeToString(newSignal))
      repository.addSignalToCluster(newSignal.id, "processed")
      val restarted = EdictNextGenerationService(
        EdictRepository(repository.paths), inspection, directory.resolve("project"),
      )
      // Six handled positives no longer outweigh two fresh ones; ties still use the cluster id.
      assertEquals(listOf("fresh", "busy-wait", "processed"), restarted.selectGenerationClusters().clusters.map { it.clusterId })
      assertFalse(restarted.finaliseCluster("processed", EdictNextClusterStatus.Generated, "Not evaluated yet").success)
      assertTrue(restarted.finaliseCluster("processed", EdictNextClusterStatus.Pending, "Retry later").success)
      assertEquals(processedIds, repository.loadCluster("processed").manifest.processedSignalIds)

      assertEquals(EdictNextInspectionAction.SKIP, restarted.getInspectionAction("processed").action)
      assertTrue(restarted.recordEvaluation("processed").success)
      assertTrue(restarted.finaliseCluster("processed", EdictNextClusterStatus.Generated, "Reused").success)
      assertEquals(processedIds + newSignal.id, repository.loadCluster("processed").manifest.processedSignalIds)

      // Replacing an already processed Signal counts it again without forgetting the other handled evidence.
      repository.paths.inboxDirectory.resolve("${newSignal.id}.json")
        .writeText(EdictNextJson.encodeToString(newSignal.copy(description = "Corrected evidence")))
      repository.replaceClusterSignalFromInbox(newSignal.id, "processed")
      assertEquals(processedIds, repository.loadCluster("processed").manifest.processedSignalIds)
      restarted.renameCluster("processed", "renamed-processed")
      assertEquals(processedIds, repository.loadCluster("renamed-processed").manifest.processedSignalIds)
      assertEquals(listOf("fresh", "busy-wait", "renamed-processed"), restarted.selectGenerationClusters().clusters.map { it.clusterId })
    }
    finally {
      inspection.stop()
    }
  }

  @Test
  fun `generation uses all enriched evidence and accepts manager chosen clusters`() = runBlocking<Unit> {
    val strongPositive = EdictNextSignalStrength.STRONG to EdictNextSignalLabel.POSITIVE
    val (generation, inspection) = generation("second" to strongPositive, defaultGenerationCount = 1)
    try {
      val selected = generation.selectGenerationClusters()
      assertEquals(listOf("busy-wait"), selected.clusters.map { it.clusterId })
      val repository = EdictRepository(EdictRepositoryDirectory(directory.resolve("state")))
      val signals = fixtureSignals(GitRepository(directory.resolve("project")))
      for (signal in signals) {
        assertTrue(enrichment.publishClusterSignal("busy-wait", signal).created)
        assertFalse(enrichment.publishClusterSignal("busy-wait", signal).created)
        assertTrue(repository.loadCluster("busy-wait").directory.signalPath(signal.id).isRegularFile())
        assertFalse(repository.paths.inboxDirectory.resolve("${signal.id}.json").exists())
      }
      assertTrue(repository.loadInboxSignals().isEmpty())
      assertEquals(setOf("s-busy-wait") + signals.map { it.id }, repository.clusterSignalIds("busy-wait"))
      assertEquals(listOf("busy-wait", "second"), generation.selectGenerationClusters(generationCount = 2).clusters.map { it.clusterId })
      assertFailsWith<IllegalArgumentException> { enrichment.publishClusterSignal("second", signals.first()) }
      assertTrue(generation.appendHistory("busy-wait", "Generate with enriched evidence").success)
      assertFalse(enrichment.publishClusterSignal("busy-wait", signals.first()).created)
      assertTrue(generation.finaliseCluster("busy-wait", EdictNextClusterStatus.Pending, "More work needed").success)
      assertEquals(setOf("s-busy-wait") + signals.map { it.id }, repository.clusterSignalIds("busy-wait"))
      assertTrue(generation.saveCandidateInspection("second", "// second report").overallSuccess)
      generation.getInspectionAction("second")

      // The manager owns scheduling; generation reads the current membership on each call.
      writeSignal("busy-wait", "s-too-late", EdictNextSignalStrength.WEAK, EdictNextSignalLabel.POSITIVE)
      assertTrue(generation.appendHistory("busy-wait", "Use the current evidence").success)
    }
    finally {
      inspection.stop()
    }
  }

  @Test
  fun `an empty suggestion neither reserves targets nor blocks enrichment and later suggestions`() = runBlocking<Unit> {
    val (generation, inspection) = generation()
    writeSignal("busy-wait", "s-busy-wait", EdictNextSignalStrength.WEAK, EdictNextSignalLabel.POSITIVE)
    try {
      val selected = generation.selectGenerationClusters()
      assertTrue(selected.clusters.isEmpty())
      assertTrue(enrichment.publishClusterSignal("busy-wait", fixtureSignals(GitRepository(directory.resolve("project"))).first()).created)
      writeSignal("busy-wait", "s-busy-wait", EdictNextSignalStrength.STRONG, EdictNextSignalLabel.POSITIVE)
      assertEquals(listOf("busy-wait"), generation.selectGenerationClusters().clusters.map { it.clusterId })
      assertTrue(generation.appendHistory("busy-wait", "Chosen after new evidence arrived").success)
    }
    finally {
      inspection.stop()
    }
  }

  @Test
  fun `generation operations leave Pending clusters open to enrichment`() = runBlocking<Unit> {
    val strongPositive = EdictNextSignalStrength.STRONG to EdictNextSignalLabel.POSITIVE
    val (generation, inspection) = generation("second" to strongPositive)
    try {
      generation.selectGenerationClusters()
      generation.appendHistory("busy-wait", "First target started")
      val signal = fixtureSignals(GitRepository(directory.resolve("project"))).first()
      assertEquals("busy-wait", enrichment.getHistoryContext("busy-wait").clusterId)
      assertEquals("second", enrichment.getHistoryContext("second").clusterId)
      assertTrue(enrichment.publishClusterSignal("second", signal).created)
      assertTrue(directory.resolve("state/clusters/second/signals/${signal.id}.json").exists())
      assertTrue(generation.appendHistory("second", "Start with all enriched evidence").success)
      assertFalse(enrichment.publishClusterSignal("second", signal).created)
      generation.getInspectionAction("second")
      assertTrue(enrichment.publishClusterSignal("second", fixtureSignals(GitRepository(directory.resolve("project"))).last()).created)
      writeSignal("second", "s-too-late", EdictNextSignalStrength.WEAK, EdictNextSignalLabel.POSITIVE)
      assertTrue(generation.appendHistory("second", "Use the current evidence").success)
      assertEquals(EdictNextInspectionAction.GENERATE, generation.getInspectionAction("second").action)
    }
    finally {
      inspection.stop()
    }
  }

  @Test
  fun `direct enrichment extends memberships independently for manager chosen targets`() = runBlocking<Unit> {
    val (generation, inspection) = generation("second" to (EdictNextSignalStrength.STRONG to EdictNextSignalLabel.POSITIVE))
    try {
      val signals = fixtureSignals(GitRepository(directory.resolve("project")))
      enrichment.publishClusterSignal("busy-wait", signals.first())
      enrichment.publishClusterSignal("second", signals.last())
      generation.appendHistory("busy-wait", "Enrichment complete")
      generation.appendHistory("second", "Enrichment complete")
      val repository = EdictRepository(EdictRepositoryDirectory(directory.resolve("state")))
      assertEquals(setOf("s-busy-wait", signals.first().id), repository.clusterSignalIds("busy-wait"))
      assertEquals(setOf("s-second", signals.last().id), repository.clusterSignalIds("second"))
    }
    finally {
      inspection.stop()
    }
  }

  @Test
  fun `direct enrichment rejects a different source language`() = runBlocking<Unit> {
    val (generation, inspection) = generation()
    val repository = EdictRepository(EdictRepositoryDirectory(directory.resolve("state")))
    try {
      val project = GitRepository(directory.resolve("project"))
      val kotlinFile = project.root.resolve("module/src/Equality.kt")
      kotlinFile.writeText("fun same(a: String, b: String) = a === b\n")
      project.git("add", ".")
      project.git("commit", "-qm", "Add Kotlin comparison")
      val parent = project.resolve("HEAD")
      kotlinFile.writeText("fun same(a: String, b: String) = a == b\n")
      project.git("add", ".")
      project.git("commit", "-qm", "Fix Kotlin comparison")
      val key = "kotlin-equality-correction"
      val signal = EdictNextSignal(
        id = stableSignalId(key), idempotencyKey = key,
        fileRevision = EdictNextFileRevision("module/src/Equality.kt", parent, listOf(EdictNextLineRange(1, 1))),
        source = EdictNextSignalSource.FromCommit(project.resolve("HEAD")), label = EdictNextSignalLabel.POSITIVE,
        description = "Reference equality does not compare string values", provenance = EdictNextSignalProvenance("kotlin-commit"),
      )
      generation.selectGenerationClusters()
      val error = assertFailsWith<IllegalArgumentException> { enrichment.publishClusterSignal("busy-wait", signal) }
      assertContains(error.message.orEmpty(), "Signal language does not match")
      assertEquals(setOf("s-busy-wait"), repository.clusterSignalIds("busy-wait"))
      assertTrue(repository.loadInboxSignals().isEmpty())
    }
    finally {
      inspection.stop()
    }
  }

  @Test
  fun `enrichment rejects weak invalid and conflicting evidence without writing`() = runBlocking<Unit> {
    val (generation, inspection) = generation()
    val repository = EdictRepository(EdictRepositoryDirectory(directory.resolve("state")))
    try {
      val signal = fixtureSignals(GitRepository(directory.resolve("project"))).first()
      generation.selectGenerationClusters()
      for (invalid in listOf(
        signal.copy(strength = EdictNextSignalStrength.WEAK),
        signal.copy(source = EdictNextSignalSource.SubmittedFeedback()),
        signal.copy(syntheticExampleId = "not-an-example"),
        signal.copy(fileRevision = signal.fileRevision.copy(expectedRanges = listOf(EdictNextLineRange(1, 1)))),
        signal.copy(fileRevision = signal.fileRevision.copy(revision = (signal.source as EdictNextSignalSource.FromCommit).commitRevision)),
      )) {
        assertFailsWith<IllegalArgumentException> { enrichment.publishClusterSignal("busy-wait", invalid) }
      }
      assertEquals(setOf("s-busy-wait"), repository.clusterSignalIds("busy-wait"))
      assertTrue(repository.loadInboxSignals().isEmpty())
      assertTrue(enrichment.publishClusterSignal("busy-wait", signal).created)
      assertFailsWith<IllegalArgumentException> {
        enrichment.publishClusterSignal("busy-wait", signal.copy(description = "Different evidence with the same ID"))
      }
      assertEquals(signal, repository.loadCluster("busy-wait").signals.single { it.id == signal.id })
      assertTrue(generation.appendHistory("busy-wait", "Generate from the accepted evidence").success)
    }
    finally {
      inspection.stop()
    }
  }

  @Test
  fun `enrichment rejects evidence already owned by another cluster or inbox`() = runBlocking<Unit> {
    val (generation, inspection) = generation("second" to (EdictNextSignalStrength.STRONG to EdictNextSignalLabel.POSITIVE))
    val repository = EdictRepository(EdictRepositoryDirectory(directory.resolve("state")))
    try {
      val signals = fixtureSignals(GitRepository(directory.resolve("project")))
      repository.saveClusterSignal("second", signals.first())
      val inboxPath = repository.paths.inboxDirectory.resolve("${signals.last().id}.json").createParentDirectories()
      inboxPath.writeText(EdictNextJson.encodeToString(signals.last()))
      generation.selectGenerationClusters()
      for (signal in signals) {
        assertFailsWith<IllegalArgumentException> { enrichment.publishClusterSignal("busy-wait", signal) }
      }
      assertEquals(setOf("s-busy-wait"), repository.clusterSignalIds("busy-wait"))
      assertEquals(listOf(signals.last()), repository.loadInboxSignals())
    }
    finally {
      inspection.stop()
    }
  }

  @Test
  fun `selection is read only and reranks current evidence without constraining generation`() = runBlocking<Unit> {
    val (generation, inspection) = generation(
      "second" to (EdictNextSignalStrength.STRONG to EdictNextSignalLabel.POSITIVE), defaultGenerationCount = 1,
    )
    try {
      val repository = EdictRepository(EdictRepositoryDirectory(directory.resolve("state")))
      fun snapshot() = Files.walk(repository.paths.root).use { paths ->
        paths.filter { it.isRegularFile() }.toList().associate { it.toString() to Files.readAllBytes(it).toList() }
      }
      val before = snapshot()
      val selected = generation.selectGenerationClusters()
      assertEquals(before, snapshot())
      assertEquals(listOf("busy-wait"), selected.clusters.map { it.clusterId })
      assertEquals(listOf("busy-wait", "second"), generation.selectGenerationClusters(generationCount = 2).clusters.map { it.clusterId })
      assertEquals(before, snapshot())
      writeSignal("second", "s-second-2", EdictNextSignalStrength.STRONG, EdictNextSignalLabel.POSITIVE)
      assertEquals(listOf("second"), generation.selectGenerationClusters().clusters.map { it.clusterId })
      val signal = fixtureSignals(GitRepository(directory.resolve("project"))).first()
      enrichment.publishClusterSignal("busy-wait", signal)
      assertEquals(listOf("busy-wait", "second"), generation.selectGenerationClusters(generationCount = 2).clusters.map { it.clusterId })
      assertTrue(generation.appendHistory("busy-wait", "Manager chose this cluster").success)
      assertTrue(generation.appendHistory("second", "Manager also chose this cluster").success)
      assertEquals(setOf("s-second", "s-second-2"), repository.clusterSignalIds("second"))
    }
    finally {
      inspection.stop()
    }
  }

  @Test
  fun `direct enrichment has no hundred signal distribution cap`() = runBlocking<Unit> {
    val (generation, inspection) = generation()
    try {
      generation.selectGenerationClusters()
      val evidence = fixtureSignals(GitRepository(directory.resolve("project"))).first()
      repeat(101) { index ->
        val key = "history-evidence-$index"
        enrichment.publishClusterSignal("busy-wait", evidence.copy(id = stableSignalId(key), idempotencyKey = key))
      }
      val repository = EdictRepository(EdictRepositoryDirectory(directory.resolve("state")))
      assertEquals(102, repository.clusterSignalIds("busy-wait").size)
      assertTrue(repository.loadInboxSignals().isEmpty())
      assertTrue(generation.appendHistory("busy-wait", "Generate from every accepted Signal").success)
    }
    finally {
      inspection.stop()
    }
  }

  @Test
  fun `finalising applies each status with its contract satisfied`() = runBlocking<Unit> {
    val strongPositive = EdictNextSignalStrength.STRONG to EdictNextSignalLabel.POSITIVE
    val (generation, inspection) = generation("discontinued" to strongPositive, "invalid" to strongPositive, "pending" to strongPositive)
    val repository = EdictRepository(EdictRepositoryDirectory(directory.resolve("state")))
    try {
      generation.selectGenerationClusters()
      for (clusterId in listOf("busy-wait", "discontinued", "invalid")) {
        generation.saveCandidateInspection(clusterId, "// $clusterId report")
      }
      generation.getInspectionAction("busy-wait")
      generation.getNewInspectionResults("busy-wait", directory.resolve("scratch").toString())

      for ((clusterId, status) in listOf(
        "busy-wait" to EdictNextClusterStatus.Generated,
        "discontinued" to EdictNextClusterStatus.Discontinued,
        "invalid" to EdictNextClusterStatus.Invalid,
        "pending" to EdictNextClusterStatus.Pending,
      )) {
        if (status == EdictNextClusterStatus.Generated) {
          val evaluation = generation.recordEvaluation(clusterId)
          assertTrue(evaluation.success, evaluation.summary)
        }
        val response = generation.finaliseCluster(clusterId, status, "Decided $status")
        assertTrue(response.success, response.summary)
        val cluster = repository.loadCluster(clusterId)
        assertEquals(status, cluster.manifest.status)
        assertContains(cluster.historyPath.readText(), "$status: Decided $status")
      }
      assertTrue(repository.paths.inspectionPath("busy-wait").isRegularFile())
      assertFalse(repository.loadCluster("busy-wait").candidateInspectionPath.exists())
      assertFalse(repository.loadCluster("discontinued").candidateInspectionPath.exists())
      assertTrue(repository.loadCluster("invalid").candidateInspectionPath.isRegularFile())
    }
    finally {
      inspection.stop()
    }
  }

  @Test
  fun `finalising rejects a status whose contract fails and changes nothing`() = runBlocking<Unit> {
    val (generation, inspection) = generation("empty-catch" to (EdictNextSignalStrength.STRONG to EdictNextSignalLabel.POSITIVE))
    val repository = EdictRepository(EdictRepositoryDirectory(directory.resolve("state")))
    // A Pending cluster may have a Signal without an example; a Discontinued one may not.
    writeSignal("busy-wait", "s-busy-wait-uncovered", EdictNextSignalStrength.WEAK, EdictNextSignalLabel.POSITIVE)
    try {
      generation.selectGenerationClusters()
      val histories = listOf("busy-wait", "empty-catch").associateWith { repository.loadCluster(it).historyPath.readText() }
      generation.saveCandidateInspection("empty-catch", "// empty-catch report")
      generation.getInspectionAction("empty-catch")

      val unanalyzed = generation.finaliseCluster("empty-catch", EdictNextClusterStatus.Generated, "Accepted")
      assertFalse(unanalyzed.success)
      assertContains(unanalyzed.summary, "has not completed project analysis")
      val uncovered = generation.finaliseCluster("busy-wait", EdictNextClusterStatus.Discontinued, "Contradiction")
      assertFalse(uncovered.success)
      assertContains(uncovered.issues.map { it.message }, "Signal 's-busy-wait-uncovered' has no code example")

      for ((clusterId, history) in histories) {
        val cluster = repository.loadCluster(clusterId)
        assertEquals(EdictNextClusterStatus.Pending, cluster.manifest.status)
        assertEquals(history, cluster.historyPath.readText())
      }
      assertTrue(repository.loadCluster("empty-catch").candidateInspectionPath.isRegularFile())
    }
    finally {
      inspection.stop()
    }
  }

  @Test
  fun `a Discontinued cluster removes its candidate and predecessor`() = runBlocking<Unit> {
    val (generation, inspection) = generation(
      "discontinued" to (EdictNextSignalStrength.STRONG to EdictNextSignalLabel.POSITIVE),
      predecessorId = "discontinued",
    )
    val repository = EdictRepository(EdictRepositoryDirectory(directory.resolve("state")))
    try {
      generation.selectGenerationClusters()
      generation.saveCandidateInspection("discontinued", "// discontinued report")
      assertTrue(generation.finaliseCluster("discontinued", EdictNextClusterStatus.Discontinued, "Contradiction").success)
      val cluster = repository.loadCluster("discontinued")
      assertEquals(null, cluster.manifest.predecessorId)
      assertFalse(repository.paths.inspectionPath("discontinued").exists())
      assertFalse(cluster.candidateInspectionPath.exists())
    }
    finally {
      inspection.stop()
    }
  }

  @Test
  fun `renaming a target renames its candidate id and keeps its progress`() = withGeneration { generation, repository ->
    val scratch = directory.resolve("scratch").toString()
    generation.saveCandidateInspection("busy-wait", "// with report\nInspectionKts(id = \"busy-wait\")")
    generation.getInspectionAction("busy-wait")
    generation.getNewInspectionResults("busy-wait", scratch)

    assertFailsWith<IllegalArgumentException> { generation.renameCluster("busy-wait", "empty-catch") }
    assertTrue(generation.renameCluster("busy-wait", "busy-loop").success)
    assertFalse(directory.resolve("state/clusters/busy-wait").exists())
    val cluster = repository.loadCluster("busy-loop")
    assertEquals("busy-loop", cluster.manifest.id)
    assertEquals("// with report\nInspectionKts(id = \"busy-loop\")", cluster.candidateInspectionPath.readText())
    assertFalse(repository.paths.candidateInspectionPath("busy-wait").exists())
    assertFailsWith<IllegalArgumentException> { generation.appendHistory("busy-wait", "Old id") }

    // Only the id changed, so the analyzed candidate can be Generated without another analysis.
    assertTrue(generation.recordEvaluation("busy-loop").success)
    val finalised = generation.finaliseCluster("busy-loop", EdictNextClusterStatus.Generated, "Accepted")
    assertTrue(finalised.success, finalised.toString())
    assertTrue(repository.paths.inspectionPath("busy-loop").isRegularFile())
    assertEquals(listOf("busy-wait"), analyses)
  }

  @Test
  fun `renaming a target to its own id changes nothing`() = withGeneration { generation, repository ->
    val code = "// with report\nInspectionKts(id = \"busy-wait\")"
    generation.saveCandidateInspection("busy-wait", code)
    assertTrue(generation.renameCluster("busy-wait", "busy-wait").success)
    assertEquals(code, repository.loadCluster("busy-wait").candidateInspectionPath.readText())
    generation.getNewInspectionResults("busy-wait", directory.resolve("scratch").toString())
  }

  @Test
  fun `weak review assignments over every round weigh the final evaluation`() = withGeneration { generation, repository ->
    val scratch = directory.resolve("scratch").toString()
    generation.saveCandidateInspection("busy-wait", "// busy-wait report findings=25")
    generation.getInspectionAction("busy-wait")
    val negative = EdictNextCodeExampleMetadata("extra-negative", "Extra.java", EdictNextSignalLabel.NEGATIVE, emptyList())
    generation.saveCodeExample("busy-wait", negative.id, EdictNextJson.encodeToString(negative), "class Extra {}")
    fun finding(line: Int, classification: EdictNextFindingClassification, exampleId: String?) =
      EdictNextReviewedFinding("src/Found.java", line, classification, exampleId)

    generation.getNewInspectionResults("busy-wait", scratch)
    generation.recordWeakReview(
      "busy-wait",
      (1..3).map { finding(it, EdictNextFindingClassification.TP, "busy-wait-example") } +
      finding(4, EdictNextFindingClassification.UNCERTAIN, null) +
      (5..6).map { finding(it, EdictNextFindingClassification.FP, negative.id) },
    )
    // The next round reclassifies line 3; the location keeps only its latest classification.
    generation.getNewInspectionResults("busy-wait", scratch)
    generation.recordWeakReview("busy-wait", listOf(finding(3, EdictNextFindingClassification.FP, negative.id)))

    val evaluation = checkNotNull(generation.recordEvaluation("busy-wait").evaluation)
    assertEquals(2, evaluation.examples.getValue("busy-wait-example").weight)
    assertEquals(3, evaluation.examples.getValue(negative.id).weight)
    // The candidate reports every example, so the weak negative is a weighted false positive.
    assertEquals(listOf(2, 3, 0), listOf(evaluation.tp, evaluation.fp, evaluation.fn))
    val review = checkNotNull(evaluation.projectReview)
    assertEquals(listOf(25, 6, 1, 0), listOf(review.totalFindings, review.reviewedFindings, review.uncertain, review.unassigned))
    assertEquals(listOf(1, 2), review.truePositiveFindings.map { it.fileRevision.expectedRanges!!.first().start })
    assertTrue(review.inspectionDescription.isNotBlank())
    assertEquals(evaluation, repository.loadEvaluation(repository.loadCluster("busy-wait")))

    // Findings of a deleted example no longer weigh anything and count as unassigned.
    generation.deleteCodeExample("busy-wait", negative.id)
    val afterDelete = checkNotNull(generation.recordEvaluation("busy-wait").evaluation)
    assertEquals(listOf(2, 0, 0), listOf(afterDelete.tp, afterDelete.fp, afterDelete.fn))
    assertEquals(3, afterDelete.projectReview?.unassigned)
  }

  @Test
  fun `each round samples findings the weak review has not classified yet`() = withGeneration { generation, _ ->
    val scratch = directory.resolve("scratch").toString()
    generation.saveCandidateInspection("busy-wait", "// busy-wait report findings=25")
    fun sampledLines(response: EdictNextInspectionResultsResponse): List<Int> {
      val config = EdictNextJson.decodeFromString<EdictNextWeakSignalReviewConfig>(
        Path.of(checkNotNull(response.weakSignalReviewConfigPath)).readText(),
      )
      return EdictNextJson.decodeFromString<EdictNextInspectionFindings>(Path.of(config.findingsPath).readText())
        .findings.map { it.location().second }
    }

    val first = sampledLines(generation.getNewInspectionResults("busy-wait", scratch))
    assertEquals(20, first.size)
    assertEquals(first.sorted(), first)
    assertTrue(first.all { it in 1..25 })
    generation.recordWeakReview(
      "busy-wait",
      first.map { EdictNextReviewedFinding("src/Found.java", it, EdictNextFindingClassification.TP, "busy-wait-example") },
    )

    val second = sampledLines(generation.getNewInspectionResults("busy-wait", scratch))
    assertEquals((1..25).toSet() - first.toSet(), second.toSet())
  }

  @Test
  fun `only an example the analyzed inspection reports can represent a finding`() = withGeneration { generation, _ ->
    val negative = EdictNextCodeExampleMetadata("fixed-negative", "Fixed.java", EdictNextSignalLabel.NEGATIVE, emptyList())
    generation.saveCodeExample("busy-wait", negative.id, EdictNextJson.encodeToString(negative), "class Fixed {}")
    generation.saveCandidateInspection("busy-wait", "// busy-wait report skip=fixed-negative findings=3")
    generation.getNewInspectionResults("busy-wait", directory.resolve("scratch").toString())
    fun finding(line: Int, classification: EdictNextFindingClassification, exampleId: String) =
      EdictNextReviewedFinding("src/Found.java", line, classification, exampleId)

    // The inspection passes the negative example but reports this finding, so the example does not reproduce it.
    val unreproduced = assertFailsWith<IllegalArgumentException> {
      generation.recordWeakReview("busy-wait", listOf(finding(1, EdictNextFindingClassification.FP, negative.id)))
    }
    assertContains(unreproduced.message.orEmpty(), "does not reproduce it")
    assertFailsWith<IllegalArgumentException> {
      generation.recordWeakReview("busy-wait", listOf(finding(1, EdictNextFindingClassification.TP, negative.id)))
    }
    // An example written after the analysis is accepted when the analyzed inspection reports it.
    val split = EdictNextCodeExampleMetadata("split-negative", "Split.java", EdictNextSignalLabel.NEGATIVE, emptyList())
    generation.saveCodeExample("busy-wait", split.id, EdictNextJson.encodeToString(split), "class Split {}")
    val accepted = generation.recordWeakReview(
      "busy-wait",
      listOf(finding(1, EdictNextFindingClassification.FP, split.id), finding(2, EdictNextFindingClassification.TP, "busy-wait-example")),
    )
    assertTrue(accepted.success, accepted.summary)
  }

  @Test
  fun `an example written from a finding must be reported by the analyzed inspection`() = withGeneration { generation, _ ->
    generation.saveCandidateInspection("busy-wait", "// busy-wait report skip=reduced-negative findings=2")
    generation.getNewInspectionResults("busy-wait", directory.resolve("scratch").toString())
    // A later candidate would report the example; the check runs the analyzed one, which reported the finding.
    generation.saveCandidateInspection("busy-wait", "// busy-wait report findings=2")
    val reduced = EdictNextCodeExampleMetadata("reduced-negative", "Reduced.java", EdictNextSignalLabel.NEGATIVE, emptyList())
    generation.saveCodeExample("busy-wait", reduced.id, EdictNextJson.encodeToString(reduced), "class Reduced {}")
    val finding = EdictNextReviewedFinding("src/Found.java", 1, EdictNextFindingClassification.FP, reduced.id)

    val rejected = assertFailsWith<IllegalArgumentException> { generation.recordWeakReview("busy-wait", listOf(finding)) }
    assertContains(rejected.message.orEmpty(), "the reduction removed what makes it report this code")

    val restored = EdictNextCodeExampleMetadata("restored-negative", "Restored.java", EdictNextSignalLabel.NEGATIVE, emptyList())
    generation.saveCodeExample("busy-wait", restored.id, EdictNextJson.encodeToString(restored), "class Restored {}")
    val accepted = generation.recordWeakReview("busy-wait", listOf(finding.copy(exampleId = restored.id)))
    assertTrue(accepted.success, accepted.summary)
  }

  @Test
  fun `the next round sees examples that no longer reproduce their findings`() = withGeneration { generation, repository ->
    val scratch = directory.resolve("scratch").toString()
    val negative = EdictNextCodeExampleMetadata("aliased-negative", "Aliased.java", EdictNextSignalLabel.NEGATIVE, emptyList())
    generation.saveCodeExample("busy-wait", negative.id, EdictNextJson.encodeToString(negative), "class Aliased {}")
    generation.getInspectionAction("busy-wait")
    generation.saveCandidateInspection("busy-wait", "// busy-wait report findings=2")
    generation.getNewInspectionResults("busy-wait", scratch)
    generation.recordWeakReview(
      "busy-wait",
      (1..2).map { EdictNextReviewedFinding("src/Found.java", it, EdictNextFindingClassification.FP, negative.id) },
    )

    // The repaired candidate passes the negative example yet still reports line 1 of the project.
    generation.saveCandidateInspection("busy-wait", "// busy-wait report skip=aliased-negative findings=1")
    val response = generation.getNewInspectionResults("busy-wait", scratch)
    val config = EdictNextJson.decodeFromString<EdictNextWeakSignalReviewConfig>(
      Path.of(checkNotNull(response.weakSignalReviewConfigPath)).readText(),
    )
    assertEquals(
      EdictNextReviewExampleStatus(EdictNextSignalLabel.NEGATIVE, strong = false, reported = false),
      config.exampleStatus[negative.id],
    )
    assertEquals(EdictNextReviewExampleStatus(EdictNextSignalLabel.POSITIVE, strong = true, reported = true), config.exampleStatus["busy-wait-example"])
    assertEquals(mapOf(negative.id to EdictNextUnreproducedFindings(listOf("src/Found.java:1"), noLongerReported = 1)), config.unreproducedFindings)

    // Not moved to a reproducing example, the still reported finding counts as a false positive by itself.
    val evaluation = checkNotNull(generation.recordEvaluation("busy-wait").evaluation)
    assertEquals(listOf(1, 1, 0), listOf(evaluation.tp, evaluation.fp, evaluation.fn))
    assertEquals(1, evaluation.projectReview?.unreproduced)
    assertEquals(1, evaluation.examples.getValue(negative.id).projectFindings)
    assertTrue(repository.loadCluster("busy-wait").examples.any { it.metadata.id == negative.id })
  }

  @Test
  fun `weak review accepts only findings of the last analysis with existing examples`() = withGeneration { generation, _ ->
    suspend fun review(vararg findings: EdictNextReviewedFinding) = generation.recordWeakReview("busy-wait", findings.toList())
    val tp = EdictNextReviewedFinding("src/Found.java", 1, EdictNextFindingClassification.TP, "busy-wait-example")
    generation.saveCandidateInspection("busy-wait", "// busy-wait report findings=2")
    assertFailsWith<IllegalStateException> { review(tp) }

    generation.getNewInspectionResults("busy-wait", directory.resolve("scratch").toString())
    assertFailsWith<IllegalArgumentException> { review(tp.copy(line = 9)) }
    assertFailsWith<IllegalArgumentException> { review(tp.copy(exampleId = "missing")) }
    assertFailsWith<IllegalArgumentException> { review(tp.copy(exampleId = null)) }
    assertTrue(review(tp, tp.copy(line = 2, classification = EdictNextFindingClassification.UNCERTAIN, exampleId = null)).success)
  }

  @Test
  fun `saving a structurally invalid example stores nothing`() = withGeneration { generation, repository ->
    val outsideFile = EdictNextCodeExampleMetadata("extra", "Extra.java", EdictNextSignalLabel.POSITIVE, listOf(EdictNextLineRange(2, 2)))
    val error = assertFailsWith<IllegalArgumentException> {
      generation.saveCodeExample("busy-wait", "extra", EdictNextJson.encodeToString(outsideFile), "class Extra {}")
    }
    assertContains(error.message.orEmpty(), "Invalid target range 2-2")
    val withoutRange = outsideFile.copy(expectedRanges = null)
    assertFailsWith<IllegalArgumentException> {
      generation.saveCodeExample("busy-wait", "extra", EdictNextJson.encodeToString(withoutRange), "class Extra {}")
    }
    assertFalse(repository.loadCluster("busy-wait").examples.any { it.metadata.id == "extra" })
  }

  @Test
  fun `correcting an expected range preserves every other field source and assignment`() = withGeneration { generation, repository ->
    val original = repository.loadCluster("busy-wait").examples.single()
    repository.saveCodeExample("busy-wait", original.metadata.id, original.metadata, "class Example {\n  void work() {}\n}")
    val before = repository.loadCluster("busy-wait")
    val example = before.examples.single()
    val metadata = JsonObject(EdictNextJson.parseToJsonElement(example.directory.metadataPath.readText()).jsonObject +
      ("futureMetadata" to JsonPrimitive("preserve me")))
    example.directory.metadataPath.writeText(metadata.toString())
    example.directory.projectDirectory.resolve("support.txt").writeText("keep the entire example project")
    fun otherFiles() = Files.walk(before.directory.root).use { paths ->
      paths.filter { it.isRegularFile() && it != example.directory.metadataPath }
        .map { it to Files.readAllBytes(it).toList() }.toList().toMap()
    }
    val files = otherFiles()

    assertTrue(generation.updateCodeExampleExpectedRange("busy-wait", example.metadata.id, EdictNextLineRange(2, 2)).success)

    val after = repository.loadCluster("busy-wait")
    assertEquals(before.manifest, after.manifest)
    assertEquals(before.signals, after.signals)
    assertEquals(listOf(example.metadata.copy(expectedRanges = listOf(EdictNextLineRange(2, 2)))), after.examples.map { it.metadata })
    val updated = EdictNextJson.parseToJsonElement(example.directory.metadataPath.readText()).jsonObject
    assertEquals(metadata - "expectedRanges", updated - "expectedRanges")
    assertEquals(files, otherFiles())
  }

  @Test
  fun `range correction rejects invalid locations negative examples missing examples and non Pending clusters`() = withGeneration { generation, repository ->
    val before = repository.loadCluster("busy-wait")
    val example = before.examples.single()
    val originalMetadata = example.directory.metadataPath.readText()
    for (range in listOf(EdictNextLineRange(0, 1), EdictNextLineRange(1, 0), EdictNextLineRange(2, 2))) {
      assertFailsWith<IllegalArgumentException> {
        generation.updateCodeExampleExpectedRange("busy-wait", example.metadata.id, range)
      }
      assertEquals(originalMetadata, example.directory.metadataPath.readText())
    }
    assertFailsWith<IllegalStateException> {
      generation.updateCodeExampleExpectedRange("busy-wait", "missing", EdictNextLineRange(1, 1))
    }
    assertEquals(before.examples, repository.loadCluster("busy-wait").examples)
    val negative = EdictNextCodeExampleMetadata("negative", "Negative.java", EdictNextSignalLabel.NEGATIVE)
    repository.saveCodeExample("busy-wait", negative.id, negative, "class Negative {}")
    assertFailsWith<IllegalArgumentException> {
      generation.updateCodeExampleExpectedRange("busy-wait", negative.id, EdictNextLineRange(1, 1))
    }
    assertEquals(negative, repository.loadCluster("busy-wait").examples.single { it.metadata.id == negative.id }.metadata)
    before.directory.manifestPath.writeText(EdictNextJson.encodeToString(before.manifest.copy(status = EdictNextClusterStatus.Generated)))
    assertFailsWith<IllegalArgumentException> {
      generation.updateCodeExampleExpectedRange("busy-wait", example.metadata.id, EdictNextLineRange(1, 1))
    }
    assertEquals(originalMetadata, example.directory.metadataPath.readText())
  }

  @Test
  fun `range correction requires fresh validation and review results without resetting the analysis budget`() = withGeneration { generation, repository ->
    val example = repository.loadCluster("busy-wait").examples.single()
    repository.saveCodeExample("busy-wait", example.metadata.id, example.metadata, "class Example {\n}")
    val code = "// busy-wait report findings=1"
    val scratch = directory.resolve("scratch").toString()
    val tp = EdictNextReviewedFinding("src/Found.java", 1, EdictNextFindingClassification.TP, example.metadata.id)
    generation.getInspectionAction("busy-wait")
    assertTrue(generation.saveCandidateInspection("busy-wait", code).overallSuccess)
    assertEquals(1, generation.getNewInspectionResults("busy-wait", scratch).remainingProjectAnalyses)
    assertTrue(generation.recordWeakReview("busy-wait", listOf(tp)).success)
    assertTrue(generation.recordEvaluation("busy-wait").success)

    generation.updateCodeExampleExpectedRange("busy-wait", example.metadata.id, EdictNextLineRange(2, 2))
    assertFailsWith<IllegalStateException> { generation.getNewInspectionResults("busy-wait", scratch) }
    assertFailsWith<IllegalArgumentException> { generation.recordWeakReview("busy-wait", listOf(tp)) }
    assertFalse(generation.finaliseCluster("busy-wait", EdictNextClusterStatus.Generated, "Stale evaluation").success)
    assertFalse(generation.saveCandidateInspection("busy-wait", code).overallSuccess)

    // A source-based correction restores the declaration line; the same candidate now passes without a workaround.
    generation.updateCodeExampleExpectedRange("busy-wait", example.metadata.id, EdictNextLineRange(1, 1))
    assertTrue(generation.validateClusterExamples("busy-wait").success)
    assertFailsWith<IllegalStateException> { generation.getNewInspectionResults("busy-wait", scratch) }
    assertTrue(generation.saveCandidateInspection("busy-wait", code).overallSuccess)
    assertEquals(0, generation.getNewInspectionResults("busy-wait", scratch).remainingProjectAnalyses)
    assertTrue(generation.recordWeakReview("busy-wait", listOf(tp)).success)
    assertTrue(generation.recordEvaluation("busy-wait").success)
    assertTrue(generation.finaliseCluster("busy-wait", EdictNextClusterStatus.Generated, "Corrected only the expected location").success)
    assertEquals(listOf("busy-wait", "busy-wait"), analyses)
  }

  private fun withGeneration(test: suspend (EdictNextGenerationService, EdictRepository) -> Unit) = runBlocking {
    val (generation, inspection) = generation("empty-catch" to (EdictNextSignalStrength.STRONG to EdictNextSignalLabel.POSITIVE))
    try {
      test(generation, EdictRepository(EdictRepositoryDirectory(directory.resolve("state"))))
    }
    finally {
      inspection.stop()
    }
  }

  /**
   * A generation service over `busy-wait` and the [clusters] given by strength and label of their single Signal; the
   * [clusters] have the predecessor [predecessorId] when one is given.
   */
  private fun generation(
    vararg clusters: Pair<String, Pair<EdictNextSignalStrength, EdictNextSignalLabel>>,
    predecessorId: String? = null,
    defaultGenerationCount: Int = 5,
  ): Pair<EdictNextGenerationService, IntellijMcpServerService> {
    val project = gitFixture(directory.resolve("project")).root
    val state = directory.resolve("state").createDirectories()
    runProcess(state, listOf("git", "init", "--quiet"))
    val repository = EdictRepository(EdictRepositoryDirectory(state))
    enrichment = EdictNextEnrichmentService(repository, project)
    pendingCluster(repository, "busy-wait", EdictNextSignalStrength.STRONG, EdictNextSignalLabel.POSITIVE)
    predecessorId?.let { repository.paths.inspectionPath(it).createParentDirectories().writeText("// $it report") }
    for ((clusterId, evidence) in clusters) pendingCluster(repository, clusterId, evidence.first, evidence.second, predecessorId)
    val inspection = IntellijMcpServerService(
      projectPath = project,
      clientFactory = InspectionKtsClientFactory { FakeIdeClient(analyses) },
      serverLifecycle = object : IntellijMcpServerLifecycle {
        override suspend fun start(): URI = URI("http://127.0.0.1:1/mcp")
        override suspend fun stop() = Unit
      },
    )
    return EdictNextGenerationService(
      repository,
      inspection,
      project,
      maxProjectAnalyses = 2,
      defaultGenerationCount = defaultGenerationCount,
    ) to inspection
  }

  /** A Pending cluster with one Signal whose example expects a finding on line 1 when positive. */
  private fun pendingCluster(
    repository: EdictRepository,
    clusterId: String,
    strength: EdictNextSignalStrength,
    label: EdictNextSignalLabel,
    predecessorId: String? = null,
  ) {
    val cluster = directory.resolve("state/clusters/$clusterId").createDirectories()
    cluster.resolve("signals").createDirectories()
    cluster.resolve("cluster.json").writeText(
      EdictNextJson.encodeToString(
        EdictNextClusterManifest(clusterId, EdictNextLanguage.Java, EdictNextClusterStatus.Pending, predecessorId),
      ),
    )
    cluster.resolve("history.md").writeText("Created.\n")
    val signalId = "s-$clusterId"
    writeSignal(clusterId, signalId, strength, label)
    val ranges = if (label == EdictNextSignalLabel.POSITIVE) listOf(EdictNextLineRange(1, 1)) else emptyList()
    val example = EdictNextCodeExampleMetadata("$clusterId-example", "Example.java", label, ranges)
    repository.saveCodeExample(clusterId, example.id, example, "class Example {}")
    repository.assignCodeExample(clusterId, signalId, example.id)
  }

  private fun writeSignal(
    clusterId: String,
    signalId: String,
    strength: EdictNextSignalStrength,
    label: EdictNextSignalLabel,
    path: String = "$signalId.java",
    revision: String = "abc",
  ) {
    directory.resolve("state/clusters/$clusterId/signals/$signalId.json").writeText(
      EdictNextJson.encodeToString(
        EdictNextSignal(
          id = signalId,
          fileRevision = EdictNextFileRevision(path, revision, listOf(EdictNextLineRange(1, 1))),
          source = EdictNextSignalSource.SubmittedFeedback(),
          label = label,
          strength = strength,
          description = "Example of $clusterId",
        ),
      ),
    )
  }

  /**
   * A candidate `// <cluster id> [report] [skip=<example id>] [findings=N]` names its cluster, unless it has an `id = "<id>"`, and reports line 1 of every
   * example except a skipped one when it says ` report`, and reports lines 1..N of `src/Found.java` in a project analysis.
   */
  private class FakeIdeClient(private val analyses: MutableList<String>) : InspectionKtsClient {
    override suspend fun runExamples(code: String, examples: List<InspectionKtsExampleRequest>): InspectionKtsBatchRunResult {
      val problems = if (" report" in code) listOf(InspectionKtsProblem("found", 1, "WARNING")) else emptyList()
      return InspectionKtsBatchRunResult(
        compilation(code),
        examples.map { InspectionKtsFileResult(it.id, it.targetFilePath, if (" skip=${it.id}" in code) emptyList() else problems) },
      )
    }

    override suspend fun analyzeProject(code: String): InspectionKtsProjectRunResult {
      val compilation = compilation(code)
      synchronized(analyses) { analyses += checkNotNull(compilation.inspectionId) }
      val findings = Regex("findings=(\\d+)").find(code)?.groupValues?.get(1)?.toInt() ?: 0
      val problems = (1..findings).map { InspectionKtsProblem("found", it, "WARNING") }
      return InspectionKtsProjectRunResult(compilation, listOf(InspectionKtsFileResult(path = "src/Found.java", foundProblems = problems)))
    }

    private fun compilation(code: String): InspectionKtsCompileResult {
      val clusterId = Regex("id = \"([^\"]+)\"").find(code)?.groupValues?.get(1) ?: code.removePrefix("// ").substringBefore(' ')
      return InspectionKtsCompileResult(true, inspectionId = clusterId, inspectionName = clusterId, inspectionDescription = clusterId)
    }

    override suspend fun compile(code: String): InspectionKtsCompileResult = error("not used")
    override suspend fun callTool(name: String, arguments: JsonObject): JsonObject = error("not used")
    override fun close() = Unit
  }
}
