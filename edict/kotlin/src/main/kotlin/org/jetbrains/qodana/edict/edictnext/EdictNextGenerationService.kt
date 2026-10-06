package org.jetbrains.qodana.edict.edictnext

import kotlinx.coroutines.withTimeout
import kotlinx.coroutines.withTimeoutOrNull
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
) {
  private val inspection = EdictNextInspection(inspectionServer)

  /** The analyzed project's HEAD, resolved on first use and fixed for the rest of the run. */
  private val projectRevision: String by lazy { GitRepository(projectRoot).resolve("HEAD") }
  private val clusterGenerationStarts = ConcurrentHashMap<Set<String>, TimeMark>()
  private val generationTargetSignalIdsByClusterId = ConcurrentHashMap<String, Set<String>>()
  private val inspectionActions = ConcurrentHashMap<Set<String>, EdictNextInspectionAction>()
  private val analyzedCandidateDigests = ConcurrentHashMap<Set<String>, String>()
  private var generationSnapshot: EdictNextGenerationSnapshot? = null

  suspend fun getGenerationClusters(): EdictNextGenerationClustersResponse {
    val state = repository.loadState()
    requireNoIssues(validateRepositoryState(repository, state))
    val targets = state.clusters.filter { it.manifest.status == EdictNextClusterStatus.Pending }
    generationSnapshot = EdictNextGenerationSnapshot(state, targets.associate { it.id to it.signalIds })
    clusterGenerationStarts.clear()
    generationTargetSignalIdsByClusterId.clear()
    generationTargetSignalIdsByClusterId.putAll(targets.associate { it.id to it.signalIds })
    inspectionActions.clear()
    analyzedCandidateDigests.clear()
    return EdictNextGenerationClustersResponse(
      clusters = targets.map { EdictNextGenerationTarget(it.id, it.directory.root.toString()) },
      summary = "${targets.size} generation target(s) are ready",
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
      EdictNextMutationResponse(true, "Deleted unassigned code example '$exampleId' from cluster '$clusterId'")
    }

  suspend fun saveCandidateInspection(clusterId: String, code: String): EdictNextMutationResponse =
    withinGenerationTarget(clusterId) { repository ->
      repository.saveCandidateInspection(clusterId, code)
      EdictNextMutationResponse(true, "Stored candidate inspection for cluster '$clusterId'")
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

  suspend fun validateInspection(clusterId: String): EdictNextInspectionValidationResponse {
    val cluster = repository.loadCluster(clusterId)
    return withinClusterGenerationDeadline(clusterId, cluster.signalIds) {
      inspection.validate(cluster, cluster.candidateInspectionPath.readText())
    }
  }

  suspend fun getNewInspectionResults(
    clusterId: String,
    privateScratchDirectory: String,
  ): EdictNextInspectionResultsResponse {
    val cluster = repository.loadCluster(clusterId)
    return withinClusterGenerationDeadline(clusterId, cluster.signalIds) {
      withTimeout(EdictNextTimeouts.analysis) {
        val code = cluster.candidateInspectionPath.readText()
        val findings = inspection.analyzeProject(cluster, code, projectRevision)
        val response = EdictNextReviewArtifacts.create(
          findings = findings,
          clusterDirectory = cluster.directory.root,
          candidateInspection = cluster.candidateInspectionPath,
          inspectedProject = projectRoot,
          privateScratchDirectory = Path.of(privateScratchDirectory).toAbsolutePath().normalize(),
        )
        analyzedCandidateDigests[cluster.signalIds] = findings.candidateDigest
        response
      }
    }
  }

  suspend fun markGenerated(clusterId: String): EdictNextMarkGeneratedResponse {
    val generationSnapshot = checkNotNull(generationSnapshot) { "Call edict_next_get_generation_clusters first" }
    val cluster = repository.loadCluster(clusterId)
    return withinClusterGenerationDeadline(clusterId, cluster.signalIds) {
      val frozenSignalIds = generationSnapshot.targetSignalIds.values.singleOrNull { it == cluster.signalIds }
                            ?: return@withinClusterGenerationDeadline rejectedGeneratedTransition(
                              "Cluster '$clusterId' is not an unchanged frozen generation target"
                            )
      val history = cluster.historyPath.takeIf(Path::isRegularFile)?.readText().orEmpty()
      val issues = validateGeneratedTransition(repository, cluster, history)
      if (issues.isNotEmpty()) {
        return@withinClusterGenerationDeadline rejectedGeneratedTransition(
          "Found ${issues.size} issue(s) preventing the Generated transition",
          issues,
        )
      }
      val action = inspectionActions[frozenSignalIds]
                   ?: return@withinClusterGenerationDeadline rejectedGeneratedTransition(
                     "Call edict_next_get_inspection_action for cluster '$clusterId' first"
                   )
      val selectedInspection = when (action) {
        EdictNextInspectionAction.CONFLICT -> return@withinClusterGenerationDeadline rejectedGeneratedTransition(
          "A cluster with conflicting Signals cannot be marked Generated"
        )
        EdictNextInspectionAction.SKIP -> {
          val predecessorId = cluster.manifest.predecessorId
                              ?: return@withinClusterGenerationDeadline rejectedGeneratedTransition(
                                "The reusable predecessor inspection is missing"
                              )
          if (generationSnapshot.isPredecessorShared(frozenSignalIds, predecessorId)) {
            return@withinClusterGenerationDeadline rejectedGeneratedTransition(
              "Predecessor inspection '$predecessorId' belongs to another frozen generation target"
            )
          }
          repository.paths.inspectionPath(predecessorId)
        }
        EdictNextInspectionAction.GENERATE -> cluster.candidateInspectionPath
      }
      if (!selectedInspection.isRegularFile()) {
        return@withinClusterGenerationDeadline rejectedGeneratedTransition(
          "Selected inspection does not exist: $selectedInspection"
        )
      }

      val code = selectedInspection.readText()
      if (action == EdictNextInspectionAction.GENERATE && analyzedCandidateDigests[frozenSignalIds] != sha256Hex(code)) {
        return@withinClusterGenerationDeadline rejectedGeneratedTransition(
          "The current candidate has not completed project analysis"
        )
      }
      val validation = inspection.validate(cluster, code)
      if (!validation.overallSuccess) {
        return@withinClusterGenerationDeadline rejectedGeneratedTransition(
          "Selected inspection does not meet the acceptance criterion: ${validation.summary}"
        )
      }

      repository.markGenerated(cluster, selectedInspection)
      EdictNextMarkGeneratedResponse(
        success = true,
        summary = "Cluster '$clusterId' is Generated",
      )
    }
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

  private fun rejectedGeneratedTransition(
    summary: String,
    issues: List<EdictNextValidationIssue> = emptyList(),
  ): EdictNextMarkGeneratedResponse = EdictNextMarkGeneratedResponse(false, summary, issues)

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

  fun isPredecessorShared(signalIds: Set<String>, predecessorId: String): Boolean = state.clusters.any { cluster ->
    cluster.signalIds != signalIds && cluster.signalIds in targetSignalIds.values && cluster.manifest.predecessorId == predecessorId
  }
}
