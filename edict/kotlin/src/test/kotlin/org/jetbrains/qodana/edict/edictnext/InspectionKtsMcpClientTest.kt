package org.jetbrains.qodana.edict.edictnext

import com.sun.net.httpserver.HttpServer
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonObject
import java.net.InetSocketAddress
import java.net.URI
import java.util.concurrent.Executors
import java.util.concurrent.atomic.AtomicInteger
import kotlin.test.Test
import kotlin.test.assertEquals

class InspectionKtsMcpClientTest {
  @Test
  fun `initializes MCP and decodes structured tool results`() = runBlocking {
    val requests = mutableListOf<JsonObject>()
    val executor = Executors.newCachedThreadPool()
    val server = HttpServer.create(InetSocketAddress("127.0.0.1", 0), 0).apply {
      this.executor = executor
      createContext("/mcp") { exchange ->
        exchange.use {
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
              put("isError", false)
              put(
                "structuredContent",
                EdictNextJson.encodeToJsonElement(
                  InspectionKtsBatchRunResult.serializer(),
                  InspectionKtsBatchRunResult(
                    InspectionKtsCompileResult(true, inspectionId = "sample-rule"),
                    listOf(InspectionKtsFileResult("example", "Example.kt", listOf(InspectionKtsProblem("hit", 3, "WARNING")))),
                  ),
                ),
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
      HttpInspectionKtsClient(URI("http://127.0.0.1:${server.address.port}/mcp")).use { client ->
        val result = client.runExamples(
          "inspection",
          listOf(InspectionKtsExampleRequest("example", "/examples/example", "Example.kt")),
        )
        assertEquals("sample-rule", result.compilation.inspectionId)
        assertEquals(3, result.files.single().foundProblems.single().lineNumber)
      }
      val toolCall = synchronized(requests) {
        requests.single { it["method"]?.jsonPrimitive?.content == "tools/call" }
      }
      val arguments = toolCall.getValue("params").jsonObject.getValue("arguments").jsonObject
      assertEquals("example", arguments.getValue("examples").jsonArray.single().jsonObject.getValue("id").jsonPrimitive.content)
    }
    finally {
      server.stop(0)
      executor.shutdownNow()
    }
  }

  @Test
  fun `reinitializes and retries once when the MCP session expires`() = runBlocking {
    val initializeCount = AtomicInteger()
    val toolSessions = mutableListOf<String?>()
    val executor = Executors.newCachedThreadPool()
    val server = HttpServer.create(InetSocketAddress("127.0.0.1", 0), 0).apply {
      this.executor = executor
      createContext("/mcp") { exchange ->
        exchange.use {
          val request = EdictNextJson.parseToJsonElement(exchange.requestBody.readAllBytes().decodeToString()).jsonObject
          val method = request.getValue("method").jsonPrimitive.content
          val requestSession = exchange.requestHeaders.getFirst("Mcp-Session-Id")
          if (method == "notifications/initialized") {
            exchange.sendResponseHeaders(202, -1)
            return@createContext
          }
          if (method == "tools/call") {
            synchronized(toolSessions) { toolSessions += requestSession }
            if (requestSession == "expired-session") {
              val bytes = "Streamable HTTP session not found".encodeToByteArray()
              exchange.sendResponseHeaders(404, bytes.size.toLong())
              exchange.responseBody.write(bytes)
              return@createContext
            }
          }
          val result = when (method) {
            "initialize" -> {
              val session = if (initializeCount.incrementAndGet() == 1) "expired-session" else "fresh-session"
              exchange.responseHeaders.set("Mcp-Session-Id", session)
              buildJsonObject {
                put("protocolVersion", "2025-03-26")
                putJsonObject("capabilities") { putJsonObject("tools") {} }
                putJsonObject("serverInfo") { put("name", "inspection"); put("version", "1") }
              }
            }
            "tools/call" -> EdictNextJson.encodeToJsonElement(
              InspectionKtsCompileResult.serializer(),
              InspectionKtsCompileResult(true, inspectionId = "sample-rule"),
            )
            else -> error("Unexpected method")
          }
          val toolResult = if (method == "tools/call") {
            buildJsonObject { put("isError", false); put("structuredContent", result) }
          }
          else result
          val response = buildJsonObject {
            put("jsonrpc", "2.0")
            put("id", request.getValue("id"))
            put("result", toolResult)
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
      HttpInspectionKtsClient(URI("http://127.0.0.1:${server.address.port}/mcp")).use { client ->
        assertEquals("sample-rule", client.compile("inspection").inspectionId)
      }
      assertEquals(2, initializeCount.get())
      assertEquals(listOf("expired-session", "fresh-session"), synchronized(toolSessions) { toolSessions.toList() })
    }
    finally {
      server.stop(0)
      executor.shutdownNow()
    }
  }
}
