package org.jetbrains.qodana.edict.edictnext

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.nio.file.Path
import kotlin.io.path.createDirectories
import kotlin.io.path.writeText

/**
 * Prepares and applies one sequential distribution batch on the run's state repository, which the batch changes in
 * place; [validateDistribution] checks those changes against the snapshot [preparePipeline] took.
 */
internal class EdictNextDistributionService(
    private val repository: EdictRepository,
    /** Where the neighbours of the prepared batch are dumped for debugging; nothing reads them back. */
    private val neighboursResponsePath: Path,
) {
    private var batch: DistributionBatch? = null
    internal var maxInboxSignalsPerRun: Int = EDICT_NEXT_MAX_INBOX_SIGNALS_PER_RUN

    suspend fun preparePipeline(): EdictNextPreparePipelineResponse {
        val initialState = repository.loadState()
        requireNoIssues(validateRepositoryState(repository, initialState))

        val signalIds = initialState.jvmInboxSignalIds.sorted().take(maxInboxSignalsPerRun)
        val neighbours = prepareNeighbours(initialState, signalIds)
        batch = DistributionBatch(initialState, signalIds.toSet(), neighbours)

        return EdictNextPreparePipelineResponse(
          summary = "${initialState.clusters.size} cluster(s), ${signalIds.size} Signal(s) selected for distribution",
        )
    }

    suspend fun nextSignal(): EdictNextSignalPreparationResponse {
        val currentBatch = requireBatch()
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
        return when (kind) {
            SIGNAL_CONTEXT_KIND -> EdictNextDistributionContextResponse.SignalContext(
                signal = requireNotNull(currentBatch.initialState.signalsById[id]) { "Signal '$id' does not exist" },
            )

            CLUSTER_CONTEXT_KIND -> {
                val cluster = repository.loadCluster(id)
                val response = EdictNextDistributionContextResponse.Cluster(
                    clusterId = cluster.id,
                    signals = cluster.signals,
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
        if (repository.loadClusters().any { it.id == clusterId }) {
            currentBatch.requireClusterContext(clusterId)
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
        val current = try {
            repository.loadState()
        } catch (e: Exception) {
            return invalidState(repository, e)
        }
        val issues = validateRepositoryState(repository, current) +
            validateDistributionChange(currentBatch.initialState, current, currentBatch.signalIds)
        return EdictNextValidationResponse(
          success = issues.isEmpty(),
          summary = if (issues.isEmpty()) "Distribution changes are valid" else "Found ${issues.size} distribution issue(s)",
          issues = issues,
          nextAction = if (issues.isEmpty()) EdictNextNextAction.STOP_DISTRIBUTION else EdictNextNextAction.REPAIR_REPOSITORY,
        )
    }

    private suspend fun prepareNeighbours(
        state: EdictNextRepositoryState,
        signalIds: List<String>,
    ): Map<String, EdictNextSignalNeighbours> {
        val neighbours = EdictNextNeighbourFinder(edictNextModelDirectory()).find(state, signalIds)
        withContext(Dispatchers.IO) {
            neighboursResponsePath.parent.createDirectories()
            neighboursResponsePath.writeText(
                EdictNextJson.encodeToString(
                    EdictNextNeighboursResponse.serializer(),
                    EdictNextNeighboursResponse(neighbours.values.toList()),
                ),
            )
        }
        return neighbours
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
                matched.minOf(EdictNextNeighbour::distance),
            )
        }
        val inboxIds = inbox.mapTo(hashSetOf(), EdictNextSignal::id)
        val signalCandidates = neighbours.closest.filter { it.signalId in inboxIds }.map { neighbour ->
            EdictNextSignalCandidate.SignalCandidate(
                neighbour.signalId,
                repository.paths.inboxDirectory.resolve("${neighbour.signalId}.json").toString(),
                neighbour.distance,
            )
        }
        return (clusterCandidates + signalCandidates).sortedBy(EdictNextSignalCandidate::nearestDistance)
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
        /** The state repository when the batch was prepared; distribution changes are validated against it. */
        val initialState: EdictNextRepositoryState,
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
        const val SIGNAL_CONTEXT_KIND: String = "signal"
        const val CLUSTER_CONTEXT_KIND: String = "cluster"
    }
}
