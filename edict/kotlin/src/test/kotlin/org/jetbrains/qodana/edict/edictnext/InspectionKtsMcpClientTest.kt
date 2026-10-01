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
      HttpInspectionKtsClient(
        URI("http://127.0.0.1:${server.address.port}/mcp"),
        "/project/root",
      ).use { client ->
        val proxyResult = client.proxyTool(
          "generate_inspection_kts_api",
          buildJsonObject {
            put("language", "Java")
            put("projectPath", "/untrusted/project")
          },
        )
        assertEquals("false", proxyResult.getValue("isError").jsonPrimitive.content)
        assertFailsWith<IllegalArgumentException> {
          client.proxyTool("run_inspection_kts", buildJsonObject { put("inspectionKtsCode", "inspection") })
        }
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
  fun `reports an expired MCP session to its lifecycle owner`() = runBlocking {
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
            val bytes = byteArrayOf()
            exchange.sendResponseHeaders(404, bytes.size.toLong())
            exchange.responseBody.write(bytes)
            return@createContext
          }
          val result = when (method) {
            "initialize" -> {
              exchange.responseHeaders.set("Mcp-Session-Id", "expired-session")
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
      HttpInspectionKtsClient(
        URI("http://127.0.0.1:${server.address.port}/mcp"),
        "/project/root",
      ).use { client ->
        assertFailsWith<StaleInspectionMcpSession> { client.compile("inspection") }
      }
      assertEquals(listOf("expired-session"), synchronized(toolSessions) { toolSessions.toList() })
    }
    finally {
      server.stop(0)
      executor.shutdownNow()
    }
  }
}
