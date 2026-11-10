package org.jetbrains.qodana.edict.core

import org.jetbrains.qodana.edict.common.DEFAULT_GENERATION_COUNT
import org.jetbrains.qodana.edict.common.DEFAULT_MAX_PROJECT_ANALYSES
import org.jetbrains.qodana.edict.common.GitRepository
import java.nio.file.Path
import java.util.concurrent.ConcurrentHashMap
import kotlin.io.path.isRegularFile
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
  private val reviewedFindings = ConcurrentHashMap<Set<String>, List<Pair<String, List<EdictLineRange>?>>>()
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
    // Prefer clusters with strong positive evidence to guide generation
    val selected = state.clusters
      .filter { it.manifest.status == EdictClusterStatus.Pending }
      .filter { cluster ->
        cluster.signals.any { it.strength == EdictSignalStrength.STRONG && it.label == EdictSignalLabel.POSITIVE }
      }
      .sortedWith(
        compareByDescending<EdictStoredCluster> { cluster ->
          cluster.signals.asSequence()
            .filter {
              it.strength == EdictSignalStrength.STRONG && it.label == EdictSignalLabel.POSITIVE &&
                it.id !in cluster.manifest.processedSignalIds
            }
            .map { it.fileRevision.path }
            .distinct()
            .count()
        }.thenBy { it.id },
      )
      .take(targetCount)
    return EdictGenerationClustersResponse(
      clusters = selected.map { EdictGenerationTarget(it.id, it.directory.root.toString()) },
      summary = "${selected.size} generation candidate(s) suggested; the manager may choose other Pending clusters",
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
    return inspection.validate(cluster, code).also { validation ->
      if (validation.overallSuccess) {
        validatedCandidateDigests[cluster.signalIds] = sha256Hex(code)
        val failing = validation.failures.map { it.exampleId }.toSet()
        validatedExampleReports[cluster.signalIds] = sha256Hex(code) to cluster.examples.associate { example ->
          example.metadata.id to ((example.metadata.label == EdictSignalLabel.POSITIVE) == (example.metadata.id !in failing))
        }
      }
      else {
        validatedCandidateDigests.remove(cluster.signalIds)
        validatedExampleReports.remove(cluster.signalIds)
      }
    }
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

  fun appendHistory(clusterId: String, entry: String): EdictMutationResponse {
    loadPendingCluster(clusterId)
    repository.appendHistory(clusterId, entry)
    return EdictMutationResponse(true, "Appended history for cluster '$clusterId'")
  }

  fun clusterExamples(clusterId: String): String {
    val cluster = repository.loadCluster(clusterId)
    if (cluster.examples.isEmpty()) return "Cluster '$clusterId' has no code examples"
    val signalsByExample = cluster.signals.filter { it.syntheticExampleId != null }.groupBy { it.syntheticExampleId }
    return cluster.examples.joinToString("\n\n") { example ->
      val metadata = example.metadata
      val signals = signalsByExample[metadata.id].orEmpty()
      val strength = if (signals.any { it.strength == EdictSignalStrength.STRONG }) "strong" else "weak"
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

  suspend fun getInspectionAction(clusterId: String): EdictInspectionActionResponse {
    val cluster = loadPendingCluster(clusterId)
    val response = duplicateSignalAction(cluster) ?: predecessorAction(repository, cluster)
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
    val unchanged = reviewedFindings[cluster.signalIds] == findings.reviewKey()
    val reports = validatedExampleReports[cluster.signalIds]?.takeIf { it.first == findings.candidateDigest }?.second.orEmpty()
    analyzedExampleReports[cluster.signalIds] = reports
    val strong = cluster.strongExampleIds()
    val labels = cluster.examples.associate { it.metadata.id to it.metadata.label }
    val exampleStatus = reports.filterKeys { it in labels }.mapValues { (id, reported) ->
      EdictReviewExampleStatus(labels.getValue(id), id in strong, reported)
    }.toSortedMap()
    val locations = findings.findings.map { it.location() }.toSet()
    val unreproducedFindings = weakReviewResults[cluster.signalIds].orEmpty().values
      .filter { it.exampleId != null && reports[it.exampleId] == false }
      .groupBy { checkNotNull(it.exampleId) }
      .mapNotNull { (exampleId, assigned) ->
        val (still, gone) = assigned.partition { it.path to it.line in locations }
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
    val locations = analysis.findings.map { it.location() }.toSet()
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
      require(finding.path to finding.line in locations) {
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
    weakReviewResults.merge(cluster.signalIds, findings.associateBy { it.path to it.line }) { old, new -> old + new }
    reviewedFindings[cluster.signalIds] = analysis.reviewKey()
    return EdictMutationResponse(true, "Recorded ${findings.size} reviewed finding(s) of cluster '$clusterId'")
  }

  /**
   * Last step before finalising Generated: scores the inspection that would be published (the analyzed candidate, or the
   * predecessor on SKIP) on every strong and weak example and writes `evaluation.json`, including failed evidence cases.
   * Compilation, metadata and execution must be valid, but evidence accuracy does not gate recording.
   */
  suspend fun recordEvaluation(clusterId: String): EdictRecordEvaluationResponse {
    val cluster = loadPendingCluster(clusterId)
    return when (val selection = selectGeneratedInspection(cluster)) {
      is InspectionSelection.Rejected -> EdictRecordEvaluationResponse(false, selection.reason)
      is InspectionSelection.Selected -> {
        val reviewed = weakReviewResults[cluster.signalIds].orEmpty().values
        val exampleIds = cluster.examples.map { it.metadata.id }.toSet()
        val classified = reviewed.filter { it.classification != EdictFindingClassification.UNCERTAIN }
        // On GENERATE the selection requires that the last analysis, stored with its digest, was of these bytes (a rename
        // carries the digest over); SKIP publishes a predecessor this run did not analyze.
        val analysis = analyzedFindings[cluster.signalIds]?.takeIf { selection.action == EdictInspectionAction.GENERATE }
        val stillReported = analysis?.findings?.map { it.location() }?.toSet().orEmpty()
        val assignments = classified.mapNotNull { finding ->
          finding.exampleId?.takeIf { it in exampleIds }?.let {
            EdictFindingAssignment(it, finding.classification, finding.path to finding.line in stillReported)
          }
        }
        val truePositiveLocations = reviewed.filter { it.classification == EdictFindingClassification.TP }
          .map { it.path to it.line }.toSet()
        val projectReview = analysis?.let {
          EdictProjectReview(
            projectRevision = it.projectRevision,
            totalFindings = it.findings.size,
            reviewedFindings = reviewed.size,
            uncertain = reviewed.size - classified.size,
            unassigned = classified.size - assignments.size,
            unreproduced = 0,
            inspectionDescription = it.inspectionDescription,
            truePositiveFindings = it.findings.filter { finding -> finding.location() in truePositiveLocations },
          )
        }
        val (validation, evaluation) = inspection.evaluate(cluster, selection.code, selection.action, assignments, projectReview)
        if (evaluation == null) {
          EdictRecordEvaluationResponse(false, "Evaluation not recorded: ${validation.summary}")
        }
        else {
          repository.saveEvaluation(clusterId, evaluation)
          EdictRecordEvaluationResponse(
            true,
            "Recorded weighted evaluation: tp=${evaluation.tp}, fp=${evaluation.fp}, fn=${evaluation.fn}" +
            (evaluation.projectReview?.let {
              "; ${it.totalFindings} project finding(s), ${it.reviewedFindings} reviewed, ${it.unreproduced} not reproduced by their example"
            } ?: "") +
            "; ${validation.summary}",
            evaluation,
          )
        }
      }
    }
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
    if (
      evaluation == null || !cluster.isCurrentEvaluation(evaluation, sha256Hex(selection.code)) || evaluation.action != selection.action
    ) {
      return rejectedFinalisation(
        generated,
        "No evaluation matches the selected inspection and the current examples; call edict_record_evaluation last",
      )
    }
    val validation = inspection.validate(cluster, selection.code)
    if (!validation.overallSuccess) {
      return rejectedFinalisation(generated, "Selected inspection has invalid compilation, metadata or execution: ${validation.summary}")
    }
    if (reportUploader != null && evaluation.projectReview != null && evaluation.projectReview.reportUrl == null) {
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

  private suspend fun predecessorAction(
    repository: EdictRepository,
    cluster: EdictStoredCluster,
  ): EdictInspectionActionResponse {
    val predecessorId = cluster.manifest.predecessorId
                        ?: return EdictInspectionActionResponse(
                          EdictInspectionAction.GENERATE,
                          "The cluster has no predecessor inspection",
                        )
    val code = repository.paths.inspectionPath(predecessorId).readText()
    val validation = inspection.validate(cluster, code)
    val reusable = validation.strongExamplesSatisfied
    return EdictInspectionActionResponse(
      action = if (reusable) EdictInspectionAction.SKIP else EdictInspectionAction.GENERATE,
      summary = if (reusable) {
        "The predecessor inspection meets the strong-example reuse criterion: ${validation.summary}"
      } else {
        "The predecessor inspection does not meet the strong-example reuse criterion: ${validation.summary}"
      },
    )
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

/** Why an example cannot run as a self-contained slice in its module, and how to repair it. */
private fun unresolvedIssue(exampleId: String, unresolved: EdictUnresolvedReferences): String =
  "Code example '$exampleId' has references that do not resolve in the module of " +
    "${unresolved.contextPath ?: "the project"}: ${unresolved.references.joinToString(", ")}" +
    ". Declare each one in the example, or set contextPath to a file whose module provides them"
