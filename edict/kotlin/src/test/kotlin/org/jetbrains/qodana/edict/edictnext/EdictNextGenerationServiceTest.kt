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
      assertEquals(listOf("strong-negative", "weak-positive"), response.clustersWithoutStrongPositiveSignal.sorted())
      assertFailsWith<IllegalStateException> { generation.saveCandidateInspection("weak-positive", "// weak-positive report") }
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
    return EdictNextGenerationService(repository, inspection, project, maxProjectAnalyses = 2) to inspection
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
   * A candidate `// <cluster id> [report]` names its cluster, unless it has an `id = "<id>"`, and reports line 1 of every
   * example when it says ` report`.
   */
  private class FakeIdeClient(private val analyses: MutableList<String>) : InspectionKtsClient {
    override suspend fun runExamples(code: String, examples: List<InspectionKtsExampleRequest>): InspectionKtsBatchRunResult {
      val problems = if (" report" in code) listOf(InspectionKtsProblem("found", 1, "WARNING")) else emptyList()
      return InspectionKtsBatchRunResult(
        compilation(code),
        examples.map { InspectionKtsFileResult(it.id, it.targetFilePath, problems) },
      )
    }

    override suspend fun analyzeProject(code: String): InspectionKtsProjectRunResult {
      val compilation = compilation(code)
      synchronized(analyses) { analyses += checkNotNull(compilation.inspectionId) }
      return InspectionKtsProjectRunResult(compilation)
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
