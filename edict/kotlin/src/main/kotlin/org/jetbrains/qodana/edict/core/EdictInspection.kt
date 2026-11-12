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

  /**
   * Runs [code] on the examples of [cluster]: [EdictMeasurement.Invalid] when compilation, metadata or the execution of an
   * example is invalid; how many examples are satisfied is advisory.
   */
  suspend fun measure(cluster: EdictStoredCluster, code: String): EdictMeasurement {
    val execution = exampleAnalysis(code, exampleRequests(cluster))
    metadataRejection(cluster.id, execution.compilation)?.let { rejection ->
      return EdictMeasurement.Invalid(
        EdictInspectionValidationResponse(compilationSuccess = false, overallSuccess = false, summary = "Inspection rejected: $rejection"),
      )
    }
    val outcomes = cluster.examples.associateWith { example ->
      evaluateExample(example, execution.files.singleOrNull { it.id == example.metadata.id })
    }
    val strongExampleIds = cluster.strongExampleIds()
    val (strong, weak) = outcomes.entries.partition { it.key.metadata.id in strongExampleIds }
    val uncovered = outcomes.failingIds(EdictSignalLabel.POSITIVE)
    val reportedNegatives = outcomes.failingIds(EdictSignalLabel.NEGATIVE)
    val executed = outcomes.values.all(ExampleOutcome::executed)
    val validation = EdictInspectionValidationResponse(
      compilationSuccess = true,
      overallSuccess = executed,
      summary = "${strong.satisfiedOfAll()} strong evidence cases correct (advisory); " +
        "${weak.satisfiedOfAll()} weak evidence cases correct (advisory); " +
        "${uncovered.size} uncovered positive(s), ${reportedNegatives.size} reported negative(s)" +
        if (executed) "" else "; example execution failed or returned missing/duplicate results",
      failures = outcomes.values.flatMap(ExampleOutcome::failures),
      achievedAccuracyPercent = if (outcomes.isEmpty()) 0 else outcomes.values.count(ExampleOutcome::satisfied) * 100 / outcomes.size,
      reportedNegativeExampleIds = reportedNegatives,
      uncoveredPositiveExampleIds = uncovered,
    )
    if (!executed) return EdictMeasurement.Invalid(validation)
    return EdictMeasurement.Measured(validation, outcomes.mapValues { it.value.satisfied }, execution.compilation)
  }

  /**
   * Scores [code] on the examples of [cluster], each with the number of project findings [attributed] to it by example id
   * (ids of other examples are ignored); null with the validation when compilation, metadata or execution is invalid.
   */
  suspend fun evaluate(
    cluster: EdictStoredCluster,
    code: String,
    attributed: Map<String, Int>,
    projectReview: EdictProjectReview,
  ): Pair<EdictInspectionValidationResponse, EdictEvaluation?> {
    val measurement = when (val measurement = measure(cluster, code)) {
      is EdictMeasurement.Invalid -> return measurement.validation to null
      is EdictMeasurement.Measured -> measurement
    }
    val examples = measurement.satisfiedByExample.entries.associate { (example, satisfied) ->
      val id = example.metadata.id
      id to EdictExampleEvaluation(example.metadata.label, satisfied, attributed[id] ?: 0)
    }.toSortedMap()
    return measurement.validation to EdictEvaluation(
      inspectionHash = sha256Hex(code),
      exampleSetDigest = exampleSetDigest(cluster),
      examples = examples,
      projectReview = projectReview,
      inspectionName = measurement.compilation.inspectionName.orEmpty(),
      inspectionDescription = measurement.compilation.inspectionDescription.orEmpty(),
    )
  }

  /** Whether [code] reports each example of [cluster]; fails unless compilation, metadata and execution are valid. */
  suspend fun reports(cluster: EdictStoredCluster, code: String): Map<String, Boolean> =
    when (val measurement = measure(cluster, code)) {
      is EdictMeasurement.Invalid -> error(measurement.validation.summary)
      is EdictMeasurement.Measured -> measurement.reported
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

  suspend fun analyzeProject(
    cluster: EdictStoredCluster,
    code: String,
    projectRevision: String,
  ): EdictInspectionFindings {
    val analysis = projectAnalysis(code)
    metadataRejection(cluster.id, analysis.compilation)?.let { error("Inspection rejected: $it") }
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
            expectedRanges = problem.line?.let { listOf(EdictLineRange(it, it)) },
          ),
          message = problem.message,
          contextSnippet = problem.elementText,
          contextStartLine = problem.line,
        )
      }
    }.distinct().sortedBy { it.fileRevision.path + ":" + it.fileRevision.expectedRanges }.toList()
    return EdictInspectionFindings(
      clusterId = cluster.id,
      candidateDigest = sha256Hex(code),
      projectRevision = projectRevision,
      inspectionDescription = analysis.compilation.inspectionDescription.orEmpty(),
      language = cluster.manifest.language,
      findings = findings,
    )
  }

  /** Why the compiled inspection cannot be the one of the cluster [expectedId]; null when it can. */
  private fun metadataRejection(expectedId: String, compilation: InspectionKtsCompileResult): String? =
    when {
      !compilation.compilationSuccess -> compilation.compilationStatus ?: "unknown compilation error"
      compilation.inspectionId == null -> "compiled inspection has no id"
      !EDICT_KEBAB_CASE.matches(compilation.inspectionId) -> "inspection id '${compilation.inspectionId}' is not lowercase kebab-case"
      compilation.inspectionId != expectedId -> "inspection id '${compilation.inspectionId}' does not match cluster id '$expectedId'"
      compilation.inspectionName.isNullOrBlank() -> "inspection name must not be blank"
      compilation.inspectionDescription.isNullOrBlank() -> "inspection description must not be blank"
      else -> null
    }

  private fun evaluateExample(example: EdictStoredExample, result: InspectionKtsFileResult?): ExampleOutcome {
    if (result == null) {
      return ExampleOutcome(listOf(EdictInspectionFailure(example.metadata.id, "Inspection returned no result")), false, executed = false)
    }
    result.executionError?.let {
      return ExampleOutcome(listOf(EdictInspectionFailure(example.metadata.id, "Inspection execution failed: $it")), false, executed = false)
    }
    val actualRanges = result.foundProblems.mapNotNull { problem -> problem.line?.let { EdictLineRange(it, it) } }
    val expectedRanges = example.metadata.expectedRanges.orEmpty()
    return when (example.metadata.label) {
      EdictSignalLabel.POSITIVE -> positiveOutcome(example.metadata.id, expectedRanges, actualRanges)
      EdictSignalLabel.NEGATIVE -> ExampleOutcome(
        failures = actualRanges.map { EdictInspectionFailure(example.metadata.id, "Unexpected finding at range: $it") },
        satisfied = actualRanges.isEmpty(),
        executed = true,
      )
    }
  }

  private fun positiveOutcome(
    exampleId: String,
    expectedRanges: List<EdictLineRange>,
    actualRanges: List<EdictLineRange>,
  ): ExampleOutcome {
    if (expectedRanges.isEmpty()) {
      return ExampleOutcome(listOf(EdictInspectionFailure(exampleId, "Positive example has no expected ranges")), false, executed = true)
    }
    val missing = expectedRanges.filterNot { expected -> actualRanges.any(expected::intersects) }
    val unexpected = actualRanges.filterNot { actual -> expectedRanges.any(actual::intersects) }
    return ExampleOutcome(
      failures = buildList {
        if (missing.isNotEmpty()) add(EdictInspectionFailure(exampleId, "Missing expected ranges: $missing"))
        if (unexpected.isNotEmpty()) add(EdictInspectionFailure(exampleId, "Unexpected findings: $unexpected"))
      },
      satisfied = missing.isEmpty() && unexpected.isEmpty(),
      executed = true,
    )
  }
}

/** How an inspection behaved on the examples of a cluster. */
internal sealed interface EdictMeasurement {
  val validation: EdictInspectionValidationResponse

  /** Compilation, metadata or the execution of an example is invalid. */
  data class Invalid(override val validation: EdictInspectionValidationResponse) : EdictMeasurement

  /** Every example ran. */
  class Measured(
    override val validation: EdictInspectionValidationResponse,
    /** Whether the inspection behaves on each example as its label expects. */
    val satisfiedByExample: Map<EdictStoredExample, Boolean>,
    val compilation: InspectionKtsCompileResult,
  ) : EdictMeasurement {
    /** Whether the inspection reports each example, by example id: a satisfied POSITIVE or an unsatisfied NEGATIVE. */
    val reported: Map<String, Boolean> = satisfiedByExample.entries.associate { (example, satisfied) ->
      example.metadata.id to ((example.metadata.label == EdictSignalLabel.POSITIVE) == satisfied)
    }
  }
}

/** [executed] is false when the example has no single result or its execution failed. */
private data class ExampleOutcome(val failures: List<EdictInspectionFailure>, val satisfied: Boolean, val executed: Boolean)

/** `<satisfied>/<all>` of these outcomes. */
private fun List<Map.Entry<EdictStoredExample, ExampleOutcome>>.satisfiedOfAll(): String = "${count { it.value.satisfied }}/$size"

/** The sorted ids of the examples with [label] whose outcome is not satisfied. */
private fun Map<EdictStoredExample, ExampleOutcome>.failingIds(label: EdictSignalLabel): List<String> =
  filter { (example, outcome) -> example.metadata.label == label && !outcome.satisfied }.map { it.key.metadata.id }.sorted()

/** The line the inspection reports the problem at; null when it gives none. */
private val InspectionKtsProblem.line: Int?
  get() = lineNumber.takeIf { it > 0 }

/**
 * Where the findings are: the file's path in the project and the 1-based line of the finding, 0 when it has none. A
 * review records one answer per location, so two analyses with the same locations need no new review, whatever their
 * messages and however many findings share a location.
 */
internal fun EdictInspectionFindings.locations(): Set<Pair<String, Int>> = findings.map { it.location() }.toSet()

internal fun EdictProjectFinding.location(): Pair<String, Int> =
  fileRevision.path to (fileRevision.expectedRanges?.firstOrNull()?.start ?: 0)

internal fun EdictReviewedFinding.location(): Pair<String, Int> = path to line

/** The [references] of an example that do not resolve in the module of [contextPath], each as `line <n> '<name>'`. */
internal data class EdictUnresolvedReferences(val contextPath: String?, val references: List<String>)

/** Reports every reference of a Kotlin or Java file that has no target, such as an import its module cannot see. */
private val UNRESOLVED_REFERENCES_INSPECTION: String =
  checkNotNull(EdictInspection::class.java.getResourceAsStream("/inspections/unresolved-references.inspection.kts")) {
    "Missing the unresolved-references inspection resource"
  }.use { it.readBytes().decodeToString() }

internal fun EdictStoredCluster.strongExampleIds(): Set<String> =
  signals.asSequence().filter { it.strength == EdictSignalStrength.STRONG }.mapNotNull(EdictSignal::syntheticExampleId).toSet()

/**
 * The digest of the code examples of [cluster]: their ids, metadata and code. Assigning a Signal to an example leaves it as it
 * is: the example stands for the Signal's case as it did, and the inspection that passed it passes it again.
 */
internal fun exampleSetDigest(cluster: EdictStoredCluster): String {
  val content = buildString {
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
