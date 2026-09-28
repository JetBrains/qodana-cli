// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict

import io.modelcontextprotocol.kotlin.sdk.server.StdioServerTransport
import kotlinx.coroutines.Job
import kotlinx.coroutines.runBlocking
import kotlinx.io.asSink
import kotlinx.io.asSource
import kotlinx.io.buffered
import org.jetbrains.qodana.edict.edictnext.EdictNextMcpToolset
import org.jetbrains.qodana.edict.edictnext.EdictNextWorkspace
import org.jetbrains.qodana.edict.edictnext.EdictSessionContext
import org.jetbrains.qodana.edict.mcp.McpServer
import org.jetbrains.qodana.edict.skills.Skills
import org.jetbrains.qodana.edict.store.Store
import java.nio.file.Path
import java.util.concurrent.CountDownLatch
import java.util.UUID
import kotlin.system.exitProcess

fun main(args: Array<String>) {
    try {
        val command = args.firstOrNull() ?: "help"
        val options = args.drop(1).chunked(2).associate { pair ->
            require(pair.size == 2 && pair[0].startsWith("--")) { "Options require --name value" }
            pair[0].removePrefix("--") to pair[1]
        }
        when (command) {
            "install-skills" -> {
                require(options.keys.all { it in listOf("directory", "skill") }) { "Unknown install option" }
                val destination = options["directory"] ?: error("Use install-skills --directory <skills-directory>")
                Skills.install(Path.of(destination), options["skill"]).forEach(::println)
            }

            "mcp", "edict-mcp" -> {
                require(options.keys.all {
                    it in listOf(
                        "project-dir",
                        "state-dir",
                        "http-port",
                        "log-dir",
                        "embedding-python"
                    )
                }) { "Unknown MCP option" }
                val project = Path.of(options["project-dir"] ?: ".").toAbsolutePath().normalize()
                Store(options["state-dir"]?.let(Path::of) ?: project.resolve(".edict")).use { store ->
                    val hook = Thread { store.close() }
                    Runtime.getRuntime().addShutdownHook(hook)
                    try {
                        val server = McpServer(
                            store,
                            logs = (options["log-dir"]?.let(Path::of) ?: project.resolve("log")).resolve("edict"),
                            embeddingPython = options["embedding-python"]?.let(Path::of),
                        )
                        if ("http-port" in options) server.serveHttp(options.getValue("http-port").toInt())
                            .use { transport ->
                                System.err.println("edict-mcp listening at ${transport.url}")
                                CountDownLatch(1).await()
                            } else server.serveStdio()
                    } finally {
                        Runtime.getRuntime().removeShutdownHook(hook)
                    }
                }
            }

            "edict-mcp-next" -> {
                require(options.keys.all {
                    it in listOf("project-dir", "state-dir", "source-repository", "log-dir", "qodana-executable")
                }) { "Unknown Edict Next option" }
                val project = Path.of(options["project-dir"] ?: error("Use --project-dir <analyzed-project>"))
                    .toAbsolutePath().normalize()
                val state = Path.of(options["state-dir"] ?: error("Use --state-dir <edict-state>"))
                    .toAbsolutePath().normalize()
                val sourceRepository = Path.of(options["source-repository"] ?: state.toString()).toAbsolutePath().normalize()
                val logs = Path.of(options["log-dir"] ?: project.resolve("log").toString()).toAbsolutePath().normalize()
                val qodanaExecutable = options["qodana-executable"]
                    ?: System.getProperty("qodana.executable")
                    ?: System.getenv("QODANA_EXECUTABLE")
                    ?: "qodana"
                val sessionId = UUID.randomUUID().toString()
                runBlocking {
                    val context = EdictSessionContext.getInstance(sessionId)
                    context.load(EdictNextWorkspace.forRun(logs, sessionId), sourceRepository, project, qodanaExecutable)
                    try {
                        Store(state).use { store ->
                            val managedServer = McpServer(store, logs = logs.resolve("edict"))
                            val server = EdictNextMcpToolset(sessionId, managedServer).createServer()
                            val transport = StdioServerTransport(
                                input = System.`in`.asSource().buffered(),
                                output = System.out.asSink().buffered(),
                            )
                            val session = server.createSession(transport)
                            val closed = Job()
                            session.onClose { closed.complete() }
                            closed.join()
                        }
                    } finally {
                        context.unload()
                    }
                }
            }

            "help", "--help", "-h" -> println(
                """
                Edict managed skills (standalone Kotlin/JVM)
                  edict install-skills --directory <skills-directory> [--skill <name>]
                  edict mcp [--project-dir <project>] [--state-dir <state>] [--log-dir <logs>] [--embedding-python <python>] [--http-port <port>]
                  edict edict-mcp-next --project-dir <project> --state-dir <state> [--source-repository <repository>] [--log-dir <logs>] [--qodana-executable <qodana>]
                MCP uses stdio by default. HTTP binds to loopback and shares one store across workers.
            """.trimIndent()
            )

            else -> error("Unknown command '$command'; use --help")
        }
    } catch (e: Exception) {
        System.err.println("edict: ${e.message}"); exitProcess(1)
    }
}
