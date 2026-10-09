// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict

import io.github.oshai.kotlinlogging.KotlinLogging
import org.jetbrains.qodana.edict.common.EdictLayout
import org.jetbrains.qodana.edict.common.EdictYamlConfiguration
import org.jetbrains.qodana.edict.edictnext.EdictNextMcpToolset
import org.jetbrains.qodana.edict.edictnext.IntellijMcpServerService
import org.jetbrains.qodana.edict.edictnext.defaultQodanaExecutable
import org.jetbrains.qodana.edict.setup.CodexSetup
import org.jetbrains.qodana.edict.setup.EdictConfig
import java.nio.file.Path
import kotlin.system.exitProcess

// Lazy, so main() turns off the kotlin-logging startup banner before the first logger is created.
private val logger by lazy { KotlinLogging.logger("org.jetbrains.qodana.edict.Main") }

fun main(args: Array<String>) {
    // kotlin-logging 8 otherwise prints a startup banner to stdout.
    System.setProperty("kotlin-logging.logStartupMessage", "false")
    // Before the first logger: Logback writes into the folder this fixes, like every EdictLayout of the process.
    EdictLayout.processRunId()
    try {
        val command = args.firstOrNull() ?: "help"
        val optionPairs = args.drop(1).chunked(2).map { pair ->
            require(pair.size == 2 && pair[0].startsWith("--")) { "Options require --name value" }
            pair[0].removePrefix("--") to pair[1]
        }
        val options = optionPairs.toMap()
        when (command) {
            "install" -> {
                require(options.keys.all { it == "deny" }) { "Unknown install option" }
                val configuration = EdictYamlConfiguration.load(EdictLayout.findQodanaYaml())
                val layout = EdictLayout.get(configuration.statePath)
                val denied = optionPairs.map { Path.of(it.second) }
                CodexSetup.install(layout, denied, EdictConfig(configuration.mcpPort))
            }

            "mcp" -> {
                require(options.keys.all {
                    it in listOf(
                        "qodana-executable",
                        "ide-dist",
                        "ide-linter",
                        "ide-property",
                        "ide-wait-timeout",
                        "parent-pid",
                        "qodana-yaml",
                    )
                }) { "Unknown Edict Next option" }
                val qodanaYaml = options["qodana-yaml"]?.let(Path::of)?.toAbsolutePath()?.normalize()
                    ?: EdictLayout.findQodanaYaml()
                val configuration = EdictYamlConfiguration.load(qodanaYaml)
                val layout = EdictLayout.get(configuration.statePath)
                val portConfig = EdictConfig(configuration.mcpPort).also { CodexSetup.requireInstalled(layout, it) }
                val port = portConfig.mcpPort
                // `=` keeps values such as -Xmx8g from being parsed as flags by the Go helper.
                val ideArguments = buildList {
                    options["ide-dist"]?.let { add("--dist=$it") }
                    options["ide-linter"]?.let { add("--linter=$it") }
                    optionPairs.filter { it.first == "ide-property" }.forEach { add("--property=${it.second}") }
                    options["ide-wait-timeout"]?.let { add("--wait-timeout=$it") }
                }
                val inspectionServer = IntellijMcpServerService(
                    layout.root,
                    options["qodana-executable"] ?: defaultQodanaExecutable(),
                    ideArguments,
                    layout.intellijMcpLogPath,
                    layout.intellijMcpResultsDirectory,
                )
                EdictServer.start(layout, port, inspectionServer, configuration = configuration).use { server ->
                    Runtime.getRuntime().addShutdownHook(Thread(server::close))
                    // A launcher killed with SIGKILL cannot stop this server, and stdin may stay open (HTTP mode never
                    // reads it). Exit with the launcher instead; the shutdown hook then stops the server.
                    options["parent-pid"]?.let { pid ->
                        val parent = ProcessHandle.of(pid.toLong())
                        if (parent.isEmpty) exitProcess(0)
                        parent.get().onExit().thenRun { exitProcess(0) }
                    }
                    logger.info { "Process log: ${layout.processLogDirectory}" }
                    logger.info { "${EdictNextMcpToolset.SERVER_NAME} listening at ${server.url}" }
                    server.await()
                }
            }

            "help", "--help", "-h" -> println(
                """
                Edict managed skills (standalone Kotlin/JVM). Run every command in the project directory.
                  edict install [--deny <path>]...
                    Installs skills into .codex/skills and writes .codex/config.toml for edict.mcpPort from qodana.yaml.
                  edict mcp [--qodana-yaml <path>] [--qodana-executable <qodana>] [--ide-dist <path> | --ide-linter <linter>] [--ide-property <property>]... [--ide-wait-timeout <duration>] [--parent-pid <pid>]
                    Serves HTTP on loopback at edict.mcpPort (default ${EdictConfig.DEFAULT_MCP_PORT}); state comes from edict.statePath (default .edict).
                    edict.calculatePrice=true enables the manager-only price-report MCP tool and private run report.
                    Each run logs to log/process-log/<run-id> and gives agents log/agent-work/<run-id>; the run id is its start time.
                The first inspection call starts the IDE through `qodana edict ide-mcp`; its output goes to intellij-mcp.log.
            """.trimIndent()
            )

            else -> error("Unknown command '$command'; use --help")
        }
    } catch (e: Exception) {
        logger.error(e) { e.message ?: e.toString() }
        exitProcess(1)
    }
}
