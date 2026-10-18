package org.jetbrains.qodana.edict.history.enrichment

import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import org.jetbrains.qodana.edict.common.GitRepository
import org.jetbrains.qodana.edict.edictnext.EdictNextClusterStatus
import org.jetbrains.qodana.edict.edictnext.EdictNextJson
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState
import org.jetbrains.qodana.edict.edictnext.EdictNextSignal
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalSource
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalStrength
import org.jetbrains.qodana.edict.edictnext.EdictRepository
import org.jetbrains.qodana.edict.signals.SignalValidation
import java.nio.file.Path
import java.time.Clock
import java.time.Duration
import java.time.Instant

internal const val HISTORY_ANALYSIS_COMMIT_DEPTH = 10000
internal const val HISTORY_ANALYSIS_DAY_DEPTH = 180L

/** Bounded historical evidence enrichment, independent of inspection generation. */
internal class EdictNextEnrichmentService(
  private val repository: EdictRepository,
  private val projectRoot: Path,
  private val clock: Clock = Clock.systemUTC(),
) {
  private val publicationMutex = Mutex()

  /** Applies both depth limits before pickaxe filtering; no evidence or analysis coverage is returned or persisted. */
  suspend fun getHistoryContext(clusterId: String): EdictNextHistoryContext {
    require(repository.loadCluster(clusterId).manifest.status == EdictNextClusterStatus.Pending) { "Cluster '$clusterId' is not Pending" }
    val git = GitRepository(projectRoot)
    val head = git.resolve("HEAD")
    val since = clock.instant().minus(Duration.ofDays(HISTORY_ANALYSIS_DAY_DEPTH))
    // Cap the unfiltered traversal first. Applying --max-count to a later pickaxe query could reach older commits.
    // Filter timestamps after traversal so a clock-skewed old commit does not hide a recent ancestor.
    val revisions = git.git("rev-list", "--timestamp", "--topo-order", "--max-count=$HISTORY_ANALYSIS_COMMIT_DEPTH", head, "--")
      .lineSequence().filter(String::isNotBlank)
      .map { it.split(' ', limit = 2) }
      .filter { Instant.ofEpochSecond(it[0].toLong()) >= since }
      .map { it[1] }.toList()
    return EdictNextHistoryContext(clusterId, EdictNextHistoryRange(head, since.toString(), revisions))
  }

  /** Publishes trusted enrichment into the supplied Pending cluster. */
  suspend fun publishClusterSignal(clusterId: String, signal: EdictNextSignal): EdictNextRepositoryState.SignalPublication =
    publicationMutex.withLock {
      SignalValidation.validate(signal)
      require(signal.strength == EdictNextSignalStrength.STRONG) { "Historical enrichment requires STRONG Signals" }
      require(signal.source is EdictNextSignalSource.FromCommit) { "Historical enrichment requires FromCommit Signals" }
      val content = EdictNextJson.encodeToString(signal).toByteArray()
      require(content.size <= 8 * 1024 * 1024) { "Signal exceeds 8 MiB" }
      GitRepository(projectRoot).validateEvidence(signal)

      val current = repository.loadState()
      val cluster = checkNotNull(current.clustersById[clusterId]) { "Cluster '$clusterId' is missing" }
      require(cluster.manifest.status == EdictNextClusterStatus.Pending) { "Cluster '$clusterId' is not Pending" }
      require(signal.language == cluster.manifest.language) { "Signal language does not match cluster '$clusterId'" }
      require(signal.id !in current.inboxSignalIds && current.clusters.none { it.id != clusterId && signal.id in it.signalIds }) {
        "Signal '${signal.id}' already belongs to the inbox or another cluster"
      }
      cluster.signals.singleOrNull { it.id == signal.id }?.let { existing ->
        require(existing.copy(syntheticExampleId = null) == signal) { "Signal '${signal.id}' already exists with different evidence" }
        return@withLock EdictNextRepositoryState.SignalPublication(existing, created = false)
      }

      repository.saveClusterSignal(clusterId, signal)
      EdictNextRepositoryState.SignalPublication(signal, created = true)
    }
}
