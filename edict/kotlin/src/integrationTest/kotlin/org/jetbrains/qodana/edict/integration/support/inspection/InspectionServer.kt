// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.integration.support.inspection

import kotlinx.serialization.json.*
import org.jetbrains.qodana.edict.integration.support.IntegrationWorkspace
import java.net.URI
import java.nio.file.Files
import java.nio.file.Path
import java.util.concurrent.TimeUnit

/** The compiler lives in a separate IDE process; the Kotlin project has no IntelliJ dependencies. */
internal class InspectionServer private constructor(
    private val process: Process,
    private val client: InspectionMcpClient,
    private val source: Path,
    private val proxy: InspectionToolProxy,
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
        fun start(workspace: IntegrationWorkspace): InspectionServer {
            val ultimate = System.getenv("ULTIMATE_EDICT_REPO")?.takeIf(String::isNotBlank)?.let(Path::of)
                ?: error("Set ULTIMATE_EDICT_REPO to the Ultimate checkout providing the external inspection MCP compiler")
            require(Files.isRegularFile(ultimate.resolve("bazel.cmd"))) { "Missing bazel.cmd in ULTIMATE_EDICT_REPO" }
            val root = workspace.output
            val source = root.resolve("inspection-project")
            Files.walk(workspace.project).use { paths ->
                paths.forEach { path ->
                    val relative = workspace.project.relativize(path)
                    if (relative.firstOrNull()?.toString() != ".edict") {
                        val target = source.resolve(relative)
                        if (Files.isDirectory(path)) Files.createDirectories(target) else Files.copy(path, target)
                    }
                }
            }
            val log = root.resolve("log/intellij-mcp.log")
            Files.createDirectories(log.parent)
            val process = ProcessBuilder(
                "/bin/sh",
                "./bazel.cmd",
                "run",
                "//build:mcp_server",
                "--",
                "--jvm_flag=-Didea.config.path=${root.resolve("intellij/config")}",
                "--jvm_flag=-Didea.system.path=${root.resolve("intellij/system")}",
                "--jvm_flag=-Didea.log.path=${root.resolve("log/intellij")}",
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
                val endpoint = Regex("(?:Streamable HTTP endpoint|SSE URL):\\s*(https?://[^\\s\\u001b]+)")
                val deadline = System.nanoTime() + TimeUnit.MINUTES.toNanos(10)
                var url: String? = null
                while (url == null) {
                    check(process.isAlive) { "External inspection compiler exited ${process.exitValue()}; see $log" }
                    check(System.nanoTime() < deadline) { "External inspection compiler did not start in 10 minutes; see $log" }
                    url = endpoint.find(Files.readString(log))?.groupValues?.get(1)
                    if (url == null) Thread.sleep(250)
                }
                client = InspectionMcpClient(URI.create(url))
                val connected = client
                val listed = connected.request("tools/list", JsonObject(emptyMap())).getValue("tools").jsonArray
                proxy = InspectionToolProxy(
                    listed.map { it.jsonObject },
                    source,
                    root.resolve("log/inspection-mcp.jsonl"),
                    connected::call
                )
                return InspectionServer(process, connected, source, proxy)
            } catch (e: Throwable) {
                proxy?.close(); runCatching { client?.close() }; stopIde(process); throw e
            }
        }

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
