// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.integration.support.inspection

import com.sun.net.httpserver.HttpServer
import kotlinx.serialization.json.*
import org.jetbrains.qodana.edict.common.json
import org.jetbrains.qodana.edict.common.obj
import org.jetbrains.qodana.edict.common.text
import org.jetbrains.qodana.edict.common.wireJson
import org.junit.jupiter.api.Timeout
import org.junit.jupiter.api.io.TempDir
import java.io.OutputStream
import java.net.InetSocketAddress
import java.net.URI
import java.nio.file.Files
import java.nio.file.Path
import java.util.concurrent.*
import java.util.concurrent.atomic.AtomicInteger
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse

@Timeout(15)
class InspectionTransportTest {
    @TempDir
    lateinit var directory: Path

    @Test
    fun `legacy SSE matches overlapping replies by request ID instead of arrival order`() {
        val requests = CopyOnWriteArrayList<JsonObject>()
        val arrived = AtomicInteger()
        LegacyInspectionEndpoint { request, reply ->
            requests += request
            if (arrived.incrementAndGet() == 2) requests.reversed().forEach { pending ->
                reply(pending, buildJsonObject { put("value", pending.obj("params").obj("arguments").text("value")) })
            }
        }.use { endpoint ->
            InspectionMcpClient(endpoint.uri).use { client ->
                val first =
                    CompletableFuture.supplyAsync { client.call("probe", buildJsonObject { put("value", "first") }) }
                val second =
                    CompletableFuture.supplyAsync { client.call("probe", buildJsonObject { put("value", "second") }) }
                assertEquals("first", first.get(5, TimeUnit.SECONDS).text("value"))
                assertEquals("second", second.get(5, TimeUnit.SECONDS).text("value"))
                assertEquals(2, requests.size)
            }
        }
    }

    @Test
    fun `inspection proxy filters tools redirects source and records actual compiler result`() {
        val calls = CopyOnWriteArrayList<Pair<String, JsonObject>>()
        val unsafe = "write_project_file"
        val definitions = (inspectionToolNames + unsafe).map { name ->
            buildJsonObject {
                put("name", name); putJsonObject("inputSchema") { put("type", "object") }
            }
        }
        val source = directory.resolve("inspection-project")
        val audit = directory.resolve("log/inspection-mcp.jsonl")
        val measured = buildJsonObject {
            putJsonObject("structuredContent") {
                put(
                    "compilationSuccess",
                    true
                ); putJsonArray("foundProblems") {}
            }
        }
        InspectionToolProxy(definitions, source, audit) { name, arguments ->
            calls += name to arguments
            measured
        }.use { proxy ->
            InspectionMcpClient(URI.create(proxy.url)).use { client ->
                val listed = client.request("tools/list", JsonObject(emptyMap())).getValue("tools").jsonArray
                assertEquals(inspectionToolNames, listed.map { it.jsonObject.text("name") }.toSet())
                val requested = buildJsonObject {
                    put("projectPath", "/original/project"); put(
                    "inspectionKtsCode",
                    "candidate bytes"
                )
                }
                assertEquals(measured, client.call("run_inspection_kts", requested))
                assertFailsWith<IllegalStateException> { client.call(unsafe, JsonObject(emptyMap())) }
                assertEquals(1, calls.size, "Unlisted tools must never reach the IDE")
                assertEquals("run_inspection_kts", calls.single().first)
                assertEquals(source.toString(), calls.single().second.text("projectPath"))
                assertEquals("candidate bytes", calls.single().second.text("inspectionKtsCode"))
                val entry = wireJson.parseToJsonElement(Files.readAllLines(audit).single()).jsonObject
                assertEquals(requested, entry["requestedArguments"])
                assertEquals(calls.single().second, entry["arguments"])
                assertEquals(measured, entry["result"])
            }
        }
    }

    @Test
    fun `inspection proxy records external compiler failures without inventing a result`() {
        val definitions = inspectionToolNames.map { name -> buildJsonObject { put("name", name) } }
        val audit = directory.resolve("log/inspection-mcp.jsonl")
        InspectionToolProxy(
            definitions,
            directory.resolve("inspection-project"),
            audit
        ) { _, _ -> error("compiler unavailable") }.use { proxy ->
            InspectionMcpClient(URI.create(proxy.url)).use { client ->
                assertFailsWith<IllegalStateException> { client.call("run_inspection_kts", JsonObject(emptyMap())) }
                val entry = wireJson.parseToJsonElement(Files.readAllLines(audit).single()).jsonObject
                assertEquals("compiler unavailable", entry.text("error"))
                assertFalse("result" in entry)
            }
        }
    }
}

/** Minimal legacy MCP endpoint used to exercise the real asynchronous client transport. */
private class LegacyInspectionEndpoint(
    private val call: (JsonObject, (JsonObject, JsonObject) -> Unit) -> Unit,
) : AutoCloseable {
    private val server = HttpServer.create(InetSocketAddress("127.0.0.1", 0), 0)
    private val executor = Executors.newCachedThreadPool()
    private val events = CompletableFuture<OutputStream>()
    private val closed = CountDownLatch(1)
    private val writeLock = Any()
    val uri: URI

    init {
        server.executor = executor
        server.createContext("/sse") { exchange ->
            exchange.use {
                exchange.responseHeaders.set("Content-Type", "text/event-stream")
                exchange.sendResponseHeaders(200, 0)
                exchange.responseBody.write("event: endpoint\ndata: /messages\n\n".toByteArray())
                exchange.responseBody.flush()
                events.complete(exchange.responseBody)
                closed.await()
            }
        }
        server.createContext("/messages") { exchange ->
            exchange.use {
                val request =
                    wireJson.parseToJsonElement(exchange.requestBody.readAllBytes().toString(Charsets.UTF_8)).jsonObject
                exchange.sendResponseHeaders(202, -1)
                when (request.text("method")) {
                    "initialize" -> reply(request, buildJsonObject { put("protocolVersion", "2025-03-26") })
                    "notifications/initialized" -> Unit
                    else -> call(request, ::reply)
                }
            }
        }
        server.start()
        uri = URI.create("http://127.0.0.1:${server.address.port}/sse")
    }

    private fun reply(request: JsonObject, result: JsonObject) {
        val response =
            buildJsonObject { put("jsonrpc", "2.0"); put("id", request.getValue("id")); put("result", result) }
        // Multiline SSE payloads must be joined before decoding JSON.
        val lines = json.encodeToString(response).lines().joinToString("\n") { "data: $it" }
        synchronized(writeLock) {
            val stream = events.get(5, TimeUnit.SECONDS)
            stream.write(": keepalive\nevent: message\n$lines\n\n".toByteArray())
            stream.flush()
        }
    }

    override fun close() {
        closed.countDown(); server.stop(0); executor.shutdownNow()
    }
}
