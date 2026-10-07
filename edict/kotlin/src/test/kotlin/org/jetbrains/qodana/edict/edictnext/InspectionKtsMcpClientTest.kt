package org.jetbrains.qodana.edict.edictnext

import com.sun.net.httpserver.HttpExchange
import com.sun.net.httpserver.HttpServer
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonObject
import org.junit.jupiter.api.Timeout
import java.io.IOException
import java.net.InetSocketAddress
import java.net.URI
import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executors
import java.util.concurrent.LinkedBlockingQueue
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

class InspectionKtsMcpClientTest {
  @Test
  fun `initializes MCP and decodes structured tool results`() = runBlocking {
    val requests = mutableListOf<JsonObject>()
    val executor = Executors.newCachedThreadPool()
    val server = HttpServer.create(InetSocketAddress("127.0.0.1", 0), 0).apply {
      this.executor = executor
      createContext("/mcp") { exchange ->
        exchange.use {
          // No session and no event stream: the client must work with such a server too.
          if (exchange.requestMethod == "GET") {
            exchange.sendResponseHeaders(405, -1)
            return@createContext
          }
          val request = EdictNextJson.parseToJsonElement(exchange.requestBody.readAllBytes().decodeToString()).jsonObject
          synchronized(requests) { requests += request }
          if (request["id"] == null) {
            exchange.sendResponseHeaders(202, -1)
            return@createContext
          }
          val result = when (request.getValue("method").jsonPrimitive.content) {
            "initialize" -> buildJsonObject {
              put("protocolVersion", "2025-03-26")
              putJsonObject("capabilities") { putJsonObject("tools") {} }
              putJsonObject("serverInfo") { put("name", "inspection"); put("version", "1") }
            }
            "tools/call" -> buildJsonObject {
              val toolName = request.getValue("params").jsonObject.getValue("name").jsonPrimitive.content
              put("isError", false)
              put(
                "structuredContent",
                if (toolName == "compile_inspection_kts") {
                  EdictNextJson.encodeToJsonElement(
                    InspectionKtsCompileResult.serializer(),
                    InspectionKtsCompileResult(true, inspectionId = "sample-rule"),
                  )
                }
                else {
                  EdictNextJson.encodeToJsonElement(
                    InspectionKtsBatchRunResult.serializer(),
                    InspectionKtsBatchRunResult(
                      InspectionKtsCompileResult(true, inspectionId = "sample-rule"),
                      listOf(InspectionKtsFileResult("example", "Example.kt", listOf(InspectionKtsProblem("hit", 3, "WARNING")))),
                    ),
                  )
                },
              )
            }
            else -> error("Unexpected method")
          }
          val response = buildJsonObject {
            put("jsonrpc", "2.0")
            put("id", request.getValue("id"))
            put("result", result)
          }
          val bytes = EdictNextJson.encodeToString(response).encodeToByteArray()
          exchange.responseHeaders.set("Content-Type", "application/json")
          exchange.sendResponseHeaders(200, bytes.size.toLong())
          exchange.responseBody.write(bytes)
        }
      }
      start()
    }
    try {
      HttpInspectionKtsClient(
        URI("http://127.0.0.1:${server.address.port}/mcp"),
        "/project/root",
      ).use { client ->
        val proxyResult = client.callTool(
          "generate_inspection_kts_api",
          buildJsonObject {
            put("language", "Java")
            put("projectPath", "/untrusted/project")
          },
        )
        assertEquals("false", proxyResult.getValue("isError").jsonPrimitive.content)
        assertEquals("sample-rule", client.compile("compiled inspection").inspectionId)
        val result = client.runExamples(
          "inspection",
          listOf(InspectionKtsExampleRequest("example", "/examples/example", "Example.kt")),
        )
        assertEquals("sample-rule", result.compilation.inspectionId)
        assertEquals(3, result.files.single().foundProblems.single().lineNumber)
        client.analyzeProject("project inspection")
      }
      val toolCalls = synchronized(requests) {
        requests.filter { it["method"]?.jsonPrimitive?.content == "tools/call" }
      }
      val apiArguments = toolCalls.single {
        it.getValue("params").jsonObject.getValue("name").jsonPrimitive.content == "generate_inspection_kts_api"
      }.getValue("params").jsonObject.getValue("arguments").jsonObject
      assertEquals("Java", apiArguments.getValue("language").jsonPrimitive.content)
      assertEquals("/project/root", apiArguments.getValue("projectPath").jsonPrimitive.content)
      val compileArguments = toolCalls.single {
        it.getValue("params").jsonObject.getValue("name").jsonPrimitive.content == "compile_inspection_kts"
      }.getValue("params").jsonObject.getValue("arguments").jsonObject
      assertEquals("compiled inspection", compileArguments.getValue("inspectionKtsCode").jsonPrimitive.content)
      assertEquals("/project/root", compileArguments.getValue("projectPath").jsonPrimitive.content)
      val exampleArguments = toolCalls.single {
        it.getValue("params").jsonObject.getValue("name").jsonPrimitive.content == "run_inspection_kts_examples"
      }.getValue("params").jsonObject.getValue("arguments").jsonObject
      assertEquals(
        "example",
        exampleArguments.getValue("examples").jsonArray.single().jsonObject.getValue("id").jsonPrimitive.content,
      )
      assertEquals("/project/root", exampleArguments.getValue("projectPath").jsonPrimitive.content)
      val projectArguments = toolCalls.single {
        it.getValue("params").jsonObject.getValue("name").jsonPrimitive.content == "run_inspection_kts_project"
      }.getValue("params").jsonObject.getValue("arguments").jsonObject
      assertEquals("project inspection", projectArguments.getValue("inspectionKtsCode").jsonPrimitive.content)
      assertEquals("/project/root", projectArguments.getValue("projectPath").jsonPrimitive.content)
    }
    finally {
      server.stop(0)
      executor.shutdownNow()
    }
  }

  @Test
  @Timeout(30)
  fun `keeps an event stream open for each session`() = runBlocking {
    SessionServer().use { server ->
      HttpInspectionKtsClient(server.endpoint, "/project/root").use { client ->
        assertEquals("sample-rule", client.compile("inspection").inspectionId)
        assertEquals(listOf("session-1"), server.awaitStreams(1))
      }
      // Closing the client ends its stream instead of waiting for it.
      assertEquals(listOf("session-1"), server.awaitClosedStreams(1))
    }
  }

  @Test
  @Timeout(30)
  fun `renews a session the server dropped and retries on the same server`() = runBlocking {
    SessionServer(rejected = setOf("session-1")).use { server ->
      HttpInspectionKtsClient(server.endpoint, "/project/root").use { client ->
        assertEquals("sample-rule", client.compile("inspection").inspectionId)
        assertEquals(listOf("session-1", "session-2"), server.toolSessions())
        assertEquals(listOf("session-1", "session-2"), server.awaitStreams(2))
      }
    }
  }

  @Test
  @Timeout(30)
  fun `reports a session still rejected after renewal to its lifecycle owner`() = runBlocking {
    SessionServer(rejected = setOf("session-1", "session-2")).use { server ->
      HttpInspectionKtsClient(server.endpoint, "/project/root").use { client ->
        val error = assertFailsWith<StaleInspectionMcpSession> { client.compile("inspection") }
        assertEquals("session-2", error.sessionId)
        assertEquals(listOf("session-1", "session-2"), server.toolSessions())
      }
    }
  }

  /**
   * Streamable HTTP server numbering its sessions and holding GET event streams open. Like the IDE after it drops a session,
   * it accepts a [rejected] session's handshake but answers its tool calls with 404.
   */
  private class SessionServer(private val rejected: Set<String> = emptySet()) : AutoCloseable {
    private val executor = Executors.newCachedThreadPool()
    private val sessions = AtomicInteger()
    private val toolSessions = mutableListOf<String?>()
    private val streams = LinkedBlockingQueue<String>()
    private val closedStreams = LinkedBlockingQueue<String>()
    private val stopped = CountDownLatch(1)
    private val server = HttpServer.create(InetSocketAddress("127.0.0.1", 0), 0).apply {
      executor = this@SessionServer.executor
      createContext("/mcp") { exchange ->
        exchange.use {
          val session = exchange.requestHeaders.getFirst("Mcp-Session-Id")
          if (exchange.requestMethod == "GET") {
            stream(exchange, checkNotNull(session))
            return@createContext
          }
          val request = EdictNextJson.parseToJsonElement(exchange.requestBody.readAllBytes().decodeToString()).jsonObject
          val method = request.getValue("method").jsonPrimitive.content
          if (method == "tools/call") synchronized(toolSessions) { toolSessions += session }
          when {
            method == "tools/call" && session in rejected -> exchange.sendResponseHeaders(404, -1)
            request["id"] == null -> exchange.sendResponseHeaders(202, -1)
            method == "initialize" -> {
              exchange.responseHeaders.set("Mcp-Session-Id", "session-${sessions.incrementAndGet()}")
              respond(exchange, request, buildJsonObject {
                put("protocolVersion", "2025-03-26")
                putJsonObject("capabilities") { putJsonObject("tools") {} }
                putJsonObject("serverInfo") { put("name", "inspection"); put("version", "1") }
              })
            }
            else -> respond(exchange, request, buildJsonObject {
              put("isError", false)
              put("structuredContent", EdictNextJson.encodeToJsonElement(
                InspectionKtsCompileResult.serializer(),
                InspectionKtsCompileResult(true, inspectionId = "sample-rule"),
              ))
            })
          }
        }
      }
      start()
    }

    val endpoint: URI = URI("http://127.0.0.1:${server.address.port}/mcp")

    fun toolSessions(): List<String?> = synchronized(toolSessions) { toolSessions.toList() }

    fun awaitStreams(count: Int): List<String> = List(count) { checkNotNull(streams.poll(10, TimeUnit.SECONDS)) }

    fun awaitClosedStreams(count: Int): List<String> = List(count) { checkNotNull(closedStreams.poll(10, TimeUnit.SECONDS)) }

    /** Sends heartbeats like the IDE until the client goes away or the server stops. */
    private fun stream(exchange: HttpExchange, session: String) {
      exchange.responseHeaders.set("Content-Type", "text/event-stream")
      exchange.sendResponseHeaders(200, 0)
      streams += session
      try {
        while (!stopped.await(100, TimeUnit.MILLISECONDS)) {
          exchange.responseBody.write(": heartbeat\n\n".encodeToByteArray())
          exchange.responseBody.flush()
        }
      }
      catch (_: IOException) {
        closedStreams += session
      }
    }

    private fun respond(exchange: HttpExchange, request: JsonObject, result: JsonObject) {
      val bytes = EdictNextJson.encodeToString(buildJsonObject {
        put("jsonrpc", "2.0")
        put("id", request.getValue("id"))
        put("result", result)
      }).encodeToByteArray()
      exchange.responseHeaders.set("Content-Type", "application/json")
      exchange.sendResponseHeaders(200, bytes.size.toLong())
      exchange.responseBody.write(bytes)
    }

    override fun close() {
      stopped.countDown()
      server.stop(0)
      executor.shutdownNow()
    }
  }
}
