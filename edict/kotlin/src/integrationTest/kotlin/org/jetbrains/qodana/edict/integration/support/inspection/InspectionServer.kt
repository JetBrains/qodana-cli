// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.integration.support.inspection

import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.*
import org.jetbrains.qodana.edict.common.flag
import org.jetbrains.qodana.edict.common.text
import org.jetbrains.qodana.edict.common.wireJson
import org.jetbrains.qodana.edict.core.HttpInspectionKtsClient
import org.jetbrains.qodana.edict.core.QodanaIntellijMcpServerLifecycle
import java.net.URI
import java.nio.file.Files
import java.nio.file.Path

internal const val liveQodanaLinter = "qodana-jvm"

/** The compiler lives in a separate IDE process; the Kotlin project has no IntelliJ dependencies. */
internal class InspectionServer private constructor(
    private val lifecycle: QodanaIntellijMcpServerLifecycle,
    private val client: HttpInspectionKtsClient,
    /** The project the IDE opened: the fixture project without its Edict state. */
    val source: Path,
    private val proxy: InspectionToolProxy,
    /** The IDE's own Streamable HTTP endpoint, which production Edict talks to directly, unlike [url]. */
    val streamableEndpoint: URI,
    val tools: List<JsonObject>,
) : AutoCloseable {
    val url = proxy.url

    fun run(code: String, path: String, content: String): InspectionResult = runBlocking {
        decodeInspectionResult(client.callTool("run_inspection_kts", buildJsonObject {
            put("inspectionKtsCode", code)
            put("contextPath", path)
            put("targetFileContent", content)
            put("projectPath", source.toString())
        })).also { check(it.compilationSuccess) { "Inspection compilation failed: ${it.compilationStatus}" } }
    }

    override fun close() {
        try {
            proxy.close()
        } finally {
            try {
                client.close()
            } finally {
                runBlocking { lifecycle.stop() }
            }
        }
    }

    companion object {
        /** Opens a copy of the Java fixture, without its Edict state, in an IDE whose files live under [output]. */
        fun start(qodanaCli: Path, project: Path, output: Path): InspectionServer {
            val source = output.resolve("inspection-project")
            Files.walk(project).use { paths ->
                paths.forEach { path ->
                    val relative = project.relativize(path)
                    if (relative.firstOrNull()?.toString() != ".edict") {
                        val target = source.resolve(relative)
                        if (Files.isDirectory(path)) Files.createDirectories(target) else Files.copy(path, target)
                    }
                }
            }
            val log = output.resolve("log/intellij-mcp.log")
            val lifecycle = QodanaIntellijMcpServerLifecycle(
                source,
                qodanaCli.toString(),
                ideArguments = listOf(
                    "--linter=$liveQodanaLinter",
                    "--wait-timeout=10m",
                    "--property=qodana.application=false",
                    "--property=jdk.configure.existing=true",
                    // SDK registration in Qodana JVM 263.7890 requires the event dispatch thread (IDEA-389470).
                    "--property=idea.background.write.action.enabled=false",
                ),
                log = log,
                resultsDirectory = output.resolve("intellij-mcp-results"),
            )
            println("Starting Qodana IntelliJ MCP; log: $log")
            val endpoint = runBlocking { lifecycle.start() }
            var client: HttpInspectionKtsClient? = null
            try {
                val connected = HttpInspectionKtsClient(endpoint, source.toString())
                client = connected
                val tools = runBlocking { connected.listTools() }
                val proxy = InspectionToolProxy(
                    tools,
                    source,
                    output.resolve("log/inspection-mcp.jsonl"),
                ) { name, arguments -> runBlocking { connected.callTool(name, arguments) } }
                return InspectionServer(lifecycle, connected, source, proxy, endpoint, tools)
            } catch (e: Throwable) {
                runCatching { client?.close() }.exceptionOrNull()?.let(e::addSuppressed)
                runCatching { runBlocking { lifecycle.stop() } }.exceptionOrNull()?.let(e::addSuppressed)
                throw e
            }
        }
    }
}

internal data class InspectionResult(
    val compilationSuccess: Boolean,
    val compilationStatus: String,
    val problemLines: List<Int>
)

internal fun decodeInspectionResult(result: JsonObject): InspectionResult {
    check(result.flag("isError") != true) { "Inspection MCP returned an error: $result" }
    val data = (result["structuredContent"] as? JsonObject) ?: (result["content"] as? JsonArray).orEmpty()
        .firstNotNullOfOrNull { item -> runCatching { wireJson.parseToJsonElement(item.jsonObject.text("text")).jsonObject }.getOrNull() }
    ?: error("Inspection result has no JSON content")
    return InspectionResult(
        data.flag("compilationSuccess") == true, data.text("compilationStatus"),
        (data["foundProblems"] as? JsonArray).orEmpty().map { it.jsonObject.getValue("lineNumber").jsonPrimitive.int })
}
