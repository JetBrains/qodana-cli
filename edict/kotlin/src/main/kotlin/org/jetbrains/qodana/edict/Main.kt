// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict

import io.modelcontextprotocol.kotlin.sdk.server.StdioServerTransport
import io.modelcontextprotocol.kotlin.sdk.server.mcpStreamableHttp
import io.ktor.server.cio.CIO
import io.ktor.server.engine.embeddedServer
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.runBlocking
import kotlinx.io.asSink
import kotlinx.io.asSource
import kotlinx.io.buffered
import org.jetbrains.qodana.edict.edictnext.EdictManagementService
import org.jetbrains.qodana.edict.edictnext.EdictNextMcpToolset
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState
import org.jetbrains.qodana.edict.edictnext.EdictNextWorkspace
import org.jetbrains.qodana.edict.edictnext.EdictPrAnalysisService
import org.jetbrains.qodana.edict.edictnext.EdictSessionContext
import org.jetbrains.qodana.edict.edictnext.IntellijMcpServerService
import org.jetbrains.qodana.edict.reviews.ReviewClient
import org.jetbrains.qodana.edict.skills.Skills
import java.nio.file.Path
import java.util.UUID
import java.util.concurrent.atomic.AtomicBoolean
import kotlin.system.exitProcess

fun main(args: Array<String>) {
    // Stdout is the MCP wire for stdio commands; kotlin-logging 8 otherwise prints a startup banner there.
    System.setProperty("kotlin-logging.logStartupMessage", "false")
    try {
        val command = args.firstOrNull() ?: "help"
        val optionPairs = args.drop(1).chunked(2).map { pair ->
            require(pair.size == 2 && pair[0].startsWith("--")) { "Options require --name value" }
            pair[0].removePrefix("--") to pair[1]
        }
        val options = optionPairs.toMap()
        when (command) {
            "install-skills" -> {
                require(options.keys.all { it in listOf("directory", "skill") }) { "Unknown install option" }
                val destination = options["directory"] ?: error("Use install-skills --directory <skills-directory>")
                Skills.install(Path.of(destination), options["skill"]).forEach(::println)
            }

            "mcp", "edict-mcp", "edict-mcp-next" -> {
                require(options.keys.all {
                    it in listOf(
                        "project-dir",
                        "state-dir",
                        "source-repository",
                        "log-dir",
                        "qodana-executable",
                        "http-port",
                        "embedding-python",
                        "ide-dist",
                        "ide-linter",
                        "ide-property",
                        "ide-wait-timeout",
                        "parent-pid",
                    )
                }) { "Unknown Edict Next option" }
                val project = Path.of(options["project-dir"] ?: ".")
                    .toAbsolutePath().normalize()
                val state = Path.of(options["state-dir"] ?: project.resolve(".edict").toString())
                    .toAbsolutePath().normalize()
                val sourceRepository = Path.of(options["source-repository"] ?: project.toString()).toAbsolutePath().normalize()
                val logs = Path.of(options["log-dir"] ?: project.resolve("log").toString()).toAbsolutePath().normalize()
                val qodanaExecutable = options["qodana-executable"]
                    ?: System.getProperty("qodana.executable")
                    ?: System.getenv("QODANA_EXECUTABLE")
                    ?: "qodana"
                val embeddingPython = options["embedding-python"]?.let(Path::of)?.toAbsolutePath()?.normalize()
                val sessionId = UUID.randomUUID().toString()
                runBlocking {
                    val context = EdictSessionContext.getInstance(sessionId)
                    // `=` keeps values such as -Xmx8g from being parsed as flags by the Go helper.
                    val ideArguments = buildList {
                        options["ide-dist"]?.let { add("--dist=$it") }
                        options["ide-linter"]?.let { add("--linter=$it") }
                        optionPairs.filter { it.first == "ide-property" }.forEach { add("--property=${it.second}") }
                        options["ide-wait-timeout"]?.let { add("--wait-timeout=$it") }
                    }
                    context.load(
                        EdictNextWorkspace.forRun(logs, sessionId),
                        sourceRepository,
                        project,
                        qodanaExecutable,
                        embeddingPython,
                        IntellijMcpServerService(project, qodanaExecutable, ideArguments, logs.resolve("edict/intellij-mcp.log")),
                    )
                    val unloaded = AtomicBoolean()
                    suspend fun unloadOnce() {
                        if (unloaded.compareAndSet(false, true)) context.unload()
                    }
                    val hook = Thread { runBlocking { unloadOnce() } }
                    Runtime.getRuntime().addShutdownHook(hook)
                    // A launcher killed with SIGKILL cannot stop this server, and stdin may stay open (HTTP mode never
                    // reads it). Exit with the launcher instead; the shutdown hook then stops the IDE helper.
                    options["parent-pid"]?.let { pid ->
                        val parent = ProcessHandle.of(pid.toLong())
                        if (parent.isEmpty) exitProcess(0)
                        parent.get().onExit().thenRun { exitProcess(0) }
                    }
                    try {
                        EdictNextRepositoryState.open(state).use { store ->
                            val management = EdictManagementService(
                                store, logs.resolve("edict"), extensions = listOf(EdictPrAnalysisService(store, ReviewClient())),
                            )
                            val toolset = EdictNextMcpToolset(sessionId, management)
                            val httpPort = options["http-port"]?.toInt()
                            if (httpPort != null) {
                                require(httpPort in 0..65535) { "HTTP port must be between 0 and 65535" }
                                val engine = embeddedServer(CIO, host = "127.0.0.1", port = httpPort) {
                                    mcpStreamableHttp { toolset.createServer() }
                                }
                                engine.start(wait = false)
                                val actualPort = engine.engine.resolvedConnectors().single().port
                                val serverName = if (command == "edict-mcp-next") "edict-mcp-next" else "edict-mcp"
                                System.err.println("$serverName listening at http://127.0.0.1:$actualPort/mcp")
                                CompletableDeferred<Unit>().await()
                            } else {
                                val server = toolset.createServer()
                                val transport = StdioServerTransport(
                                    input = System.`in`.asSource().buffered(),
                                    output = System.out.asSink().buffered(),
                                )
                                val closed = CompletableDeferred<Unit>()
                                transport.onClose { closed.complete(Unit) }
                                server.createSession(transport)
                                closed.await()
                            }
                        }
                    } finally {
                        unloadOnce()
                        try {
                            Runtime.getRuntime().removeShutdownHook(hook)
                        } catch (_: IllegalStateException) {
                            // JVM shutdown already started and is running the hook.
                        }
                    }
                }
            }

            "help", "--help", "-h" -> println(
                """
                Edict managed skills (standalone Kotlin/JVM)
                  edict install-skills --directory <skills-directory> [--skill <name>]
                  edict edict-mcp-next --project-dir <project> --state-dir <state> [--source-repository <repository>] [--log-dir <logs>] [--qodana-executable <qodana>] [--http-port <port>] [--ide-dist <path> | --ide-linter <linter>] [--ide-property <property>]... [--ide-wait-timeout <duration>] [--parent-pid <pid>]
                  edict mcp is a compatibility alias for edict-mcp-next.
                Edict Next MCP uses stdio by default; HTTP binds to loopback and shares one store across workers.
                The first inspection call starts the IDE through `qodana edict ide-mcp`; output goes to <logs>/edict/intellij-mcp.log.
            """.trimIndent()
            )

            else -> error("Unknown command '$command'; use --help")
        }
    } catch (e: Exception) {
        System.err.println("edict: ${e.message}"); exitProcess(1)
    }
}
