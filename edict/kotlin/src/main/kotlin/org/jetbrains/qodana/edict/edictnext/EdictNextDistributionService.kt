package org.jetbrains.qodana.edict.edictnext

import java.util.concurrent.ConcurrentHashMap

/** Prepares and applies one sequential distribution batch. */
internal class EdictNextDistributionService private constructor(private val sessionId: String) {
    private val context: EdictSessionContext get() = EdictSessionContext.getInstance(sessionId)
    private var batch: DistributionBatch? = null
    internal var maxInboxSignalsPerRun: Int = EDICT_NEXT_MAX_INBOX_SIGNALS_PER_RUN

    suspend fun preparePipeline(): EdictNextPreparePipelineResponse {
        val sourceRepository = EdictRepository(EdictRepositoryDirectory(context.sourceRepository))
        val sourceState = sourceRepository.loadState()
        requireValidState(sourceRepository, sourceState)

        val repository = EdictRepository(EdictRepositoryDirectory(context.managedRepository))
        val initialState = repository.loadState()
        requireValidState(repository, initialState)
        requireNoIssues(validateDistributionChange(sourceState, initialState, emptySet()))

        val signalIds = initialState.jvmInboxSignalIds.sorted().take(maxInboxSignalsPerRun)
        val neighbours = prepareNeighbours(repository, initialState, signalIds)
        context.useRepository(repository)
        batch = DistributionBatch(sourceRepository, sourceState, initialState, signalIds.toSet(), neighbours)

        return EdictNextPreparePipelineResponse(
          summary = "${initialState.clusters.size} cluster(s), ${signalIds.size} Signal(s) selected for distribution",
        )
    }

    suspend fun nextSignal(): EdictNextSignalPreparationResponse {
        val currentBatch = requireBatch()
        val repository = context.repository()
        val state = repository.loadState()
        val inbox = state.inboxSignals
        val signalId = inbox.asSequence().map(EdictNextSignal::id).filter(currentBatch.signalIds::contains).minOrNull()
        if (signalId == null) {
            currentBatch.clearCurrentSignal()
            return EdictNextSignalPreparationResponse(summary = "All selected Signals were distributed")
        }
        currentBatch.startSignal(signalId)
        return EdictNextSignalPreparationResponse(
          signalId = signalId,
          signalPath = repository.paths.inboxDirectory.resolve("$signalId.json").toString(),
          signal = state.inboxSignalsById.getValue(signalId),
          candidates = mergeCandidates(currentBatch.neighbours.getValue(signalId), state.clusters, inbox, repository),
          summary = "Prepared Signal '$signalId' with its nearest candidates",
        )
    }

    fun getContext(kind: String, id: String): EdictNextDistributionContextResponse {
        val currentBatch = requireBatch()
        currentBatch.requireCurrentSignal()
        val repository = context.repository()
        return when (kind) {
            SIGNAL_CONTEXT_KIND -> EdictNextDistributionContextResponse.Signal(
                signal = requireNotNull(currentBatch.worktreeState.signalsById[id]) { "Signal '$id' does not exist" }
                    .toDistributionSignal(),
            )

            CLUSTER_CONTEXT_KIND -> {
                val cluster = repository.loadCluster(id)
                val response = EdictNextDistributionContextResponse.Cluster(
                    clusterId = cluster.id,
                    signals = cluster.signals.map(EdictNextSignal::toDistributionSignal),
                )
                currentBatch.recordClusterContext(id)
                response
            }

            else -> error("Unsupported distribution context kind '$kind'; expected '$SIGNAL_CONTEXT_KIND' or '$CLUSTER_CONTEXT_KIND'")
        }
    }

    fun addSignalToCluster(
        signalId: String,
        clusterId: String,
    ): EdictNextSignalValidationResponse {
        val currentBatch = requireBatch()
        require(signalId in currentBatch.signalIds) { "Signal '$signalId' was not selected for this run" }
        currentBatch.requireCurrentSignal(signalId)
        val repository = context.repository()
        val clusters = repository.loadClusters()
        if (clusters.any { it.id == clusterId }) {
            currentBatch.requireClusterContext(clusterId)
        }
        else {
            val signal = repository.loadInboxSignals().single { it.id == signalId }
            val identityMatches = clustersSharingInspectionIdentity(signal, clusters)
            if (identityMatches.isNotEmpty()) {
                return EdictNextSignalValidationResponse(
                  signalId = signalId,
                  summary = "Signal '$signalId' has the same SubmittedFeedback inspectionName as existing " +
                    "cluster(s) ${identityMatches.joinToString()}; load that cluster's context and reuse it instead " +
                    "of creating '$clusterId'",
                  added = false,
                )
            }
        }
        repository.addSignalToCluster(signalId, clusterId)
        currentBatch.completeSignal(signalId)
        return EdictNextSignalValidationResponse(
          signalId = signalId,
          summary = "Assigned Signal '$signalId' to '$clusterId'",
          added = true,
        )
    }

    suspend fun validateDistribution(): EdictNextValidationResponse {
        val currentBatch = requireBatch()
        val repository = context.repository()
        val current = try {
            repository.loadState()
        } catch (e: Exception) {
            return invalidState(repository, e)
        }
        // A local managed run may intentionally use one directory as both source and worktree. In that case the
        // immutable source is the snapshot captured by preparePipeline; reloading the shared path would mistake this
        // distribution's own moves for concurrent source changes. Separate source/worktree runs remain strict.
        val source = if (currentBatch.sourceRepository.paths.root == repository.paths.root) {
            currentBatch.sourceState
        }
        else try {
            currentBatch.sourceRepository.loadState()
        }
        catch (e: Exception) {
            return invalidState(currentBatch.sourceRepository, e)
        }
        val issues = validateRepositoryState(repository, current).toMutableList()
        issues += validateDistributionChange(currentBatch.sourceState, source, emptySet())
        issues += validateDistributionChange(currentBatch.worktreeState, current, currentBatch.signalIds)
        return EdictNextValidationResponse(
          success = issues.isEmpty(),
          summary = if (issues.isEmpty()) "Distribution changes are valid" else "Found ${issues.size} distribution issue(s)",
          issues = issues,
          nextAction = if (issues.isEmpty()) EdictNextNextAction.STOP_DISTRIBUTION else EdictNextNextAction.REPAIR_REPOSITORY,
        )
    }

    fun clear() {
        batch = null
    }

    private suspend fun prepareNeighbours(
        repository: EdictRepository,
        state: EdictNextRepositoryState,
        signalIds: List<String>,
    ): Map<String, EdictNextSignalNeighbours> {
        if (signalIds.isEmpty()) return emptyMap()
        val runner = EdictScriptRunner(context.workspace, context.embeddingPython)
        try {
            runner.prepareEnvironment()
            runner.importEmbeddingCache(repository.paths.embeddingsDirectory)
            val corpus =
                state.clusters.flatMap(EdictNextStoredCluster::signals) + signalIds.map(state.signalsById::getValue)
            return runner.prepareCorpus(
                corpus,
                signalIds,
            ).also { runner.exportEmbeddingCache(repository.paths.embeddingsDirectory) }
        } finally {
            runner.deleteEnvironment()
        }
    }

    private fun mergeCandidates(
        neighbours: EdictNextSignalNeighbours,
        clusters: List<EdictNextStoredCluster>,
        inbox: List<EdictNextSignal>,
        repository: EdictRepository,
    ): List<EdictNextSignalCandidate> {
        val clusterBySignalId = buildMap {
            clusters.forEach { cluster -> cluster.signalIds.forEach { put(it, cluster.id) } }
        }
        val clusterCandidates = neighbours.closest.mapNotNull { neighbour ->
            clusterBySignalId[neighbour.signalId]?.let { it to neighbour }
        }.groupBy({ it.first }, { it.second }).map { (clusterId, matched) ->
            EdictNextSignalCandidate.Cluster(
                clusterId,
                matched.minOf(EdictNextScriptNeighbour::distance),
            )
        }
        val inboxIds = inbox.mapTo(hashSetOf(), EdictNextSignal::id)
        val signalCandidates = neighbours.closest.filter { it.signalId in inboxIds }.map { neighbour ->
            EdictNextSignalCandidate.Signal(
                neighbour.signalId,
                repository.paths.inboxDirectory.resolve("${neighbour.signalId}.json").toString(),
                neighbour.distance,
            )
        }
        return (clusterCandidates + signalCandidates).sortedBy(EdictNextSignalCandidate::nearestDistance)
    }

    private suspend fun requireValidState(repository: EdictRepository, state: EdictNextRepositoryState) {
        requireNoIssues(validateRepositoryState(repository, state))
    }

    private fun requireNoIssues(issues: List<EdictNextValidationIssue>) {
        check(issues.isEmpty()) { issues.joinToString("\n") { "${it.path}: ${it.message}" } }
    }

    private fun requireBatch(): DistributionBatch = checkNotNull(batch) { "Call edict_next_prepare_pipeline first" }

    private fun invalidState(repository: EdictRepository, error: Exception): EdictNextValidationResponse =
      EdictNextValidationResponse(
        false,
        "Repository structure is invalid",
        listOf(EdictNextValidationIssue(repository.paths.root.toString(), error.message.orEmpty())),
      )

    internal class DistributionBatch(
        val sourceRepository: EdictRepository,
        val sourceState: EdictNextRepositoryState,
        val worktreeState: EdictNextRepositoryState,
        val signalIds: Set<String>,
        val neighbours: Map<String, EdictNextSignalNeighbours>,
    ) {
        private var currentSignalId: String? = null
        private val loadedClusterIds = HashSet<String>()

        fun startSignal(signalId: String) {
            if (currentSignalId == signalId) return
            currentSignalId = signalId
            loadedClusterIds.clear()
        }

        fun recordClusterContext(clusterId: String) {
            requireCurrentSignal()
            loadedClusterIds.add(clusterId)
        }

        fun requireClusterContext(clusterId: String) {
            check(clusterId in loadedClusterIds) {
                "Call edict_next_get_distribution_context with kind='$CLUSTER_CONTEXT_KIND' and id='$clusterId' before assigning the current Signal"
            }
        }

        fun requireCurrentSignal(signalId: String? = null) {
            val current = checkNotNull(currentSignalId) { "Call edict_next_next_signal first" }
            if (signalId != null) {
                require(signalId == current) { "Signal '$signalId' is not the current prepared Signal '$current'" }
            }
        }

        fun completeSignal(signalId: String) {
            requireCurrentSignal(signalId)
            clearCurrentSignal()
        }

        fun clearCurrentSignal() {
            currentSignalId = null
            loadedClusterIds.clear()
        }
    }

    companion object {
        private val servicesBySessionId = ConcurrentHashMap<String, EdictNextDistributionService>()

        fun getInstance(sessionId: String): EdictNextDistributionService =
          servicesBySessionId.computeIfAbsent(sessionId, ::EdictNextDistributionService)

        const val SIGNAL_CONTEXT_KIND: String = "signal"
        const val CLUSTER_CONTEXT_KIND: String = "cluster"
    }
}

private fun EdictNextSignal.toDistributionSignal(): EdictNextDistributionSignal = EdictNextDistributionSignal(
  id = id,
  fileRevision = fileRevision,
  source = source,
  label = label,
  description = description,
)

internal fun clustersSharingInspectionIdentity(
    signal: EdictNextSignal,
    clusters: List<EdictNextStoredCluster>,
): List<String> {
    val inspectionName = (signal.source as? EdictNextSignalSource.SubmittedFeedback)
      ?.inspectionName?.takeIf(String::isNotBlank) ?: return emptyList()
    return clusters.asSequence()
      .filter { it.manifest.language == signal.language }
      .filter { cluster ->
          cluster.signals.any { existing ->
              (existing.source as? EdictNextSignalSource.SubmittedFeedback)
                ?.inspectionName?.takeIf(String::isNotBlank) == inspectionName
          }
      }
      .map(EdictNextStoredCluster::id)
      .sorted()
      .toList()
}
