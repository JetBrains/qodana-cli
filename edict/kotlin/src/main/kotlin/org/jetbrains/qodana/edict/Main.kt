// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict

import io.github.oshai.kotlinlogging.KotlinLogging
import org.jetbrains.qodana.edict.ci.getReviewAPI
import org.jetbrains.qodana.edict.common.EdictLayout
import org.jetbrains.qodana.edict.common.DEFAULT_EDICT_STATE_BRANCH
import org.jetbrains.qodana.edict.common.EdictYamlConfiguration
import org.jetbrains.qodana.edict.common.StateBranch
import org.jetbrains.qodana.edict.edictnext.EdictNextMcpToolset
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState
import org.jetbrains.qodana.edict.edictnext.IntellijMcpServerService
import org.jetbrains.qodana.edict.edictnext.defaultQodanaExecutable
import org.jetbrains.qodana.edict.feedback.actionsEventPath
import org.jetbrains.qodana.edict.feedback.actionsFeedbackApi
import org.jetbrains.qodana.edict.feedback.actionsPullRequest
import org.jetbrains.qodana.edict.feedback.postFeedback
import org.jetbrains.qodana.edict.feedback.prepareFeedback
import org.jetbrains.qodana.edict.feedback.serverFeedbackApi
import org.jetbrains.qodana.edict.setup.CodexSetup
import org.jetbrains.qodana.edict.setup.EdictConfig
import java.nio.file.Files
import java.nio.file.Path
import java.time.Clock
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
                require(options.keys.all { it in listOf("deny", "qodana-yaml") }) { "Unknown install option" }
                install(qodanaYaml(options), optionPairs.filter { it.first == "deny" }.map { Path.of(it.second) })
            }

            "state-push" -> {
                require(options.keys.all { it in listOf("message", "qodana-yaml") }) { "Unknown state-push option" }
                pushState(qodanaYaml(options), options["message"] ?: "Update the Edict state")
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
                val configuration = EdictYamlConfiguration.load(qodanaYaml(options))
                val ciReviews = configuration.ci?.let { getReviewAPI(it, configuration.bots) }
                val promotionReviews = configuration.promotionRepository?.let { getReviewAPI(it, configuration.bots) }
                val feedbackApi = configuration.ci?.let(::serverFeedbackApi)
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
                    projectAnalysisTimeoutMinutes = configuration.projectAnalysisTimeoutMinutes,
                )
                EdictServer.start(layout, port, inspectionServer, configuration, ciReviews, promotionReviews, feedbackApi).use { server ->
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

            "feedback-prepare" -> {
                require(options.keys.all { it in listOf("qodana-yaml", "state-dir", "output-dir") }) { "Unknown feedback-prepare option" }
                val stateDirectory = requireNotNull(options["state-dir"]) { "feedback-prepare requires --state-dir" }
                val outputDirectory = requireNotNull(options["output-dir"]) { "feedback-prepare requires --output-dir" }
                prepareFeedbackInspections(qodanaYaml(options), stateDirectory, Path.of(outputDirectory))
            }

            "feedback-post" -> {
                require(options.keys.all { it in listOf("qodana-yaml", "state-dir", "sarif") }) { "Unknown feedback-post option" }
                val stateDirectory = requireNotNull(options["state-dir"]) { "feedback-post requires --state-dir" }
                val sarif = requireNotNull(options["sarif"]) { "feedback-post requires --sarif" }
                postFeedbackComments(qodanaYaml(options), stateDirectory, Path.of(sarif))
            }

            "help", "--help", "-h" -> println(
                """
                Edict managed skills (standalone Kotlin/JVM). Run every command in the project directory.
                  edict install [--qodana-yaml <path>] [--deny <path>]...
                    Installs skills into .codex/skills and adds Edict settings for edict.mcpPort from qodana.yaml to .codex/config.toml.
                    Checks out edict.stateBranch (default $DEFAULT_EDICT_STATE_BRANCH) as a worktree at edict.statePath, from origin when it has the branch.
                  edict state-push [--qodana-yaml <path>] [--message <message>]
                    Commits the state on edict.stateBranch and pushes it to origin; fails while a server owns the state.
                  edict feedback-prepare --state-dir <path> --output-dir <path> [--qodana-yaml <path>]
                    For a GitHub Actions run of a pull request of the repository itself: checks edict.stateBranch out at
                    --state-dir and copies the inspections to run for feedback into --output-dir, outside the project and missing or empty.
                  edict feedback-post --state-dir <path> --sarif <path> [--qodana-yaml <path>]
                    After the scan of that run: reserves the findings of those inspections in the SARIF report on lines the pull request
                    adds, pushing them to edict.stateBranch from --state-dir, and posts them as review comments at the commit it analysed.
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

/** The Qodana configuration that `--qodana-yaml` names, else the one in the working directory, if any. */
private fun qodanaYaml(options: Map<String, String>): Path? =
    options["qodana-yaml"]?.let(Path::of)?.toAbsolutePath()?.normalize() ?: EdictLayout.findQodanaYaml()

/** Sets Codex up for the project in the working directory and checks the state branch out. */
private fun install(qodanaYaml: Path?, deniedPaths: List<Path>) {
    val configuration = EdictYamlConfiguration.load(qodanaYaml)
    val layout = EdictLayout.get(configuration.statePath)
    CodexSetup.install(layout, deniedPaths, EdictConfig(configuration.mcpPort))
    val stateBranch = StateBranch(layout, configuration.stateBranch)
    val checkout = stateBranch.checkOut()
    logger.info { stateBranch.describe(checkout) }
}

/** Copies the inspections the workflow's pull request runs for feedback into [outputDirectory]. */
private fun prepareFeedbackInspections(qodanaYaml: Path?, stateDirectory: String, outputDirectory: Path) {
    logOutsideTheProject()
    val configuration = EdictYamlConfiguration.load(qodanaYaml)
    val event = actionsPullRequest(actionsEventPath())
    prepareFeedback(EdictLayout.get(stateDirectory), configuration.stateBranch, event, outputDirectory)
}

/** Comments on the workflow's pull request with the feedback findings of the [sarif] report, reserved in the state first. */
private fun postFeedbackComments(qodanaYaml: Path?, stateDirectory: String, sarif: Path) {
    logOutsideTheProject()
    val configuration = EdictYamlConfiguration.load(qodanaYaml)
    val event = actionsPullRequest(actionsEventPath())
    postFeedback(EdictLayout.get(stateDirectory), configuration.stateBranch, event, sarif, actionsFeedbackApi(), Clock.systemUTC())
}

/**
 * Moves the log files of a command that runs in a CI checkout of the project, which Qodana analyses and its quick fixes may
 * commit, to a temporary directory; only before the first logger, which fixes the folder Logback writes to.
 */
private fun logOutsideTheProject() {
    if (System.getProperty(EdictLayout.LOG_DIRECTORY_PROPERTY) == null) {
        System.setProperty(EdictLayout.LOG_DIRECTORY_PROPERTY, Files.createTempDirectory("edict-log-").toString())
    }
}

private fun pushState(qodanaYaml: Path?, message: String) {
    val configuration = EdictYamlConfiguration.load(qodanaYaml)
    val layout = EdictLayout.get(configuration.statePath)
    // Before the lock, which creates files in the state directory that would keep install from checking it out.
    val worktree = StateBranch(layout, configuration.stateBranch).worktree()
    // Holding the state lock keeps a running server from writing while the state is committed.
    EdictNextRepositoryState.open(layout.stateDirectory).use { worktree.push(message) }
    logger.info { "Pushed the Edict state in ${layout.stateDirectory} to ${configuration.stateBranch}" }
}

