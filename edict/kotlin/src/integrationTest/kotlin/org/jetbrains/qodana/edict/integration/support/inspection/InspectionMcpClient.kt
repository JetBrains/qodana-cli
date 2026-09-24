// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.integration.support.inspection

import java.io.BufferedReader
import java.io.InputStream
import java.net.URI
import java.net.http.HttpClient
import java.net.http.HttpRequest
import java.net.http.HttpResponse
import java.time.Duration
import java.util.concurrent.CompletableFuture
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicLong
import kotlinx.serialization.json.*
import org.jetbrains.qodana.edict.common.flag
import org.jetbrains.qodana.edict.common.text
import org.jetbrains.qodana.edict.common.wireJson

internal data class InspectionResult(val compilationSuccess: Boolean, val compilationStatus: String, val problemLines: List<Int>)

internal fun decodeInspectionResult(result: JsonObject): InspectionResult {
    check(result.flag("isError") != true) { "Inspection MCP returned an error: $result" }
    val data = (result["structuredContent"] as? JsonObject) ?: (result["content"] as? JsonArray).orEmpty()
        .firstNotNullOfOrNull { item -> runCatching { wireJson.parseToJsonElement(item.jsonObject.text("text")).jsonObject }.getOrNull() }
        ?: error("Inspection result has no JSON content")
    return InspectionResult(data.flag("compilationSuccess") == true, data.text("compilationStatus"),
        (data["foundProblems"] as? JsonArray).orEmpty().map { it.jsonObject.getValue("lineNumber").jsonPrimitive.int })
}

/** Test-only client for both IDE MCP transports; SSE messages are correlated with their request IDs. */
internal class InspectionMcpClient(private val endpoint: URI) : AutoCloseable {
    private val client = HttpClient.newBuilder().connectTimeout(Duration.ofSeconds(30)).build()
    private val sequence = AtomicLong()
    private val pending = ConcurrentHashMap<String, CompletableFuture<JsonObject>>()
    private val postEndpoint = CompletableFuture<URI>()
    private var events: InputStream? = null
    private var eventThread: Thread? = null
    private var session: String? = null
    private var protocol = "2025-03-26"
    private val legacySse = endpoint.path.endsWith("/sse")

    init {
        try {
            if (legacySse) {
                val response = client.send(HttpRequest.newBuilder(endpoint).header("Accept", "text/event-stream").GET().build(), HttpResponse.BodyHandlers.ofInputStream())
                check(response.statusCode() == 200) { "Inspection SSE connection failed: HTTP ${response.statusCode()}" }
                events = response.body()
                eventThread = Thread({
                    try {
                        readEvents(response.body().bufferedReader()) { event, data ->
                            if (event == "endpoint") {
                                val uri = endpoint.resolve(data)
                                require(uri.scheme == endpoint.scheme && uri.authority == endpoint.authority) { "Inspection SSE redirected to another origin" }
                                postEndpoint.complete(uri)
                            } else runCatching { wireJson.parseToJsonElement(data).jsonObject }.getOrNull()?.let { value ->
                                value["id"]?.let { pending[it.toString()]?.complete(value) }
                            }
                            false
                        }
                        error("Inspection SSE stream closed")
                    } catch (e: Exception) {
                        postEndpoint.completeExceptionally(e)
                        pending.values.forEach { it.completeExceptionally(e) }
                    }
                }, "inspection-mcp-events").apply { isDaemon = true; start() }
                postEndpoint.get(60, TimeUnit.SECONDS)
            } else postEndpoint.complete(endpoint)
            val initialized = request("initialize", buildJsonObject {
                put("protocolVersion", protocol)
                putJsonObject("capabilities") {}
                putJsonObject("clientInfo") { put("name", "kotlin-managed-generation-test"); put("version", "1") }
            })
            protocol = initialized.text("protocolVersion").ifBlank { protocol }
            send(buildJsonObject { put("jsonrpc", "2.0"); put("method", "notifications/initialized") })
        } catch (e: Throwable) { close(); throw e }
    }

    fun call(name: String, arguments: JsonObject): JsonObject = request("tools/call", buildJsonObject { put("name", name); put("arguments", arguments) })

    fun request(method: String, params: JsonObject): JsonObject {
        val id = sequence.incrementAndGet()
        val future = CompletableFuture<JsonObject>()
        pending[id.toString()] = future
        try {
            send(buildJsonObject { put("jsonrpc", "2.0"); put("id", id); put("method", method); put("params", params) })
                ?.let { future.complete(it) }
            val response = future.get(5, TimeUnit.MINUTES)
            check("error" !in response) { "Inspection MCP $method failed: ${response["error"]}" }
            return response.getValue("result").jsonObject
        } finally { pending.remove(id.toString()) }
    }

    private fun send(message: JsonObject): JsonObject? {
        val request = HttpRequest.newBuilder(postEndpoint.get(60, TimeUnit.SECONDS)).timeout(Duration.ofMinutes(5))
            .header("Content-Type", "application/json").header("Accept", "application/json, text/event-stream")
            .header("MCP-Protocol-Version", protocol)
        session?.let { request.header("Mcp-Session-Id", it) }
        val response = client.send(request.POST(HttpRequest.BodyPublishers.ofString(wireJson.encodeToString(message))).build(), HttpResponse.BodyHandlers.ofInputStream())
        response.headers().firstValue("Mcp-Session-Id").ifPresent { session = it }
        return response.body().use { body ->
            check(response.statusCode() in 200..299) { "Inspection MCP HTTP ${response.statusCode()}: ${body.readNBytes(4096).toString(Charsets.UTF_8)}" }
            if (legacySse || message["id"] == null || response.statusCode() == 202) return@use null
            if (response.headers().firstValue("Content-Type").orElse("").contains("text/event-stream")) {
                var result: JsonObject? = null
                readEvents(body.bufferedReader()) { _, data ->
                    val value = wireJson.parseToJsonElement(data).jsonObject
                    if (value["id"] == message["id"]) result = value
                    result != null
                }
                checkNotNull(result) { "Inspection SSE response did not contain the request result" }
            } else wireJson.parseToJsonElement(body.readAllBytes().toString(Charsets.UTF_8)).jsonObject
        }
    }

    override fun close() {
        events?.close()
        eventThread?.interrupt()
        client.shutdownNow()
    }
}

private fun readEvents(reader: BufferedReader, consume: (String, String) -> Boolean) {
    var event = "message"
    val data = mutableListOf<String>()
    while (true) {
        val line = reader.readLine() ?: break
        if (line.isEmpty()) {
            if (data.isNotEmpty() && consume(event, data.joinToString("\n"))) return
            event = "message"; data.clear()
        } else when {
            line.startsWith("event:") -> event = line.removePrefix("event:").removePrefix(" ")
            line.startsWith("data:") -> data += line.removePrefix("data:").removePrefix(" ")
        }
    }
    if (data.isNotEmpty()) consume(event, data.joinToString("\n"))
}
