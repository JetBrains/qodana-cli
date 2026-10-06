// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.setup

import io.github.oshai.kotlinlogging.KotlinLogging
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.jetbrains.qodana.edict.common.EdictLayout
import org.jetbrains.qodana.edict.common.runProcess
import org.jetbrains.qodana.edict.edictnext.EdictNextMcpToolset
import org.jetbrains.qodana.edict.skills.Skills
import org.tomlj.Toml
import java.io.IOException
import java.net.URI
import java.nio.file.Path
import kotlin.io.path.createDirectories
import kotlin.io.path.deleteIfExists
import kotlin.io.path.exists
import kotlin.io.path.writeText

/**
 * Sets Edict up for Codex sessions started in the working directory: skills in `.codex/skills` and session settings in
 * `.codex/config.toml`. The provider stays in the user's `$CODEX_HOME`, which Edict never writes; Codex reads the local
 * config only for a project trusted there, so [install] asks Codex whether it loads the config and fails otherwise.
 */
internal object CodexSetup {
  /** The user-level Codex home: `$CODEX_HOME`, or `~/.codex` like Codex itself. */
  fun userCodexHome(): Path =
    System.getenv("CODEX_HOME")?.let(Path::of) ?: Path.of(System.getProperty("user.home"), ".codex")

  /**
   * Installs skills and rewrites the local config for the server port from [config], then asks Codex, with [codexHome],
   * whether it loads that config. Codex ignores it for a project it does not trust: install then removes it and fails.
   */
  fun install(
    layout: EdictLayout,
    deniedPaths: List<Path>,
    config: EdictConfig,
    codexHome: Path = userCodexHome(),
  ): List<String> {
    val skills = Skills.install(layout.codexSkillsDirectory)
    layout.codexConfigPath.parent.createDirectories()
    layout.codexConfigPath.writeText(
      codexConfig(layout, deniedPaths.map { layout.root.resolve(it).canonical() }, config.mcpPort),
    )
    try {
      requireLoadedByCodex(layout, codexHome)
    }
    catch (e: Exception) {
      layout.codexConfigPath.deleteIfExists()
      throw e
    }
    logger.info { "Installed skills ${skills.joinToString()} into ${layout.codexSkillsDirectory}" }
    logger.info { "Wrote ${layout.codexConfigPath} for port ${config.mcpPort}" }
    return skills
  }

  /** Fails unless [install] configured Codex for the server port from [config]. */
  fun requireInstalled(layout: EdictLayout, config: EdictConfig) {
    val port = configuredPort(layout) ?: error("Edict is not installed in ${layout.root}; run `qodana edict install` first")
    check(port == config.mcpPort) {
      "${layout.codexConfigPath} uses port $port, but edict.mcpPort is ${config.mcpPort}; re-run `qodana edict install`"
    }
  }

  /** Asks Codex which MCP servers it loads in the project: none from `.codex/config.toml` unless the project is trusted. */
  private fun requireLoadedByCodex(layout: EdictLayout, codexHome: Path) {
    val executable = System.getenv("CODEX_BIN") ?: "codex"
    val output = try {
      runProcess(layout.root, listOf(executable, "mcp", "list", "--json"), environment = mapOf("CODEX_HOME" to codexHome.toString()))
    }
    catch (e: IOException) {
      throw IllegalStateException("Edict install checks the project with Codex, but `$executable` cannot run: ${e.message}", e)
    }
    val servers = Json.parseToJsonElement(output).jsonArray.map { it.jsonObject["name"]?.jsonPrimitive?.content }
    check(EdictNextMcpToolset.SERVER_NAME in servers) {
      "Codex does not load ${layout.codexConfigPath}: ${layout.root} is not trusted in ${codexHome.resolve("config.toml")}. " +
        "Trust it in Codex, or add:\n[projects.${quote(layout.root)}]\ntrust_level = \"trusted\""
    }
  }

  private fun configuredPort(layout: EdictLayout): Int? {
    if (!layout.codexConfigPath.exists()) return null
    val config = Toml.parse(layout.codexConfigPath)
    require(!config.hasErrors()) { "Cannot parse ${layout.codexConfigPath}: ${config.errors().first()}" }
    val url = config.getString(listOf("mcp_servers", EdictNextMcpToolset.SERVER_NAME, "url")) ?: return null
    return URI(url).port.takeIf { it != -1 } ?: error("${layout.codexConfigPath} has no port in $url")
  }
}

private val logger = KotlinLogging.logger {}

// The sandbox matches resolved paths, so symlinks such as macOS /tmp -> /private/tmp must be resolved for existing paths.
private fun Path.canonical(): Path = if (exists()) toRealPath() else toAbsolutePath().normalize()

private fun quote(value: Any): String = JsonPrimitive(value.toString()).toString()

private fun codexConfig(layout: EdictLayout, deniedPaths: List<Path>, port: Int): String = """
  |# Written by `qodana edict install`; re-running it rewrites this file. The port comes from edict.mcpPort in qodana.yaml.
  |approval_policy = "never"
  |default_permissions = "edict"
  |model_reasoning_effort = "high"
  |
  |# State is read-only to agents: every state change goes through Edict MCP tools.
  |[permissions.edict]
  |extends = ":read-only"
  |
  |[permissions.edict.filesystem]
  |":tmpdir" = "write"
  |${quote(layout.root)} = "read"
  |${quote(layout.agentWorkRoot)} = "write"
  |${quote(layout.processLogRoot)} = "deny"
  |${deniedPaths.joinToString("") { "${quote(it)} = \"deny\"\n" }}
  |[permissions.edict.network]
  |enabled = true
  |mode = "full"
  |
  |[features]
  |multi_agent = true
  |
  |[agents]
  |enabled = true
  |max_depth = 5
  |max_concurrent_threads_per_session = 50
  |
  |[mcp_servers.${EdictNextMcpToolset.SERVER_NAME}]
  |url = "http://127.0.0.1:$port/mcp"
  |required = true
  |default_tools_approval_mode = "approve"
  |tool_timeout_sec = 3600
  |omit_tools_from = ["code_mode", "deferred"]
  |""".trimMargin()
