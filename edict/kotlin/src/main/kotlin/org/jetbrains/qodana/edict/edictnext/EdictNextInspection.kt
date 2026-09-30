package org.jetbrains.qodana.edict.edictnext

/** Keeps cluster semantics outside IntelliJ; the IDE returns only generic compiler and execution results. */
internal class EdictNextInspection(
  private val client: suspend () -> InspectionKtsClient,
  private val projectAnalysis: suspend (String) -> InspectionKtsProjectRunResult,
  private val exampleAnalysis: suspend (String, List<InspectionKtsExampleRequest>) -> InspectionKtsBatchRunResult =
    { code, requests -> client().runExamples(code, requests) },
) {
  constructor(server: IntellijMcpServerService) : this(
    client = { server.start() },
    projectAnalysis = { code -> server.waitForAnalysis { it.analyzeProject(code) } },
    exampleAnalysis = { code, requests -> server.withClient { it.runExamples(code, requests) } },
  )

  suspend fun validate(cluster: EdictNextStoredCluster, code: String): EdictNextInspectionValidationResponse {
    val requests = cluster.examples.map { example ->
      InspectionKtsExampleRequest(
        id = example.metadata.id,
        projectPath = example.directory.projectDirectory.toString(),
        targetFilePath = example.directory.projectDirectory.relativize(example.sourcePath).toString(),
      )
    }
    val execution = exampleAnalysis(code, requests)
    rejectMetadata(cluster.id, execution.compilation)?.let { return it }

    val failures = mutableListOf<EdictNextInspectionFailure>()
    val outcomesById = cluster.examples.associate { example ->
      val result = execution.files.singleOrNull { it.id == example.metadata.id }
      val outcome = evaluateExample(example, result)
      failures += outcome.failures
      example.metadata.id to outcome
    }
    val examplesById = cluster.examples.associateBy { it.metadata.id }
    val scoredExampleIds = examplesById.keys
    val strongExampleIds = cluster.signals.asSequence()
      .filter { it.strength == EdictNextSignalStrength.STRONG }
      .mapNotNull(EdictNextSignal::syntheticExampleId)
      .toSet()
    val weakExampleIds = scoredExampleIds - strongExampleIds
    val strongPositiveCount = strongExampleIds.count { examplesById[it]?.metadata?.label == EdictNextSignalLabel.POSITIVE }
    val strongCorrectCount = strongExampleIds.count { outcomesById[it]?.satisfied == true }
    val weakCorrectCount = weakExampleIds.count { outcomesById[it]?.satisfied == true }
    val correctCount = scoredExampleIds.count { outcomesById[it]?.satisfied == true }
    val achieved = if (scoredExampleIds.isEmpty()) 0 else correctCount * 100 / scoredExampleIds.size
    val uncovered = cluster.examples.filter {
      it.metadata.label == EdictNextSignalLabel.POSITIVE && outcomesById[it.metadata.id]?.satisfied != true
    }.map { it.metadata.id }.sorted()
    val reportedNegatives = cluster.examples.filter {
      it.metadata.label == EdictNextSignalLabel.NEGATIVE && outcomesById[it.metadata.id]?.satisfied != true
    }.map { it.metadata.id }.sorted()
    val accepted = strongPositiveCount > 0 && strongCorrectCount == strongExampleIds.size
    val summary = if (strongPositiveCount == 0) {
      "The cluster has no strong positive evidence case, so no rule can be accepted for it"
    }
    else {
      "$strongCorrectCount/${strongExampleIds.size} strong evidence cases correct (all required); " +
      "$weakCorrectCount/${weakExampleIds.size} weak evidence cases correct (advisory); " +
      "${uncovered.size} uncovered positive(s), ${reportedNegatives.size} reported negative(s)"
    }
    return EdictNextInspectionValidationResponse(
      compilationSuccess = true,
      overallSuccess = accepted,
      summary = summary,
      failures = failures,
      achievedAccuracyPercent = achieved,
      reportedNegativeExampleIds = reportedNegatives,
      uncoveredPositiveExampleIds = uncovered,
    )
  }

  suspend fun analyzeProject(
    cluster: EdictNextStoredCluster,
    code: String,
    projectRevision: String,
  ): EdictNextInspectionFindings {
    val analysis = projectAnalysis(code)
    val compilation = analysis.compilation
    check(compilation.compilationSuccess) { "Inspection compilation failed: ${compilation.compilationStatus}" }
    check(compilation.inspectionId == cluster.id) {
      "Inspection id '${compilation.inspectionId}' does not match cluster id '${cluster.id}'"
    }
    val executionErrors = analysis.files.filter { it.executionError != null }
    check(executionErrors.isEmpty()) {
      "Inspection execution failed for ${executionErrors.size} project file(s): ${executionErrors.first().executionError}"
    }
    val findings = analysis.files.asSequence().flatMap { file ->
      file.foundProblems.asSequence().map { problem ->
        EdictNextProjectFinding(
          fileRevision = EdictNextFileRevision(
            path = file.path,
            revision = projectRevision,
            expectedRanges = problem.lineNumber.takeIf { it > 0 }?.let { listOf(EdictNextLineRange(it, it)) },
          ),
          message = problem.message,
          contextSnippet = problem.elementText,
          contextStartLine = problem.lineNumber.takeIf { it > 0 },
        )
      }
    }.distinct().sortedBy { it.fileRevision.path + ":" + it.fileRevision.expectedRanges }.toList()
    return EdictNextInspectionFindings(
      clusterId = cluster.id,
      candidateDigest = sha256Hex(code),
      projectRevision = projectRevision,
      inspectionDescription = compilation.inspectionDescription.orEmpty(),
      language = cluster.manifest.language,
      findings = findings,
    )
  }

  private fun rejectMetadata(
    expectedId: String,
    compilation: InspectionKtsCompileResult,
  ): EdictNextInspectionValidationResponse? {
    val rejection = when {
      !compilation.compilationSuccess -> compilation.compilationStatus ?: "unknown compilation error"
      compilation.inspectionId == null -> "compiled inspection has no id"
      !EDICT_NEXT_KEBAB_CASE.matches(compilation.inspectionId) -> "inspection id '${compilation.inspectionId}' is not lowercase kebab-case"
      compilation.inspectionId != expectedId -> "inspection id '${compilation.inspectionId}' does not match cluster id '$expectedId'"
      compilation.inspectionName.isNullOrBlank() -> "inspection name must not be blank"
      compilation.inspectionDescription.isNullOrBlank() -> "inspection description must not be blank"
      else -> return null
    }
    return EdictNextInspectionValidationResponse(
      compilationSuccess = false,
      overallSuccess = false,
      summary = "Inspection rejected: $rejection",
    )
  }

  private fun evaluateExample(example: EdictNextStoredExample, result: InspectionKtsFileResult?): ExampleOutcome {
    if (result == null) {
      return ExampleOutcome(listOf(EdictNextInspectionFailure(example.metadata.id, "Inspection returned no result")), false)
    }
    result.executionError?.let {
      return ExampleOutcome(listOf(EdictNextInspectionFailure(example.metadata.id, "Inspection execution failed: $it")), false)
    }
    val actualRanges = result.foundProblems.mapNotNull { problem ->
      problem.lineNumber.takeIf { it > 0 }?.let { EdictNextLineRange(it, it) }
    }
    val expectedRanges = example.metadata.expectedRanges.orEmpty()
    return when (example.metadata.label) {
      EdictNextSignalLabel.POSITIVE -> positiveOutcome(example.metadata.id, expectedRanges, actualRanges)
      EdictNextSignalLabel.NEGATIVE -> ExampleOutcome(
        failures = actualRanges.map { EdictNextInspectionFailure(example.metadata.id, "Unexpected finding at range: $it") },
        satisfied = actualRanges.isEmpty(),
      )
    }
  }

  private fun positiveOutcome(
    exampleId: String,
    expectedRanges: List<EdictNextLineRange>,
    actualRanges: List<EdictNextLineRange>,
  ): ExampleOutcome {
    if (expectedRanges.isEmpty()) {
      return ExampleOutcome(listOf(EdictNextInspectionFailure(exampleId, "Positive example has no expected ranges")), false)
    }
    val missing = expectedRanges.filterNot { expected -> actualRanges.any(expected::intersects) }
    val unexpected = actualRanges.filterNot { actual -> expectedRanges.any(actual::intersects) }
    return ExampleOutcome(
      failures = buildList {
        if (missing.isNotEmpty()) add(EdictNextInspectionFailure(exampleId, "Missing expected ranges: $missing"))
        if (unexpected.isNotEmpty()) add(EdictNextInspectionFailure(exampleId, "Unexpected findings: $unexpected"))
      },
      satisfied = missing.isEmpty() && unexpected.isEmpty(),
    )
  }

  private data class ExampleOutcome(val failures: List<EdictNextInspectionFailure>, val satisfied: Boolean)
}
