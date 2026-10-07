package org.jetbrains.qodana.edict.edictnext

import kotlinx.coroutines.withTimeout
import kotlinx.coroutines.withTimeoutOrNull
import org.jetbrains.qodana.edict.common.DEFAULT_MAX_PROJECT_ANALYSES
import org.jetbrains.qodana.edict.common.GitRepository
import java.nio.file.Path
import java.util.concurrent.ConcurrentHashMap
import kotlin.io.path.isRegularFile
import kotlin.io.path.readText
import kotlin.time.TimeMark
import kotlin.time.TimeSource

/** Generates inspections for the run's Pending clusters, checked against the snapshot [getGenerationClusters] took. */
internal class EdictNextGenerationService(
  private val repository: EdictRepository,
  inspectionServer: IntellijMcpServerService,
  private val projectRoot: Path,
  private val maxProjectAnalyses: Int = DEFAULT_MAX_PROJECT_ANALYSES,
) {
  private val inspection = EdictNextInspection(inspectionServer)

  /** The analyzed project's HEAD, resolved on first use and fixed for the rest of the run. */
  private val projectRevision: String by lazy { GitRepository(projectRoot).resolve("HEAD") }
  private val clusterGenerationStarts = ConcurrentHashMap<Set<String>, TimeMark>()
  private val generationTargetSignalIdsByClusterId = ConcurrentHashMap<String, Set<String>>()
  private val inspectionActions = ConcurrentHashMap<Set<String>, EdictNextInspectionAction>()
  private val validatedCandidateDigests = ConcurrentHashMap<Set<String>, String>()
  private val analyzedCandidateDigests = ConcurrentHashMap<Set<String>, String>()
  private val projectAnalysisCounts = ConcurrentHashMap<Set<String>, Int>()
  private val reviewedFindings = ConcurrentHashMap<Set<String>, List<Pair<String, List<EdictNextLineRange>?>>>()
  private var generationSnapshot: EdictNextGenerationSnapshot? = null

  suspend fun getGenerationClusters(): EdictNextGenerationClustersResponse {
    val state = repository.loadState()
    requireNoIssues(validateRepositoryState(repository, state))
    // No candidate can pass without strong positive evidence, so such a cluster waits Pending for a later run.
    val (targets, withoutStrongPositive) = state.clusters
      .filter { it.manifest.status == EdictNextClusterStatus.Pending }
      .partition { cluster ->
        cluster.signals.any { it.strength == EdictNextSignalStrength.STRONG && it.label == EdictNextSignalLabel.POSITIVE }
      }
    generationSnapshot = EdictNextGenerationSnapshot(state, targets.associate { it.id to it.signalIds })
    clusterGenerationStarts.clear()
    generationTargetSignalIdsByClusterId.clear()
    generationTargetSignalIdsByClusterId.putAll(targets.associate { it.id to it.signalIds })
    inspectionActions.clear()
    validatedCandidateDigests.clear()
    analyzedCandidateDigests.clear()
    reviewedFindings.clear()
    projectAnalysisCounts.clear()
    return EdictNextGenerationClustersResponse(
      clusters = targets.map { EdictNextGenerationTarget(it.id, it.directory.root.toString()) },
      clustersWithoutStrongPositiveSignal = withoutStrongPositive.map { it.id },
      summary = "${targets.size} generation target(s) are ready; ${withoutStrongPositive.size} Pending cluster(s) have no " +
        "strong positive Signal and stay Pending without a generation attempt",
    )
  }

  suspend fun validateCodeExample(clusterId: String, exampleId: String): EdictNextCodeExampleValidationResponse =
    withinClusterGenerationDeadline(clusterId, generationTargetSignalIdsByClusterId[clusterId]) {
      validateCodeExample(repository, clusterId, exampleId)
    }

  suspend fun saveCodeExample(
    clusterId: String,
    exampleId: String,
    metadataJson: String,
    sourceCode: String,
  ): EdictNextMutationResponse = withinGenerationTarget(clusterId) { repository ->
    val metadata = EdictNextJson.decodeFromString<EdictNextCodeExampleMetadata>(metadataJson)
    val directory = EdictNextExampleDirectory(
      EdictNextClusterDirectory(repository.paths.clustersDirectory.resolve(clusterId)).examplesDirectory.resolve(exampleId),
    )
    val validation = validateCodeExample(directory, metadata, directory.sourcePath(metadata.fileName), sourceCode)
    require(validation.success) { validation.summary + ": " + validation.issues.joinToString("; ") }
    repository.saveCodeExample(clusterId, exampleId, metadata, sourceCode)
    EdictNextMutationResponse(true, "Stored code example '$exampleId' in cluster '$clusterId'")
  }

  suspend fun assignCodeExample(
    clusterId: String,
    signalId: String,
    exampleId: String,
  ): EdictNextMutationResponse = withinGenerationTarget(clusterId) { repository ->
    val validation = validateCodeExample(repository, clusterId, exampleId)
    require(validation.success) { validation.summary + ": " + validation.issues.joinToString("; ") }
    repository.assignCodeExample(clusterId, signalId, exampleId)
    EdictNextMutationResponse(true, "Assigned code example '$exampleId' to Signal '$signalId'")
  }

  suspend fun deleteCodeExample(clusterId: String, exampleId: String): EdictNextMutationResponse =
    withinGenerationTarget(clusterId) { repository ->
      repository.deleteCodeExample(clusterId, exampleId)
      EdictNextMutationResponse(true, "Deleted code example '$exampleId' from cluster '$clusterId' and unassigned its Signals")
    }

  /**
   * Stores the candidate whatever its quality, then compiles it and runs every example, so the stored candidate is always
   * the latest attempt. Only a candidate that passes every strong example may go on to project analysis.
   */
  suspend fun saveCandidateInspection(clusterId: String, code: String): EdictNextInspectionValidationResponse =
    withinGenerationTarget(clusterId) { repository ->
      repository.saveCandidateInspection(clusterId, code)
      val cluster = repository.loadCluster(clusterId)
      inspection.validate(cluster, code).also { validation ->
        if (validation.overallSuccess) validatedCandidateDigests[cluster.signalIds] = sha256Hex(code)
        else validatedCandidateDigests.remove(cluster.signalIds)
      }
    }

  /**
   * Renames a frozen Pending target and the `id = "<clusterId>"` literal of its candidate. Only the id changes, so a
   * validated or analyzed candidate stays so under the new id.
   */
  suspend fun renameCluster(clusterId: String, newClusterId: String): EdictNextMutationResponse =
    withinGenerationTarget(clusterId) { repository ->
      if (newClusterId == clusterId) {
        return@withinGenerationTarget EdictNextMutationResponse(true, "Cluster '$clusterId' already has this id")
      }
      val cluster = repository.loadCluster(clusterId)
      val code = cluster.candidateInspectionPath.takeIf(Path::isRegularFile)?.readText()
      val renamedCode = code?.replace(Regex("(\\bid\\s*=\\s*)\"${Regex.escape(clusterId)}\""), "$1\"$newClusterId\"")
      repository.renameCluster(cluster, newClusterId, renamedCode)
      generationTargetSignalIdsByClusterId[newClusterId] = cluster.signalIds
      generationTargetSignalIdsByClusterId.remove(clusterId)
      if (code != null && renamedCode != null) {
        for (digests in listOf(validatedCandidateDigests, analyzedCandidateDigests)) {
          digests.replace(cluster.signalIds, sha256Hex(code), sha256Hex(renamedCode))
        }
      }
      EdictNextMutationResponse(true, "Renamed cluster '$clusterId' to '$newClusterId'")
    }

  suspend fun appendHistory(clusterId: String, entry: String): EdictNextMutationResponse =
    withinGenerationTarget(clusterId) { repository ->
      repository.appendHistory(clusterId, entry)
      EdictNextMutationResponse(true, "Appended history for cluster '$clusterId'")
    }

  suspend fun validateClusterExamples(clusterId: String): EdictNextCodeExampleValidationResponse {
    val cluster = try {
      repository.loadCluster(clusterId)
    }
    catch (e: EdictNextCodeExampleReadException) {
      return EdictNextCodeExampleValidationResponse(
        success = false,
        summary = "Found ${e.issues.size} cluster code example issue(s)",
        issues = e.issues,
      )
    }
    return withinClusterGenerationDeadline(clusterId, cluster.signalIds) {
      val issues = validateClusterExamples(cluster)
      EdictNextCodeExampleValidationResponse(
        success = issues.isEmpty(),
        summary = if (issues.isEmpty()) {
          "Every Signal has a structurally valid focused code example"
        } else {
          "Found ${issues.size} cluster code example issue(s)"
        },
        issues = issues.map { "${it.path}: ${it.message}" },
      )
    }
  }

  suspend fun getInspectionAction(clusterId: String): EdictNextInspectionActionResponse {
    val cluster = repository.loadCluster(clusterId)
    return withinClusterGenerationDeadline(clusterId, cluster.signalIds, start = true) {
      require(cluster.manifest.status == EdictNextClusterStatus.Pending) { "Cluster '$clusterId' is not Pending" }
      val response = duplicateSignalAction(cluster) ?: predecessorAction(repository, cluster)
      inspectionActions[cluster.signalIds] = response.action
      response
    }
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
  ): EdictNextInspectionResultsResponse {
    val cluster = repository.loadCluster(clusterId)
    return withinClusterGenerationDeadline(clusterId, cluster.signalIds) {
      val code = cluster.candidateInspectionPath.takeIf(Path::isRegularFile)?.readText()
      check(code != null && validatedCandidateDigests[cluster.signalIds] == sha256Hex(code)) {
        "The current candidate of cluster '$clusterId' has not passed validation; " +
        "save a candidate that passes every strong example first"
      }
      val analysis = checkNotNull(projectAnalysisCounts.merge(cluster.signalIds, 1, Int::plus))
      check(analysis <= maxProjectAnalyses) {
        "Cluster '$clusterId' used all $maxProjectAnalyses project analyses of this run; stop generating. " +
        "Finalise the last analyzed candidate Generated if it passes every strong example, otherwise finalise Pending"
      }
      withTimeout(EdictNextTimeouts.analysis) {
        val findings = inspection.analyzeProject(cluster, code, projectRevision)
        val key = findings.reviewKey()
        val unchanged = reviewedFindings.put(cluster.signalIds, key) == key
        val response = EdictNextReviewArtifacts.create(
          findings = findings,
          clusterDirectory = cluster.directory.root,
          candidateInspection = cluster.candidateInspectionPath,
          inspectedProject = projectRoot,
          privateScratchDirectory = Path.of(privateScratchDirectory).toAbsolutePath().normalize(),
          remainingProjectAnalyses = maxProjectAnalyses - analysis,
        )
        analyzedCandidateDigests[cluster.signalIds] = findings.candidateDigest
        response.copy(findingsUnchanged = unchanged)
      }
    }
  }

  /**
   * Last step before finalising Generated: scores the inspection that would be published (the analyzed candidate, or the
   * predecessor on SKIP) on every strong and weak example and writes `evaluation.json`. Nothing is written unless every
   * strong example passes.
   */
  suspend fun recordEvaluation(clusterId: String): EdictNextRecordEvaluationResponse = withinGenerationTarget(clusterId) { repository ->
    val cluster = repository.loadCluster(clusterId)
    require(cluster.manifest.status == EdictNextClusterStatus.Pending) { "Cluster '$clusterId' is not Pending" }
    when (val selection = selectGeneratedInspection(cluster)) {
      is InspectionSelection.Rejected -> EdictNextRecordEvaluationResponse(false, selection.reason)
      is InspectionSelection.Selected -> {
        val (validation, evaluation) = inspection.evaluate(cluster, selection.code, selection.action)
        if (evaluation == null) {
          EdictNextRecordEvaluationResponse(false, "Evaluation not recorded: ${validation.summary}")
        }
        else {
          repository.saveEvaluation(clusterId, evaluation)
          EdictNextRecordEvaluationResponse(
            true,
            "Recorded evaluation: tp=${evaluation.tp}, fp=${evaluation.fp}, fn=${evaluation.fn}; ${validation.summary}",
            evaluation,
          )
        }
      }
    }
  }

  /**
   * Ends the processing of a frozen target in [status], recording [reason] in its history. Each status is reached only
   * with its contract satisfied, so [validateGeneration] accepts the result; otherwise the cluster stays as it was.
   */
  suspend fun finaliseCluster(
    clusterId: String,
    status: EdictNextClusterStatus,
    reason: String,
  ): EdictNextFinaliseClusterResponse = withinGenerationTarget(clusterId) { repository ->
    require(reason.isNotBlank()) { "Reason must not be blank" }
    val cluster = repository.loadCluster(clusterId)
    require(cluster.manifest.status == EdictNextClusterStatus.Pending) { "Cluster '$clusterId' is not Pending" }
    val history = cluster.historyPath.takeIf(Path::isRegularFile)?.readText().orEmpty() + reason
    if (status == EdictNextClusterStatus.Generated) return@withinGenerationTarget finaliseGenerated(cluster, history, reason)

    val issues = validateCluster(cluster.copy(manifest = cluster.manifest.copy(status = status)), history)
    if (issues.isNotEmpty()) return@withinGenerationTarget rejectedFinalisation(status, "Found ${issues.size} issue(s)", issues)
    repository.appendHistory(clusterId, "$status: $reason")
    when (status) {
      EdictNextClusterStatus.Discontinued -> repository.markDiscontinued(cluster)
      EdictNextClusterStatus.Invalid -> repository.markInvalid(cluster)
      EdictNextClusterStatus.Pending -> Unit
    }
    EdictNextFinaliseClusterResponse(success = true, summary = "Cluster '$clusterId' is $status")
  }

  suspend fun validateGeneration(): EdictNextValidationResponse {
    val generationSnapshot = checkNotNull(generationSnapshot) { "Call edict_next_get_generation_clusters first" }
    val current = try {
      repository.loadState()
    }
    catch (e: Exception) {
      return EdictNextValidationResponse(
        false,
        "Repository structure is invalid",
        listOf(EdictNextValidationIssue(repository.paths.root.toString(), e.message.orEmpty())),
      )
    }
    val issues = validateRepositoryState(repository, current).toMutableList()
    issues += validateGenerationChange(generationSnapshot.state, current, generationSnapshot.signalIds)
    generationSnapshot.targetSignalIds.values.forEach { signalIds ->
      val cluster = current.clusters.singleOrNull { it.signalIds == signalIds }
      if (cluster?.manifest?.status == EdictNextClusterStatus.Generated) {
        val code = current.inspectionCode(cluster.id) ?: return@forEach
        val validation = inspection.validate(cluster, code)
        if (!validation.overallSuccess) {
          issues += EdictNextValidationIssue(
            repository.paths.inspectionPath(cluster.id).toString(),
            "Generated inspection does not meet the acceptance criterion: ${validation.summary}",
          )
        }
      }
    }
    return EdictNextValidationResponse(
      success = issues.isEmpty(),
      summary = if (issues.isEmpty()) "Generation changes are valid" else "Found ${issues.size} generation issue(s)",
      issues = issues,
    )
  }

  private suspend fun finaliseGenerated(
    cluster: EdictNextStoredCluster,
    history: String,
    reason: String,
  ): EdictNextFinaliseClusterResponse {
    val generated = EdictNextClusterStatus.Generated
    val issues = validateGeneratedTransition(repository, cluster, history)
    if (issues.isNotEmpty()) return rejectedFinalisation(generated, "Found ${issues.size} issue(s)", issues)
    val selection = when (val selection = selectGeneratedInspection(cluster)) {
      is InspectionSelection.Rejected -> return rejectedFinalisation(generated, selection.reason)
      is InspectionSelection.Selected -> selection
    }
    val evaluation = repository.loadEvaluation(cluster)
    if (
      evaluation == null ||
      evaluation.action != selection.action ||
      evaluation.inspectionHash != sha256Hex(selection.code) ||
      evaluation.exampleSetDigest != exampleSetDigest(cluster)
    ) {
      return rejectedFinalisation(
        generated,
        "No evaluation matches the selected inspection and the current examples; call edict_next_record_evaluation last",
      )
    }
    val validation = inspection.validate(cluster, selection.code)
    if (!validation.overallSuccess) {
      return rejectedFinalisation(generated, "Selected inspection does not meet the acceptance criterion: ${validation.summary}")
    }
    repository.appendHistory(cluster.id, "$generated: $reason")
    repository.markGenerated(cluster, selection.path)
    return EdictNextFinaliseClusterResponse(success = true, summary = "Cluster '${cluster.id}' is $generated")
  }

  /**
   * The inspection a Generated cluster publishes: the predecessor on SKIP; on GENERATE the candidate, which must be the
   * exact bytes the last project analysis ran.
   */
  private fun selectGeneratedInspection(cluster: EdictNextStoredCluster): InspectionSelection {
    val action = inspectionActions[cluster.signalIds]
                 ?: return InspectionSelection.Rejected("Call edict_next_get_inspection_action for cluster '${cluster.id}' first")
    val path = when (action) {
      EdictNextInspectionAction.CONFLICT -> return InspectionSelection.Rejected("A cluster with conflicting Signals cannot be Generated")
      EdictNextInspectionAction.SKIP -> {
        val predecessorId = cluster.manifest.predecessorId
                            ?: return InspectionSelection.Rejected("The reusable predecessor inspection is missing")
        repository.paths.inspectionPath(predecessorId)
      }
      EdictNextInspectionAction.GENERATE -> cluster.candidateInspectionPath
    }
    if (!path.isRegularFile()) return InspectionSelection.Rejected("Selected inspection does not exist: $path")
    val code = path.readText()
    if (action == EdictNextInspectionAction.GENERATE && analyzedCandidateDigests[cluster.signalIds] != sha256Hex(code)) {
      return InspectionSelection.Rejected("The current candidate has not completed project analysis")
    }
    return InspectionSelection.Selected(action, path, code)
  }

  private sealed interface InspectionSelection {
    data class Selected(val action: EdictNextInspectionAction, val path: Path, val code: String) : InspectionSelection
    data class Rejected(val reason: String) : InspectionSelection
  }

  // TODO - think about better comparison than exact fileRevision equality. Deferred for now
  private fun duplicateSignalAction(cluster: EdictNextStoredCluster): EdictNextInspectionActionResponse? {
    val conflictingSignalIds = cluster.signals.groupBy(EdictNextSignal::fileRevision).values.asSequence()
      // both positive and negative present
      .filter { signals -> signals.mapTo(hashSetOf(), EdictNextSignal::label).size > 1 }
      .flatten()
      .map(EdictNextSignal::id)
      .distinct()
      .sorted()
      .toList()
    return conflictingSignalIds.takeIf(List<String>::isNotEmpty)?.let {
      EdictNextInspectionActionResponse(
        EdictNextInspectionAction.CONFLICT,
        "Cluster state is invalid: Signals with the same file revision have conflicting labels; mark this cluster Invalid",
        conflictingSignalIds = it,
      )
    }
  }

  private suspend fun predecessorAction(
    repository: EdictRepository,
    cluster: EdictNextStoredCluster,
  ): EdictNextInspectionActionResponse {
    val predecessorId = cluster.manifest.predecessorId
                        ?: return EdictNextInspectionActionResponse(
                          EdictNextInspectionAction.GENERATE,
                          "The cluster has no predecessor inspection",
                        )
    val code = repository.paths.inspectionPath(predecessorId).readText()
    val validation = inspection.validate(cluster, code)
    val reusable = validation.overallSuccess
    return EdictNextInspectionActionResponse(
      action = if (reusable) EdictNextInspectionAction.SKIP else EdictNextInspectionAction.GENERATE,
      summary = if (reusable) {
        "The predecessor inspection meets the acceptance criterion: ${validation.summary}"
      } else {
        "The predecessor inspection does not meet the acceptance criterion: ${validation.summary}"
      },
    )
  }

  private suspend fun <T : Any> withinGenerationTarget(
    clusterId: String,
    action: suspend (EdictRepository) -> T,
  ): T {
    val frozenSignalIds = generationTargetSignalIdsByClusterId[clusterId]
                          ?: error("Cluster '$clusterId' is not a frozen generation target")
    require(repository.clusterSignalIds(clusterId) == frozenSignalIds) { "Cluster '$clusterId' Signal membership changed" }
    return withinClusterGenerationDeadline(clusterId, frozenSignalIds) { action(repository) }
  }

  private fun requireNoIssues(issues: List<EdictNextValidationIssue>) {
    check(issues.isEmpty()) { issues.joinToString("\n") { "${it.path}: ${it.message}" } }
  }

  private fun rejectedFinalisation(
    status: EdictNextClusterStatus,
    summary: String,
    issues: List<EdictNextValidationIssue> = emptyList(),
  ): EdictNextFinaliseClusterResponse =
    EdictNextFinaliseClusterResponse(false, "Cluster cannot be $status and stays as it was: $summary", issues)

  private suspend fun <T : Any> withinClusterGenerationDeadline(
    clusterId: String,
    signalIds: Set<String>?,
    start: Boolean = false,
    action: suspend () -> T,
  ): T {
    if (signalIds == null) return action()
    generationTargetSignalIdsByClusterId[clusterId] = signalIds
    val startedAt = if (start) {
      clusterGenerationStarts.computeIfAbsent(signalIds) { TimeSource.Monotonic.markNow() }
    }
    else {
      clusterGenerationStarts[signalIds]
    }
    if (startedAt == null) return action()

    val remaining = EdictNextTimeouts.clusterGeneration - startedAt.elapsedNow()
    if (!remaining.isPositive()) clusterGenerationTimedOut(clusterId)
    return withTimeoutOrNull(remaining) { action() } ?: clusterGenerationTimedOut(clusterId)
  }

  private fun clusterGenerationTimedOut(clusterId: String): Nothing {
    error(
      "Cluster '$clusterId' generation exceeded its 120-minute limit. " +
      "Cleanup current session to valid Pending state and stop generation"
    )
  }
}

internal data class EdictNextGenerationSnapshot(
  val state: EdictNextRepositoryState,
  val targetSignalIds: Map<String, Set<String>>,
) {
  val signalIds: Set<String> = targetSignalIds.values.flatten().toSet()
}
