package org.jetbrains.qodana.edict.edictnext

import kotlinx.coroutines.async
import kotlinx.coroutines.delay
import kotlinx.coroutines.runBlocking
import org.junit.jupiter.api.io.TempDir
import java.net.URI
import java.nio.file.Path
import java.util.concurrent.atomic.AtomicInteger
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertSame
import kotlin.test.assertTrue

class IntellijMcpServerServiceTest {
  @TempDir
  lateinit var project: Path

  @Test
  fun `starts once creates client from ready endpoint and stops`() = runBlocking {
    val runner = FakeCommandRunner()
    val client = EmptyClient()
    var endpoint: URI? = null
    val service = IntellijMcpServerService(
      projectPath = project,
      qodanaExecutable = "/qodana",
      commandRunner = runner,
      clientFactory = InspectionKtsClientFactory { uri -> endpoint = uri; client },
    )

    assertSame(client, service.start())
    assertSame(client, service.start())
    assertEquals(URI("http://127.0.0.1:9876/mcp"), endpoint)
    assertEquals(1, runner.commands.size)

    service.stop()
    assertEquals(2, runner.commands.size)
    assertEquals(listOf("/qodana", "edict", "linter-mcp", "stop"), runner.commands.last().take(4))
    assertEquals(1, client.closeCount)
  }

  @Test
  fun `wait for analysis serializes project analyses`() = runBlocking {
    val service = IntellijMcpServerService(
      projectPath = project,
      commandRunner = FakeCommandRunner(),
      clientFactory = InspectionKtsClientFactory { EmptyClient() },
    )
    val active = AtomicInteger()
    val maximum = AtomicInteger()
    val jobs = List(4) {
      async {
        service.waitForAnalysis {
          val current = active.incrementAndGet()
          maximum.updateAndGet { previous -> maxOf(previous, current) }
          delay(10)
          active.decrementAndGet()
        }
      }
    }
    jobs.forEach { it.await() }

    assertEquals(1, maximum.get())
    service.stop()
  }

  @Test
  fun `uses an injected lifecycle without invoking qodana`() = runBlocking {
    val runner = FakeCommandRunner()
    val lifecycle = FakeLifecycle()
    val client = EmptyClient()
    val service = IntellijMcpServerService(
      projectPath = project,
      commandRunner = runner,
      clientFactory = InspectionKtsClientFactory { client },
      serverLifecycle = lifecycle,
    )

    assertSame(client, service.start())
    assertEquals(1, lifecycle.startCount)
    assertTrue(runner.commands.isEmpty())

    service.stop()
    assertEquals(1, lifecycle.stopCount)
    assertTrue(runner.commands.isEmpty())
  }

  @Test
  fun `restarts the endpoint and retries once when its session expires`() = runBlocking {
    val lifecycle = FakeLifecycle()
    val clients = mutableListOf<RestartClient>()
    val service = IntellijMcpServerService(
      projectPath = project,
      clientFactory = InspectionKtsClientFactory {
        RestartClient(expired = clients.isEmpty()).also(clients::add)
      },
      serverLifecycle = lifecycle,
    )

    val result = service.withClient { it.compile("inspection") }

    assertTrue(result.compilationSuccess)
    assertEquals(2, lifecycle.startCount)
    assertEquals(1, lifecycle.stopCount)
    assertEquals(1, clients.first().closeCount)
    service.stop()
    assertEquals(2, lifecycle.stopCount)
    assertEquals(1, clients.last().closeCount)
  }

  @Test
  fun `restarts the endpoint when closing the stale client is unsupported`() = runBlocking {
    val lifecycle = FakeLifecycle()
    val clients = mutableListOf<RestartClient>()
    val service = IntellijMcpServerService(
      projectPath = project,
      clientFactory = InspectionKtsClientFactory {
        RestartClient(
          expired = clients.isEmpty(),
          closeFailure = if (clients.isEmpty()) UnsupportedOperationException("shutdownNow") else null,
        ).also(clients::add)
      },
      serverLifecycle = lifecycle,
    )

    val result = service.withClient { it.compile("inspection") }

    assertTrue(result.compilationSuccess)
    assertEquals(2, lifecycle.startCount)
    assertEquals(1, lifecycle.stopCount)
    assertEquals(1, clients.first().closeCount)
    service.stop()
  }

  private class FakeCommandRunner : CommandRunner {
    val commands = mutableListOf<List<String>>()

    override suspend fun run(command: List<String>): CommandResult {
      commands += command
      return if (command.contains("start")) {
        CommandResult(0, """{"status":"ready","url":"http://127.0.0.1:9876/mcp"}""")
      }
      else {
        CommandResult(0, "{}")
      }
    }
  }

  private class EmptyClient : InspectionKtsClient {
    var closeCount = 0
    override suspend fun compile(code: String) = error("not used")
    override suspend fun runExamples(code: String, examples: List<InspectionKtsExampleRequest>) = error("not used")
    override suspend fun analyzeProject(code: String) = error("not used")
    override fun close() { closeCount++ }
  }

  private class RestartClient(
    private val expired: Boolean,
    private val closeFailure: RuntimeException? = null,
  ) : InspectionKtsClient {
    var closeCount = 0
    override suspend fun compile(code: String): InspectionKtsCompileResult {
      if (expired) throw StaleInspectionMcpSession("expired-session")
      return InspectionKtsCompileResult(compilationSuccess = true)
    }
    override suspend fun runExamples(code: String, examples: List<InspectionKtsExampleRequest>) = error("not used")
    override suspend fun analyzeProject(code: String) = error("not used")
    override fun close() {
      closeCount++
      closeFailure?.let { throw it }
    }
  }

  private class FakeLifecycle : IntellijMcpServerLifecycle {
    var startCount = 0
    var stopCount = 0
    override suspend fun start(): URI {
      startCount++
      return URI("http://127.0.0.1:9876/mcp")
    }
    override suspend fun stop() { stopCount++ }
  }
}
