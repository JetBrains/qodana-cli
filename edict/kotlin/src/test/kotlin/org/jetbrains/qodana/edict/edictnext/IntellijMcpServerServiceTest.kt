package org.jetbrains.qodana.edict.edictnext

import kotlinx.coroutines.async
import kotlinx.coroutines.delay
import kotlinx.coroutines.runBlocking
import org.jetbrains.qodana.edict.history.enrichment.EdictNextEnrichmentService
import org.jetbrains.qodana.edict.support.FakeInspectionKtsClient
import org.jetbrains.qodana.edict.support.FakeIntellijMcpServerLifecycle
import org.junit.jupiter.api.io.TempDir
import java.net.URI
import java.nio.file.Files
import java.nio.file.Path
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger
import kotlin.test.Test
import kotlin.test.assertContains
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertSame
import kotlin.test.assertTrue

class IntellijMcpServerServiceTest {
  @TempDir
  lateinit var project: Path

  @Test
  fun `starts the helper once and stops it by closing its stdin`() = runBlocking {
    val qodana = fakeQodana(
      """
      echo "Downloading IDE"
      echo '{"status":"ready","url":"http://127.0.0.1:9876/mcp"}'
      echo "IDE log line" >&2
      cat > /dev/null
      echo stopped > "${'$'}(dirname "${'$'}0")/stopped"
      """,
    )
    val log = project.resolve("logs/intellij-mcp.log")
    val client = FakeInspectionKtsClient()
    var endpoint: URI? = null
    val service = IntellijMcpServerService(
      projectPath = project,
      qodanaExecutable = qodana.toString(),
      ideArguments = listOf("--dist=/opt/idea", "--property=-Xmx8g"),
      log = log,
      resultsDirectory = project.resolve("logs/intellij-mcp"),
      clientFactory = InspectionKtsClientFactory { uri -> endpoint = uri; client },
    )

    assertSame(client, service.start())
    assertSame(client, service.start())
    assertEquals(URI("http://127.0.0.1:9876/mcp"), endpoint)
    assertEquals(
      listOf(
        "edict", "ide-mcp", "--project-dir", project.toAbsolutePath().normalize().toString(),
        "--results-dir=${project.resolve("logs/intellij-mcp").toAbsolutePath().normalize()}",
        "--dist=/opt/idea", "--property=-Xmx8g",
      ),
      Files.readAllLines(qodana.resolveSibling("args")),
    )
    assertFalse(Files.exists(qodana.resolveSibling("stopped")))

    service.stop()
    assertTrue(Files.exists(qodana.resolveSibling("stopped")), "helper was not stopped through stdin EOF")
    assertEquals(1, client.closes)
    assertContains(Files.readString(log), "IDE log line")
  }

  @Test
  fun `stops the IDE of a killed helper and starts a new one on next use`() = runBlocking {
    val qodana = fakeQodana(
      """
      directory="${'$'}(dirname "${'$'}0")"
      echo started >> "${'$'}directory/starts"
      sleep 300 &
      echo ${'$'}! > "${'$'}directory/ide"
      echo ${'$'}${'$'} > "${'$'}directory/helper"
      echo "{\"status\":\"ready\",\"url\":\"http://127.0.0.1:9876/mcp\",\"pid\":${'$'}!}"
      cat > /dev/null
      """,
    )
    val first = FakeInspectionKtsClient()
    val clients = ArrayDeque(listOf(first, FakeInspectionKtsClient()))
    val service = IntellijMcpServerService(
      projectPath = project,
      qodanaExecutable = qodana.toString(),
      clientFactory = InspectionKtsClientFactory { clients.removeFirst() },
    )
    fun pid(name: String) = ProcessHandle.of(Files.readString(qodana.resolveSibling(name)).trim().toLong()).get()

    assertSame(first, service.start())
    val ide = pid("ide")
    pid("helper").destroyForcibly()
    ide.onExit().get(30, TimeUnit.SECONDS)
    assertFalse(ide.isAlive, "IDE outlived its killed helper")

    assertFalse(service.start() === first, "client of the dead helper was reused")
    assertEquals(1, first.closes)
    assertEquals(2, Files.readAllLines(qodana.resolveSibling("starts")).size)
    service.stop()
  }

  @Test
  fun `reports helper failure with its log`() = runBlocking {
    val qodana = fakeQodana(
      """
      echo "error running command: IntelliJ for Edict inspections requires --dist, --linter, or QODANA_DIST" >&2
      exit 1
      """,
    )
    val service = IntellijMcpServerService(
      projectPath = project,
      qodanaExecutable = qodana.toString(),
      log = project.resolve("intellij-mcp.log"),
      clientFactory = InspectionKtsClientFactory { FakeInspectionKtsClient() },
    )

    val error = assertFailsWith<IllegalStateException> { service.start() }
    assertContains(error.message.orEmpty(), "exit code 1")
    assertContains(error.message.orEmpty(), "requires --dist, --linter, or QODANA_DIST")
    service.stop()
  }

  @Test
  fun `stop without a started IDE starts no helper`() = runBlocking {
    val qodana = fakeQodana("exit 1")
    val service = IntellijMcpServerService(projectPath = project, qodanaExecutable = qodana.toString())

    service.stop()
    assertFalse(Files.exists(qodana.resolveSibling("args")))
  }

  @Test
  fun `a run needs neither Git nor the IDE until they are used`() = runBlocking {
    val qodana = fakeQodana("exit 1")
    val inspection = IntellijMcpServerService(projectPath = project, qodanaExecutable = qodana.toString())
    try {
      // The project is no Git repository, so resolving its HEAD here would throw.
      val repository = EdictRepository(EdictRepositoryDirectory(project))
      EdictNextGenerationService(repository, inspection, project)
      EdictNextEnrichmentService(repository, project)
    }
    finally {
      inspection.stop()
    }
    assertFalse(Files.exists(qodana.resolveSibling("args")))
  }

  @Test
  fun `uses an injected lifecycle without invoking qodana`() = runBlocking {
    val lifecycle = FakeIntellijMcpServerLifecycle()
    val client = FakeInspectionKtsClient()
    val service = IntellijMcpServerService(
      projectPath = project,
      qodanaExecutable = "/nonexistent/qodana",
      clientFactory = InspectionKtsClientFactory { client },
      serverLifecycle = lifecycle,
    )

    assertSame(client, service.start())
    assertEquals(1, lifecycle.starts)

    service.stop()
    assertEquals(1, lifecycle.stops)
  }

  @Test
  fun `restarts the endpoint and retries once when its session expires`() = runBlocking {
    val lifecycle = FakeIntellijMcpServerLifecycle()
    val clients = mutableListOf<FakeInspectionKtsClient>()
    val service = IntellijMcpServerService(
      projectPath = project,
      clientFactory = InspectionKtsClientFactory {
        restartingClient(expired = clients.isEmpty()).also(clients::add)
      },
      serverLifecycle = lifecycle,
    )

    val result = service.withClient { it.compile("inspection") }

    assertTrue(result.compilationSuccess)
    assertEquals(2, lifecycle.starts)
    assertEquals(1, lifecycle.stops)
    assertEquals(1, clients.first().closes)
    service.stop()
    assertEquals(2, lifecycle.stops)
    assertEquals(1, clients.last().closes)
  }

  @Test
  fun `restarts the endpoint when closing the stale client is unsupported`() = runBlocking {
    val lifecycle = FakeIntellijMcpServerLifecycle()
    val clients = mutableListOf<FakeInspectionKtsClient>()
    val service = IntellijMcpServerService(
      projectPath = project,
      clientFactory = InspectionKtsClientFactory {
        restartingClient(
          expired = clients.isEmpty(),
          closeFailure = if (clients.isEmpty()) UnsupportedOperationException("shutdownNow") else null,
        ).also(clients::add)
      },
      serverLifecycle = lifecycle,
    )

    val result = service.withClient { it.compile("inspection") }

    assertTrue(result.compilationSuccess)
    assertEquals(2, lifecycle.starts)
    assertEquals(1, lifecycle.stops)
    assertEquals(1, clients.first().closes)
    service.stop()
  }

  /** A stand-in for `qodana edict ide-mcp` that records its arguments next to itself. */
  private fun fakeQodana(body: String): Path {
    val directory = Files.createTempDirectory(project, "qodana")
    return directory.resolve("qodana").also { script ->
      Files.writeString(script, "#!/bin/sh\nprintf '%s\\n' \"\$@\" > \"\$(dirname \"\$0\")/args\"\n" + body.trimIndent() + "\n")
      assertTrue(script.toFile().setExecutable(true))
    }
  }
}

/** A client whose session the IDE has [expired], so compiling fails until the IDE is restarted. */
private fun restartingClient(expired: Boolean, closeFailure: RuntimeException? = null) = FakeInspectionKtsClient(
  compilation = {
    if (expired) throw StaleInspectionMcpSession("expired-session")
    InspectionKtsCompileResult(compilationSuccess = true)
  },
  closeFailure = closeFailure,
)
