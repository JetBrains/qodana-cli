package org.jetbrains.qodana.edict.edictnext

import java.nio.file.Path
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.locks.ReentrantLock
import kotlin.concurrent.withLock

/** Resources shared by one Edict session run. */
internal class EdictSessionContext private constructor(private val sessionId: String) {
  private val lock = ReentrantLock()
  private var activeRun: ActiveRun? = null



  fun load(workspace: EdictNextWorkspace, sourceRepository: Path) {
    lock.withLock {
      check(activeRun == null) { "An Edict run is already active" }
      activeRun = ActiveRun(
        config = config,
        workspace = workspace,
        sourceRepository = sourceRepository,
        projectRevision = GitOperations.getInstance(project).saveCurrentState().commit,
      )
    }
  }

  fun unload() {
    lock.withLock {
      checkNotNull(activeRun) { "No Edict Next run is active" }
      EdictNextDistributionService.getInstance(sessionId).clear()
      activeRun = null
    }
  }

  fun useRepository(repository: EdictRepository) {
    lock.withLock { active().repository = repository }
  }

  fun repository(): EdictRepository = lock.withLock {
    checkNotNull(active().repository) { "Call edict_next_prepare_pipeline first" }
  }

  val config: EdictConfig get() = lock.withLock { active().config }
  val workspace: EdictNextWorkspace get() = lock.withLock { active().workspace }
  val sourceRepository: Path get() = lock.withLock { active().sourceRepository }
  val projectRevision: String get() = lock.withLock { active().projectRevision }

  private fun active(): ActiveRun = checkNotNull(activeRun) { "No Edict Next run is active" }

  private data class ActiveRun(
    val config: EdictConfig,
    val workspace: EdictNextWorkspace,
    val sourceRepository: Path,
    val projectRevision: String,
    var repository: EdictRepository? = null,
  )

  companion object {
    private val contextsBySessionId = ConcurrentHashMap<String, EdictSessionContext>()

    fun getInstance(sessionId: String): EdictSessionContext =
      contextsBySessionId.computeIfAbsent(sessionId, ::EdictSessionContext)
  }
}

class EdictConfig() {

}
