// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.benchmark

import com.sun.net.httpserver.HttpServer
import kotlinx.serialization.json.*
import java.net.InetSocketAddress
import java.net.URI
import java.nio.file.Path
import java.nio.file.StandardOpenOption.*
import java.security.MessageDigest
import java.util.concurrent.Executors
import kotlin.io.path.*

internal val inspectionTools = setOf("generate_psi_tree", "generate_inspection_kts_api", "generate_inspection_kts_examples", "run_inspection_kts")
internal val probeCode = """
    val probe = localInspection { file, inspection ->
        if (file.text.contains("EDICT_PROBE_MARKER")) inspection.registerProblem(file, "Benchmark infrastructure probe")
    }
    listOf(InspectionKts(id = "EdictBenchmarkInfrastructureProbe", localTool = probe,
        name = "Benchmark infrastructure probe", htmlDescription = "<html>Probe</html>", level = HighlightDisplayLevel.WARNING))
""".trimIndent()

internal class InspectionProxy(private val client: InspectionClient, tools: JsonArray, private val project: Path,
                               private val output: Path, private val scans: ProjectRunner) : AutoCloseable {
    @Volatile var failure: Throwable? = null
        private set
    private val executor = Executors.newCachedThreadPool()
    private val server = HttpServer.create(InetSocketAddress("127.0.0.1", 0), 0)
    private val allowedTools = JsonArray(tools.filter { it.jsonObject.string("name") in inspectionTools } +
        json.parseToJsonElement("""{"name":"run_project_inspection","description":"Compile and execute exactly one inspection on the entire project. Returns complete SARIF findings, candidate hash and source revision. Does not write Edict state.","inputSchema":{"type":"object","properties":{"inspectionKtsCode":{"type":"string"},"inspectionId":{"type":"string"}},"required":["inspectionKtsCode","inspectionId"],"additionalProperties":false}}"""))
    val url = "http://127.0.0.1:${server.address.port}/mcp"

    init {
        output.resolve("log").createDirectories()
        server.executor = executor
        server.createContext("/mcp") { exchange ->
            exchange.use {
                if (exchange.requestMethod != "POST") { exchange.sendResponseHeaders(405, -1); return@createContext }
                val bytes = exchange.requestBody.readNBytes(20 * 1024 * 1024 + 1)
                if (bytes.size > 20 * 1024 * 1024) { exchange.sendResponseHeaders(413, -1); return@createContext }
                var id: JsonElement = JsonNull
                val response = try {
                    val request = json.parseToJsonElement(bytes.decodeToString()).jsonObject
                    if ("id" !in request) { exchange.sendResponseHeaders(202, -1); return@createContext }
                    id = request.getValue("id")
                    val params = request["params"]?.jsonObject ?: obj()
                    val result = when (request.string("method")) {
                        "initialize" -> obj("protocolVersion" to params.getValue("protocolVersion"), "capabilities" to obj("tools" to obj()),
                            "serverInfo" to obj("name" to text("inspection"), "version" to text("2")))
                        "tools/list" -> obj("tools" to allowedTools)
                        "ping" -> obj()
                        "tools/call" -> call(params.string("name"), params["arguments"]?.jsonObject ?: obj())
                        else -> error("Unsupported method")
                    }
                    obj("jsonrpc" to text("2.0"), "id" to id, "result" to result)
                } catch (error: Exception) {
                    obj("jsonrpc" to text("2.0"), "id" to id, "error" to obj("code" to JsonPrimitive(-32603), "message" to text(error.message.orEmpty())))
                }
                val data = response.toString().toByteArray()
                exchange.responseHeaders.set("Content-Type", "application/json")
                exchange.sendResponseHeaders(200, data.size.toLong())
                exchange.responseBody.write(data)
            }
        }
        server.start()
    }

    @Synchronized
    fun call(name: String, arguments: JsonObject): JsonObject {
        val started = System.nanoTime()
        var audit = obj()
        try {
            failure?.let { throw InfrastructureFailure(it.message.orEmpty()) }
            val result = if (name == "run_project_inspection") {
                val code = arguments.string("inspectionKtsCode")
                val rule = arguments.string("inspectionId")
                require(rule.matches(Regex("EdictBenchmark[A-Za-z0-9]+"))) { "Use the assigned EdictBenchmark<OriginalRuleId> ID" }
                val digest = MessageDigest.getInstance("SHA-256").digest(code.toByteArray()).joinToString("") { "%02x".format(it) }
                val destination = output.resolve("project-runs/$rule/$digest")
                val cached = destination.resolve("response.json")
                val measured = if (cached.exists()) readObject(cached) else {
                    val sarif = scans.scan(destination, mapOf(rule to code))
                    obj("candidateHash" to text(digest), "sarif" to sarif,
                        "sourceRevision" to text(commandOutput("git", "-C", project.toString(), "rev-parse", "HEAD"))).also { writeJson(cached, it) }
                }
                obj("content" to JsonArray(listOf(obj("type" to text("text"), "text" to text(measured.toString())))), "isError" to JsonPrimitive(false))
            } else {
                require(name in inspectionTools) { "Only generic inspection tools are exposed" }
                client.request("tools/call", obj("name" to text(name), "arguments" to JsonObject(arguments + ("projectPath" to text(project.toString())))))
            }
            audit = obj("arguments" to arguments, "result" to result)
            return result
        } catch (error: Exception) {
            audit = obj("error" to text(error.message.orEmpty()))
            if (error is InfrastructureFailure || error is java.io.IOException) failure = error
            throw error
        } finally {
            val entry = JsonObject(mapOf("tool" to text(name),
                "elapsedSeconds" to JsonPrimitive((System.nanoTime() - started) / 1e9)) + audit)
            java.nio.file.Files.writeString(output.resolve("log/inspection-mcp.jsonl"), "$entry\n", CREATE, APPEND)
        }
    }

    fun preflight() {
        teamCity("message", "text" to "Checking MCP compiler, concurrent project scan and persistent MCP session")
        val started = System.nanoTime()
        val result = call("run_inspection_kts", obj("inspectionKtsCode" to text(probeCode),
            "contextPath" to text("core/src/main/java/BenchmarkProbe.java"),
            "targetFileContent" to text("class BenchmarkProbe { /* EDICT_PROBE_MARKER */ }")))
        val structured = (result["structuredContent"] as? JsonObject) ?: result["content"]?.jsonArray?.firstNotNullOfOrNull {
            runCatching { json.parseToJsonElement(it.jsonObject.string("text")) as? JsonObject }.getOrNull()
        }
        if (result["isError"]?.jsonPrimitive?.booleanOrNull == true || structured?.get("compilationSuccess")?.jsonPrimitive?.booleanOrNull != true ||
            structured?.get("foundProblems")?.jsonArray.isNullOrEmpty()) {
            writeJson(output.resolve("log/compiler-probe.json"), result)
            throw InfrastructureFailure("Compiler/execution probe failed; see log/compiler-probe.json")
        }
        call("run_project_inspection", obj("inspectionKtsCode" to text(probeCode), "inspectionId" to text("EdictBenchmarkInfrastructureProbe")))
        val remaining = 16_000 - (System.nanoTime() - started) / 1_000_000
        if (remaining > 0) Thread.sleep(remaining)
        check(call("generate_inspection_kts_api", obj("language" to text("Java")))["isError"]?.jsonPrimitive?.booleanOrNull != true)
        teamCity("message", "text" to "Execution preflight passed in ${(System.nanoTime() - started) / 1_000_000_000}s")
    }

    override fun close() { server.stop(0); executor.shutdownNow() }
}

internal class InspectionServer(project: Path, private val output: Path) : AutoCloseable {
    private val log = output.resolve("log/inspection-server.log")
    private val process = qodanaProcess(project, output.resolve("mcp-results"), output.resolve("mcp-cache"), log,
        "--script", "mcp-server", "--profile-name", "empty").start()
    var client: InspectionClient? = null
        private set

    fun connect(): Pair<InspectionClient, JsonArray> {
        val deadline = System.nanoTime() + java.util.concurrent.TimeUnit.MINUTES.toNanos(20)
        var ticks = 0
        while (System.nanoTime() < deadline) {
            if (!process.isAlive) throw InfrastructureFailure("Inspections MCP exited ${process.exitValue()}; see $log")
            val endpoint = Regex("Streamable HTTP endpoint:\\s*(https?://[^\\s\\u001b]+)").find(log.readText())?.groupValues?.get(1)
            if (endpoint != null) {
                val connected = InspectionClient(URI(endpoint)).also { client = it }
                val tools = connected.request("tools/list").getValue("tools").jsonArray
                check(tools.map { it.jsonObject.string("name") }.containsAll(inspectionTools)) { "Inspections MCP is missing required tools" }
                writeJson(output.resolve("inspection-tools.json"), tools)
                return connected to tools
            }
            if (ticks++ % 30 == 0) teamCity("message", "text" to "Waiting for inspections MCP project import; see inspection-server.log")
            Thread.sleep(1000)
        }
        throw InfrastructureFailure("Inspections MCP did not become ready in 20 minutes; see $log")
    }
    override fun close() { try { client?.close() } finally { stop(process) } }
}
