package org.jetbrains.qodana.edict.core

import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonObject
import org.jetbrains.qodana.edict.common.runProcess
import org.jetbrains.qodana.edict.common.GitRepository
import org.jetbrains.qodana.edict.history.enrichment.EdictEnrichmentService
import org.jetbrains.qodana.edict.support.FakeInspectionKtsClient
import org.jetbrains.qodana.edict.support.FakeIntellijMcpServerLifecycle
import org.jetbrains.qodana.edict.support.fixtureSignals
import org.jetbrains.qodana.edict.support.gitFixture
import org.junit.jupiter.api.io.TempDir
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

class EdictGenerationServiceTest {
  @TempDir
  lateinit var directory: Path

  private val analyses = mutableListOf<String>()
  private lateinit var enrichment: EdictEnrichmentService

  @Test
  fun `enrichment remains available after restart and generation rename`() = runBlocking<Unit> {
    val (generation, inspection) = generation("second" to (EdictSignalStrength.STRONG to EdictSignalLabel.POSITIVE))
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

      val restartedEnrichment = EdictEnrichmentService(EdictRepository(repository.paths), project.root)
      val restarted = EdictGenerationService(EdictRepository(repository.paths), inspection, project.root)
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
  fun `cluster examples are read in one answer with their strength and numbered source`() = withGeneration { generation, repository ->
    val weak = EdictCodeExampleMetadata("weak-positive", "Weak.java", EdictSignalLabel.POSITIVE, listOf(EdictLineRange(1, 2)))
    repository.saveCodeExample("busy-wait", weak.id, weak, "class Weak {\n  void run() {}\n}\n")

    assertEquals(
      """
        |== busy-wait-example · POSITIVE · strong (s-busy-wait) · Example.java · expected 1
        |    1| class Example {}
        |
        |== weak-positive · POSITIVE · weak · Weak.java · expected 1-2
        |    1| class Weak {
        |    2|   void run() {}
        |    3| }
      """.trimMargin(),
      generation.clusterExamples("busy-wait"),
    )
  }

  @Test
  fun `saving requires valid metadata before project analysis`() = withGeneration { generation, repository ->
    val scratch = directory.resolve("scratch").toString()
    assertFailsWith<IllegalStateException> { generation.getNewInspectionResults("busy-wait", scratch) }

    val failing = generation.saveCandidateInspection("busy-wait", "// different-rule")
    // Stored although it fails, so the state always holds the latest attempt.
    assertEquals("// different-rule", repository.loadCluster("busy-wait").candidateInspectionPath.readText())
    assertFalse(failing.overallSuccess)
    assertEquals(EdictNextAction.REPAIR_INSPECTION, failing.nextAction)
    val unvalidated = assertFailsWith<IllegalStateException> { generation.getNewInspectionResults("busy-wait", scratch) }
    assertContains(unvalidated.message.orEmpty(), "has not passed validation")

    val passing = generation.saveCandidateInspection("busy-wait", "// busy-wait report")
    assertTrue(passing.overallSuccess)
    assertEquals(EdictNextAction.REVIEW_INSPECTION, passing.nextAction)
    generation.getNewInspectionResults("busy-wait", scratch)
    assertEquals(listOf("busy-wait"), analyses)
  }

  @Test
  fun `generation publishes an analyzed inspection even when every strong and weak example fails`() = withGeneration { generation, repository ->
    val strongNegative = EdictCodeExampleMetadata("strong-negative", "Negative.java", EdictSignalLabel.NEGATIVE, emptyList())
    val weakNegative = EdictCodeExampleMetadata("weak-negative", "WeakNegative.java", EdictSignalLabel.NEGATIVE, emptyList())
    val weakPositive = EdictCodeExampleMetadata("weak-positive", "WeakPositive.java", EdictSignalLabel.POSITIVE, listOf(EdictLineRange(1, 1)))
    for (example in listOf(strongNegative, weakNegative, weakPositive)) {
      repository.saveCodeExample("busy-wait", example.id, example, "class Example {}")
    }
    writeSignal("busy-wait", "s-negative", EdictSignalStrength.STRONG, EdictSignalLabel.NEGATIVE)
    repository.assignCodeExample("busy-wait", "s-negative", strongNegative.id)
    assertEquals(EdictInspectionAction.GENERATE, generation.getInspectionAction("busy-wait").action)

    val code = "// busy-wait report skip=busy-wait-example skip=weak-positive"
    val validation = generation.saveCandidateInspection("busy-wait", code)
    assertTrue(validation.overallSuccess)
    assertEquals(0, validation.achievedAccuracyPercent)
    assertEquals(EdictNextAction.REVIEW_INSPECTION, validation.nextAction)
    val scratch = directory.resolve("scratch").toString()
    assertEquals(1, generation.getNewInspectionResults("busy-wait", scratch).remainingProjectAnalyses)
    assertEquals(0, generation.getNewInspectionResults("busy-wait", scratch).remainingProjectAnalyses)
    val limit = assertFailsWith<IllegalStateException> { generation.getNewInspectionResults("busy-wait", scratch) }
    assertContains(limit.message.orEmpty(), "Generated regardless of strong- or weak-example accuracy")

    val recorded = generation.recordEvaluation("busy-wait")
    assertTrue(recorded.success, recorded.summary)
    val evaluation = checkNotNull(recorded.evaluation)
    assertEquals(listOf(0, 2, 2), listOf(evaluation.tp, evaluation.fp, evaluation.fn))
    assertEquals(0.0, evaluation.precision)
    assertEquals(0.0, evaluation.recall)
    assertTrue(evaluation.examples.values.all { !it.satisfied })

    val finalised = generation.finaliseCluster("busy-wait", EdictClusterStatus.Generated, "All strong and weak examples fail; recorded as limitations")
    assertTrue(finalised.success, finalised.summary)
    val cluster = repository.loadCluster("busy-wait")
    assertEquals(EdictClusterStatus.Generated, cluster.manifest.status)
    assertEquals(code, repository.paths.inspectionPath("busy-wait").readText())
    assertFalse(cluster.candidateInspectionPath.exists())
    assertEquals(cluster.signalIds, cluster.manifest.processedSignalIds)
    assertEquals(evaluation, repository.loadEvaluation(cluster))
  }

  @Test
  fun `Pending is rejected as a generation outcome without changing history or artifacts`() = withGeneration { generation, repository ->
    generation.saveCandidateInspection("busy-wait", "// busy-wait")
    val before = repository.loadCluster("busy-wait")
    val history = before.historyPath.readText()
    val code = before.candidateInspectionPath.readText()
    val response = generation.finaliseCluster("busy-wait", EdictClusterStatus.Pending, "Poor coverage")
    assertFalse(response.success)
    assertContains(response.summary, "Pending is an input state, not a generation outcome")
    val after = repository.loadCluster("busy-wait")
    assertEquals(before.manifest, after.manifest)
    assertEquals(history, after.historyPath.readText())
    assertEquals(code, after.candidateInspectionPath.readText())
    assertFalse(repository.paths.inspectionPath("busy-wait").exists())
  }

  @Test
  fun `SKIP keeps the predecessor evaluated on the current examples, whatever examples it fails`() = runBlocking<Unit> {
    val (generation, inspection) = generation("retry" to STRONG_POSITIVE, predecessorId = "retry")
    val repository = EdictRepository(EdictRepositoryDirectory(directory.resolve("state")))
    try {
      repository.paths.inspectionPath("retry").writeText("// retry report skip=retry-example")
      val unevaluated = generation.getInspectionAction("retry")
      val evaluation = storePredecessorEvaluation(repository, "retry", satisfied = false)

      val action = generation.getInspectionAction("retry")

      assertEquals(
        listOf(
          EdictInspectionActionResponse(EdictInspectionAction.GENERATE, "No predecessor inspection was evaluated on the current code examples"),
          EdictInspectionActionResponse(
            EdictInspectionAction.SKIP,
            "The predecessor inspection and the code examples are those of its evaluation; the new Signals are cases they already cover",
          ),
        ),
        listOf(unevaluated, action),
      )
      assertEquals(
        EdictRecordEvaluationResponse(true, "The predecessor's evaluation stays: neither it nor the examples changed", evaluation),
        generation.recordEvaluation("retry"),
      )
      assertTrue(generation.finaliseCluster("retry", EdictClusterStatus.Generated, "Reuse with a known miss").success)
    }
    finally {
      inspection.stop()
    }
  }

  @Test
  fun `GENERATE once a code example changed since the predecessor's evaluation`() = runBlocking<Unit> {
    val (generation, inspection) = generation("retry" to STRONG_POSITIVE, predecessorId = "retry")
    val repository = EdictRepository(EdictRepositoryDirectory(directory.resolve("state")))
    try {
      storePredecessorEvaluation(repository, "retry", satisfied = true)
      val added = EdictCodeExampleMetadata("added", "Added.java", EdictSignalLabel.POSITIVE, listOf(EdictLineRange(1, 1)))
      repository.saveCodeExample("retry", added.id, added, "class Added {}")

      assertEquals(
        EdictInspectionActionResponse(EdictInspectionAction.GENERATE, "No predecessor inspection was evaluated on the current code examples"),
        generation.getInspectionAction("retry"),
      )
    }
    finally {
      inspection.stop()
    }
  }

  @Test
  fun `GENERATE when a user answered a finding of the predecessor as a false positive`() = runBlocking<Unit> {
    val (generation, inspection) = generation("retry" to STRONG_POSITIVE, predecessorId = "retry")
    val repository = EdictRepository(EdictRepositoryDirectory(directory.resolve("state")))
    try {
      val source = feedbackSource(predecessorDigest(repository, "retry"))
      writeSignal("retry", "answered", EdictSignalStrength.STRONG, EdictSignalLabel.NEGATIVE, source = source)
      storePredecessorEvaluation(repository, "retry", satisfied = true)

      assertEquals(
        EdictInspectionActionResponse(
          EdictInspectionAction.GENERATE,
          "Users answered findings of the predecessor inspection as false positives: answered",
        ),
        generation.getInspectionAction("retry"),
      )
    }
    finally {
      inspection.stop()
    }
  }

  @Test
  fun `a false-positive answer to an earlier version of the inspection does not force GENERATE`() = runBlocking<Unit> {
    val (generation, inspection) = generation("retry" to STRONG_POSITIVE, predecessorId = "retry")
    val repository = EdictRepository(EdictRepositoryDirectory(directory.resolve("state")))
    try {
      val earlier = feedbackSource(sha256Hex("// retry"))
      writeSignal("retry", "answered", EdictSignalStrength.STRONG, EdictSignalLabel.NEGATIVE, source = earlier)
      storePredecessorEvaluation(repository, "retry", satisfied = true)

      assertEquals(EdictInspectionAction.SKIP, generation.getInspectionAction("retry").action)
    }
    finally {
      inspection.stop()
    }
  }

  @Test
  fun `SKIP records nothing once a code example changed after it was decided`() = runBlocking<Unit> {
    val (generation, inspection) = generation("retry" to STRONG_POSITIVE, predecessorId = "retry")
    val repository = EdictRepository(EdictRepositoryDirectory(directory.resolve("state")))
    try {
      val evaluation = storePredecessorEvaluation(repository, "retry", satisfied = true)
      generation.getInspectionAction("retry")
      val added = EdictCodeExampleMetadata("added", "Added.java", EdictSignalLabel.POSITIVE, listOf(EdictLineRange(1, 1)))
      repository.saveCodeExample("retry", added.id, added, "class Added {}")

      val recorded = generation.recordEvaluation("retry")

      assertEquals(
        EdictRecordEvaluationResponse(
          false,
          "No predecessor inspection was evaluated on the current code examples any more; call edict_get_inspection_action again",
        ),
        recorded,
      )
      assertEquals(evaluation, repository.loadEvaluation(repository.loadCluster("retry")))
    }
    finally {
      inspection.stop()
    }
  }

  @Test
  fun `the signal mapping holds the new Signals and the examples, without their labels`() = runBlocking<Unit> {
    val (generation, inspection) = generation()
    val scratch = directory.resolve("scratch")
    try {
      val mapping = generation.prepareSignalMapping("busy-wait", scratch)

      val root = scratch.resolve("signal-mapping/busy-wait")
      val files = Files.walk(root).use { paths ->
        paths.filter(Files::isRegularFile).toList().associate { root.relativize(it).toString() to it.readText() }
      }
      val signal = EdictUnlabelledSignal("s-busy-wait", EdictFileRevision("s-busy-wait.java", "abc", listOf(EdictLineRange(1, 1))))
      assertEquals(EdictSignalMappingResponse(root.toString(), listOf("s-busy-wait"), listOf("busy-wait-example")), mapping)
      assertEquals(
        mapOf(
          "signals/s-busy-wait.json" to EdictJson.encodeToString(EdictUnlabelledSignal.serializer(), signal),
          "examples/busy-wait-example/Example.java" to "class Example {}",
        ),
        files,
      )
    }
    finally {
      inspection.stop()
    }
  }

  /** The digest of the predecessor inspection of [clusterId]. */
  private fun predecessorDigest(repository: EdictRepository, clusterId: String): String =
    sha256Hex(repository.paths.inspectionPath(checkNotNull(repository.loadCluster(clusterId).manifest.predecessorId)).readText())

  /** Stores the evaluation of the predecessor of [clusterId] on its current examples, each [satisfied] or not. */
  private fun storePredecessorEvaluation(repository: EdictRepository, clusterId: String, satisfied: Boolean): EdictEvaluation {
    val cluster = repository.loadCluster(clusterId)
    val code = repository.paths.inspectionPath(checkNotNull(cluster.manifest.predecessorId)).readText()
    val evaluation = EdictEvaluation(
      inspectionHash = sha256Hex(code),
      exampleSetDigest = exampleSetDigest(cluster),
      examples = cluster.examples.associate { it.metadata.id to EdictExampleEvaluation(it.metadata.label, satisfied, 0) },
      projectReview = null,
    )
    cluster.directory.evaluationPath.writeText(EdictJson.encodeToString(evaluation))
    return evaluation
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
      "weak-positive" to (EdictSignalStrength.WEAK to EdictSignalLabel.POSITIVE),
      "strong-negative" to (EdictSignalStrength.STRONG to EdictSignalLabel.NEGATIVE),
    )
    try {
      val response = generation.selectGenerationClusters()
      assertEquals(listOf("busy-wait"), response.clusters.map { it.clusterId })
      // Suggestions are not an allowlist; evidence accuracy is advisory for explicitly chosen clusters too.
      val validation = generation.saveCandidateInspection("weak-positive", "// weak-positive report")
      assertTrue(validation.overallSuccess)
    }
    finally {
      inspection.stop()
    }
  }

  @Test
  fun `requested generation count overrides default and ranks only strong positive Signals`() = runBlocking<Unit> {
    val strongPositive = EdictSignalStrength.STRONG to EdictSignalLabel.POSITIVE
    val strongNegative = EdictSignalStrength.STRONG to EdictSignalLabel.NEGATIVE
    val (generation, inspection) = generation(
      "most" to strongPositive,
      "tie-b" to strongPositive,
      "tie-a" to strongPositive,
      "negative-heavy" to strongNegative,
      defaultGenerationCount = 1,
    )
    writeSignal("most", "s-most-2", EdictSignalStrength.STRONG, EdictSignalLabel.POSITIVE)
    writeSignal("most", "s-most-3", EdictSignalStrength.STRONG, EdictSignalLabel.POSITIVE)
    writeSignal("tie-a", "s-tie-a-2", EdictSignalStrength.STRONG, EdictSignalLabel.POSITIVE)
    writeSignal("tie-b", "s-tie-b-2", EdictSignalStrength.STRONG, EdictSignalLabel.POSITIVE)
    writeSignal("tie-b", "s-tie-b-3", EdictSignalStrength.STRONG, EdictSignalLabel.NEGATIVE)
    writeSignal("tie-b", "s-tie-b-4", EdictSignalStrength.STRONG, EdictSignalLabel.NEGATIVE)
    writeSignal("negative-heavy", "s-negative-heavy-2", EdictSignalStrength.STRONG, EdictSignalLabel.NEGATIVE)
    writeSignal("negative-heavy", "s-negative-heavy-3", EdictSignalStrength.STRONG, EdictSignalLabel.NEGATIVE)
    repeat(5) { writeSignal("busy-wait", "s-weak-$it", EdictSignalStrength.WEAK, EdictSignalLabel.POSITIVE) }

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
    val strongPositive = EdictSignalStrength.STRONG to EdictSignalLabel.POSITIVE
    val (generation, inspection) = generation(
      "second" to strongPositive,
      defaultGenerationCount = 1,
    )
    writeSignal("second", "s-second-2", EdictSignalStrength.STRONG, EdictSignalLabel.POSITIVE)

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
    val strongPositive = EdictSignalStrength.STRONG to EdictSignalLabel.POSITIVE
    val (generation, inspection) = generation("a-repeated" to strongPositive, "z-diverse" to strongPositive)
    val repository = EdictRepository(EdictRepositoryDirectory(directory.resolve("state")))
    val repeated = repository.loadCluster("a-repeated").signals.single().fileRevision.path
    repeat(8) { index ->
      writeSignal("a-repeated", "s-repeat-$index", strongPositive.first, strongPositive.second, repeated, "revision-$index")
    }
    writeSignal("z-diverse", "s-diverse", strongPositive.first, strongPositive.second)
    writeSignal("a-repeated", "s-negative", EdictSignalStrength.STRONG, EdictSignalLabel.NEGATIVE)
    writeSignal("a-repeated", "s-weak", EdictSignalStrength.WEAK, EdictSignalLabel.POSITIVE)
    writeSignal("a-repeated", "s-processed", strongPositive.first, strongPositive.second)
    val cluster = repository.loadCluster("a-repeated")
    cluster.directory.manifestPath.writeText(EdictJson.encodeToString(cluster.manifest.copy(processedSignalIds = setOf("s-processed"))))

    try {
      assertEquals(listOf("z-diverse", "a-repeated", "busy-wait"), generation.selectGenerationClusters().clusters.map { it.clusterId })
    } finally {
      inspection.stop()
    }
  }

  @Test
  fun `Pending clusters whose Signals were all processed are not selected`() = runBlocking<Unit> {
    val (generation, inspection) = generation("fresh" to (EdictSignalStrength.STRONG to EdictSignalLabel.POSITIVE))
    val repository = EdictRepository(EdictRepositoryDirectory(directory.resolve("state")))
    val cluster = repository.loadCluster("busy-wait")
    cluster.directory.manifestPath.writeText(
      EdictJson.encodeToString(cluster.manifest.copy(processedSignalIds = cluster.signalIds)),
    )
    try {
      assertEquals(listOf("fresh"), generation.selectGenerationClusters().clusters.map { it.clusterId })
    }
    finally {
      inspection.stop()
    }
  }

  @Test
  fun `successful generation and reuse persist processed Signals without discarding validation evidence`() = runBlocking<Unit> {
    val strongPositive = EdictSignalStrength.STRONG to EdictSignalLabel.POSITIVE
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
      assertTrue(generation.finaliseCluster("processed", EdictClusterStatus.Generated, "Accepted").success)
      val processedIds = repository.clusterSignalIds("processed")
      assertEquals(processedIds, repository.loadCluster("processed").manifest.processedSignalIds)

      val newSignal = repository.loadCluster("processed").signals.first().copy(id = "s-new")
      repository.paths.inboxDirectory.createDirectories().resolve("${newSignal.id}.json")
        .writeText(EdictJson.encodeToString(newSignal))
      repository.addSignalToCluster(newSignal.id, "processed")
      val restarted = EdictGenerationService(
        EdictRepository(repository.paths), inspection, directory.resolve("project"),
      )
      // Six handled positives no longer outweigh two fresh ones; ties still use the cluster id.
      assertEquals(listOf("fresh", "busy-wait", "processed"), restarted.selectGenerationClusters().clusters.map { it.clusterId })
      assertFalse(restarted.finaliseCluster("processed", EdictClusterStatus.Generated, "Not evaluated yet").success)
      assertFalse(restarted.finaliseCluster("processed", EdictClusterStatus.Pending, "Retry later").success)
      assertEquals(processedIds, repository.loadCluster("processed").manifest.processedSignalIds)

      assertEquals(EdictInspectionAction.SKIP, restarted.getInspectionAction("processed").action)
      assertTrue(restarted.recordEvaluation("processed").success)
      assertTrue(restarted.finaliseCluster("processed", EdictClusterStatus.Generated, "Reused").success)
      assertEquals(processedIds + newSignal.id, repository.loadCluster("processed").manifest.processedSignalIds)

      // Replacing an already processed Signal counts it again without forgetting the other handled evidence.
      repository.paths.inboxDirectory.resolve("${newSignal.id}.json")
        .writeText(EdictJson.encodeToString(newSignal.copy(description = "Corrected evidence")))
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
    val strongPositive = EdictSignalStrength.STRONG to EdictSignalLabel.POSITIVE
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
      assertTrue(generation.appendHistory("busy-wait", "More work needed before generation").success)
      assertEquals(setOf("s-busy-wait") + signals.map { it.id }, repository.clusterSignalIds("busy-wait"))
      assertTrue(generation.saveCandidateInspection("second", "// second report").overallSuccess)
      generation.getInspectionAction("second")

      // The manager owns scheduling; generation reads the current membership on each call.
      writeSignal("busy-wait", "s-too-late", EdictSignalStrength.WEAK, EdictSignalLabel.POSITIVE)
      assertTrue(generation.appendHistory("busy-wait", "Use the current evidence").success)
    }
    finally {
      inspection.stop()
    }
  }

  @Test
  fun `an empty suggestion neither reserves targets nor blocks enrichment and later suggestions`() = runBlocking<Unit> {
    val (generation, inspection) = generation()
    writeSignal("busy-wait", "s-busy-wait", EdictSignalStrength.WEAK, EdictSignalLabel.POSITIVE)
    try {
      val selected = generation.selectGenerationClusters()
      assertTrue(selected.clusters.isEmpty())
      assertTrue(enrichment.publishClusterSignal("busy-wait", fixtureSignals(GitRepository(directory.resolve("project"))).first()).created)
      writeSignal("busy-wait", "s-busy-wait", EdictSignalStrength.STRONG, EdictSignalLabel.POSITIVE)
      assertEquals(listOf("busy-wait"), generation.selectGenerationClusters().clusters.map { it.clusterId })
      assertTrue(generation.appendHistory("busy-wait", "Chosen after new evidence arrived").success)
    }
    finally {
      inspection.stop()
    }
  }

  @Test
  fun `generation operations leave Pending clusters open to enrichment`() = runBlocking<Unit> {
    val strongPositive = EdictSignalStrength.STRONG to EdictSignalLabel.POSITIVE
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
      writeSignal("second", "s-too-late", EdictSignalStrength.WEAK, EdictSignalLabel.POSITIVE)
      assertTrue(generation.appendHistory("second", "Use the current evidence").success)
      assertEquals(EdictInspectionAction.GENERATE, generation.getInspectionAction("second").action)
    }
    finally {
      inspection.stop()
    }
  }

  @Test
  fun `direct enrichment extends memberships independently for manager chosen targets`() = runBlocking<Unit> {
    val (generation, inspection) = generation("second" to (EdictSignalStrength.STRONG to EdictSignalLabel.POSITIVE))
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
      val signal = EdictSignal(
        id = stableSignalId(key), idempotencyKey = key,
        fileRevision = EdictFileRevision("module/src/Equality.kt", parent, listOf(EdictLineRange(1, 1))),
        source = EdictSignalSource.FromCommit(project.resolve("HEAD")), label = EdictSignalLabel.POSITIVE,
        description = "Reference equality does not compare string values", provenance = EdictSignalProvenance("kotlin-commit"),
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
        signal.copy(strength = EdictSignalStrength.WEAK),
        signal.copy(source = EdictSignalSource.SubmittedFeedback()),
        signal.copy(syntheticExampleId = "not-an-example"),
        signal.copy(fileRevision = signal.fileRevision.copy(expectedRanges = listOf(EdictLineRange(1, 1)))),
        signal.copy(fileRevision = signal.fileRevision.copy(revision = (signal.source as EdictSignalSource.FromCommit).commitRevision)),
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
    val (generation, inspection) = generation("second" to (EdictSignalStrength.STRONG to EdictSignalLabel.POSITIVE))
    val repository = EdictRepository(EdictRepositoryDirectory(directory.resolve("state")))
    try {
      val signals = fixtureSignals(GitRepository(directory.resolve("project")))
      repository.saveClusterSignal("second", signals.first())
      val inboxPath = repository.paths.inboxDirectory.resolve("${signals.last().id}.json").createParentDirectories()
      inboxPath.writeText(EdictJson.encodeToString(signals.last()))
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
      "second" to (EdictSignalStrength.STRONG to EdictSignalLabel.POSITIVE), defaultGenerationCount = 1,
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
      writeSignal("second", "s-second-2", EdictSignalStrength.STRONG, EdictSignalLabel.POSITIVE)
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
    val strongPositive = EdictSignalStrength.STRONG to EdictSignalLabel.POSITIVE
    val (generation, inspection) = generation("discontinued" to strongPositive, "invalid" to strongPositive)
    val repository = EdictRepository(EdictRepositoryDirectory(directory.resolve("state")))
    try {
      generation.selectGenerationClusters()
      for (clusterId in listOf("busy-wait", "discontinued", "invalid")) {
        generation.saveCandidateInspection(clusterId, "// $clusterId report")
      }
      generation.getInspectionAction("busy-wait")
      generation.getNewInspectionResults("busy-wait", directory.resolve("scratch").toString())

      for ((clusterId, status) in listOf(
        "busy-wait" to EdictClusterStatus.Generated,
        "discontinued" to EdictClusterStatus.Discontinued,
        "invalid" to EdictClusterStatus.Invalid,
      )) {
        if (status == EdictClusterStatus.Generated) {
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
    val (generation, inspection) = generation("empty-catch" to (EdictSignalStrength.STRONG to EdictSignalLabel.POSITIVE))
    val repository = EdictRepository(EdictRepositoryDirectory(directory.resolve("state")))
    // A Pending cluster may have a Signal without an example; a Discontinued one may not.
    writeSignal("busy-wait", "s-busy-wait-uncovered", EdictSignalStrength.WEAK, EdictSignalLabel.POSITIVE)
    try {
      generation.selectGenerationClusters()
      val histories = listOf("busy-wait", "empty-catch").associateWith { repository.loadCluster(it).historyPath.readText() }
      generation.saveCandidateInspection("empty-catch", "// empty-catch report")
      generation.getInspectionAction("empty-catch")

      val unanalyzed = generation.finaliseCluster("empty-catch", EdictClusterStatus.Generated, "Accepted")
      assertFalse(unanalyzed.success)
      assertContains(unanalyzed.summary, "has not completed project analysis")
      val uncovered = generation.finaliseCluster("busy-wait", EdictClusterStatus.Discontinued, "Contradiction")
      assertFalse(uncovered.success)
      assertContains(uncovered.issues.map { it.message }, "Signal 's-busy-wait-uncovered' has no code example")

      for ((clusterId, history) in histories) {
        val cluster = repository.loadCluster(clusterId)
        assertEquals(EdictClusterStatus.Pending, cluster.manifest.status)
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
      "discontinued" to (EdictSignalStrength.STRONG to EdictSignalLabel.POSITIVE),
      predecessorId = "discontinued",
    )
    val repository = EdictRepository(EdictRepositoryDirectory(directory.resolve("state")))
    try {
      generation.selectGenerationClusters()
      generation.saveCandidateInspection("discontinued", "// discontinued report")
      assertTrue(generation.finaliseCluster("discontinued", EdictClusterStatus.Discontinued, "Contradiction").success)
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
    val finalised = generation.finaliseCluster("busy-loop", EdictClusterStatus.Generated, "Accepted")
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
    val negative = EdictCodeExampleMetadata("extra-negative", "Extra.java", EdictSignalLabel.NEGATIVE, emptyList())
    generation.saveCodeExample("busy-wait", negative.id, EdictJson.encodeToString(negative), "class Extra {}")
    fun finding(line: Int, classification: EdictFindingClassification, exampleId: String?) =
      EdictReviewedFinding("src/Found.java", line, classification, exampleId)

    generation.getNewInspectionResults("busy-wait", scratch)
    generation.recordWeakReview(
      "busy-wait",
      (1..3).map { finding(it, EdictFindingClassification.TP, "busy-wait-example") } +
      finding(4, EdictFindingClassification.UNCERTAIN, null) +
      (5..6).map { finding(it, EdictFindingClassification.FP, negative.id) },
    )
    // The next round reclassifies line 3; the location keeps only its latest classification.
    generation.getNewInspectionResults("busy-wait", scratch)
    generation.recordWeakReview("busy-wait", listOf(finding(3, EdictFindingClassification.FP, negative.id)))

    val evaluation = checkNotNull(generation.recordEvaluation("busy-wait").evaluation)
    assertEquals(2, evaluation.examples.getValue("busy-wait-example").weight)
    assertEquals(3, evaluation.examples.getValue(negative.id).weight)
    // The candidate reports every example, so the weak negative is a weighted false positive.
    assertEquals(listOf(2, 3, 0), listOf(evaluation.tp, evaluation.fp, evaluation.fn))
    val review = checkNotNull(evaluation.projectReview)
    assertEquals(listOf(25, 6), listOf(review.totalFindings, review.reviewedFindings))
    assertEquals(listOf(1, 2), review.truePositiveFindings.map { it.fileRevision.expectedRanges!!.first().start })
    assertEquals(evaluation, repository.loadEvaluation(repository.loadCluster("busy-wait")))

    // Findings of a deleted example no longer weigh anything.
    generation.deleteCodeExample("busy-wait", negative.id)
    val afterDelete = checkNotNull(generation.recordEvaluation("busy-wait").evaluation)
    assertEquals(listOf(2, 0, 0), listOf(afterDelete.tp, afterDelete.fp, afterDelete.fn))
  }

  @Test
  fun `each round samples findings the weak review has not classified yet`() = withGeneration { generation, _ ->
    val scratch = directory.resolve("scratch").toString()
    generation.saveCandidateInspection("busy-wait", "// busy-wait report findings=25")
    fun sampledLines(response: EdictInspectionResultsResponse): List<Int> {
      val config = EdictJson.decodeFromString<EdictWeakSignalReviewConfig>(
        Path.of(checkNotNull(response.weakSignalReviewConfigPath)).readText(),
      )
      return EdictJson.decodeFromString<EdictInspectionFindings>(Path.of(config.findingsPath).readText())
        .findings.map { it.location().second }
    }

    val first = sampledLines(generation.getNewInspectionResults("busy-wait", scratch))
    assertEquals(20, first.size)
    assertEquals(first.sorted(), first)
    assertTrue(first.all { it in 1..25 })
    generation.recordWeakReview(
      "busy-wait",
      first.map { EdictReviewedFinding("src/Found.java", it, EdictFindingClassification.TP, "busy-wait-example") },
    )

    val second = sampledLines(generation.getNewInspectionResults("busy-wait", scratch))
    assertEquals((1..25).toSet() - first.toSet(), second.toSet())
  }

  @Test
  fun `a round whose findings this run reviewed already needs no weak review`() = withGeneration { generation, _ ->
    val scratch = directory.resolve("scratch").toString()
    generation.saveCandidateInspection("busy-wait", "// busy-wait report findings=2")
    generation.getNewInspectionResults("busy-wait", scratch)
    generation.recordWeakReview(
      "busy-wait",
      (1..2).map { EdictReviewedFinding("src/Found.java", it, EdictFindingClassification.TP, "busy-wait-example") },
    )
    // The next candidate reports a part of the reviewed findings, so its findings differ from the previous ones.
    generation.saveCandidateInspection("busy-wait", "// busy-wait report findings=1")

    val response = generation.getNewInspectionResults("busy-wait", scratch)

    val config = EdictJson.decodeFromString<EdictWeakSignalReviewConfig>(Path.of(response.weakSignalReviewConfigPath).readText())
    val sample = EdictJson.decodeFromString<EdictInspectionFindings>(Path.of(config.findingsPath).readText()).findings
    assertEquals(listOf(false, false, true), listOf(response.findingsUnchanged, response.weakReviewNeeded, sample.isEmpty()))
  }

  @Test
  fun `the project review counts only the reviewed findings the evaluated candidate reports`() = withGeneration { generation, _ ->
    val scratch = directory.resolve("scratch").toString()
    generation.getInspectionAction("busy-wait")
    generation.saveCandidateInspection("busy-wait", "// busy-wait report findings=2")
    generation.getNewInspectionResults("busy-wait", scratch)
    generation.recordWeakReview(
      "busy-wait",
      (1..2).map { EdictReviewedFinding("src/Found.java", it, EdictFindingClassification.TP, "busy-wait-example") },
    )
    // The evaluated candidate reports line 1 only; line 2 was a finding of the earlier one.
    generation.saveCandidateInspection("busy-wait", "// busy-wait report findings=1")
    generation.getNewInspectionResults("busy-wait", scratch)

    val review = checkNotNull(generation.recordEvaluation("busy-wait").evaluation?.projectReview)

    assertEquals(listOf(1, 1, 1), listOf(review.totalFindings, review.reviewedFindings, review.truePositiveFindings.size))
  }

  @Test
  fun `findings are unchanged only when the previous analysis was reviewed`() = withGeneration { generation, _ ->
    val scratch = directory.resolve("scratch").toString()
    generation.saveCandidateInspection("empty-catch", "// empty-catch report findings=1")
    assertFalse(generation.getNewInspectionResults("empty-catch", scratch).findingsUnchanged)
    assertFalse(generation.getNewInspectionResults("empty-catch", scratch).findingsUnchanged)

    generation.saveCandidateInspection("busy-wait", "// busy-wait report findings=1")
    assertFalse(generation.getNewInspectionResults("busy-wait", scratch).findingsUnchanged)
    generation.recordWeakReview(
      "busy-wait",
      listOf(EdictReviewedFinding("src/Found.java", 1, EdictFindingClassification.TP, "busy-wait-example")),
    )
    assertTrue(generation.getNewInspectionResults("busy-wait", scratch).findingsUnchanged)
  }

  @Test
  fun `only an example the analyzed inspection reports can represent a finding`() = withGeneration { generation, _ ->
    val negative = EdictCodeExampleMetadata("fixed-negative", "Fixed.java", EdictSignalLabel.NEGATIVE, emptyList())
    generation.saveCodeExample("busy-wait", negative.id, EdictJson.encodeToString(negative), "class Fixed {}")
    generation.saveCandidateInspection("busy-wait", "// busy-wait report skip=fixed-negative findings=3")
    generation.getNewInspectionResults("busy-wait", directory.resolve("scratch").toString())
    fun finding(line: Int, classification: EdictFindingClassification, exampleId: String) =
      EdictReviewedFinding("src/Found.java", line, classification, exampleId)

    // The inspection passes the negative example but reports this finding, so the example does not reproduce it.
    val unreproduced = assertFailsWith<IllegalArgumentException> {
      generation.recordWeakReview("busy-wait", listOf(finding(1, EdictFindingClassification.FP, negative.id)))
    }
    assertContains(unreproduced.message.orEmpty(), "does not reproduce it")
    assertFailsWith<IllegalArgumentException> {
      generation.recordWeakReview("busy-wait", listOf(finding(1, EdictFindingClassification.TP, negative.id)))
    }
    // An example written after the analysis is accepted when the analyzed inspection reports it.
    val split = EdictCodeExampleMetadata("split-negative", "Split.java", EdictSignalLabel.NEGATIVE, emptyList())
    generation.saveCodeExample("busy-wait", split.id, EdictJson.encodeToString(split), "class Split {}")
    val accepted = generation.recordWeakReview(
      "busy-wait",
      listOf(finding(1, EdictFindingClassification.FP, split.id), finding(2, EdictFindingClassification.TP, "busy-wait-example")),
    )
    assertTrue(accepted.success, accepted.summary)
  }

  @Test
  fun `an example written from a finding must be reported by the analyzed inspection`() = withGeneration { generation, _ ->
    generation.saveCandidateInspection("busy-wait", "// busy-wait report skip=reduced-negative findings=2")
    generation.getNewInspectionResults("busy-wait", directory.resolve("scratch").toString())
    // A later candidate would report the example; the check runs the analyzed one, which reported the finding.
    generation.saveCandidateInspection("busy-wait", "// busy-wait report findings=2")
    val reduced = EdictCodeExampleMetadata("reduced-negative", "Reduced.java", EdictSignalLabel.NEGATIVE, emptyList())
    generation.saveCodeExample("busy-wait", reduced.id, EdictJson.encodeToString(reduced), "class Reduced {}")
    val finding = EdictReviewedFinding("src/Found.java", 1, EdictFindingClassification.FP, reduced.id)

    val rejected = assertFailsWith<IllegalArgumentException> { generation.recordWeakReview("busy-wait", listOf(finding)) }
    assertContains(rejected.message.orEmpty(), "the reduction removed what makes it report this code")

    val restored = EdictCodeExampleMetadata("restored-negative", "Restored.java", EdictSignalLabel.NEGATIVE, emptyList())
    generation.saveCodeExample("busy-wait", restored.id, EdictJson.encodeToString(restored), "class Restored {}")
    val accepted = generation.recordWeakReview("busy-wait", listOf(finding.copy(exampleId = restored.id)))
    assertTrue(accepted.success, accepted.summary)
  }

  @Test
  fun `an example written from a finding keeps the finding's path`() = withGeneration { generation, repository ->
    generation.saveCandidateInspection("busy-wait", "// busy-wait report skip=reduced-negative findings=2")
    generation.getNewInspectionResults("busy-wait", directory.resolve("scratch").toString())
    val reduced = EdictCodeExampleMetadata("reduced-negative", "Reduced.java", EdictSignalLabel.NEGATIVE, emptyList())
    generation.saveCodeExample("busy-wait", reduced.id, EdictJson.encodeToString(reduced), "class Reduced {}")
    val finding = EdictReviewedFinding("src/Found.java", 1, EdictFindingClassification.FP, reduced.id)

    assertFailsWith<IllegalArgumentException> { generation.recordWeakReview("busy-wait", listOf(finding)) }
    val stored = repository.loadCluster("busy-wait").examples.single { it.metadata.id == reduced.id }
    assertEquals("src/Found.java", stored.metadata.contextPath)
  }

  @Test
  fun `the next round sees examples that no longer reproduce their findings`() = withGeneration { generation, repository ->
    val scratch = directory.resolve("scratch").toString()
    val negative = EdictCodeExampleMetadata("aliased-negative", "Aliased.java", EdictSignalLabel.NEGATIVE, emptyList())
    generation.saveCodeExample("busy-wait", negative.id, EdictJson.encodeToString(negative), "class Aliased {}")
    generation.getInspectionAction("busy-wait")
    generation.saveCandidateInspection("busy-wait", "// busy-wait report findings=2")
    generation.getNewInspectionResults("busy-wait", scratch)
    generation.recordWeakReview(
      "busy-wait",
      (1..2).map { EdictReviewedFinding("src/Found.java", it, EdictFindingClassification.FP, negative.id) },
    )

    // The repaired candidate passes the negative example yet still reports line 1 of the project.
    generation.saveCandidateInspection("busy-wait", "// busy-wait report skip=aliased-negative findings=1")
    val response = generation.getNewInspectionResults("busy-wait", scratch)
    val config = EdictJson.decodeFromString<EdictWeakSignalReviewConfig>(
      Path.of(checkNotNull(response.weakSignalReviewConfigPath)).readText(),
    )
    assertEquals(
      EdictReviewExampleStatus(EdictSignalLabel.NEGATIVE, strong = false, reported = false),
      config.exampleStatus[negative.id],
    )
    assertEquals(EdictReviewExampleStatus(EdictSignalLabel.POSITIVE, strong = true, reported = true), config.exampleStatus["busy-wait-example"])
    assertEquals(mapOf(negative.id to EdictUnreproducedFindings(listOf("src/Found.java:1"), noLongerReported = 1)), config.unreproducedFindings)

    // Both findings count by their example, which the repaired candidate passes.
    val evaluation = checkNotNull(generation.recordEvaluation("busy-wait").evaluation)
    assertEquals(listOf(1, 0, 0), listOf(evaluation.tp, evaluation.fp, evaluation.fn))
    assertEquals(2, evaluation.examples.getValue(negative.id).projectFindings)
    assertTrue(repository.loadCluster("busy-wait").examples.any { it.metadata.id == negative.id })
  }

  @Test
  fun `weak review accepts only findings of the last analysis with existing examples`() = withGeneration { generation, _ ->
    suspend fun review(vararg findings: EdictReviewedFinding) = generation.recordWeakReview("busy-wait", findings.toList())
    val tp = EdictReviewedFinding("src/Found.java", 1, EdictFindingClassification.TP, "busy-wait-example")
    generation.saveCandidateInspection("busy-wait", "// busy-wait report findings=2")
    assertFailsWith<IllegalStateException> { review(tp) }

    generation.getNewInspectionResults("busy-wait", directory.resolve("scratch").toString())
    assertFailsWith<IllegalArgumentException> { review(tp.copy(line = 9)) }
    assertFailsWith<IllegalArgumentException> { review(tp.copy(exampleId = "missing")) }
    assertFailsWith<IllegalArgumentException> { review(tp.copy(exampleId = null)) }
    assertTrue(review(tp, tp.copy(line = 2, classification = EdictFindingClassification.UNCERTAIN, exampleId = null)).success)
  }

  @Test
  fun `saving a structurally invalid example stores nothing`() = withGeneration { generation, repository ->
    val outsideFile = EdictCodeExampleMetadata("extra", "Extra.java", EdictSignalLabel.POSITIVE, listOf(EdictLineRange(2, 2)))
    val error = assertFailsWith<IllegalArgumentException> {
      generation.saveCodeExample("busy-wait", "extra", EdictJson.encodeToString(outsideFile), "class Extra {}")
    }
    assertContains(error.message.orEmpty(), "Invalid target range 2-2")
    val withoutRange = outsideFile.copy(expectedRanges = null)
    assertFailsWith<IllegalArgumentException> {
      generation.saveCodeExample("busy-wait", "extra", EdictJson.encodeToString(withoutRange), "class Extra {}")
    }
    assertFalse(repository.loadCluster("busy-wait").examples.any { it.metadata.id == "extra" })
  }

  @Test
  fun `correcting an expected range preserves every other field source and assignment`() = withGeneration { generation, repository ->
    val original = repository.loadCluster("busy-wait").examples.single()
    repository.saveCodeExample("busy-wait", original.metadata.id, original.metadata, "class Example {\n  void work() {}\n}")
    val before = repository.loadCluster("busy-wait")
    val example = before.examples.single()
    val metadata = JsonObject(EdictJson.parseToJsonElement(example.directory.metadataPath.readText()).jsonObject +
      ("futureMetadata" to JsonPrimitive("preserve me")))
    example.directory.metadataPath.writeText(metadata.toString())
    example.directory.projectDirectory.resolve("support.txt").writeText("keep the entire example project")
    fun otherFiles() = Files.walk(before.directory.root).use { paths ->
      paths.filter { it.isRegularFile() && it != example.directory.metadataPath }
        .map { it to Files.readAllBytes(it).toList() }.toList().toMap()
    }
    val files = otherFiles()

    assertTrue(generation.updateCodeExampleExpectedRange("busy-wait", example.metadata.id, EdictLineRange(2, 2)).success)

    val after = repository.loadCluster("busy-wait")
    assertEquals(before.manifest, after.manifest)
    assertEquals(before.signals, after.signals)
    assertEquals(listOf(example.metadata.copy(expectedRanges = listOf(EdictLineRange(2, 2)))), after.examples.map { it.metadata })
    val updated = EdictJson.parseToJsonElement(example.directory.metadataPath.readText()).jsonObject
    assertEquals(metadata - "expectedRanges", updated - "expectedRanges")
    assertEquals(files, otherFiles())
  }

  @Test
  fun `range correction rejects invalid locations negative examples missing examples and non Pending clusters`() = withGeneration { generation, repository ->
    val before = repository.loadCluster("busy-wait")
    val example = before.examples.single()
    val originalMetadata = example.directory.metadataPath.readText()
    for (range in listOf(EdictLineRange(0, 1), EdictLineRange(1, 0), EdictLineRange(2, 2))) {
      assertFailsWith<IllegalArgumentException> {
        generation.updateCodeExampleExpectedRange("busy-wait", example.metadata.id, range)
      }
      assertEquals(originalMetadata, example.directory.metadataPath.readText())
    }
    assertFailsWith<IllegalStateException> {
      generation.updateCodeExampleExpectedRange("busy-wait", "missing", EdictLineRange(1, 1))
    }
    assertEquals(before.examples, repository.loadCluster("busy-wait").examples)
    val negative = EdictCodeExampleMetadata("negative", "Negative.java", EdictSignalLabel.NEGATIVE)
    repository.saveCodeExample("busy-wait", negative.id, negative, "class Negative {}")
    assertFailsWith<IllegalArgumentException> {
      generation.updateCodeExampleExpectedRange("busy-wait", negative.id, EdictLineRange(1, 1))
    }
    assertEquals(negative, repository.loadCluster("busy-wait").examples.single { it.metadata.id == negative.id }.metadata)
    before.directory.manifestPath.writeText(EdictJson.encodeToString(before.manifest.copy(status = EdictClusterStatus.Generated)))
    assertFailsWith<IllegalArgumentException> {
      generation.updateCodeExampleExpectedRange("busy-wait", example.metadata.id, EdictLineRange(1, 1))
    }
    assertEquals(originalMetadata, example.directory.metadataPath.readText())
  }

  @Test
  fun `range correction requires fresh validation and review results without resetting the analysis budget`() = withGeneration { generation, repository ->
    val example = repository.loadCluster("busy-wait").examples.single()
    repository.saveCodeExample("busy-wait", example.metadata.id, example.metadata, "class Example {\n}")
    val code = "// busy-wait report findings=1"
    val scratch = directory.resolve("scratch").toString()
    val tp = EdictReviewedFinding("src/Found.java", 1, EdictFindingClassification.TP, example.metadata.id)
    generation.getInspectionAction("busy-wait")
    assertTrue(generation.saveCandidateInspection("busy-wait", code).overallSuccess)
    assertEquals(1, generation.getNewInspectionResults("busy-wait", scratch).remainingProjectAnalyses)
    assertTrue(generation.recordWeakReview("busy-wait", listOf(tp)).success)
    assertTrue(generation.recordEvaluation("busy-wait").success)

    generation.updateCodeExampleExpectedRange("busy-wait", example.metadata.id, EdictLineRange(2, 2))
    assertFailsWith<IllegalStateException> { generation.getNewInspectionResults("busy-wait", scratch) }
    assertFailsWith<IllegalArgumentException> { generation.recordWeakReview("busy-wait", listOf(tp)) }
    assertFalse(generation.finaliseCluster("busy-wait", EdictClusterStatus.Generated, "Stale evaluation").success)
    val missed = generation.saveCandidateInspection("busy-wait", code)
    assertTrue(missed.overallSuccess)

    // A source-based correction restores the declaration line; the same candidate now passes without a workaround.
    generation.updateCodeExampleExpectedRange("busy-wait", example.metadata.id, EdictLineRange(1, 1))
    assertTrue(generation.validateClusterExamples("busy-wait").success)
    assertFailsWith<IllegalStateException> { generation.getNewInspectionResults("busy-wait", scratch) }
    assertTrue(generation.saveCandidateInspection("busy-wait", code).overallSuccess)
    assertEquals(0, generation.getNewInspectionResults("busy-wait", scratch).remainingProjectAnalyses)
    assertTrue(generation.recordWeakReview("busy-wait", listOf(tp)).success)
    assertTrue(generation.recordEvaluation("busy-wait").success)
    assertTrue(generation.finaliseCluster("busy-wait", EdictClusterStatus.Generated, "Corrected only the expected location").success)
    assertEquals(listOf("busy-wait", "busy-wait"), analyses)
  }

  @Test
  fun `final generation uploads the evaluated findings and persists the cloud report link`() = runBlocking {
    val uploads = mutableListOf<EdictInspectionFindings>()
    val (generation, server) = generation(reportUploader = EdictReportUploader { _, name, findings ->
      assertTrue(name.isNotBlank())
      uploads += findings
      "https://qodana.cloud/reports/final-report"
    })
    try {
      generation.getInspectionAction("busy-wait")
      val repository = EdictRepository(EdictRepositoryDirectory(directory.resolve("state")))
      val code = "// busy-wait report"
      assertTrue(generation.saveCandidateInspection("busy-wait", code).overallSuccess)
      generation.getNewInspectionResults("busy-wait", directory.resolve("scratch").toString())
      assertTrue(generation.recordEvaluation("busy-wait").success)
      assertTrue(uploads.isEmpty())
      assertTrue(generation.finaliseCluster("busy-wait", EdictClusterStatus.Generated, "Accepted").success)
      val cluster = repository.loadCluster("busy-wait")
      val review = checkNotNull(repository.loadEvaluation(cluster)?.projectReview)
      assertEquals(sha256Hex(code), uploads.single().candidateDigest)
      assertEquals(review.totalFindings, uploads.single().findings.size)
      assertEquals("https://qodana.cloud/reports/final-report", review.reportUrl)
      assertEquals(EdictClusterStatus.Generated, cluster.manifest.status)
    } finally { server.stop() }
  }

  @Test
  fun `failed cloud upload leaves generation pending and evaluation without report link`() = runBlocking {
    val (generation, server) = generation(reportUploader = EdictReportUploader { _, _, _ -> error("Upload unavailable") })
    try {
      generation.getInspectionAction("busy-wait")
      val repository = EdictRepository(EdictRepositoryDirectory(directory.resolve("state")))
      assertTrue(generation.saveCandidateInspection("busy-wait", "// busy-wait report").overallSuccess)
      generation.getNewInspectionResults("busy-wait", directory.resolve("scratch").toString())
      assertTrue(generation.recordEvaluation("busy-wait").success)
      assertFailsWith<IllegalStateException> {
        generation.finaliseCluster("busy-wait", EdictClusterStatus.Generated, "Accepted")
      }
      val cluster = repository.loadCluster("busy-wait")
      assertEquals(EdictClusterStatus.Pending, cluster.manifest.status)
      assertEquals(null, repository.loadEvaluation(cluster)?.projectReview?.reportUrl)
    } finally { server.stop() }
  }

  private fun withGeneration(test: suspend (EdictGenerationService, EdictRepository) -> Unit) = runBlocking {
    val (generation, inspection) = generation("empty-catch" to (EdictSignalStrength.STRONG to EdictSignalLabel.POSITIVE))
    try {
      test(generation, EdictRepository(EdictRepositoryDirectory(directory.resolve("state"))))
    }
    finally {
      inspection.stop()
    }
  }

  @Test
  fun `an example with a reference its module cannot resolve is not assigned and fails validation`() = runBlocking {
    val (generation, inspection) = generation()
    try {
      val metadata = EdictCodeExampleMetadata("imported", "Example.java", EdictSignalLabel.POSITIVE, listOf(EdictLineRange(1, 1)))
      generation.saveCodeExample("busy-wait", metadata.id, EdictJson.encodeToString(metadata), "class Example {} // unresolved=Missing\n")
      val issue = "Code example 'imported' has references that do not resolve in the module of s-busy-wait.java: " +
        "line 1 'Missing'. Declare each one in the example, or set contextPath to a file whose module provides them"

      val refused = assertFailsWith<IllegalArgumentException> { generation.assignCodeExample("busy-wait", "s-busy-wait", metadata.id) }
      assertEquals(issue, refused.message)
      val validation = generation.validateClusterExamples("busy-wait")
      assertFalse(validation.success)
      assertEquals(listOf("${directory.resolve("state/clusters/busy-wait/synthetic-examples/imported")}: $issue"), validation.issues)
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
    vararg clusters: Pair<String, Pair<EdictSignalStrength, EdictSignalLabel>>,
    predecessorId: String? = null,
    defaultGenerationCount: Int = 5,
    reportUploader: EdictReportUploader? = null,
  ): Pair<EdictGenerationService, IntellijMcpServerService> {
    val project = gitFixture(directory.resolve("project")).root
    val state = directory.resolve("state").createDirectories()
    runProcess(state, listOf("git", "init", "--quiet"))
    val repository = EdictRepository(EdictRepositoryDirectory(state))
    enrichment = EdictEnrichmentService(repository, project)
    pendingCluster(repository, "busy-wait", EdictSignalStrength.STRONG, EdictSignalLabel.POSITIVE)
    predecessorId?.let { repository.paths.inspectionPath(it).createParentDirectories().writeText("// $it report") }
    for ((clusterId, evidence) in clusters) pendingCluster(repository, clusterId, evidence.first, evidence.second, predecessorId)
    val inspection = IntellijMcpServerService(
      projectPath = project,
      clientFactory = InspectionKtsClientFactory {
        FakeInspectionKtsClient(examples = ::markedExamples, projectAnalysis = { code ->
          markedProjectAnalysis(code).also { synchronized(analyses) { analyses += checkNotNull(it.compilation.inspectionId) } }
        })
      },
      serverLifecycle = FakeIntellijMcpServerLifecycle(),
    )
    return EdictGenerationService(
      repository,
      inspection,
      project,
      maxProjectAnalyses = 2,
      defaultGenerationCount = defaultGenerationCount,
      reportUploader = reportUploader,
    ) to inspection
  }

  /** A Pending cluster with one Signal whose example expects a finding on line 1 when positive. */
  private fun pendingCluster(
    repository: EdictRepository,
    clusterId: String,
    strength: EdictSignalStrength,
    label: EdictSignalLabel,
    predecessorId: String? = null,
  ) {
    val cluster = directory.resolve("state/clusters/$clusterId").createDirectories()
    cluster.resolve("signals").createDirectories()
    cluster.resolve("cluster.json").writeText(
      EdictJson.encodeToString(
        EdictClusterManifest(clusterId, EdictLanguage.Java, EdictClusterStatus.Pending, predecessorId),
      ),
    )
    cluster.resolve("history.md").writeText("Created.\n")
    val signalId = "s-$clusterId"
    writeSignal(clusterId, signalId, strength, label)
    val ranges = if (label == EdictSignalLabel.POSITIVE) listOf(EdictLineRange(1, 1)) else emptyList()
    val example = EdictCodeExampleMetadata("$clusterId-example", "Example.java", label, ranges)
    repository.saveCodeExample(clusterId, example.id, example, "class Example {}")
    repository.assignCodeExample(clusterId, signalId, example.id)
  }

  private fun writeSignal(
    clusterId: String,
    signalId: String,
    strength: EdictSignalStrength,
    label: EdictSignalLabel,
    path: String = "$signalId.java",
    revision: String = "abc",
    source: EdictSignalSource = EdictSignalSource.SubmittedFeedback(),
  ) {
    directory.resolve("state/clusters/$clusterId/signals/$signalId.json").writeText(
      EdictJson.encodeToString(
        EdictSignal(
          id = signalId,
          fileRevision = EdictFileRevision(path, revision, listOf(EdictLineRange(1, 1))),
          source = source,
          label = label,
          strength = strength,
          description = "Example of $clusterId",
        ),
      ),
    )
  }
}

// A candidate `// <cluster id> [report] [skip=<example id>] [findings=N]` names its cluster, unless it has an
// `id = "<id>"`, and reports line 1 of every example except a skipped one when it says ` report`, and reports lines
// 1..N of `src/Found.java` in a project analysis.
private fun markedExamples(code: String, examples: List<InspectionKtsExampleRequest>): InspectionKtsBatchRunResult {
  if ("id = \"unresolved-references\"" in code) return unresolvedReferences(code, examples)
  val problems = if (" report" in code) listOf(InspectionKtsProblem("found", 1, "WARNING")) else emptyList()
  return InspectionKtsBatchRunResult(
    markedCompilation(code),
    examples.map { InspectionKtsFileResult(it.id, it.targetFilePath, if (" skip=${it.id}" in code) emptyList() else problems) },
  )
}

/** The unresolved-reference check, which finds the `unresolved=<name>` markers of each example's source on line 1. */
private fun unresolvedReferences(code: String, examples: List<InspectionKtsExampleRequest>) = InspectionKtsBatchRunResult(
  markedCompilation(code),
  examples.map { example ->
    val source = Path.of(example.projectPath).resolve(example.targetFilePath).readText()
    val names = Regex("unresolved=(\\w+)").findAll(source).map { it.groupValues[1] }
    InspectionKtsFileResult(example.id, example.targetFilePath, names.map { InspectionKtsProblem("Unresolved reference '$it'", 1, "WARNING") }.toList())
  },
)

private fun markedProjectAnalysis(code: String): InspectionKtsProjectRunResult {
  val findings = Regex("findings=(\\d+)").find(code)?.groupValues?.get(1)?.toInt() ?: 0
  val problems = (1..findings).map { InspectionKtsProblem("found", it, "WARNING") }
  return InspectionKtsProjectRunResult(markedCompilation(code), listOf(InspectionKtsFileResult(path = "src/Found.java", foundProblems = problems)))
}

private fun markedCompilation(code: String): InspectionKtsCompileResult {
  val clusterId = Regex("id = \"([^\"]+)\"").find(code)?.groupValues?.get(1) ?: code.removePrefix("// ").substringBefore(' ')
  return InspectionKtsCompileResult(true, inspectionId = clusterId, inspectionName = clusterId, inspectionDescription = clusterId)
}

private val STRONG_POSITIVE = EdictSignalStrength.STRONG to EdictSignalLabel.POSITIVE

/** A user's answer in a review comment to a finding of the inspection bytes of [inspectionDigest]. */
private fun feedbackSource(inspectionDigest: String) = EdictSignalSource.GitHubEvaluatedFinding(
  "Rule", "Reports the rule.", "Breaks the rule", "https://github.com/owner/project/pull/7#discussion_r1", emptyList(), inspectionDigest,
)
