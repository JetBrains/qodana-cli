package org.jetbrains.qodana.edict.core

/** Keeps cluster semantics outside IntelliJ; the IDE returns only generic compiler and execution results. */
internal class EdictInspection(
  private val client: suspend () -> InspectionKtsClient,
  private val projectAnalysis: suspend (String) -> InspectionKtsProjectRunResult,
  private val exampleAnalysis: suspend (String, List<InspectionKtsExampleRequest>) -> InspectionKtsBatchRunResult =
    { code, requests -> client().runExamples(code, requests) },
) {
  constructor(server: IntellijMcpServerService) : this(
    client = { server.start() },
    projectAnalysis = { code -> server.withClient { it.analyzeProject(code) } },
    exampleAnalysis = { code, requests -> server.withClient { it.runExamples(code, requests) } },
  )

  suspend fun validate(cluster: EdictStoredCluster, code: String): EdictInspectionValidationResponse =
    measure(cluster, code).validation

  suspend fun evaluate(
    cluster: EdictStoredCluster,
    code: String,
    action: EdictInspectionAction,
    assignments: List<EdictFindingAssignment> = emptyList(),
    projectReview: EdictProjectReview? = null,
  ): Pair<EdictInspectionValidationResponse, EdictEvaluation?> {
    val measurement = measure(cluster, code)
    if (!measurement.validation.overallSuccess) return measurement.validation to null
    val labels = cluster.examples.associate { it.metadata.id to it.metadata.label }
    fun reported(exampleId: String) =
      (labels[exampleId] == EdictSignalLabel.POSITIVE) == (measurement.satisfiedByExampleId[exampleId] == true)
    val (unreproduced, reproduced) = assignments.partition { it.stillReported && !reported(it.exampleId) }
    val findingsByExampleId = reproduced.groupingBy { it.exampleId }.eachCount()
    val examples = cluster.examples.associate { example ->
      val id = example.metadata.id
      val findings = findingsByExampleId[id] ?: 0
      id to EdictExampleEvaluation(
        label = example.metadata.label,
        strong = id in measurement.strongExampleIds,
        satisfied = measurement.satisfiedByExampleId[id] == true,
        projectFindings = findings,
        weight = maxOf(1, findings),
      )
    }.toSortedMap()
    fun weight(label: EdictSignalLabel, satisfied: Boolean) =
      examples.values.filter { it.label == label && it.satisfied == satisfied }.sumOf { it.weight }
    fun direct(classification: EdictFindingClassification) = unreproduced.count { it.classification == classification }
    val tp = weight(EdictSignalLabel.POSITIVE, satisfied = true) + direct(EdictFindingClassification.TP)
    val fn = weight(EdictSignalLabel.POSITIVE, satisfied = false)
    val fp = weight(EdictSignalLabel.NEGATIVE, satisfied = false) + direct(EdictFindingClassification.FP)
    return measurement.validation to EdictEvaluation(
      inspectionHash = sha256Hex(code),
      inspectionName = measurement.inspectionName,
      inspectionDescription = measurement.inspectionDescription,
      exampleSetDigest = exampleSetDigest(cluster),
      action = action,
      tp = tp,
      fp = fp,
      fn = fn,
      precision = if (tp + fp == 0) 0.0 else tp.toDouble() / (tp + fp),
      recall = if (tp + fn == 0) 0.0 else tp.toDouble() / (tp + fn),
      examples = examples,
      projectReview = projectReview?.copy(unreproduced = unreproduced.size),
    )
  }

  suspend fun reports(cluster: EdictStoredCluster, code: String): Map<String, Boolean> {
    val measurement = measure(cluster, code)
    check(measurement.validation.overallSuccess) { measurement.validation.summary }
    return cluster.examples.associate { example ->
      val satisfied = measurement.satisfiedByExampleId[example.metadata.id] == true
      example.metadata.id to ((example.metadata.label == EdictSignalLabel.POSITIVE) == satisfied)
    }
  }

  suspend fun unresolvedReferences(
    cluster: EdictStoredCluster,
    examples: List<EdictStoredExample> = cluster.examples,
  ): Map<String, EdictUnresolvedReferences> {
    if (examples.isEmpty()) return emptyMap()
    val requests = exampleRequests(cluster, examples)
    val execution = exampleAnalysis(UNRESOLVED_REFERENCES_INSPECTION, requests)
    check(execution.compilation.compilationSuccess) {
      "The unresolved-reference check did not compile: ${execution.compilation.compilationStatus}"
    }
    return requests.mapNotNull { request ->
      val result = execution.files.singleOrNull { it.id == request.id }
      val references = result?.executionError?.let { listOf("the check failed: $it") }
        ?: result?.foundProblems?.map { "line ${it.lineNumber} ${it.message.substringAfter("reference ")}" }
        ?: listOf("the check returned no result")
      if (references.isEmpty()) null else request.id to EdictUnresolvedReferences(request.contextPath, references)
    }.toMap()
  }

  // The example's module comes from the source it was reduced from: its finding, its Signal, or another cluster Signal.
  private fun exampleRequests(
    cluster: EdictStoredCluster,
    examples: List<EdictStoredExample> = cluster.examples,
  ): List<InspectionKtsExampleRequest> {
    val signalPaths = cluster.signals.filter { it.syntheticExampleId != null }
      .associate { it.syntheticExampleId to it.fileRevision.path }
    val clusterPath = cluster.signals.minByOrNull(EdictSignal::id)?.fileRevision?.path
    return examples.map { example ->
      InspectionKtsExampleRequest(
        id = example.metadata.id,
        projectPath = example.directory.projectDirectory.toString(),
        targetFilePath = example.directory.projectDirectory.relativize(example.sourcePath).toString(),
        contextPath = example.metadata.contextPath ?: signalPaths[example.metadata.id] ?: clusterPath,
      )
    }
  }

  private suspend fun measure(cluster: EdictStoredCluster, code: String): Measurement {
    val execution = exampleAnalysis(code, exampleRequests(cluster))
    rejectMetadata(cluster.id, execution.compilation)?.let { return Measurement(it, emptyMap(), emptySet()) }

    val failures = mutableListOf<EdictInspectionFailure>()
    val outcomesById = cluster.examples.associate { example ->
      val result = execution.files.singleOrNull { it.id == example.metadata.id }
      val outcome = evaluateExample(example, result)
      failures += outcome.failures
      example.metadata.id to outcome
    }
    val examplesById = cluster.examples.associateBy { it.metadata.id }
    val scoredExampleIds = examplesById.keys
    val strongExampleIds = cluster.strongExampleIds()
    val weakExampleIds = scoredExampleIds - strongExampleIds
    val strongPositiveCount = strongExampleIds.count { examplesById[it]?.metadata?.label == EdictSignalLabel.POSITIVE }
    val strongCorrectCount = strongExampleIds.count { outcomesById[it]?.satisfied == true }
    val weakCorrectCount = weakExampleIds.count { outcomesById[it]?.satisfied == true }
    val correctCount = scoredExampleIds.count { outcomesById[it]?.satisfied == true }
    val achieved = if (scoredExampleIds.isEmpty()) 0 else correctCount * 100 / scoredExampleIds.size
    val uncovered = cluster.examples.filter {
      it.metadata.label == EdictSignalLabel.POSITIVE && outcomesById[it.metadata.id]?.satisfied != true
    }.map { it.metadata.id }.sorted()
    val reportedNegatives = cluster.examples.filter {
      it.metadata.label == EdictSignalLabel.NEGATIVE && outcomesById[it.metadata.id]?.satisfied != true
    }.map { it.metadata.id }.sorted()
    val executionSuccess = cluster.examples.all { example ->
      execution.files.singleOrNull { it.id == example.metadata.id }?.let { it.executionError == null } == true
    }
    val summary =
      "$strongCorrectCount/${strongExampleIds.size} strong evidence cases correct (advisory); " +
      "$weakCorrectCount/${weakExampleIds.size} weak evidence cases correct (advisory); " +
      "${uncovered.size} uncovered positive(s), ${reportedNegatives.size} reported negative(s)" +
      if (executionSuccess) "" else "; example execution failed or returned missing/duplicate results"
    val validation = EdictInspectionValidationResponse(
      compilationSuccess = true,
      overallSuccess = executionSuccess,
      summary = summary,
      strongExamplesSatisfied = executionSuccess && strongPositiveCount > 0 && strongCorrectCount == strongExampleIds.size,
      failures = failures,
      achievedAccuracyPercent = achieved,
      reportedNegativeExampleIds = reportedNegatives,
      uncoveredPositiveExampleIds = uncovered,
    )
    return Measurement(validation, outcomesById.mapValues { it.value.satisfied }, strongExampleIds,
      execution.compilation.inspectionName.orEmpty(), execution.compilation.inspectionDescription.orEmpty())
  }

  suspend fun analyzeProject(
    cluster: EdictStoredCluster,
    code: String,
    projectRevision: String,
  ): EdictInspectionFindings {
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
        EdictProjectFinding(
          fileRevision = EdictFileRevision(
            path = file.path,
            revision = projectRevision,
            expectedRanges = problem.lineNumber.takeIf { it > 0 }?.let { listOf(EdictLineRange(it, it)) },
          ),
          message = problem.message,
          contextSnippet = problem.elementText,
          contextStartLine = problem.lineNumber.takeIf { it > 0 },
        )
      }
    }.distinct().sortedBy { it.fileRevision.path + ":" + it.fileRevision.expectedRanges }.toList()
    return EdictInspectionFindings(
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
  ): EdictInspectionValidationResponse? {
    val rejection = when {
      !compilation.compilationSuccess -> compilation.compilationStatus ?: "unknown compilation error"
      compilation.inspectionId == null -> "compiled inspection has no id"
      !EDICT_KEBAB_CASE.matches(compilation.inspectionId) -> "inspection id '${compilation.inspectionId}' is not lowercase kebab-case"
      compilation.inspectionId != expectedId -> "inspection id '${compilation.inspectionId}' does not match cluster id '$expectedId'"
      compilation.inspectionName.isNullOrBlank() -> "inspection name must not be blank"
      compilation.inspectionDescription.isNullOrBlank() -> "inspection description must not be blank"
      else -> return null
    }
    return EdictInspectionValidationResponse(
      compilationSuccess = false,
      overallSuccess = false,
      summary = "Inspection rejected: $rejection",
    )
  }

  private fun evaluateExample(example: EdictStoredExample, result: InspectionKtsFileResult?): ExampleOutcome {
    if (result == null) {
      return ExampleOutcome(listOf(EdictInspectionFailure(example.metadata.id, "Inspection returned no result")), false)
    }
    result.executionError?.let {
      return ExampleOutcome(listOf(EdictInspectionFailure(example.metadata.id, "Inspection execution failed: $it")), false)
    }
    val actualRanges = result.foundProblems.mapNotNull { problem ->
      problem.lineNumber.takeIf { it > 0 }?.let { EdictLineRange(it, it) }
    }
    val expectedRanges = example.metadata.expectedRanges.orEmpty()
    return when (example.metadata.label) {
      EdictSignalLabel.POSITIVE -> positiveOutcome(example.metadata.id, expectedRanges, actualRanges)
      EdictSignalLabel.NEGATIVE -> ExampleOutcome(
        failures = actualRanges.map { EdictInspectionFailure(example.metadata.id, "Unexpected finding at range: $it") },
        satisfied = actualRanges.isEmpty(),
      )
    }
  }

  private fun positiveOutcome(
    exampleId: String,
    expectedRanges: List<EdictLineRange>,
    actualRanges: List<EdictLineRange>,
  ): ExampleOutcome {
    if (expectedRanges.isEmpty()) {
      return ExampleOutcome(listOf(EdictInspectionFailure(exampleId, "Positive example has no expected ranges")), false)
    }
    val missing = expectedRanges.filterNot { expected -> actualRanges.any(expected::intersects) }
    val unexpected = actualRanges.filterNot { actual -> expectedRanges.any(actual::intersects) }
    return ExampleOutcome(
      failures = buildList {
        if (missing.isNotEmpty()) add(EdictInspectionFailure(exampleId, "Missing expected ranges: $missing"))
        if (unexpected.isNotEmpty()) add(EdictInspectionFailure(exampleId, "Unexpected findings: $unexpected"))
      },
      satisfied = missing.isEmpty() && unexpected.isEmpty(),
    )
  }

  private data class ExampleOutcome(val failures: List<EdictInspectionFailure>, val satisfied: Boolean)

  private class Measurement(
    val validation: EdictInspectionValidationResponse,
    val satisfiedByExampleId: Map<String, Boolean>,
    val strongExampleIds: Set<String>,
    val inspectionName: String = "",
    val inspectionDescription: String = "",
  )
}

/** What makes two analyses' findings the same for review: every reported location, in order. */
internal fun EdictInspectionFindings.reviewKey(): List<Pair<String, List<EdictLineRange>?>> =
  findings.map { it.fileRevision.path to it.fileRevision.expectedRanges }

/** A reviewed finding assigned to an existing example; [stillReported] when the published inspection reports it. */
internal data class EdictFindingAssignment(
  val exampleId: String,
  val classification: EdictFindingClassification,
  val stillReported: Boolean,
)

internal fun EdictProjectFinding.location(): Pair<String, Int> =
  fileRevision.path to (fileRevision.expectedRanges?.firstOrNull()?.start ?: 0)

/** The [references] of an example that do not resolve in the module of [contextPath], each as `line <n> '<name>'`. */
internal data class EdictUnresolvedReferences(val contextPath: String?, val references: List<String>)

/** Reports every reference of a Kotlin or Java file that has no target, such as an import its module cannot see. */
private val UNRESOLVED_REFERENCES_INSPECTION: String =
  checkNotNull(EdictInspection::class.java.getResourceAsStream("/inspections/unresolved-references.inspection.kts")) {
    "Missing the unresolved-references inspection resource"
  }.use { it.readBytes().decodeToString() }

internal fun EdictStoredCluster.strongExampleIds(): Set<String> =
  signals.asSequence().filter { it.strength == EdictSignalStrength.STRONG }.mapNotNull(EdictSignal::syntheticExampleId).toSet()

internal fun exampleSetDigest(cluster: EdictStoredCluster): String {
  val strong = cluster.signals.asSequence()
    .filter { it.strength == EdictSignalStrength.STRONG }
    .mapNotNull(EdictSignal::syntheticExampleId)
    .toSortedSet()
  val content = buildString {
    appendLine("strong:${strong.joinToString(",")}")
    cluster.examples.sortedBy { it.metadata.id }.forEach { example ->
      appendLine("example:${example.metadata.id}")
      appendLine(EdictJson.encodeToString(EdictCodeExampleMetadata.serializer(), example.metadata))
      appendLine(sha256Hex(example.code))
    }
  }
  return sha256Hex(content)
}

/** Whether [evaluation] scored the inspection whose SHA-256 hex is [inspectionDigest] on this cluster's current examples. */
internal fun EdictStoredCluster.isCurrentEvaluation(evaluation: EdictEvaluation, inspectionDigest: String): Boolean =
  evaluation.inspectionHash == inspectionDigest && evaluation.exampleSetDigest == exampleSetDigest(this)
