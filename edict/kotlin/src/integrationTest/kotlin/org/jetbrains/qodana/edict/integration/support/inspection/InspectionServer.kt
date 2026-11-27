// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.integration.support.inspection

import kotlinx.serialization.json.*
import org.jetbrains.qodana.edict.core.IntellijMcpEndpoint
import java.net.URI
import java.nio.file.Files
import java.nio.file.Path
import java.util.concurrent.TimeUnit
import kotlin.io.path.readText

/** The compiler lives in a separate IDE process; the Kotlin project has no IntelliJ dependencies. */
internal class InspectionServer private constructor(
    private val process: Process,
    private val client: InspectionMcpClient,
    /** The project the IDE opened: the fixture project without its Edict state. */
    val source: Path,
    private val proxy: InspectionToolProxy,
    /** The IDE's own Streamable HTTP endpoint, which production Edict talks to directly, unlike [url]. */
    val endpoint: IntellijMcpEndpoint,
) : AutoCloseable {
    val url = proxy.url

    fun run(code: String, path: String, content: String): InspectionResult =
        decodeInspectionResult(client.call("run_inspection_kts", buildJsonObject {
            put("inspectionKtsCode", code)
            put("contextPath", path)
            put("targetFileContent", content)
            put("projectPath", source.toString())
        })).also { check(it.compilationSuccess) { "Inspection compilation failed: ${it.compilationStatus}" } }

    override fun close() {
        proxy.close()
        try {
            client.close()
        } finally {
            stopIde(process)
        }
    }

    companion object {
        /** Opens a copy of [project], without its Edict state, in an IDE whose files live under [output]. */
        fun start(project: Path, output: Path, prepareSource: (Path) -> Unit = {}): InspectionServer {
            val ultimate = System.getenv("ULTIMATE_EDICT_REPO")?.takeIf(String::isNotBlank)?.let(Path::of)
                ?: error("Set ULTIMATE_EDICT_REPO to the Ultimate checkout providing the external inspection MCP compiler")
            require(Files.isRegularFile(ultimate.resolve("bazel.cmd"))) { "Missing bazel.cmd in ULTIMATE_EDICT_REPO" }
            val root = output
            val source = root.resolve("inspection-project")
            Files.walk(project).use { paths ->
                paths.forEach { path ->
                    val relative = project.relativize(path)
                    if (relative.firstOrNull()?.toString() != ".edict") {
                        val target = source.resolve(relative)
                        if (Files.isDirectory(path)) Files.createDirectories(target) else Files.copy(path, target)
                    }
                }
            }
            prepareSource(source)
            val log = root.resolve("log/intellij-mcp.log")
            Files.createDirectories(log.parent)
            val process = ProcessBuilder(
                "/bin/sh",
                "./bazel.cmd",
                "run",
                "//build:mcp_server",
                "--",
                "mcpServer",
                "--project=$source",
                "--invocation-mode=direct",
                "--allowed-tools=${inspectionToolNames.joinToString(",")}"
            )
                .directory(ultimate.toFile()).redirectErrorStream(true).redirectOutput(log.toFile()).start()
            process.outputStream.close()
            var client: InspectionMcpClient? = null
            var proxy: InspectionToolProxy? = null
            try {
                println("Starting external inspection compiler; log: $log")
                // The endpoint, and the complete line after it, which names the token file as `qodana edict ide-mcp` reads it.
                val announcement = Regex("(?:Streamable HTTP endpoint|SSE URL):\\s*(https?://[^\\s\\u001b]+)[^\\n]*\\n([^\\n]*)\\n")
                val deadline = System.nanoTime() + TimeUnit.MINUTES.toNanos(10)
                var announced: MatchResult? = null
                while (announced == null) {
                    check(process.isAlive) { "External inspection compiler exited ${process.exitValue()}; see $log" }
                    check(System.nanoTime() < deadline) { "External inspection compiler did not start in 10 minutes; see $log" }
                    announced = announcement.find(Files.readString(log))
                    if (announced == null) Thread.sleep(250)
                }
                val url = announced.groupValues[1]
                val authToken = authTokenFile.find(announced.groupValues[2].trim())?.let { Path.of(it.groupValues[1]).readText().trim() }
                client = InspectionMcpClient(URI.create(url), authToken)
                val connected = client
                val listed = connected.request("tools/list", JsonObject(emptyMap())).getValue("tools").jsonArray
                proxy = InspectionToolProxy(
                    listed.map { it.jsonObject },
                    source,
                    root.resolve("log/inspection-mcp.jsonl"),
                    connected::call
                )
                // The same derivation as `qodana edict ide-mcp`, whose endpoint production Edict uses.
                val streamable = URI.create(if (url.endsWith("/sse")) url.removeSuffix("/sse") + "/stream" else url)
                return InspectionServer(process, connected, source, proxy, IntellijMcpEndpoint(streamable, authToken))
            } catch (e: Throwable) {
                proxy?.close(); runCatching { client?.close() }; stopIde(process); throw e
            }
        }

        private val authTokenFile = Regex("IJ_MCP_AUTH_TOKEN with the token in (.+?)(?:\\u001b\\[[0-9;]*m)?$")

        private fun stopIde(process: Process) {
            if (!process.isAlive) return
            // Bazel forwards SIGINT to its IDE subprocess; terminating only the client leaks the IDE.
            runCatching {
                ProcessBuilder("/bin/kill", "-INT", process.pid().toString()).start().waitFor(5, TimeUnit.SECONDS)
            }
            if (!process.waitFor(15, TimeUnit.SECONDS)) {
                process.descendants().forEach { it.destroyForcibly() }
                process.destroyForcibly()
                process.waitFor(5, TimeUnit.SECONDS)
            }
        }
    }
}
