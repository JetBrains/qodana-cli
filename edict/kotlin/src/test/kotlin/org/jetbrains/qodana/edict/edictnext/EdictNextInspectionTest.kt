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
    assertTrue(result.strongExamplesSatisfied)
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

    assertTrue(result.overallSuccess)
    assertFalse(result.strongExamplesSatisfied)
    assertEquals(EdictNextNextAction.REVIEW_INSPECTION, result.nextAction)
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
    assertFalse(result.overallSuccess)
    assertFalse(result.strongExamplesSatisfied)
    assertTrue(result.summary.contains("does not match cluster id"))
  }

  @Test
  fun `compilation and example execution failures still prevent generation and evaluation`() = runBlocking {
    val results = listOf(
      batchResult().copy(compilation = successfulCompilation().copy(compilationSuccess = false, compilationStatus = "Compile error")),
      batchResult(),
      batchResult(
        InspectionKtsFileResult("positive", "Positive.kt", executionError = "Execution error"),
        InspectionKtsFileResult("negative", "Negative.kt"),
      ),
      batchResult(
        InspectionKtsFileResult("positive", "Positive.kt", listOf(problem(2))),
        InspectionKtsFileResult("positive", "Positive.kt", listOf(problem(2))),
        InspectionKtsFileResult("negative", "Negative.kt"),
      ),
    )
    for (result in results) {
      val client = FakeInspectionClient(result)
      val (validation, evaluation) = EdictNextInspection({ client }, { client.projectResult })
        .evaluate(cluster(), "inspection", EdictNextInspectionAction.GENERATE)
      assertFalse(validation.overallSuccess)
      assertFalse(validation.strongExamplesSatisfied)
      assertEquals(EdictNextNextAction.REPAIR_INSPECTION, validation.nextAction)
      assertEquals(null, evaluation)
    }
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
  fun `evaluation weighs every example by its assigned project findings`() = runBlocking {
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
    val review = EdictNextProjectReview("abcdef", totalFindings = 40, reviewedFindings = 12, uncertain = 1, unassigned = 0, unreproduced = 0)
    fun assigned(exampleId: String, count: Int, classification: EdictNextFindingClassification) =
      List(count) { EdictNextFindingAssignment(exampleId, classification, stillReported = false) }
    val (validation, evaluation) = EdictNextInspection({ client }, { client.projectResult }).evaluate(
      cluster,
      "inspection",
      EdictNextInspectionAction.GENERATE,
      assignments = assigned("positive", 6, EdictNextFindingClassification.TP) +
        assigned("weak-missed", 2, EdictNextFindingClassification.TP) +
        assigned("weak-reported", 3, EdictNextFindingClassification.FP),
      projectReview = review,
    )

    assertTrue(validation.overallSuccess)
    checkNotNull(evaluation)
    // The negative strong example has no findings and still weighs 1; it is satisfied, so it adds to no count.
    assertEquals(listOf(6, 3, 2), listOf(evaluation.tp, evaluation.fp, evaluation.fn))
    assertEquals(6.0 / 9, evaluation.precision)
    assertEquals(6.0 / 8, evaluation.recall)
    assertEquals(
      mapOf(
        "negative" to EdictNextExampleEvaluation(EdictNextSignalLabel.NEGATIVE, strong = true, satisfied = true, projectFindings = 0, weight = 1),
        "positive" to EdictNextExampleEvaluation(EdictNextSignalLabel.POSITIVE, strong = true, satisfied = true, projectFindings = 6, weight = 6),
        "weak-missed" to EdictNextExampleEvaluation(EdictNextSignalLabel.POSITIVE, strong = false, satisfied = false, projectFindings = 2, weight = 2),
        "weak-reported" to EdictNextExampleEvaluation(EdictNextSignalLabel.NEGATIVE, strong = false, satisfied = false, projectFindings = 3, weight = 3),
      ),
      evaluation.examples,
    )
    assertEquals(review, evaluation.projectReview)
    assertEquals(sha256Hex("inspection"), evaluation.inspectionHash)
    assertEquals(exampleSetDigest(cluster), evaluation.exampleSetDigest)
  }

  @Test
  fun `a still reported finding its example does not reproduce counts by its classification`() = runBlocking {
    val cluster = cluster()
    val client = FakeInspectionClient(
      examplesResult = batchResult(
        InspectionKtsFileResult("positive", "Positive.kt", listOf(problem(2))),
        InspectionKtsFileResult("negative", "Negative.kt"),
      ),
    )
    val review = EdictNextProjectReview("abcdef", totalFindings = 10, reviewedFindings = 3, uncertain = 0, unassigned = 0, unreproduced = 0)
    val (_, evaluation) = EdictNextInspection({ client }, { client.projectResult }).evaluate(
      cluster,
      "inspection",
      EdictNextInspectionAction.GENERATE,
      assignments = listOf(
        // The inspection no longer reports the negative example but still reports this finding: a real false positive.
        EdictNextFindingAssignment("negative", EdictNextFindingClassification.FP, stillReported = true),
        // Fixed together with the negative example, so it only weighs it.
        EdictNextFindingAssignment("negative", EdictNextFindingClassification.FP, stillReported = false),
        EdictNextFindingAssignment("positive", EdictNextFindingClassification.TP, stillReported = true),
      ),
      projectReview = review,
    )

    checkNotNull(evaluation)
    assertEquals(listOf(1, 1, 0), listOf(evaluation.tp, evaluation.fp, evaluation.fn))
    assertEquals(1, evaluation.examples.getValue("negative").projectFindings)
    assertEquals(1, evaluation.projectReview?.unreproduced)
  }

  @Test
  fun `without review data every example weighs 1`() = runBlocking {
    val cluster = cluster(example("weak-missed", EdictNextSignalLabel.POSITIVE, listOf(EdictNextLineRange(1, 1))))
    val client = FakeInspectionClient(
      examplesResult = batchResult(
        InspectionKtsFileResult("positive", "Positive.kt", listOf(problem(2))),
        InspectionKtsFileResult("negative", "Negative.kt"),
        InspectionKtsFileResult("weak-missed", "WeakMissed.kt"),
      ),
    )
    val (_, evaluation) = EdictNextInspection({ client }, { client.projectResult })
      .evaluate(cluster, "inspection", EdictNextInspectionAction.SKIP)

    checkNotNull(evaluation)
    assertEquals(listOf(1, 0, 1), listOf(evaluation.tp, evaluation.fp, evaluation.fn))
    assertTrue(evaluation.examples.values.all { it.weight == 1 && it.projectFindings == 0 })
    assertEquals(null, evaluation.projectReview)
  }

  @Test
  fun `evaluation records failing strong positive and negative examples without blocking generation`() = runBlocking {
    val client = FakeInspectionClient(
      examplesResult = batchResult(
        InspectionKtsFileResult("positive", "Positive.kt"),
        InspectionKtsFileResult("negative", "Negative.kt", listOf(problem(1))),
      ),
    )
    val (validation, evaluation) = EdictNextInspection({ client }, { client.projectResult })
      .evaluate(cluster(), "inspection", EdictNextInspectionAction.GENERATE)

    assertTrue(validation.overallSuccess)
    assertFalse(validation.strongExamplesSatisfied)
    checkNotNull(evaluation)
    assertEquals(listOf(0, 1, 1), listOf(evaluation.tp, evaluation.fp, evaluation.fn))
    assertEquals(0.0, evaluation.precision)
    assertEquals(0.0, evaluation.recall)
    assertTrue(evaluation.examples.values.all { it.strong && !it.satisfied })
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
