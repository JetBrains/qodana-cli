package org.jetbrains.qodana.edict.edictnext

import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.JsonObject
import org.jetbrains.qodana.edict.common.runProcess
import org.jetbrains.qodana.edict.support.gitFixture
import org.junit.jupiter.api.io.TempDir
import java.net.URI
import java.nio.file.Files
import java.nio.file.Path
import kotlin.io.path.appendText
import kotlin.io.path.createDirectories
import kotlin.io.path.createParentDirectories
import kotlin.io.path.deleteIfExists
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
    val error = assertFailsWith<IllegalStateException> { generation.getNewInspectionResults("busy-wait", scratch) }
    assertContains(error.message.orEmpty(), "used all 2 project analyses")
    // Another cluster keeps its own count, and a refused call starts no analysis.
    assertEquals(1, generation.getNewInspectionResults("empty-catch", scratch).remainingProjectAnalyses)
    assertEquals(listOf("busy-wait", "busy-wait", "empty-catch"), analyses)
  }

  @Test
  fun `clusters without a strong positive Signal stay Pending without generation`() = runBlocking<Unit> {
    val (generation, inspection) = generation(
      "weak-positive" to (EdictNextSignalStrength.WEAK to EdictNextSignalLabel.POSITIVE),
      "strong-negative" to (EdictNextSignalStrength.STRONG to EdictNextSignalLabel.NEGATIVE),
    )
    try {
      val response = generation.getGenerationClusters()
      assertEquals(listOf("busy-wait"), response.clusters.map { it.clusterId })
      assertFailsWith<IllegalStateException> { generation.saveCandidateInspection("weak-positive", "// weak-positive report") }
    }
    finally {
      inspection.stop()
    }
  }

  @Test
  fun `requested generation count overrides default and selects Pending clusters with the most positive Signals`() = runBlocking<Unit> {
    val strongPositive = EdictNextSignalStrength.STRONG to EdictNextSignalLabel.POSITIVE
    val strongNegative = EdictNextSignalStrength.STRONG to EdictNextSignalLabel.NEGATIVE
    val (generation, inspection) = generation(
      "most" to strongPositive,
      "tie-b" to strongPositive,
      "tie-a" to strongPositive,
      "negative-heavy" to strongNegative,
      defaultGenerationCount = 1,
    )
    writeSignal("most", "s-most-2", EdictNextSignalStrength.WEAK, EdictNextSignalLabel.POSITIVE)
    writeSignal("most", "s-most-3", EdictNextSignalStrength.WEAK, EdictNextSignalLabel.POSITIVE)
    writeSignal("tie-a", "s-tie-a-2", EdictNextSignalStrength.WEAK, EdictNextSignalLabel.POSITIVE)
    writeSignal("tie-b", "s-tie-b-2", EdictNextSignalStrength.WEAK, EdictNextSignalLabel.POSITIVE)
    writeSignal("tie-b", "s-tie-b-3", EdictNextSignalStrength.STRONG, EdictNextSignalLabel.NEGATIVE)
    writeSignal("tie-b", "s-tie-b-4", EdictNextSignalStrength.STRONG, EdictNextSignalLabel.NEGATIVE)
    writeSignal("negative-heavy", "s-negative-heavy-2", EdictNextSignalStrength.STRONG, EdictNextSignalLabel.NEGATIVE)
    writeSignal("negative-heavy", "s-negative-heavy-3", EdictNextSignalStrength.STRONG, EdictNextSignalLabel.NEGATIVE)

    try {
      val response = generation.getGenerationClusters(generationCount = 2)
      assertEquals(listOf("most", "tie-a"), response.clusters.map { it.clusterId })
      val error = assertFailsWith<IllegalStateException> {
        generation.saveCandidateInspection("tie-b", "// tie-b report")
      }
      assertContains(error.message.orEmpty(), "not a frozen generation target")
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
    writeSignal("second", "s-second-2", EdictNextSignalStrength.WEAK, EdictNextSignalLabel.POSITIVE)

    try {
      val response = generation.getGenerationClusters()
      assertEquals(listOf("second"), response.clusters.map { it.clusterId })
      val error = assertFailsWith<IllegalArgumentException> { generation.getGenerationClusters(generationCount = 0) }
      assertContains(error.message.orEmpty(), "generationCount must be at least 1")
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
      generation.getGenerationClusters()
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
      val validation = generation.validateGeneration()
      assertTrue(validation.success, validation.issues.toString())
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
      generation.getGenerationClusters()
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
      generation.getGenerationClusters()
      generation.saveCandidateInspection("discontinued", "// discontinued report")
      assertTrue(generation.finaliseCluster("discontinued", EdictNextClusterStatus.Discontinued, "Contradiction").success)
      val cluster = repository.loadCluster("discontinued")
      assertEquals(null, cluster.manifest.predecessorId)
      assertFalse(repository.paths.inspectionPath("discontinued").exists())
      assertFalse(cluster.candidateInspectionPath.exists())
      val validation = generation.validateGeneration()
      assertTrue(validation.success, validation.issues.toString())
    }
    finally {
      inspection.stop()
    }
  }

  @Test
  fun `generation validation follows a renamed target by its Signals`() = runBlocking<Unit> {
    val (generation, inspection) = generation(
      "old-name" to (EdictNextSignalStrength.STRONG to EdictNextSignalLabel.POSITIVE),
      predecessorId = "old-name",
    )
    try {
      generation.getGenerationClusters()
      renameCluster("old-name", "new-name", EdictNextClusterStatus.Generated)
      directory.resolve("state/inspections/old-name$EDICT_NEXT_INSPECTION_SUFFIX").deleteIfExists()
      directory.resolve("state/inspections/new-name$EDICT_NEXT_INSPECTION_SUFFIX").writeText("// new-name report")
      val validation = generation.validateGeneration()
      assertTrue(validation.success, validation.issues.toString())

      // A renamed Pending target still owes its frozen predecessor.
      renameCluster("new-name", "pending-name", EdictNextClusterStatus.Pending)
      directory.resolve("state/inspections/new-name$EDICT_NEXT_INSPECTION_SUFFIX").deleteIfExists()
      assertContains(
        generation.validateGeneration().issues.map { it.message },
        "Pending or Invalid generation target must retain its frozen predecessor",
      )
    }
    finally {
      inspection.stop()
    }
  }

  /**
   * Renames a cluster the way a rename transition would: its directory and manifest, keeping Signals and examples.
   * The new manifest has [status] and no predecessor.
   */
  private fun renameCluster(from: String, to: String, status: EdictNextClusterStatus) {
    val target = directory.resolve("state/clusters/$to")
    Files.move(directory.resolve("state/clusters/$from"), target)
    target.resolve("cluster.json").writeText(
      EdictNextJson.encodeToString(EdictNextClusterManifest(to, EdictNextLanguage.Java, status)),
    )
    target.resolve("history.md").appendText("Renamed from `$from`.\n")
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
    assertFailsWith<IllegalStateException> { generation.appendHistory("busy-wait", "Old id") }

    // Only the id changed, so the analyzed candidate can be Generated without another analysis.
    assertTrue(generation.recordEvaluation("busy-loop").success)
    val finalised = generation.finaliseCluster("busy-loop", EdictNextClusterStatus.Generated, "Accepted")
    assertTrue(finalised.success, finalised.toString())
    assertTrue(repository.paths.inspectionPath("busy-loop").isRegularFile())
    assertEquals(listOf("busy-wait"), analyses)
    val validation = generation.validateGeneration()
    assertTrue(validation.success, validation.issues.toString())
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

  private fun withGeneration(test: suspend (EdictNextGenerationService, EdictRepository) -> Unit) = runBlocking {
    val (generation, inspection) = generation("empty-catch" to (EdictNextSignalStrength.STRONG to EdictNextSignalLabel.POSITIVE))
    try {
      generation.getGenerationClusters()
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

  private fun writeSignal(clusterId: String, signalId: String, strength: EdictNextSignalStrength, label: EdictNextSignalLabel) {
    directory.resolve("state/clusters/$clusterId/signals/$signalId.json").writeText(
      EdictNextJson.encodeToString(
        EdictNextSignal(
          id = signalId,
          fileRevision = EdictNextFileRevision("Example.java", "abc", listOf(EdictNextLineRange(1, 1))),
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
