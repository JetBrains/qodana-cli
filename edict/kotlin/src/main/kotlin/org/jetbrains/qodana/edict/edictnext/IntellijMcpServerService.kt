package org.jetbrains.qodana.edict.edictnext

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import kotlinx.serialization.Serializable
import java.net.URI
import java.nio.file.Path
import kotlin.io.path.absolutePathString

internal data class CommandResult(val exitCode: Int, val output: String)

internal fun interface CommandRunner {
  suspend fun run(command: List<String>): CommandResult
}

internal object ProcessCommandRunner : CommandRunner {
  override suspend fun run(command: List<String>): CommandResult = withContext(Dispatchers.IO) {
    val process = ProcessBuilder(command).redirectErrorStream(true).start()
    val output = process.inputStream.bufferedReader().use { it.readText() }
    CommandResult(process.waitFor(), output)
  }
}

internal fun interface InspectionKtsClientFactory {
  fun create(endpoint: URI): InspectionKtsClient
}

/** Owns the IDE MCP lifecycle and serializes whole-project analyses on its opened project. */
internal class IntellijMcpServerService(
  private val projectPath: Path,
  private val qodanaExecutable: String = System.getProperty("qodana.executable")
    ?: System.getenv("QODANA_EXECUTABLE")
    ?: "qodana",
  private val commandRunner: CommandRunner = ProcessCommandRunner,
  private val clientFactory: InspectionKtsClientFactory = InspectionKtsClientFactory(::HttpInspectionKtsClient),
) {
  private val lifecycle = Mutex()
  private val analyses = Mutex()
  private var client: InspectionKtsClient? = null

  suspend fun start(): InspectionKtsClient = lifecycle.withLock {
    client?.let { return@withLock it }
    val result = commandRunner.run(
      listOf(
        qodanaExecutable,
        "edict",
        "linter-mcp",
        "start",
        "--project-dir",
        projectPath.toAbsolutePath().normalize().absolutePathString(),
      ),
    )
    check(result.exitCode == 0) { "Failed to start IntelliJ MCP: ${result.output.trim()}" }
    val ready = EdictNextJson.decodeFromString<McpReady>(result.output.trim())
    check(ready.status == "ready" && ready.url.isNotBlank()) { "IntelliJ MCP did not become ready: ${result.output.trim()}" }
    clientFactory.create(URI.create(ready.url)).also { client = it }
  }

  suspend fun <T> waitForAnalysis(action: suspend (InspectionKtsClient) -> T): T = analyses.withLock {
    action(start())
  }

  suspend fun stop() {
    analyses.withLock {
      lifecycle.withLock {
        client?.close()
        client = null
        val result = commandRunner.run(
          listOf(
            qodanaExecutable,
            "edict",
            "linter-mcp",
            "stop",
            "--project-dir",
            projectPath.toAbsolutePath().normalize().absolutePathString(),
          ),
        )
        check(result.exitCode == 0) { "Failed to stop IntelliJ MCP: ${result.output.trim()}" }
      }
    }
  }

  @Serializable
  private data class McpReady(val status: String, val url: String)
}
