package org.jetbrains.qodana.edict.core

import kotlinx.coroutines.runBlocking
import org.jetbrains.qodana.edict.support.FakeInspectionKtsClient
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class EdictInspectionTest {
  @TempDir
  lateinit var directory: Path

  @Test
  fun `scores raw IDE findings against strong examples`() = runBlocking {
    val cluster = cluster()
    val client = ideClient(
      examples = batchResult(
        InspectionKtsFileResult("positive", "Positive.kt", listOf(problem(2))),
        InspectionKtsFileResult("negative", "Negative.kt"),
      ),
    )
    val result = EdictInspection({ client }, client::analyzeProject).measure(cluster, "inspection").validation

    assertTrue(result.compilationSuccess)
    assertTrue(result.overallSuccess)
    assertEquals(
      "2/2 strong evidence cases correct (advisory); 0/0 weak evidence cases correct (advisory); " +
        "0 uncovered positive(s), 0 reported negative(s)",
      result.summary,
    )
    assertEquals(100, result.achievedAccuracyPercent)
    assertTrue(result.failures.isEmpty())
  }

  @Test
  fun `reports uncovered positives and reported negatives independently`() = runBlocking {
    val cluster = cluster()
    val client = ideClient(
      examples = batchResult(
        InspectionKtsFileResult("positive", "Positive.kt"),
        InspectionKtsFileResult("negative", "Negative.kt", listOf(problem(1))),
      ),
    )
    val result = EdictInspection({ client }, client::analyzeProject).measure(cluster, "inspection").validation

    assertTrue(result.overallSuccess)
    assertEquals(EdictNextAction.REVIEW_INSPECTION, result.nextAction)
    assertEquals(listOf("positive"), result.uncoveredPositiveExampleIds)
    assertEquals(listOf("negative"), result.reportedNegativeExampleIds)
    assertEquals(2, result.failures.size)
  }

  @Test
  fun `examples run in the module of the source they were reduced from`() = runBlocking {
    val fromFinding = example("from-finding", EdictSignalLabel.POSITIVE, listOf(EdictLineRange(1, 1)))
      .let { it.copy(metadata = it.metadata.copy(contextPath = "src/Finding.kt")) }
    val unlinked = example("unlinked", EdictSignalLabel.NEGATIVE, null)
    val results = batchResult(
      InspectionKtsFileResult("positive", "Positive.kt", listOf(problem(2))),
      InspectionKtsFileResult("negative", "Negative.kt"),
      InspectionKtsFileResult("from-finding", "FromFinding.kt", listOf(problem(1))),
      InspectionKtsFileResult("unlinked", "Unlinked.kt"),
    )
    val requests = mutableListOf<InspectionKtsExampleRequest>()
    val client = FakeInspectionKtsClient(
      compilation = { results.compilation },
      examples = { _, examples -> results.also { requests += examples } },
    )
    EdictInspection({ client }, client::analyzeProject).measure(cluster(fromFinding, unlinked), "inspection").validation

    assertEquals(
      mapOf(
        "positive" to "src/positive-signal.kt",
        "negative" to "src/negative-signal.kt",
        "from-finding" to "src/Finding.kt",
        "unlinked" to "src/negative-signal.kt",
      ),
      requests.associate { it.id to it.contextPath },
    )
  }

  @Test
  fun `the unresolved references of each example are reported with the module it runs in`() = runBlocking {
    val results = batchResult(
      InspectionKtsFileResult("positive", "Positive.kt", listOf(InspectionKtsProblem("Unresolved reference 'extensionList'", 4, "WARNING"))),
      InspectionKtsFileResult("negative", "Negative.kt"),
    )
    val client = FakeInspectionKtsClient(compilation = { results.compilation }, examples = { _, _ -> results })

    assertEquals(
      mapOf("positive" to EdictUnresolvedReferences("src/positive-signal.kt", listOf("line 4 'extensionList'"))),
      EdictInspection({ client }, client::analyzeProject).unresolvedReferences(cluster()),
    )
  }

  @Test
  fun `an example the unresolved-reference check returns nothing for is reported`() = runBlocking {
    val results = batchResult(InspectionKtsFileResult("positive", "Positive.kt"))
    val client = FakeInspectionKtsClient(compilation = { results.compilation }, examples = { _, _ -> results })

    assertEquals(
      mapOf("negative" to EdictUnresolvedReferences("src/negative-signal.kt", listOf("the check returned no result"))),
      EdictInspection({ client }, client::analyzeProject).unresolvedReferences(cluster()),
    )
  }

  @Test
  fun `rejects compiled metadata that does not match cluster`() = runBlocking {
    val cluster = cluster()
    val client = ideClient(
      examples = batchResult().copy(
        compilation = successfulCompilation().copy(inspectionId = "different-rule"),
      ),
    )
    val result = EdictInspection({ client }, client::analyzeProject).measure(cluster, "inspection").validation

    assertFalse(result.compilationSuccess)
    assertFalse(result.overallSuccess)
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
      val client = ideClient(result)
      val (validation, evaluation) = EdictInspection({ client }, client::analyzeProject)
        .evaluate(cluster(), "inspection", emptyMap(), REVIEW)
      assertFalse(validation.overallSuccess)
      assertEquals(EdictNextAction.REPAIR_INSPECTION, validation.nextAction)
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
    val client = ideClient(batchResult(), projectResult)
    val result = EdictInspection({ client }, client::analyzeProject)
      .analyzeProject(cluster, "inspection bytes", "abcdef")

    assertEquals("sample-rule", result.clusterId)
    assertEquals("src/Sample.kt", result.findings.single().fileRevision.path)
    assertEquals(listOf(EdictLineRange(7, 7)), result.findings.single().fileRevision.expectedRanges)
    assertEquals("abcdef", result.projectRevision)
  }

  @Test
  fun `evaluation weighs every example by its assigned project findings`() = runBlocking {
    val cluster = cluster(
      example("weak-missed", EdictSignalLabel.POSITIVE, listOf(EdictLineRange(1, 1))),
      example("weak-reported", EdictSignalLabel.NEGATIVE, null),
    )
    val client = ideClient(
      examples = batchResult(
        InspectionKtsFileResult("positive", "Positive.kt", listOf(problem(2))),
        InspectionKtsFileResult("negative", "Negative.kt"),
        InspectionKtsFileResult("weak-missed", "WeakMissed.kt"),
        InspectionKtsFileResult("weak-reported", "WeakReported.kt", listOf(problem(1))),
      ),
    )
    val review = EdictProjectReview("abcdef", totalFindings = 40, reviewedFindings = 12)
    val (validation, evaluation) = EdictInspection({ client }, client::analyzeProject).evaluate(
      cluster,
      "inspection",
      attributed = mapOf("positive" to 6, "weak-missed" to 2, "weak-reported" to 3),
      projectReview = review,
    )

    assertTrue(validation.overallSuccess)
    checkNotNull(evaluation)
    assertEquals("Sample rule", evaluation.inspectionName)
    assertEquals("Finds samples", evaluation.inspectionDescription)
    // The negative strong example has no findings and still weighs 1; it is satisfied, so it adds to no count.
    assertEquals(listOf(6, 3, 2), listOf(evaluation.tp, evaluation.fp, evaluation.fn))
    assertEquals(6.0 / 9, evaluation.precision)
    assertEquals(6.0 / 8, evaluation.recall)
    assertEquals(
      mapOf(
        "negative" to EdictExampleEvaluation(EdictSignalLabel.NEGATIVE, satisfied = true, projectFindings = 0),
        "positive" to EdictExampleEvaluation(EdictSignalLabel.POSITIVE, satisfied = true, projectFindings = 6),
        "weak-missed" to EdictExampleEvaluation(EdictSignalLabel.POSITIVE, satisfied = false, projectFindings = 2),
        "weak-reported" to EdictExampleEvaluation(EdictSignalLabel.NEGATIVE, satisfied = false, projectFindings = 3),
      ),
      evaluation.examples,
    )
    assertEquals(review, evaluation.projectReview)
    assertEquals(sha256Hex("inspection"), evaluation.inspectionHash)
    assertEquals(exampleSetDigest(cluster), evaluation.exampleSetDigest)
  }

  @Test
  fun `without attributed findings every example weighs 1`() = runBlocking {
    val cluster = cluster(example("weak-missed", EdictSignalLabel.POSITIVE, listOf(EdictLineRange(1, 1))))
    val client = ideClient(
      examples = batchResult(
        InspectionKtsFileResult("positive", "Positive.kt", listOf(problem(2))),
        InspectionKtsFileResult("negative", "Negative.kt"),
        InspectionKtsFileResult("weak-missed", "WeakMissed.kt"),
      ),
    )
    val (_, evaluation) = EdictInspection({ client }, client::analyzeProject)
      .evaluate(cluster, "inspection", emptyMap(), REVIEW)

    checkNotNull(evaluation)
    assertEquals(listOf(1, 0, 1), listOf(evaluation.tp, evaluation.fp, evaluation.fn))
    assertTrue(evaluation.examples.values.all { it.weight == 1 && it.projectFindings == 0 })
  }

  @Test
  fun `evaluation records failing strong positive and negative examples without blocking generation`() = runBlocking {
    val client = ideClient(
      examples = batchResult(
        InspectionKtsFileResult("positive", "Positive.kt"),
        InspectionKtsFileResult("negative", "Negative.kt", listOf(problem(1))),
      ),
    )
    val (validation, evaluation) = EdictInspection({ client }, client::analyzeProject)
      .evaluate(cluster(), "inspection", emptyMap(), REVIEW)

    assertTrue(validation.overallSuccess)
    checkNotNull(evaluation)
    assertEquals(listOf(0, 1, 1), listOf(evaluation.tp, evaluation.fp, evaluation.fn))
    assertEquals(0.0, evaluation.precision)
    assertEquals(0.0, evaluation.recall)
    assertTrue(evaluation.examples.values.all { !it.satisfied })
  }

  @Test
  fun `findings compare by reported locations only`() {
    assertEquals(findings(3, 7).locations(), findings(3, 7, message = "reworded").locations())
    assertEquals(findings(3, 7).locations(), findings(3, 7, 7).locations())
    assertNotEquals(findings(3, 7).locations(), findings(3, 8).locations())
    assertNotEquals(findings(3, 7).locations(), findings(3).locations())
  }

  @Test
  fun `the example set digest changes with the examples, not with the Signals assigned to them`() {
    val base = cluster()
    val weak = cluster(example("weak", EdictSignalLabel.NEGATIVE, null))
    val reassigned = base.copy(signals = base.signals.map { it.copy(syntheticExampleId = null) })

    assertTrue(exampleSetDigest(base) != exampleSetDigest(weak))
    assertEquals(exampleSetDigest(base), exampleSetDigest(reassigned))
  }

  private fun cluster(vararg weakExamples: EdictStoredExample): EdictStoredCluster {
    val clusterDirectory = EdictClusterDirectory(directory.resolve("sample-rule"))
    val positive = example("positive", EdictSignalLabel.POSITIVE, listOf(EdictLineRange(2, 2)))
    val negative = example("negative", EdictSignalLabel.NEGATIVE, null)
    return EdictStoredCluster(
      directory = clusterDirectory,
      manifest = EdictClusterManifest("sample-rule", EdictLanguage.Kotlin, EdictClusterStatus.Pending),
      signals = listOf(signal("positive-signal", EdictSignalLabel.POSITIVE, "positive"), signal("negative-signal", EdictSignalLabel.NEGATIVE, "negative")),
      examples = listOf(positive, negative) + weakExamples,
      candidateInspectionPath = directory.resolve("sample-rule.candidate.kts"),
    )
  }

  private fun example(
    id: String,
    label: EdictSignalLabel,
    ranges: List<EdictLineRange>?,
  ): EdictStoredExample {
    val exampleDirectory = EdictExampleDirectory(directory.resolve(id))
    val source = exampleDirectory.projectDirectory.resolve("$id.kt")
    return EdictStoredExample(
      directory = exampleDirectory,
      metadata = EdictCodeExampleMetadata(id, "$id.kt", label, ranges),
      sourcePath = source,
      code = "fun first() = Unit\nfun second() = Unit\n",
    )
  }

  private fun signal(id: String, label: EdictSignalLabel, exampleId: String): EdictSignal = EdictSignal(
    id = id,
    fileRevision = EdictFileRevision("src/$id.kt", "abcdef"),
    source = EdictSignalSource.Generated(),
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
}

/** An IDE that compiles every inspection as [examples] did, finds [examples] in the examples and [project] in the project. */
private fun ideClient(
  examples: InspectionKtsBatchRunResult,
  project: InspectionKtsProjectRunResult = InspectionKtsProjectRunResult(examples.compilation),
) = FakeInspectionKtsClient(compilation = { examples.compilation }, examples = { _, _ -> examples }, projectAnalysis = { project })

/** A project analysis of the inspection that reported no finding. */
private val REVIEW = EdictProjectReview("abcdef", totalFindings = 0, reviewedFindings = 0)

/** Findings of one candidate on the [lines] of one file, each with [message]. */
private fun findings(vararg lines: Int, message: String = "problem") = EdictInspectionFindings(
  clusterId = "sample-rule",
  candidateDigest = message,
  projectRevision = "abcdef",
  inspectionDescription = message,
  language = EdictLanguage.Java,
  findings = lines.map {
    EdictProjectFinding(EdictFileRevision("src/Sample.java", "abcdef", listOf(EdictLineRange(it, it))), message)
  },
)
