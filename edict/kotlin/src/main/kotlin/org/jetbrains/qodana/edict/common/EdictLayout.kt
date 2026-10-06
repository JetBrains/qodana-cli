// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.common

import java.nio.file.Path
import java.time.Instant
import java.time.ZoneOffset
import java.time.format.DateTimeFormatter
import kotlin.io.path.isRegularFile

/**
 * Every path Edict uses for the project in the working directory, the one place that names them: the Edict state
 * repository (`.edict` unless `--state-dir` moves it), the logs (`log` unless the [LOG_DIRECTORY_PROPERTY] system
 * property moves them) and the local Codex setup. The layout inside the state repository belongs to its own classes.
 * Each process keeps its logs and its agents' work in folders named after its [runId].
 */
internal data class EdictLayout(
  val root: Path,
  val stateDirectory: Path = root.resolve(".edict"),
  val logDirectory: Path = root.resolve(System.getProperty(LOG_DIRECTORY_PROPERTY) ?: "log").normalize(),
  val runId: String = processRunId(),
) {
  /** Every run's server logs and task records; they carry capability tokens, so agents must not read them. */
  val processLogRoot: Path = logDirectory.resolve("process-log")
  val processLogDirectory: Path = processLogRoot.resolve(runId)

  val mcpLogPath: Path = processLogDirectory.resolve("edict-mcp.log")
  val mcpSystemLogPath: Path = processLogDirectory.resolve("edict-mcp-system.log")
  val agentsLogPath: Path = processLogDirectory.resolve("edict-agents.log")
  val agentShortLogPath: Path = processLogDirectory.resolve("edict-agent-short.log")
  val tasksLogPath: Path = processLogDirectory.resolve("edict-tasks.log")
  val taskLogDirectory: Path = processLogDirectory.resolve("tasks")
  val intellijMcpLogPath: Path = processLogDirectory.resolve("intellij-mcp.log")

  /** Every run's agent work; the agent sandbox can write here. */
  val agentWorkRoot: Path = logDirectory.resolve("agent-work")
  val agentWorkDirectory: Path = agentWorkRoot.resolve(runId)

  /** Private agent scratch, writable from the agent sandbox unlike the state root. */
  val scratchDirectory: Path = agentWorkDirectory.resolve("scratch")
  val neighboursResponsePath: Path = agentWorkDirectory.resolve("neighbours.response.json")

  /** The Qodana configuration, found like the Qodana CLI does: `qodana.yml` first, then `qodana.yaml`. */
  val qodanaYamlPath: Path? get() = listOf("qodana.yml", "qodana.yaml").map(root::resolve).firstOrNull { it.isRegularFile() }

  val codexConfigPath: Path = root.resolve(".codex").resolve("config.toml")
  val codexSkillsDirectory: Path = root.resolve(".codex").resolve("skills")

  fun taskLogPath(taskId: String): Path = taskLogDirectory.resolve("$taskId.log")

  companion object {
    /** Moves the log folder; a relative value is resolved against the project root. */
    const val LOG_DIRECTORY_PROPERTY = "edict.log.dir"

    /** Holds [processRunId]; logback.xml names this process's log folder with it. */
    const val RUN_ID_PROPERTY = "edict.run.id"

    private val RUN_ID_FORMAT = DateTimeFormatter.ofPattern("yyyy-MM-dd'T'HH-mm-ss.SSS'Z'").withZone(ZoneOffset.UTC)

    /**
     * This process's run id, its start time in UTC unless [RUN_ID_PROPERTY] was given; fixed by the first call, which
     * main() makes before any logging so that Logback writes into the same folder.
     */
    @Synchronized
    fun processRunId(): String = System.getProperty(RUN_ID_PROPERTY)
      ?: RUN_ID_FORMAT.format(Instant.now()).also { System.setProperty(RUN_ID_PROPERTY, it) }

    /** The working directory, with [stateDirectory] resolved against it when given. */
    fun get(stateDirectory: String? = null): EdictLayout {
      val root = Path.of("").toRealPath()
      return stateDirectory?.let { EdictLayout(root, root.resolve(it).normalize()) } ?: EdictLayout(root)
    }
  }
}
