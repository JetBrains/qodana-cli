// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.integration.support.inspection

import com.sun.net.httpserver.HttpServer
import java.net.InetSocketAddress
import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.StandardOpenOption.APPEND
import java.nio.file.StandardOpenOption.CREATE
import java.util.concurrent.Executors
import kotlinx.serialization.json.*
import org.jetbrains.qodana.edict.common.obj
import org.jetbrains.qodana.edict.common.text
import org.jetbrains.qodana.edict.common.wireJson

internal val inspectionToolNames = setOf("generate_psi_tree", "generate_inspection_kts_api", "generate_inspection_kts_examples", "run_inspection_kts")

/** Restrict external IDE access to generic compiler tools and redirect all execution to the disposable copy. */
internal class InspectionToolProxy(
    availableTools: List<JsonObject>,
    private val source: Path,
    private val audit: Path,
    private val call: (String, JsonObject) -> JsonObject,
) : AutoCloseable {
    private val tools = availableTools.filter { it.text("name") in inspectionToolNames }
    private val server: HttpServer
    private val executor = Executors.newCachedThreadPool()
    private val auditLock = Any()
    val url: String

    init {
        check(tools.map { it.text("name") }.toSet() == inspectionToolNames) { "External inspection MCP must expose all four generic compiler/documentation tools" }
        Files.createDirectories(audit.parent)
        server = HttpServer.create(InetSocketAddress("127.0.0.1", 0), 0)
        server.executor = executor
        server.createContext("/mcp") { exchange -> exchange.use {
            if (exchange.requestMethod != "POST") { exchange.sendResponseHeaders(405, -1); return@createContext }
            val body = exchange.requestBody.readNBytes(20 * 1024 * 1024 + 1)
            if (body.size > 20 * 1024 * 1024) { exchange.sendResponseHeaders(413, -1); return@createContext }
            try {
                val request = wireJson.parseToJsonElement(body.toString(Charsets.UTF_8)).jsonObject
                val id = request["id"]
                if (id == null) { exchange.sendResponseHeaders(202, -1); return@createContext }
                val params = request.obj("params")
                val result = when (request.text("method")) {
                    "initialize" -> buildJsonObject {
                        put("protocolVersion", params.text("protocolVersion").ifBlank { "2025-03-26" })
                        putJsonObject("capabilities") { putJsonObject("tools") {} }
                        putJsonObject("serverInfo") { put("name", "inspection"); put("version", "1") }
                    }
                    "ping" -> JsonObject(emptyMap())
                    "tools/list" -> buildJsonObject { put("tools", JsonArray(tools)) }
                    "tools/call" -> callTool(params)
                    else -> error("Unsupported inspection MCP method")
                }
                val response = buildJsonObject { put("jsonrpc", "2.0"); put("id", id); put("result", result) }
                val bytes = wireJson.encodeToString(response).toByteArray()
                exchange.responseHeaders.set("Content-Type", "application/json")
                exchange.sendResponseHeaders(200, bytes.size.toLong())
                exchange.responseBody.write(bytes)
            } catch (_: Exception) { exchange.sendResponseHeaders(500, -1) }
        } }
        server.start()
        url = "http://127.0.0.1:${server.address.port}/mcp"
    }

    private fun callTool(params: JsonObject): JsonObject {
        val name = params.text("name")
        require(name in inspectionToolNames) { "Only generic inspection tools are exposed" }
        val requested = params.obj("arguments")
        val arguments = JsonObject(requested + ("projectPath" to JsonPrimitive(source.toString())))
        var response: JsonObject? = null
        var failure: Throwable? = null
        try { return call(name, arguments).also { response = it } }
        catch (e: Exception) { failure = e; throw e }
        finally {
            val entry = buildJsonObject {
                put("tool", name); put("arguments", arguments); put("requestedArguments", requested)
                response?.let { put("result", it) }
                failure?.let { put("error", it.message ?: it.javaClass.simpleName) }
            }
            synchronized(auditLock) { Files.writeString(audit, wireJson.encodeToString(entry) + "\n", CREATE, APPEND) }
        }
    }

    override fun close() { server.stop(0); executor.shutdownNow() }
}
