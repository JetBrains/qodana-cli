package org.jetbrains.qodana.edict.core

import org.jetbrains.qodana.edict.common.DEFAULT_GENERATION_COUNT
import org.jetbrains.qodana.edict.common.DEFAULT_MAX_PROJECT_ANALYSES
import org.jetbrains.qodana.edict.common.GitRepository
import java.nio.file.Path
import java.util.concurrent.ConcurrentHashMap
import kotlin.io.path.ExperimentalPathApi
import kotlin.io.path.createDirectories
import kotlin.io.path.deleteRecursively
import kotlin.io.path.isRegularFile
import kotlin.io.path.readBytes
import kotlin.io.path.writeText
import kotlin.io.path.readText

/** Generates inspections for Pending clusters chosen by the manager. */
internal class EdictGenerationService(
  private val repository: EdictRepository,
  inspectionServer: IntellijMcpServerService,
  private val projectRoot: Path,
  private val maxProjectAnalyses: Int = DEFAULT_MAX_PROJECT_ANALYSES,
  private val defaultGenerationCount: Int = DEFAULT_GENERATION_COUNT,
  private val reportUploader: EdictReportUploader? = EdictCloudReports.fromEnvironment(),
) {
  private val inspection = EdictInspection(inspectionServer)

  /** The analyzed project's HEAD, resolved on first use and fixed for the rest of the run. */
  private val projectRevision: String by lazy { GitRepository(projectRoot).resolve("HEAD") }
  private val inspectionActions = ConcurrentHashMap<Set<String>, EdictInspectionAction>()
  private val validatedCandidateDigests = ConcurrentHashMap<Set<String>, String>()
  private val analyzedCandidateDigests = ConcurrentHashMap<Set<String>, String>()
  private val projectAnalysisCounts = ConcurrentHashMap<Set<String>, Int>()
  private val lastReviewedLocations = ConcurrentHashMap<Set<String>, Set<Pair<String, Int>>>()
  private val analyzedFindings = ConcurrentHashMap<Set<String>, EdictInspectionFindings>()
  private val weakReviewResults = ConcurrentHashMap<Set<String>, Map<Pair<String, Int>, EdictReviewedFinding>>()
  private val validatedExampleReports = ConcurrentHashMap<Set<String>, Pair<String, Map<String, Boolean>>>()
  private val analyzedExampleReports = ConcurrentHashMap<Set<String>, Map<String, Boolean>>()
  private val analyzedCodes = ConcurrentHashMap<Set<String>, String>()

  /** Read-only suggestions from the current repository; does not reserve clusters or start generation. */
  suspend fun selectGenerationClusters(generationCount: Int? = null): EdictGenerationClustersResponse {
    val targetCount = generationCount ?: defaultGenerationCount
    require(targetCount >= 1) { "generationCount must be at least 1: $targetCount" }
    val state = repository.loadState()
    requireNoIssues(validateRepositoryState(repository, state))
    // Only new Signals give a generation something to do; strong positive evidence guides it.
    val selected = state.clusters
      .filter { it.manifest.status == EdictClusterStatus.Pending && it.unprocessedSignals.isNotEmpty() }
      .filter { cluster ->
        cluster.signals.any { it.strength == EdictSignalStrength.STRONG && it.label == EdictSignalLabel.POSITIVE }
      }
      .sortedWith(
        compareByDescending<EdictStoredCluster> { cluster ->
          cluster.unprocessedSignals.asSequence()
            .filter { it.strength == EdictSignalStrength.STRONG && it.label == EdictSignalLabel.POSITIVE }
            .map { it.fileRevision.path }
            .distinct()
            .count()
        }.thenBy { it.id },
      )
      .take(targetCount)
    return EdictGenerationClustersResponse(
      clusters = selected.map { EdictGenerationTarget(it.id, it.directory.root.toString()) },
      summary = "${selected.size} generation candidate(s) suggested, of the Pending clusters with new Signals",
    )
  }

  suspend fun saveCodeExample(
    clusterId: String,
    exampleId: String,
    metadataJson: String,
    sourceCode: String,
  ): EdictMutationResponse {
    loadPendingCluster(clusterId)
    val metadata = EdictJson.decodeFromString<EdictCodeExampleMetadata>(metadataJson)
    val directory = EdictExampleDirectory(
      EdictClusterDirectory(repository.paths.clustersDirectory.resolve(clusterId)).examplesDirectory.resolve(exampleId),
    )
    val validation = validateCodeExample(directory, metadata, directory.sourcePath(metadata.fileName), sourceCode)
    require(validation.success) { validation.summary + ": " + validation.issues.joinToString("; ") }
    repository.saveCodeExample(clusterId, exampleId, metadata, sourceCode)
    return EdictMutationResponse(true, "Stored code example '$exampleId' in cluster '$clusterId'; it is structurally valid")
  }

  suspend fun updateCodeExampleExpectedRange(
    clusterId: String,
    exampleId: String,
    range: EdictLineRange,
  ): EdictMutationResponse {
    val cluster = loadPendingCluster(clusterId)
    repository.updateCodeExampleExpectedRange(clusterId, exampleId, range)
    // Project findings and their analysis budget are unchanged, but example results used the old expectation.
    validatedCandidateDigests.remove(cluster.signalIds)
    validatedExampleReports.remove(cluster.signalIds)
    analyzedExampleReports.remove(cluster.signalIds)
    return EdictMutationResponse(true, "Updated only the expected range of code example '$exampleId' to ${range.start}-${range.end}")
  }

  suspend fun assignCodeExample(
    clusterId: String,
    signalId: String,
    exampleId: String,
  ): EdictMutationResponse {
    val cluster = loadPendingCluster(clusterId)
    val validation = validateCodeExample(repository, clusterId, exampleId)
    require(validation.success) { validation.summary + ": " + validation.issues.joinToString("; ") }
    // Checked as assigned: the example then runs in the module of its Signal's file unless it names another.
    val assigned = cluster.copy(
      signals = cluster.signals.map { if (it.id == signalId) it.copy(syntheticExampleId = exampleId) else it },
    )
    val example = assigned.examples.single { it.metadata.id == exampleId }
    inspection.unresolvedReferences(assigned, listOf(example))[exampleId]?.let { unresolved ->
      throw IllegalArgumentException(unresolvedIssue(exampleId, unresolved))
    }
    repository.assignCodeExample(clusterId, signalId, exampleId)
    return EdictMutationResponse(true, "Assigned code example '$exampleId' to Signal '$signalId'")
  }

  suspend fun deleteCodeExample(clusterId: String, exampleId: String): EdictMutationResponse {
    loadPendingCluster(clusterId)
    repository.deleteCodeExample(clusterId, exampleId)
    return EdictMutationResponse(true, "Deleted code example '$exampleId' from cluster '$clusterId' and unassigned its Signals")
  }

  /**
   * Stores the candidate whatever its quality, then compiles it and runs every example, so the stored candidate is always
   * the latest attempt. A candidate with valid compilation, metadata and example execution may go on to project analysis,
   * regardless of its strong- or weak-example accuracy.
   */
  suspend fun saveCandidateInspection(clusterId: String, code: String): EdictInspectionValidationResponse {
    val cluster = loadPendingCluster(clusterId)
    repository.saveCandidateInspection(clusterId, code)
    val measurement = inspection.measure(cluster, code)
    when (measurement) {
      is EdictMeasurement.Measured -> {
        validatedCandidateDigests[cluster.signalIds] = sha256Hex(code)
        validatedExampleReports[cluster.signalIds] = sha256Hex(code) to measurement.reported
      }
      is EdictMeasurement.Invalid -> {
        validatedCandidateDigests.remove(cluster.signalIds)
        validatedExampleReports.remove(cluster.signalIds)
      }
    }
    return measurement.validation
  }

  /**
   * Renames a Pending cluster and the `id = "<clusterId>"` literal of its candidate. Only the id changes, so a
   * validated or analyzed candidate stays so under the new id.
   */
  fun renameCluster(clusterId: String, newClusterId: String): EdictMutationResponse {
    val cluster = loadPendingCluster(clusterId)
    if (newClusterId == clusterId) {
      return EdictMutationResponse(true, "Cluster '$clusterId' already has this id")
    }
    val code = cluster.candidateInspectionPath.takeIf(Path::isRegularFile)?.readText()
    fun rename(code: String) = code.replace(Regex("(\\bid\\s*=\\s*)\"${Regex.escape(clusterId)}\""), "$1\"$newClusterId\"")
    val renamedCode = code?.let(::rename)
    repository.renameCluster(cluster, newClusterId, renamedCode)
    analyzedCodes.computeIfPresent(cluster.signalIds) { _, analyzed -> rename(analyzed) }
    if (code != null && renamedCode != null) {
      for (digests in listOf(validatedCandidateDigests, analyzedCandidateDigests)) {
        digests.replace(cluster.signalIds, sha256Hex(code), sha256Hex(renamedCode))
      }
      validatedExampleReports.computeIfPresent(cluster.signalIds) { _, (digest, reports) ->
        (if (digest == sha256Hex(code)) sha256Hex(renamedCode) else digest) to reports
      }
    }
    return EdictMutationResponse(true, "Renamed cluster '$clusterId' to '$newClusterId'")
  }

  /**
   * Writes the new Signals of a Pending cluster and its code examples into `signal-mapping/<clusterId>` of [scratchDirectory],
   * without their labels, so that a mapper that cannot see them tells which example stands for each Signal's case.
   */
  @OptIn(ExperimentalPathApi::class)
  fun prepareSignalMapping(clusterId: String, scratchDirectory: Path): EdictSignalMappingResponse {
    val cluster = loadPendingCluster(clusterId)
    val directory = scratchDirectory.resolve("signal-mapping").resolve(cluster.id)
    directory.deleteRecursively()
    val signals = directory.resolve("signals").createDirectories()
    cluster.unprocessedSignals.forEach { signal ->
      signals.resolve("${signal.id}.json")
        .writeText(EdictJson.encodeToString(EdictUnlabelledSignal.serializer(), EdictUnlabelledSignal(signal.id, signal.fileRevision)))
    }
    cluster.examples.forEach { example ->
      directory.resolve("examples").resolve(example.metadata.id).createDirectories().resolve(example.metadata.fileName).writeText(example.code)
    }
    return EdictSignalMappingResponse(
      directory = directory.toString(),
      signalIds = cluster.unprocessedSignals.map(EdictSignal::id),
      exampleIds = cluster.examples.map { it.metadata.id },
    )
  }

  fun appendHistory(clusterId: String, entry: String): EdictMutationResponse {
    loadPendingCluster(clusterId)
    repository.appendHistory(clusterId, entry)
    return EdictMutationResponse(true, "Appended history for cluster '$clusterId'")
  }

  fun clusterExamples(clusterId: String): String {
    val cluster = repository.loadCluster(clusterId)
    if (cluster.examples.isEmpty()) return "Cluster '$clusterId' has no code examples"
    val signalsByExample = cluster.signals.filter { it.syntheticExampleId != null }.groupBy { it.syntheticExampleId }
    val strong = cluster.strongExampleIds()
    return cluster.examples.joinToString("\n\n") { example ->
      val metadata = example.metadata
      val signals = signalsByExample[metadata.id].orEmpty()
      val strength = if (metadata.id in strong) "strong" else "weak"
      val assigned = if (signals.isEmpty()) "" else signals.joinToString(", ", " (", ")") { it.id }
      val ranges = metadata.expectedRanges.orEmpty()
        .joinToString(", ") { if (it.start == it.end) "${it.start}" else "${it.start}-${it.end}" }
        .ifEmpty { "none" }
      "== ${metadata.id} · ${metadata.label} · $strength$assigned · ${metadata.fileName} · expected $ranges\n" +
        numberedLines(example.code, null, 0)
    }
  }

  suspend fun validateClusterExamples(clusterId: String): EdictCodeExampleValidationResponse {
    val cluster = try {
      loadPendingCluster(clusterId)
    }
    catch (e: EdictCodeExampleReadException) {
      return EdictCodeExampleValidationResponse(
        success = false,
        summary = "Found ${e.issues.size} cluster code example issue(s)",
        issues = e.issues,
      )
    }
    val directories = cluster.examples.associate { it.metadata.id to it.directory.root.toString() }
    val unresolved = inspection.unresolvedReferences(cluster).map { (exampleId, references) ->
      EdictValidationIssue(directories.getValue(exampleId), unresolvedIssue(exampleId, references))
    }
    val issues = validateClusterExamples(cluster) + unresolved
    return EdictCodeExampleValidationResponse(
      success = issues.isEmpty(),
      summary = if (issues.isEmpty()) {
        "Every Signal has a structurally valid focused code example, and every reference of every example resolves"
      } else {
        "Found ${issues.size} cluster code example issue(s)"
      },
      issues = issues.map { "${it.path}: ${it.message}" },
    )
  }

  fun getInspectionAction(clusterId: String): EdictInspectionActionResponse {
    val cluster = loadPendingCluster(clusterId)
    val response = duplicateSignalAction(cluster) ?: predecessorAction(cluster)
    inspectionActions[cluster.signalIds] = response.action
    return response
  }

  /**
   * Starts the expensive stage of a generation cycle: project analysis, then weak-signal review. It takes only the
   * validated latest candidate. Each cluster gets [maxProjectAnalyses] of them per run, counted from the call, so the
   * stage stays bounded however often the cheap generation, example and shallow review steps before it repeat.
   *
   * TODO: change the weak-signal review into the full review of a generation cycle: weak examples, the project findings,
   *  and the correctness review that the shallow code review no longer does.
   * TODO: move the limit to the task MCP once that review is one subagent. Count its tasks per cluster rather than per
   *  parent task, and allow this call only with that task's token; then the limit also bounds the review itself.
   */
  suspend fun getNewInspectionResults(
    clusterId: String,
    privateScratchDirectory: String,
  ): EdictInspectionResultsResponse {
    val cluster = loadPendingCluster(clusterId)
    val code = cluster.candidateInspectionPath.takeIf(Path::isRegularFile)?.readText()
    check(code != null && validatedCandidateDigests[cluster.signalIds] == sha256Hex(code)) {
      "The current candidate of cluster '$clusterId' has not passed validation; " +
      "save a candidate with valid compilation, metadata and example execution first"
    }
    val analysis = checkNotNull(projectAnalysisCounts.merge(cluster.signalIds, 1, Int::plus))
    check(analysis <= maxProjectAnalyses) {
      "Cluster '$clusterId' used all $maxProjectAnalyses project analyses of this run; stop generating. " +
      "Finalise the last analyzed candidate Generated regardless of strong- or weak-example accuracy"
    }
    val findings = inspection.analyzeProject(cluster, code, projectRevision)
    val locations = findings.locations()
    val unchanged = lastReviewedLocations[cluster.signalIds] == locations
    val reports = validatedExampleReports[cluster.signalIds]?.takeIf { it.first == findings.candidateDigest }?.second.orEmpty()
    analyzedExampleReports[cluster.signalIds] = reports
    val strong = cluster.strongExampleIds()
    val exampleStatus = cluster.examples.filter { it.metadata.id in reports }.associate { example ->
      val id = example.metadata.id
      id to EdictReviewExampleStatus(example.metadata.label, id in strong, reports.getValue(id))
    }.toSortedMap()
    val unreproducedFindings = weakReviewResults[cluster.signalIds].orEmpty().values
      .filter { it.exampleId != null && reports[it.exampleId] == false }
      .groupBy { checkNotNull(it.exampleId) }
      .mapNotNull { (exampleId, assigned) ->
        val (still, gone) = assigned.partition { it.location() in locations }
        if (still.isEmpty()) null
        else exampleId to EdictUnreproducedFindings(still.map { "${it.path}:${it.line}" }.sorted(), gone.size)
      }.toMap().toSortedMap()
    val response = EdictReviewArtifacts.create(
      findings = findings,
      clusterDirectory = cluster.directory.root,
      candidateInspection = cluster.candidateInspectionPath,
      inspectedProject = projectRoot,
      privateScratchDirectory = Path.of(privateScratchDirectory).toAbsolutePath().normalize(),
      remainingProjectAnalyses = maxProjectAnalyses - analysis,
      reviewedLocations = weakReviewResults[cluster.signalIds].orEmpty().keys,
      exampleStatus = exampleStatus,
      unreproducedFindings = unreproducedFindings,
    )
    analyzedCandidateDigests[cluster.signalIds] = findings.candidateDigest
    analyzedFindings[cluster.signalIds] = findings
    analyzedCodes[cluster.signalIds] = code
    return response.copy(findingsUnchanged = unchanged)
  }

  /**
   * Records how the weak-signal review classified findings of the cluster's last project analysis and which example
   * represents each. Kept for the run and counted by [recordEvaluation] as the weight of each example.
   */
  suspend fun recordWeakReview(
    clusterId: String,
    findings: List<EdictReviewedFinding>,
  ): EdictMutationResponse {
    val cluster = loadPendingCluster(clusterId)
    val analysis = checkNotNull(analyzedFindings[cluster.signalIds]) {
      "Cluster '$clusterId' has no project analysis in this run to review"
    }
    val locations = analysis.locations()
    val labels = cluster.examples.associate { it.metadata.id to it.metadata.label }
    val reports = analyzedExampleReports[cluster.signalIds].orEmpty()
    // An example written after the analysis is checked now: an example made from a finding must be reported as it is.
    val created = findings.mapNotNull { it.exampleId }.filter { it in labels && it !in reports }
    val assigned = cluster.signals.mapNotNull(EdictSignal::syntheticExampleId).toSet()
    for (finding in findings) {
      val exampleId = finding.exampleId?.takeIf { it in created && it !in assigned } ?: continue
      val example = cluster.examples.single { it.metadata.id == exampleId }
      if (example.metadata.contextPath != finding.path) repository.setCodeExampleContextPath(clusterId, exampleId, finding.path)
    }
    val createdReports = if (created.isEmpty()) emptyMap() else {
      inspection.reports(loadPendingCluster(clusterId), checkNotNull(analyzedCodes[cluster.signalIds]))
    }
    for (finding in findings) {
      val where = "${finding.path}:${finding.line}"
      require(finding.location() in locations) {
        "No finding at $where in the last project analysis of cluster '$clusterId'"
      }
      val exampleId = finding.exampleId
      if (exampleId == null) {
        require(finding.classification == EdictFindingClassification.UNCERTAIN) {
          "Finding $where is ${finding.classification} but names no example"
        }
        continue
      }
      val label = requireNotNull(labels[exampleId]) { "Cluster '$clusterId' has no example '$exampleId'" }
      val expected = when (finding.classification) {
        EdictFindingClassification.TP -> EdictSignalLabel.POSITIVE
        EdictFindingClassification.FP -> EdictSignalLabel.NEGATIVE
        EdictFindingClassification.UNCERTAIN -> label
      }
      require(label == expected) { "Finding $where is ${finding.classification}, but example '$exampleId' is $label" }
      require(reports[exampleId] ?: createdReports.getValue(exampleId)) {
        if (exampleId in reports) {
          "The analyzed inspection reports $where but not example '$exampleId', so '$exampleId' does not reproduce it. " +
          "Create an example from this finding's code: split '$exampleId', or replace it when it reproduces none of its findings"
        }
        else {
          "The analyzed inspection reports $where but not example '$exampleId'" +
          (if (label == EdictSignalLabel.POSITIVE) " at its expected range" else "") +
          ", so the reduction removed what makes it report this code. Restore that from the finding's code, " +
          "save '$exampleId' again and record again"
        }
      }
    }
    weakReviewResults.merge(cluster.signalIds, findings.associateBy(EdictReviewedFinding::location)) { old, new -> old + new }
    lastReviewedLocations[cluster.signalIds] = locations
    return EdictMutationResponse(true, "Recorded ${findings.size} reviewed finding(s) of cluster '$clusterId'")
  }

  /**
   * Last step before finalising Generated. On GENERATE, writes the [EdictEvaluation] of the analyzed candidate, with the
   * findings the weak-signal review classified TP or FP attributed to their examples; compilation, metadata and execution
   * must be valid, but evidence accuracy does not gate recording. On SKIP, the predecessor's evaluation stays: neither it nor
   * the examples changed.
   */
  suspend fun recordEvaluation(clusterId: String): EdictRecordEvaluationResponse {
    val cluster = loadPendingCluster(clusterId)
    val selection = when (val result = selectGeneratedInspection(cluster)) {
      is InspectionSelection.Rejected -> return EdictRecordEvaluationResponse(false, result.reason)
      is InspectionSelection.Selected -> result
    }
    if (selection.action == EdictInspectionAction.SKIP) {
      return predecessorEvaluation(cluster)
        ?.let { EdictRecordEvaluationResponse(true, "The predecessor's evaluation stays: neither it nor the examples changed", it) }
        ?: EdictRecordEvaluationResponse(false, "$NO_PREDECESSOR_EVALUATION any more; call edict_get_inspection_action again")
    }
    val reviewed = weakReviewResults[cluster.signalIds].orEmpty().values
    // Findings of an example deleted since count for nothing: the evaluation weighs only the cluster's examples.
    val attributed = reviewed.filter { it.classification != EdictFindingClassification.UNCERTAIN }
      .mapNotNull(EdictReviewedFinding::exampleId)
      .groupingBy { it }.eachCount()
    // On GENERATE the selection requires that the last analysis, stored with its digest, was of these bytes (a rename
    // carries the digest over).
    val analysis = analyzedFindings.getValue(cluster.signalIds)
    val (validation, evaluation) = inspection.evaluate(cluster, selection.code, attributed, analysis.review(reviewed))
    if (evaluation == null) return EdictRecordEvaluationResponse(false, "Evaluation not recorded: ${validation.summary}")
    repository.saveEvaluation(clusterId, evaluation)
    return EdictRecordEvaluationResponse(true, "Recorded the evaluation; ${validation.summary}", evaluation)
  }

  /**
   * Ends the processing of a cluster in [status], recording [reason] in its history. Each status is reached only
   * with its contract satisfied; otherwise the cluster stays as it was.
   */
  suspend fun finaliseCluster(
    clusterId: String,
    status: EdictClusterStatus,
    reason: String,
  ): EdictFinaliseClusterResponse {
    require(reason.isNotBlank()) { "Reason must not be blank" }
    val cluster = loadPendingCluster(clusterId)
    if (status == EdictClusterStatus.Pending) {
      return rejectedFinalisation(status, "Pending is an input state, not a generation outcome; use Generated, Invalid or Discontinued")
    }
    val history = cluster.historyPath.takeIf(Path::isRegularFile)?.readText().orEmpty() + reason
    if (status == EdictClusterStatus.Generated) return finaliseGenerated(cluster, history, reason)

    val issues = validateCluster(cluster.copy(manifest = cluster.manifest.copy(status = status)), history)
    if (issues.isNotEmpty()) return rejectedFinalisation(status, "Found ${issues.size} issue(s)", issues)
    repository.appendHistory(clusterId, "$status: $reason")
    when (status) {
      EdictClusterStatus.Discontinued -> repository.markDiscontinued(cluster)
      EdictClusterStatus.Invalid -> repository.markInvalid(cluster)
      EdictClusterStatus.Pending -> error("Pending is not a generation outcome")
    }
    return EdictFinaliseClusterResponse(success = true, summary = "Cluster '$clusterId' is $status")
  }

  private suspend fun finaliseGenerated(
    cluster: EdictStoredCluster,
    history: String,
    reason: String,
  ): EdictFinaliseClusterResponse {
    val generated = EdictClusterStatus.Generated
    val issues = validateGeneratedTransition(repository, cluster, history)
    if (issues.isNotEmpty()) return rejectedFinalisation(generated, "Found ${issues.size} issue(s)", issues)
    val selection = when (val selection = selectGeneratedInspection(cluster)) {
      is InspectionSelection.Rejected -> return rejectedFinalisation(generated, selection.reason)
      is InspectionSelection.Selected -> selection
    }
    val evaluation = repository.loadEvaluation(cluster)
    if (evaluation == null || !cluster.isCurrentEvaluation(evaluation, sha256Hex(selection.code))) {
      return rejectedFinalisation(
        generated,
        "No evaluation matches the selected inspection and the current examples; call edict_record_evaluation last",
      )
    }
    val validation = inspection.measure(cluster, selection.code).validation
    if (!validation.overallSuccess) {
      return rejectedFinalisation(generated, "Selected inspection has invalid compilation, metadata or execution: ${validation.summary}")
    }
    // On SKIP the evaluation keeps the predecessor's project review, which this run did not analyze.
    if (
      reportUploader != null && selection.action == EdictInspectionAction.GENERATE &&
      evaluation.projectReview != null && evaluation.projectReview.reportUrl == null
    ) {
      val findings = analyzedFindings[cluster.signalIds]
        ?: return rejectedFinalisation(generated, "Project analysis is unavailable for cloud upload; analyze and evaluate again")
      check(findings.candidateDigest == evaluation.inspectionHash && findings.projectRevision == evaluation.projectReview.projectRevision) {
        "Cloud upload requires the evaluated inspection's project analysis"
      }
      val reportUrl = reportUploader.upload(projectRoot, evaluation.inspectionName, findings)
      require(validReportUrl(reportUrl)) { "Cloud upload returned an invalid report URL" }
      repository.saveEvaluation(cluster.id, evaluation.copy(projectReview = evaluation.projectReview.copy(reportUrl = reportUrl)))
    }
    repository.appendHistory(cluster.id, "$generated: $reason")
    repository.markGenerated(cluster, selection.path)
    return EdictFinaliseClusterResponse(success = true, summary = "Cluster '${cluster.id}' is $generated")
  }

  /**
   * The inspection a Generated cluster publishes: the predecessor on SKIP; on GENERATE the candidate, which must be the
   * exact bytes the last project analysis ran.
   */
  private fun selectGeneratedInspection(cluster: EdictStoredCluster): InspectionSelection {
    val action = inspectionActions[cluster.signalIds]
                 ?: return InspectionSelection.Rejected("Call edict_get_inspection_action for cluster '${cluster.id}' first")
    val path = when (action) {
      EdictInspectionAction.CONFLICT -> return InspectionSelection.Rejected("A cluster with conflicting Signals cannot be Generated")
      EdictInspectionAction.SKIP -> {
        val predecessorId = cluster.manifest.predecessorId
                            ?: return InspectionSelection.Rejected("The reusable predecessor inspection is missing")
        repository.paths.inspectionPath(predecessorId)
      }
      EdictInspectionAction.GENERATE -> cluster.candidateInspectionPath
    }
    if (!path.isRegularFile()) return InspectionSelection.Rejected("Selected inspection does not exist: $path")
    val code = path.readText()
    if (action == EdictInspectionAction.GENERATE && analyzedCandidateDigests[cluster.signalIds] != sha256Hex(code)) {
      return InspectionSelection.Rejected("The current candidate has not completed project analysis")
    }
    return InspectionSelection.Selected(action, path, code)
  }

  private sealed interface InspectionSelection {
    data class Selected(val action: EdictInspectionAction, val path: Path, val code: String) : InspectionSelection
    data class Rejected(val reason: String) : InspectionSelection
  }

  // TODO - think about better comparison than exact fileRevision equality. Deferred for now
  private fun duplicateSignalAction(cluster: EdictStoredCluster): EdictInspectionActionResponse? {
    val conflictingSignalIds = cluster.signals.groupBy(EdictSignal::fileRevision).values.asSequence()
      // both positive and negative present
      .filter { signals -> signals.mapTo(hashSetOf(), EdictSignal::label).size > 1 }
      .flatten()
      .map(EdictSignal::id)
      .distinct()
      .sorted()
      .toList()
    return conflictingSignalIds.takeIf(List<String>::isNotEmpty)?.let {
      EdictInspectionActionResponse(
        EdictInspectionAction.CONFLICT,
        "Cluster state is invalid: Signals with the same file revision have conflicting labels; mark this cluster Invalid",
        conflictingSignalIds = it,
      )
    }
  }

  /**
   * SKIP when the predecessor inspection is unchanged and was evaluated on exactly the current code examples, so its
   * results stand, and no user answered one of its findings as a false positive since: a false positive is a case to look
   * into, even when an example already stands for it. Otherwise GENERATE.
   */
  private fun predecessorAction(cluster: EdictStoredCluster): EdictInspectionActionResponse {
    val evaluation = predecessorEvaluation(cluster)
      ?: return EdictInspectionActionResponse(EdictInspectionAction.GENERATE, NO_PREDECESSOR_EVALUATION)
    val falsePositives = cluster.unprocessedSignals
      .filter { it.isAnswer(EdictSignalLabel.NEGATIVE, evaluation.inspectionHash) }
      .map(EdictSignal::id)
    if (falsePositives.isNotEmpty()) {
      return EdictInspectionActionResponse(
        EdictInspectionAction.GENERATE,
        "Users answered findings of the predecessor inspection as false positives: ${falsePositives.joinToString()}",
      )
    }
    return EdictInspectionActionResponse(
      EdictInspectionAction.SKIP,
      "The predecessor inspection and the code examples are those of its evaluation; the new Signals are cases they already cover",
    )
  }

  /** The evaluation of the unchanged predecessor inspection on exactly the current code examples; null when there is none. */
  private fun predecessorEvaluation(cluster: EdictStoredCluster): EdictEvaluation? {
    val path = cluster.manifest.predecessorId?.let(repository.paths::inspectionPath)?.takeIf(Path::isRegularFile) ?: return null
    return repository.loadEvaluation(cluster)?.takeIf { cluster.isCurrentEvaluation(it, sha256Hex(path.readBytes())) }
  }


  private fun loadPendingCluster(clusterId: String): EdictStoredCluster {
    val cluster = repository.loadCluster(clusterId)
    require(cluster.manifest.status == EdictClusterStatus.Pending) { "Cluster '$clusterId' is not Pending" }
    return cluster
  }

  private fun requireNoIssues(issues: List<EdictValidationIssue>) {
    check(issues.isEmpty()) { issues.joinToString("\n") { "${it.path}: ${it.message}" } }
  }

  private fun rejectedFinalisation(
    status: EdictClusterStatus,
    summary: String,
    issues: List<EdictValidationIssue> = emptyList(),
  ): EdictFinaliseClusterResponse =
    EdictFinaliseClusterResponse(false, "Cluster cannot be $status and stays as it was: $summary", issues)
}

/**
 * The review of this analysis by the [reviewed] findings of the run: of those, it counts and keeps only the findings this
 * analysis reports, not those of earlier candidates.
 */
private fun EdictInspectionFindings.review(reviewed: Collection<EdictReviewedFinding>): EdictProjectReview {
  val locations = locations()
  val current = reviewed.filter { it.location() in locations }
  val truePositives = current.filter { it.classification == EdictFindingClassification.TP }.map { it.location() }.toSet()
  return EdictProjectReview(
    projectRevision = projectRevision,
    totalFindings = findings.size,
    reviewedFindings = current.size,
    truePositiveFindings = findings.filter { it.location() in truePositives },
  )
}

/** Why an example cannot run as a self-contained slice in its module, and how to repair it. */
private fun unresolvedIssue(exampleId: String, unresolved: EdictUnresolvedReferences): String =
  "Code example '$exampleId' has references that do not resolve in the module of " +
    "${unresolved.contextPath ?: "the project"}: ${unresolved.references.joinToString(", ")}" +
    ". Declare each one in the example, or set contextPath to a file whose module provides them"

private const val NO_PREDECESSOR_EVALUATION = "No predecessor inspection was evaluated on the current code examples"
