// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.benchmark

import kotlinx.serialization.json.*
import java.io.InputStream
import java.net.URI
import java.net.http.HttpClient
import java.net.http.HttpRequest
import java.net.http.HttpResponse
import java.time.Duration
import java.util.concurrent.atomic.AtomicBoolean
import kotlin.concurrent.thread

internal class InfrastructureFailure(message: String) : IllegalStateException(message)
internal fun obj(vararg values: Pair<String, JsonElement>) = JsonObject(mapOf(*values))
internal fun text(value: String) = JsonPrimitive(value)
internal fun JsonObject.string(name: String) = getValue(name).jsonPrimitive.content

/** IntelliJ requires a live GET stream, including when POST responses use plain JSON. */
internal class InspectionClient(private val endpoint: URI) : AutoCloseable {
    private val http = HttpClient.newBuilder().connectTimeout(Duration.ofSeconds(30)).build()
    private var session: String? = null
    private var protocol = "2025-03-26"
    private var sequence = 0
    private val closed = AtomicBoolean()
    private val disconnected = AtomicBoolean()
    private val events: InputStream
    private val eventThread: Thread

    init {
        val initialized = request("initialize", obj("protocolVersion" to text(protocol), "capabilities" to obj(),
            "clientInfo" to obj("name" to text("edict-benchmark"), "version" to text("2"))))
        protocol = initialized.string("protocolVersion")
        request("notifications/initialized", notification = true)
        val response = http.send(headers().GET().header("Accept", "text/event-stream").build(), HttpResponse.BodyHandlers.ofInputStream())
        if (response.statusCode() != 200) {
            response.body().close()
            http.close()
            throw InfrastructureFailure("Inspections MCP event stream returned HTTP ${response.statusCode()}")
        }
        events = response.body()
        eventThread = thread(name = "inspection-events", isDaemon = true) {
            try { events.bufferedReader().use { reader -> while (reader.readLine() != null) Unit } }
            catch (_: Exception) { /* Closing the stream interrupts the reader normally. */ }
            finally { disconnected.set(true) }
        }
    }

    private fun headers() = HttpRequest.newBuilder(endpoint).apply {
        session?.let { header("Mcp-Session-Id", it) }
        header("MCP-Protocol-Version", protocol)
    }

    @Synchronized
    fun request(method: String, params: JsonObject = obj(), notification: Boolean = false): JsonObject {
        if (closed.get() || disconnected.get()) throw InfrastructureFailure("Inspections MCP stream disconnected")
        val id = ++sequence
        val body = buildJsonObject {
            put("jsonrpc", "2.0"); put("method", method); put("params", params)
            if (!notification) put("id", id)
        }
        val response = http.send(headers().header("Content-Type", "application/json")
            .header("Accept", "application/json, text/event-stream").timeout(Duration.ofMinutes(30))
            .POST(HttpRequest.BodyPublishers.ofString(body.toString())).build(), HttpResponse.BodyHandlers.ofInputStream())
        session = response.headers().firstValue("Mcp-Session-Id").orElse(session)
        response.body().bufferedReader().use { reader ->
            if (response.statusCode() !in 200..299) {
                throw InfrastructureFailure("Inspections MCP $method returned HTTP ${response.statusCode()}: ${reader.readText().take(500)}")
            }
            if (notification || response.statusCode() == 202) return obj()
            val result = if (response.headers().firstValue("Content-Type").orElse("").contains("text/event-stream")) {
                var found: JsonObject? = null
                val data = StringBuilder()
                while (found == null) {
                    val line = reader.readLine() ?: throw InfrastructureFailure("MCP response stream ended early")
                    if (line.startsWith("data:")) data.appendLine(line.substring(5).trimStart())
                    if (line.isEmpty() && data.isNotEmpty()) {
                        val message = json.parseToJsonElement(data.toString()).jsonObject
                        data.clear()
                        if (message["id"]?.jsonPrimitive?.intOrNull == id) found = message
                    }
                }
                found
            } else json.parseToJsonElement(reader.readText()).jsonObject
            result["error"]?.let { throw InfrastructureFailure("MCP $method: $it") }
            return result.getValue("result").jsonObject
        }
    }

    override fun close() {
        if (!closed.compareAndSet(false, true)) return
        events.close()
        eventThread.join(5_000)
        http.shutdownNow()
    }
}
