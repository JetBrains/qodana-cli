package org.jetbrains.qodana.edict.edictnext

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.decodeFromJsonElement
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonObject
import java.net.URI
import java.net.http.HttpClient
import java.net.http.HttpRequest
import java.net.http.HttpResponse
import java.time.Duration
import java.util.concurrent.atomic.AtomicLong

@Serializable
internal data class InspectionKtsCompileResult(
  val compilationSuccess: Boolean,
  val compilationStatus: String? = null,
  val compilationErrorDetails: String? = null,
  val inspectionId: String? = null,
  val inspectionName: String? = null,
  val inspectionDescription: String? = null,
)

@Serializable
internal data class InspectionKtsExampleRequest(
  val id: String,
  val projectPath: String,
  val targetFilePath: String,
)

@Serializable
internal data class InspectionKtsProblem(
  val message: String,
  val lineNumber: Int,
  val highlightType: String,
  val startOffset: Int? = null,
  val endOffset: Int? = null,
  val elementText: String? = null,
)

@Serializable
internal data class InspectionKtsFileResult(
  val id: String? = null,
  val path: String,
  val foundProblems: List<InspectionKtsProblem> = emptyList(),
  val executionError: String? = null,
)

@Serializable
internal data class InspectionKtsBatchRunResult(
  val compilation: InspectionKtsCompileResult,
  val files: List<InspectionKtsFileResult> = emptyList(),
)

@Serializable
internal data class InspectionKtsProjectRunResult(
  val compilation: InspectionKtsCompileResult,
  val files: List<InspectionKtsFileResult> = emptyList(),
)

internal interface InspectionKtsClient : AutoCloseable {
  suspend fun compile(code: String): InspectionKtsCompileResult
  suspend fun runExamples(code: String, examples: List<InspectionKtsExampleRequest>): InspectionKtsBatchRunResult
  suspend fun analyzeProject(code: String): InspectionKtsProjectRunResult

  /** Calls an IDE MCP tool verbatim and returns its raw `CallToolResult` JSON, including error results. */
  suspend fun callTool(name: String, arguments: JsonObject): JsonObject
}

internal class StaleInspectionMcpSession(val sessionId: String) : RuntimeException()

/**
 * MCP client for the IDE that the Edict run opened. [projectPath] is that IDE project; every tool call is scoped to it,
 * because the IDE cannot otherwise tell which project a call targets.
 */
internal class HttpInspectionKtsClient(
  private val endpoint: URI,
  private val projectPath: String? = null,
  private val requestTimeout: Duration = Duration.ofMinutes(45),
) : InspectionKtsClient {
  private val client = HttpClient.newBuilder().connectTimeout(Duration.ofSeconds(30)).build()
  private val sequence = AtomicLong()
  @Volatile private var sessionId: String? = null
  @Volatile private var protocolVersion: String = "2025-03-26"

  init {
    val initialized = requestBlocking(
      "initialize",
      buildJsonObject {
        put("protocolVersion", protocolVersion)
        putJsonObject("capabilities") {}
        putJsonObject("clientInfo") {
          put("name", "edict-next")
          put("version", "1")
        }
      },
    )
    protocolVersion = initialized["protocolVersion"]?.jsonPrimitive?.content ?: protocolVersion
    sendBlocking(buildJsonObject {
      put("jsonrpc", "2.0")
      put("method", "notifications/initialized")
    })
  }

  override suspend fun compile(code: String): InspectionKtsCompileResult =
    call("compile_inspection_kts", buildJsonObject { put("inspectionKtsCode", code) })

  override suspend fun runExamples(
    code: String,
    examples: List<InspectionKtsExampleRequest>,
  ): InspectionKtsBatchRunResult = call(
    "run_inspection_kts_examples",
    buildJsonObject {
      put("inspectionKtsCode", code)
      put("projectPath", projectPath)
      put("examples", EdictNextJson.encodeToJsonElement(ListSerializer, examples))
    },
  )

  override suspend fun analyzeProject(code: String): InspectionKtsProjectRunResult =
    call("run_inspection_kts_project", buildJsonObject {
      put("inspectionKtsCode", code)
      put("projectPath", projectPath)
    })

  override suspend fun callTool(name: String, arguments: JsonObject): JsonObject = withContext(Dispatchers.IO) {
    requestBlocking(
      "tools/call",
      buildJsonObject {
        put("name", name)
        put("arguments", projectPath?.let { JsonObject(arguments + ("projectPath" to JsonPrimitive(it))) } ?: arguments)
      },
    )
  }

  private suspend inline fun <reified T> call(name: String, arguments: JsonObject): T {
    val result = callTool(name, arguments)
    check(result["isError"]?.jsonPrimitive?.content != "true") { "Inspection MCP tool '$name' failed: $result" }
    val structured = result["structuredContent"] as? JsonObject
      ?: (result["content"] as? JsonArray).orEmpty().firstNotNullOfOrNull { item ->
        runCatching {
          EdictNextJson.parseToJsonElement(item.jsonObject.getValue("text").jsonPrimitive.content).jsonObject
        }.getOrNull()
      }
      ?: error("Inspection MCP tool '$name' returned no JSON result")
    return EdictNextJson.decodeFromJsonElement<T>(structured)
  }

  private fun requestBlocking(method: String, params: JsonObject): JsonObject {
    val id = sequence.incrementAndGet()
    val response = sendBlocking(buildJsonObject {
      put("jsonrpc", "2.0")
      put("id", id)
      put("method", method)
      put("params", params)
    }) ?: error("Inspection MCP returned no response for '$method'")
    check("error" !in response) { "Inspection MCP '$method' failed: ${response["error"]}" }
    return response.getValue("result").jsonObject
  }

  private fun sendBlocking(message: JsonObject): JsonObject? {
    val requestSessionId = sessionId
    val builder = HttpRequest.newBuilder(endpoint)
      .timeout(requestTimeout)
      .header("Content-Type", "application/json")
      .header("Accept", "application/json, text/event-stream")
      .header("MCP-Protocol-Version", protocolVersion)
    requestSessionId?.let { builder.header("Mcp-Session-Id", it) }
    val response = client.send(
      builder.POST(HttpRequest.BodyPublishers.ofString(EdictNextJson.encodeToString(message))).build(),
      HttpResponse.BodyHandlers.ofString(),
    )
    if (
      requestSessionId != null &&
      response.statusCode() == 404
    ) {
      throw StaleInspectionMcpSession(requestSessionId)
    }
    response.headers().firstValue("Mcp-Session-Id").ifPresent { sessionId = it }
    check(response.statusCode() in 200..299) {
      "Inspection MCP HTTP ${response.statusCode()}: ${response.body().take(4_096)}"
    }
    if (message["id"] == null || response.statusCode() == 202) return null
    val body = response.body()
    return if (response.headers().firstValue("Content-Type").orElse("").contains("text/event-stream")) {
      body.lineSequence().filter { it.startsWith("data:") }.map { it.removePrefix("data:").trim() }
        .filter(String::isNotEmpty).mapNotNull { runCatching { EdictNextJson.parseToJsonElement(it).jsonObject }.getOrNull() }
        .firstOrNull { it["id"] == message["id"] }
        ?: error("Inspection MCP event stream did not contain the response")
    }
    else {
      EdictNextJson.parseToJsonElement(body).jsonObject
    }
  }

  override fun close() {
    client.close()
  }

  private companion object {
    val ListSerializer = kotlinx.serialization.builtins.ListSerializer(InspectionKtsExampleRequest.serializer())
  }
}

internal val INSPECTION_KTS_AGENT_TOOL_NAMES = setOf(
  "generate_psi_tree",
  "generate_inspection_kts_api",
  "generate_inspection_kts_examples",
)

internal val INSPECTION_KTS_UPSTREAM_TOOL_NAMES = INSPECTION_KTS_AGENT_TOOL_NAMES + setOf(
  "run_inspection_kts",
  "compile_inspection_kts",
  "run_inspection_kts_examples",
  "run_inspection_kts_project",
)
