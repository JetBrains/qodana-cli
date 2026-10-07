package org.jetbrains.qodana.edict.edictnext

import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.JsonObject
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class EdictNextInspectionTest {
  @TempDir
  lateinit var directory: Path

  @Test
  fun `scores raw IDE findings against strong examples`() = runBlocking {
    val cluster = cluster()
    val client = FakeInspectionClient(
      examplesResult = batchResult(
        InspectionKtsFileResult("positive", "Positive.kt", listOf(problem(2))),
        InspectionKtsFileResult("negative", "Negative.kt"),
      ),
    )
    val result = EdictNextInspection({ client }, { client.projectResult }).validate(cluster, "inspection")

    assertTrue(result.compilationSuccess)
    assertTrue(result.overallSuccess)
    assertEquals(100, result.achievedAccuracyPercent)
    assertTrue(result.failures.isEmpty())
  }

  @Test
  fun `reports uncovered positives and reported negatives independently`() = runBlocking {
    val cluster = cluster()
    val client = FakeInspectionClient(
      examplesResult = batchResult(
        InspectionKtsFileResult("positive", "Positive.kt"),
        InspectionKtsFileResult("negative", "Negative.kt", listOf(problem(1))),
      ),
    )
    val result = EdictNextInspection({ client }, { client.projectResult }).validate(cluster, "inspection")

    assertFalse(result.overallSuccess)
    assertEquals(listOf("positive"), result.uncoveredPositiveExampleIds)
    assertEquals(listOf("negative"), result.reportedNegativeExampleIds)
    assertEquals(2, result.failures.size)
  }

  @Test
  fun `rejects compiled metadata that does not match cluster`() = runBlocking {
    val cluster = cluster()
    val client = FakeInspectionClient(
      examplesResult = batchResult().copy(
        compilation = successfulCompilation().copy(inspectionId = "different-rule"),
      ),
    )
    val result = EdictNextInspection({ client }, { client.projectResult }).validate(cluster, "inspection")

    assertFalse(result.compilationSuccess)
    assertTrue(result.summary.contains("does not match cluster id"))
  }

  @Test
  fun `converts generic project findings to review findings`() = runBlocking {
    val cluster = cluster()
    val projectResult = InspectionKtsProjectRunResult(
      successfulCompilation(),
      listOf(InspectionKtsFileResult(path = "src/Sample.kt", foundProblems = listOf(problem(7)))),
    )
    val client = FakeInspectionClient(batchResult(), projectResult)
    val result = EdictNextInspection({ client }, { client.projectResult })
      .analyzeProject(cluster, "inspection bytes", "abcdef")

    assertEquals("sample-rule", result.clusterId)
    assertEquals("src/Sample.kt", result.findings.single().fileRevision.path)
    assertEquals(listOf(EdictNextLineRange(7, 7)), result.findings.single().fileRevision.expectedRanges)
    assertEquals("abcdef", result.projectRevision)
  }

  @Test
  fun `evaluation scores strong and weak examples once strong examples pass`() = runBlocking {
    val cluster = cluster(
      example("weak-missed", EdictNextSignalLabel.POSITIVE, listOf(EdictNextLineRange(1, 1))),
      example("weak-reported", EdictNextSignalLabel.NEGATIVE, null),
    )
    val client = FakeInspectionClient(
      examplesResult = batchResult(
        InspectionKtsFileResult("positive", "Positive.kt", listOf(problem(2))),
        InspectionKtsFileResult("negative", "Negative.kt"),
        InspectionKtsFileResult("weak-missed", "WeakMissed.kt"),
        InspectionKtsFileResult("weak-reported", "WeakReported.kt", listOf(problem(1))),
      ),
    )
    val (validation, evaluation) = EdictNextInspection({ client }, { client.projectResult })
      .evaluate(cluster, "inspection", EdictNextInspectionAction.GENERATE)

    assertTrue(validation.overallSuccess)
    checkNotNull(evaluation)
    assertEquals(listOf(1, 1, 1), listOf(evaluation.tp, evaluation.fp, evaluation.fn))
    assertEquals(0.5, evaluation.precision)
    assertEquals(0.5, evaluation.recall)
    assertEquals(listOf("negative", "positive"), evaluation.strongExampleIds)
    assertEquals(
      mapOf("negative" to true, "positive" to true, "weak-missed" to false, "weak-reported" to false),
      evaluation.satisfiedByExampleId,
    )
    assertEquals(sha256Hex("inspection"), evaluation.inspectionHash)
    assertEquals(exampleSetDigest(cluster), evaluation.exampleSetDigest)
  }

  @Test
  fun `evaluation is not produced while a strong example fails`() = runBlocking {
    val client = FakeInspectionClient(
      examplesResult = batchResult(
        InspectionKtsFileResult("positive", "Positive.kt"),
        InspectionKtsFileResult("negative", "Negative.kt"),
      ),
    )
    val (validation, evaluation) = EdictNextInspection({ client }, { client.projectResult })
      .evaluate(cluster(), "inspection", EdictNextInspectionAction.GENERATE)

    assertFalse(validation.overallSuccess)
    assertEquals(null, evaluation)
  }

  @Test
  fun `findings compare by reported locations only`() {
    fun findings(vararg lines: Int, message: String = "problem") = EdictNextInspectionFindings(
      clusterId = "sample-rule",
      candidateDigest = message,
      projectRevision = "abcdef",
      inspectionDescription = message,
      language = EdictNextLanguage.Java,
      findings = lines.map {
        EdictNextProjectFinding(EdictNextFileRevision("src/Sample.java", "abcdef", listOf(EdictNextLineRange(it, it))), message)
      },
    )

    assertEquals(findings(3, 7).reviewKey(), findings(3, 7, message = "reworded").reviewKey())
    assertTrue(findings(3, 7).reviewKey() != findings(3, 8).reviewKey())
    assertTrue(findings(3, 7).reviewKey() != findings(3).reviewKey())
  }

  @Test
  fun `example set digest changes with examples and strong assignments`() {
    val base = cluster()
    val weak = cluster(example("weak", EdictNextSignalLabel.NEGATIVE, null))
    val reassigned = base.copy(signals = base.signals.map { it.copy(syntheticExampleId = null) })

    assertEquals(exampleSetDigest(base), exampleSetDigest(cluster()))
    assertTrue(exampleSetDigest(base) != exampleSetDigest(weak))
    assertTrue(exampleSetDigest(base) != exampleSetDigest(reassigned))
  }

  private fun cluster(vararg weakExamples: EdictNextStoredExample): EdictNextStoredCluster {
    val clusterDirectory = EdictNextClusterDirectory(directory.resolve("sample-rule"))
    val positive = example("positive", EdictNextSignalLabel.POSITIVE, listOf(EdictNextLineRange(2, 2)))
    val negative = example("negative", EdictNextSignalLabel.NEGATIVE, null)
    return EdictNextStoredCluster(
      directory = clusterDirectory,
      manifest = EdictNextClusterManifest("sample-rule", EdictNextLanguage.Kotlin, EdictNextClusterStatus.Pending),
      signals = listOf(signal("positive-signal", EdictNextSignalLabel.POSITIVE, "positive"), signal("negative-signal", EdictNextSignalLabel.NEGATIVE, "negative")),
      examples = listOf(positive, negative) + weakExamples,
      candidateInspectionPath = directory.resolve("sample-rule.candidate.kts"),
    )
  }

  private fun example(
    id: String,
    label: EdictNextSignalLabel,
    ranges: List<EdictNextLineRange>?,
  ): EdictNextStoredExample {
    val exampleDirectory = EdictNextExampleDirectory(directory.resolve(id))
    val source = exampleDirectory.projectDirectory.resolve("$id.kt")
    return EdictNextStoredExample(
      directory = exampleDirectory,
      metadata = EdictNextCodeExampleMetadata(id, "$id.kt", label, ranges),
      sourcePath = source,
      code = "fun first() = Unit\nfun second() = Unit\n",
    )
  }

  private fun signal(id: String, label: EdictNextSignalLabel, exampleId: String): EdictNextSignal = EdictNextSignal(
    id = id,
    fileRevision = EdictNextFileRevision("src/$id.kt", "abcdef"),
    source = EdictNextSignalSource.Generated(),
    label = label,
    description = id,
    syntheticExampleId = exampleId,
  )

  private fun successfulCompilation() = InspectionKtsCompileResult(
    compilationSuccess = true,
    inspectionId = "sample-rule",
    inspectionName = "Sample rule",
    inspectionDescription = "Finds samples",
  )

  private fun batchResult(vararg files: InspectionKtsFileResult) =
    InspectionKtsBatchRunResult(successfulCompilation(), files.toList())

  private fun problem(line: Int) = InspectionKtsProblem("problem", line, "WARNING")

  private class FakeInspectionClient(
    private val examplesResult: InspectionKtsBatchRunResult,
    val projectResult: InspectionKtsProjectRunResult = InspectionKtsProjectRunResult(examplesResult.compilation),
  ) : InspectionKtsClient {
    override suspend fun compile(code: String): InspectionKtsCompileResult = examplesResult.compilation
    override suspend fun runExamples(code: String, examples: List<InspectionKtsExampleRequest>): InspectionKtsBatchRunResult = examplesResult
    override suspend fun analyzeProject(code: String): InspectionKtsProjectRunResult = projectResult
    override suspend fun callTool(name: String, arguments: JsonObject): JsonObject = error("not used")
    override fun close() = Unit
  }
}
