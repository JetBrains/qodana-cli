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
    managedRepository: Path,
    sourceRepository: Path,
    analyzedProject: Path,
    qodanaExecutable: String,
    embeddingPython: Path? = null,
    inspectionServer: IntellijMcpServerService = IntellijMcpServerService(analyzedProject, qodanaExecutable),
  ) {
    lock.withLock { check(activeRun == null) { "An Edict run is already active" } }
    val projectRevision = GitRepository(analyzedProject).resolve("HEAD")
    val server = inspectionServer
    server.start()
    lock.withLock {
      activeRun = ActiveRun(
        workspace = workspace,
        managedRepository = managedRepository,
        sourceRepository = sourceRepository,
        analyzedProject = analyzedProject,
        projectRevision = projectRevision,
        embeddingPython = embeddingPython,
        inspectionServer = server,
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
  val managedRepository: Path get() = lock.withLock { active().managedRepository }
  val sourceRepository: Path get() = lock.withLock { active().sourceRepository }
  val analyzedProject: Path get() = lock.withLock { active().analyzedProject }
  val projectRevision: String get() = lock.withLock { active().projectRevision }
  val embeddingPython: Path? get() = lock.withLock { active().embeddingPython }
  val inspectionServer: IntellijMcpServerService get() = lock.withLock { active().inspectionServer }

  private fun active(): ActiveRun = checkNotNull(activeRun) { "No Edict Next run is active" }

  private data class ActiveRun(
    val workspace: EdictNextWorkspace,
    val managedRepository: Path,
    val sourceRepository: Path,
    val analyzedProject: Path,
    val projectRevision: String,
    val embeddingPython: Path?,
    val inspectionServer: IntellijMcpServerService,
    var repository: EdictRepository? = null,
  )

  companion object {
    private val contextsBySessionId = ConcurrentHashMap<String, EdictSessionContext>()

    fun getInstance(sessionId: String): EdictSessionContext =
      contextsBySessionId.computeIfAbsent(sessionId, ::EdictSessionContext)
  }
}
