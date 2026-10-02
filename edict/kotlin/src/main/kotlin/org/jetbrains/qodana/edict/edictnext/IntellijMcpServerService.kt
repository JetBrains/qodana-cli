package org.jetbrains.qodana.edict.edictnext

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonObject
import java.net.URI
import java.nio.file.Files
import java.nio.file.Path
import java.util.concurrent.CompletableFuture
import java.util.concurrent.TimeUnit
import kotlin.io.path.absolutePathString

internal fun interface InspectionKtsClientFactory {
  fun create(endpoint: URI): InspectionKtsClient
}

/** Starts and stops the IntelliJ MCP endpoint used by one Edict run. */
internal interface IntellijMcpServerLifecycle {
  suspend fun start(): URI

  /** False once a started server has died, so its clients must not be reused. */
  val running: Boolean get() = true

  suspend fun stop()
}

/**
 * Production lifecycle: runs `qodana edict ide-mcp` as a child process that owns the IDE. Closing the child's stdin,
 * including when this JVM dies, makes it stop the IDE, so no IDE outlives the Edict server. A helper killed before it
 * can stop the IDE leaves it orphaned, so the IDE process tree is stopped here whenever the helper exits.
 */
internal class QodanaIntellijMcpServerLifecycle(
  private val projectPath: Path,
  private val qodanaExecutable: String,
  private val ideArguments: List<String> = emptyList(),
  private val log: Path? = null,
) : IntellijMcpServerLifecycle {
  @Volatile private var helper: Process? = null

  override val running: Boolean get() = helper?.isAlive == true

  override suspend fun start(): URI = withContext(Dispatchers.IO) {
    check(helper == null) { "IntelliJ MCP is already running" }
    val command = listOf(
      qodanaExecutable, "edict", "ide-mcp",
      "--project-dir", projectPath.toAbsolutePath().normalize().absolutePathString(),
    ) + ideArguments
    log?.let { Files.createDirectories(it.toAbsolutePath().parent) }
    val process = ProcessBuilder(command)
      .redirectError(log?.let { ProcessBuilder.Redirect.appendTo(it.toFile()) } ?: ProcessBuilder.Redirect.DISCARD)
      .start()
    helper = process
    try {
      val ready = awaitReady(process)
      // Captured now: the handle tracks this exact process, so a reused pid is never signalled.
      val ide = ready.pid?.let { ProcessHandle.of(it).orElse(null) }
      if (ide != null) process.onExit().thenRun { stopIde(ide) }
      URI.create(ready.url)
    }
    catch (e: Throwable) {
      stop(process)
      helper = null
      throw e
    }
  }

  override suspend fun stop() = withContext(Dispatchers.IO) {
    helper?.let(::stop)
    helper = null
  }

  /** The helper enforces its own readiness timeout, so this returns at readiness, failure, or exit. */
  private fun awaitReady(process: Process): McpReady {
    // IDE provisioning may print progress to stdout before the readiness line.
    val skipped = mutableListOf<String>()
    process.inputReader().lineSequence().forEach { line ->
      val ready = runCatching { EdictNextJson.decodeFromString<McpReady>(line.trim()) }.getOrNull()
      if (ready?.status == "ready" && ready.url.isNotBlank()) return ready
      skipped += line
    }
    val output = (skipped + logTail()).joinToString("\n").ifBlank { "no output" }
    error("IntelliJ MCP exited before becoming ready (exit code ${process.waitFor()}):\n$output")
  }

  private fun logTail(): List<String> =
    log?.takeIf(Files::exists)?.let { runCatching { Files.readAllLines(it).takeLast(LOG_TAIL_LINES) }.getOrNull() }.orEmpty()

  private fun stop(process: Process) {
    runCatching { process.outputStream.close() }
    if (process.waitFor(HELPER_STOP_SECONDS, TimeUnit.SECONDS)) return
    process.destroy()
    if (!process.waitFor(HELPER_KILL_SECONDS, TimeUnit.SECONDS)) process.destroyForcibly()
  }

  /** A no-op after a normal stop, since the helper exits only once the IDE has. */
  private fun stopIde(ide: ProcessHandle) {
    if (!ide.isAlive) return
    // The IDE script may launch the JVM as its child; stop the whole tree.
    val tree = ide.descendants().toList() + ide
    tree.forEach(ProcessHandle::destroy)
    runCatching {
      CompletableFuture.allOf(*tree.map(ProcessHandle::onExit).toTypedArray()).get(IDE_STOP_SECONDS, TimeUnit.SECONDS)
    }
    tree.filter(ProcessHandle::isAlive).forEach(ProcessHandle::destroyForcibly)
  }

  @Serializable
  private data class McpReady(val status: String, val url: String, val pid: Long? = null)

  private companion object {
    // The helper itself allows the IDE ten seconds to exit before killing it.
    const val HELPER_STOP_SECONDS = 15L
    const val HELPER_KILL_SECONDS = 5L
    const val IDE_STOP_SECONDS = 10L
    const val LOG_TAIL_LINES = 20
  }
}

/** Owns the IDE MCP lifecycle and serializes whole-project analyses on its opened project. */
internal class IntellijMcpServerService(
  private val projectPath: Path,
  private val qodanaExecutable: String = System.getProperty("qodana.executable")
    ?: System.getenv("QODANA_EXECUTABLE")
    ?: "qodana",
  ideArguments: List<String> = emptyList(),
  log: Path? = null,
  // The helper opens the project by its canonical path, so name it the same way in tool calls.
  private val clientFactory: InspectionKtsClientFactory = InspectionKtsClientFactory { endpoint ->
    HttpInspectionKtsClient(endpoint, runCatching { projectPath.toRealPath() }.getOrElse { projectPath.toAbsolutePath().normalize() }.toString())
  },
  private val serverLifecycle: IntellijMcpServerLifecycle = QodanaIntellijMcpServerLifecycle(
    projectPath,
    qodanaExecutable,
    ideArguments,
    log,
  ),
) {
  private val lifecycle = Mutex()
  private val analyses = Mutex()
  private var client: InspectionKtsClient? = null
  private var started = false

  /** Starts the IDE on first use, and again if it died; runs that never inspect never launch it. */
  suspend fun start(): InspectionKtsClient = lifecycle.withLock {
    client?.let { current ->
      if (serverLifecycle.running) return@withLock current
      runCatching { current.close() }
      client = null
      serverLifecycle.stop()
    }
    val endpoint = serverLifecycle.start()
    started = true
    clientFactory.create(endpoint).also { client = it }
  }

  suspend fun <T> withClient(action: suspend (InspectionKtsClient) -> T): T {
    val initial = start()
    return try {
      action(initial)
    }
    catch (_: StaleInspectionMcpSession) {
      action(restart(initial))
    }
  }

  suspend fun <T> waitForAnalysis(action: suspend (InspectionKtsClient) -> T): T = analyses.withLock {
    withClient(action)
  }

  private suspend fun restart(staleClient: InspectionKtsClient): InspectionKtsClient = lifecycle.withLock {
    client?.takeIf { it !== staleClient }?.let { return@withLock it }
    runCatching { staleClient.close() }
    client = null
    serverLifecycle.stop()
    clientFactory.create(serverLifecycle.start()).also { client = it }
  }

  suspend fun stop() {
    analyses.withLock {
      lifecycle.withLock {
        client?.close()
        client = null
        if (started) {
          started = false
          serverLifecycle.stop()
        }
      }
    }
  }
}
