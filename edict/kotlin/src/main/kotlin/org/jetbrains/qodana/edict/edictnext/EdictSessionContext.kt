package org.jetbrains.qodana.edict.edictnext

import org.jetbrains.qodana.edict.git.GitRepository
import java.nio.file.Path
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.locks.ReentrantLock
import kotlin.concurrent.withLock

/** Resources shared by one Edict session run. */
internal class EdictSessionContext private constructor(private val sessionId: String) {
  private val lock = ReentrantLock()
  private var activeRun: ActiveRun? = null



  suspend fun load(
    workspace: EdictNextWorkspace,
    sourceRepository: Path,
    analyzedProject: Path,
    qodanaExecutable: String,
    embeddingPython: Path? = null,
    inspectionServer: IntellijMcpServerService = IntellijMcpServerService(analyzedProject, qodanaExecutable),
  ) {
    // Record paths only: the MCP host waits for initialize, and management or extraction runs need neither Git nor the IDE.
    lock.withLock {
      check(activeRun == null) { "An Edict run is already active" }
      activeRun = ActiveRun(
        workspace = workspace,
        sourceRepository = sourceRepository,
        analyzedProject = analyzedProject,
        embeddingPython = embeddingPython,
        inspectionServer = inspectionServer,
      )
    }
  }

  suspend fun unload() {
    val server = lock.withLock { active().inspectionServer }
    server.stop()
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

  val workspace: EdictNextWorkspace get() = lock.withLock { active().workspace }
  val sourceRepository: Path get() = lock.withLock { active().sourceRepository }
  val analyzedProject: Path get() = lock.withLock { active().analyzedProject }
  /** The analyzed project's HEAD, resolved on first use and fixed for the rest of the run. */
  val projectRevision: String get() = lock.withLock {
    active().let { run -> run.projectRevision ?: GitRepository(run.analyzedProject).resolve("HEAD").also { run.projectRevision = it } }
  }
  val embeddingPython: Path? get() = lock.withLock { active().embeddingPython }
  val inspectionServer: IntellijMcpServerService get() = lock.withLock { active().inspectionServer }

  private fun active(): ActiveRun = checkNotNull(activeRun) { "No Edict Next run is active" }

  private data class ActiveRun(
    val workspace: EdictNextWorkspace,
    val sourceRepository: Path,
    val analyzedProject: Path,
    val embeddingPython: Path?,
    val inspectionServer: IntellijMcpServerService,
    var projectRevision: String? = null,
    var repository: EdictRepository? = null,
  )

  companion object {
    private val contextsBySessionId = ConcurrentHashMap<String, EdictSessionContext>()

    fun getInstance(sessionId: String): EdictSessionContext =
      contextsBySessionId.computeIfAbsent(sessionId, ::EdictSessionContext)
  }
}
