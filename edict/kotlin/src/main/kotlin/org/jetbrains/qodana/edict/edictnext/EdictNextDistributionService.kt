package org.jetbrains.qodana.edict.edictnext

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import org.jetbrains.qodana.edict.common.DEFAULT_DISTRIBUTION_SIGNAL_COUNT
import java.nio.file.Path
import kotlin.io.path.createDirectories
import kotlin.io.path.writeText

/**
 * Applies one sequential distribution batch on the run's state repository, which the batch changes in place. The batch is
 * selected on the first [nextSignal] from the inbox Signals whose neighbours [calculateNeighbours] found. Each task
 * assigns at most [taskSignalCount] Signals, so no agent session grows long; [nextSignal] then hands off to a new task.
 */
internal class EdictNextDistributionService(
    private val repository: EdictRepository,
    /** Where the calculated neighbours are dumped for debugging; nothing reads them back. */
    private val neighboursResponsePath: Path,
    private val defaultSignalCount: Int = DEFAULT_DISTRIBUTION_SIGNAL_COUNT,
    private val taskSignalCount: Int = DISTRIBUTION_TASK_SIGNAL_COUNT,
) {
    private var neighbours: Map<String, EdictNextSignalNeighbours>? = null
    private var batch: DistributionBatch? = null

    suspend fun calculateNeighbours(): EdictNextCalculateNeighboursResponse {
        // A later task of the same batch keeps the neighbours its Signals were selected with.
        batch?.let {
            return EdictNextCalculateNeighboursResponse("Neighbours of this run's ${it.signalIds.size} selected Signal(s) were already calculated")
        }
        var state = repository.loadState()
        val replacementOwners = state.inboxSignals.associateWith { signal ->
            state.clusters.filter { signal.id in it.signalIds }
        }
        if (replacementOwners.values.any { it.size > 1 }) {
            requireNoIssues(validateRepositoryState(repository, state))
        }
        val replacedSignals = replacementOwners.mapNotNull { (signal, owners) ->
            val owner = owners.singleOrNull() ?: return@mapNotNull null
            repository.replaceClusterSignalFromInbox(signal.id, owner.id)
            signal.id
        }
        if (replacedSignals.isNotEmpty()) state = repository.loadState()
        requireNoIssues(validateRepositoryState(repository, state))
        val signalIds = state.jvmInboxSignalIds.sorted()
        neighbours = prepareNeighbours(state, signalIds)
        return EdictNextCalculateNeighboursResponse(
          summary = buildString {
              append("Calculated neighbours of ${signalIds.size} inbox Signal(s) among ${state.clusters.size} cluster(s)")
              if (replacedSignals.isNotEmpty()) {
                  append(", ${replacedSignals.size} re-published Signal(s) replaced in existing clusters")
              }
          },
        )
    }

    suspend fun nextSignal(): EdictNextSignalPreparationResponse {
        val currentBatch = batch ?: selectBatch().also { batch = it }
        val state = repository.loadState()
        val inbox = state.inboxSignals
        val signalId = inbox.asSequence().map(EdictNextSignal::id).filter(currentBatch.signalIds::contains).minOrNull()
        if (signalId == null) {
            currentBatch.clearCurrentSignal()
            return EdictNextSignalPreparationResponse(summary = "All selected Signals were distributed")
        }
        if (currentBatch.taskAssignedCount >= taskSignalCount) {
            val remaining = inbox.count { it.id in currentBatch.signalIds }
            currentBatch.handOff()
            return EdictNextSignalPreparationResponse(
              summary = "This task assigned $taskSignalCount Signal(s); $remaining selected Signal(s) remain for a new distribution task",
              nextAction = EdictNextNextAction.HAND_OFF,
            )
        }
        currentBatch.startSignal(signalId)
        val candidates = mergeCandidates(currentBatch.neighbours.getValue(signalId), state.clusters, inbox, repository)
        candidates.filterIsInstance<EdictNextSignalCandidate.Cluster>()
            .filter { it.signals != null }
            .forEach { currentBatch.recordClusterContext(it.clusterId) }
        return EdictNextSignalPreparationResponse(
          signalId = signalId,
          signalPath = repository.paths.inboxDirectory.resolve("$signalId.json").toString(),
          signal = state.inboxSignalsById.getValue(signalId),
          candidates = candidates,
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
                    signals = cluster.signals.map(EdictNextSignal::asEvidence),
                )
                currentBatch.recordClusterContext(id)
                response
            }

            else -> error("Unsupported distribution context kind '$kind'; expected '$SIGNAL_CONTEXT_KIND' or '$CLUSTER_CONTEXT_KIND'")
        }
    }

    /** Assigns the current Signal and prepares the next one as [nextSignal] would, so the loop needs one call per Signal. */
    suspend fun addSignalToCluster(
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
        // The assignment stands even if preparing the next Signal fails; edict_next_next_signal then reports the problem.
        val next = runCatching { nextSignal() }.getOrNull()
        return EdictNextSignalValidationResponse(
          signalId = signalId,
          summary = "Assigned Signal '$signalId' to '$clusterId'" + if (next == null) "; call edict_next_next_signal" else "",
          added = true,
          next = next,
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
            clusters.forEach { cluster -> cluster.signalIds.forEach { put(it, cluster) } }
        }
        val clusterCandidates = neighbours.closest.mapNotNull { neighbour ->
            clusterBySignalId[neighbour.signalId]?.let { it to neighbour }
        }.groupBy({ it.first }, { it.second }).map { (cluster, matched) ->
            cluster to matched.minOf(EdictNextNeighbour::distance)
        }.sortedBy { it.second }.mapIndexed { index, (cluster, distance) ->
            // The nearest small clusters come with their members, so most assignments need no context call.
            val members = cluster.signals.map(EdictNextSignal::asEvidence)
            val included = index < INCLUDED_CLUSTER_COUNT &&
                EdictNextJson.encodeToString(members).length <= INCLUDED_CLUSTER_MAX_CHARS
            EdictNextSignalCandidate.Cluster(cluster.id, distance, cluster.signals.size, members.takeIf { included })
        }
        val inboxById = inbox.associateBy(EdictNextSignal::id)
        val signalCandidates = neighbours.closest.mapNotNull { neighbour ->
            val signal = inboxById[neighbour.signalId] ?: return@mapNotNull null
            EdictNextSignalCandidate.SignalCandidate(
                neighbour.signalId,
                repository.paths.inboxDirectory.resolve("${neighbour.signalId}.json").toString(),
                neighbour.distance,
                signal.label,
                signal.fileRevision.path,
                signal.description,
            )
        }
        return (clusterCandidates + signalCandidates).sortedBy(EdictNextSignalCandidate::nearestDistance)
    }

    private fun requireNoIssues(issues: List<EdictNextValidationIssue>) {
        check(issues.isEmpty()) { issues.joinToString("\n") { "${it.path}: ${it.message}" } }
    }

    private fun requireBatch(): DistributionBatch = checkNotNull(batch) { "Call edict_next_next_signal first" }

    /** Selects the run's batch once; Signals that arrived after [calculateNeighbours] wait for a later run. */
    private suspend fun selectBatch(): DistributionBatch {
        val neighbours = checkNotNull(neighbours) { "Call edict_next_calculate_neighbours_by_embeddings first" }
        val state = repository.loadState()
        val signalIds = state.jvmInboxSignalIds.filter(neighbours::containsKey).sorted().take(defaultSignalCount)
        return DistributionBatch(state, signalIds.toSet(), neighbours)
    }

    internal class DistributionBatch(
        /** The state repository when the batch was selected; Signal context is read from it. */
        val initialState: EdictNextRepositoryState,
        val signalIds: Set<String>,
        val neighbours: Map<String, EdictNextSignalNeighbours>,
    ) {
        private var currentSignalId: String? = null
        var taskAssignedCount = 0
            private set
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
            taskAssignedCount++
        }

        fun handOff() {
            clearCurrentSignal()
            taskAssignedCount = 0
        }

        fun clearCurrentSignal() {
            currentSignalId = null
            loadedClusterIds.clear()
        }
    }

    companion object {
        const val SIGNAL_CONTEXT_KIND: String = "signal"
        const val CLUSTER_CONTEXT_KIND: String = "cluster"
        /** Keeps a task's context small: every assignment adds the Signal and cluster evidence it read. */
        const val DISTRIBUTION_TASK_SIGNAL_COUNT: Int = 10
        const val INCLUDED_CLUSTER_COUNT: Int = 2
        const val INCLUDED_CLUSTER_MAX_CHARS: Int = 12_000
    }
}
