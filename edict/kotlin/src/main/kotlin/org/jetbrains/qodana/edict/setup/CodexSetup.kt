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
import org.tomlj.TomlArray
import org.tomlj.TomlParseResult
import org.tomlj.TomlTable
import java.io.IOException
import java.net.URI
import java.nio.file.Path
import kotlin.io.path.createDirectories
import kotlin.io.path.deleteIfExists
import kotlin.io.path.exists
import kotlin.io.path.readText
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
   * Installs skills and merges the Edict settings for the server port from [config] into the local config, then asks
   * Codex, with [codexHome], whether it loads that config. Codex ignores it for a project it does not trust: install then
   * restores the previous config and fails.
   */
  fun install(
    layout: EdictLayout,
    deniedPaths: List<Path>,
    config: EdictConfig,
    codexHome: Path = userCodexHome(),
  ): List<String> {
    val previous = layout.codexConfigPath.takeIf { it.exists() }?.readText()
    val merged = mergeCodexConfig(
      previous.orEmpty(),
      codexConfig(layout, deniedPaths.map { layout.root.resolve(it).canonical() }, config.mcpPort),
      layout.codexConfigPath,
    )
    val skills = Skills.install(layout.codexSkillsDirectory)
    layout.codexConfigPath.parent.createDirectories()
    layout.codexConfigPath.writeText(merged)
    try {
      requireLoadedByCodex(layout, codexHome)
    }
    catch (e: Exception) {
      if (previous == null) layout.codexConfigPath.deleteIfExists() else layout.codexConfigPath.writeText(previous)
      throw e
    }
    logger.info { "Installed skills ${skills.joinToString()} into ${layout.codexSkillsDirectory}" }
    logger.info { "Merged Edict settings into ${layout.codexConfigPath} for port ${config.mcpPort}" }
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

/** Ends every line that install writes, so a later install replaces exactly those lines. */
private const val EDICT_MARKER = "# qodana-edict"

/** The settings Edict needs, one entry per line; [mergeCodexConfig] copies the missing lines into the user's config. */
private fun codexConfig(layout: EdictLayout, deniedPaths: List<Path>, port: Int): String = """
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

/**
 * Adds the [edict] settings to the user's [existing] config without changing any user line. Lines from an earlier
 * install are dropped first. A user value equal to Edict's stays as it is; a different one fails, because Edict cannot
 * run with it. Root keys go to the top, the keys of a table that the user declares go right after its header, and other
 * tables are appended.
 */
internal fun mergeCodexConfig(existing: String, edict: String, path: Path): String {
  val userLines = existing.lines().filterNot { it.trimEnd().endsWith(EDICT_MARKER) }.let {
    if (it.lastOrNull()?.isEmpty() == true) it.dropLast(1) else it
  }
  val user = parseCodexConfig(userLines.joinToString("\n"), path)
  val template = Toml.parse(edict).also { check(!it.hasErrors()) { "Invalid Edict settings: ${it.errors().first()}" } }
  val edictLines = edict.lines()
  val settings = template.keyPathSet(true).filterNot { template.isTable(it) }.sortedBy { template.line(it) }
  val conflicts = settings.mapNotNull { key ->
    val current = user.get(key) ?: return@mapNotNull null
    if (sameValue(current, template.get(key))) null
    else "line ${user.line(key)}: ${userLines[user.line(key) - 1].trim()} (Edict needs ${edictLines[template.line(key) - 1].trim()})"
  }
  check(conflicts.isEmpty()) {
    "$path sets values that Edict needs differently. Change or remove them, then re-run `qodana edict install`:\n" +
      conflicts.joinToString("\n")
  }
  val insertions = mutableMapOf<Int, MutableList<String>>()
  val appended = mutableListOf<String>()
  settings.filter { user.get(it) == null }.groupBy { it.dropLast(1) }.forEach { (table, keys) ->
    val lines = keys.map { "${edictLines[template.line(it) - 1].trim()} $EDICT_MARKER" }
    val header = if (table.isEmpty()) 0 else user.headerLine(table)
    when {
      header != null -> insertions.getOrPut(header) { mutableListOf() }.addAll(lines)
      else -> appended += listOf(EDICT_MARKER, "[${Toml.joinKeyPath(table)}] $EDICT_MARKER") + lines
    }
  }
  val merged = buildList {
    insertions[0]?.let(::addAll)
    userLines.forEachIndexed { index, line ->
      add(line)
      insertions[index + 1]?.let(::addAll)
    }
    addAll(appended)
  }.joinToString("\n", postfix = "\n")
  parseCodexConfig(merged, path)
  return merged
}

/**
 * The 1-based line of the `[table]` header that declares [table], or null without one. An implicit table has the
 * position of its first subtable and a table of dotted keys that of its first key, so a table has its own header only
 * when none of its entries shares its position.
 */
private fun TomlTable.headerLine(table: List<String>): Int? {
  if (!isTable(table)) return null
  val position = inputPositionOf(table)
  return line(table).takeIf { getTable(table)!!.keySet().none { inputPositionOf(table + it) == position } }
}

/** The 1-based line where [key], which must exist, is defined. */
private fun TomlTable.line(key: List<String>): Int = inputPositionOf(key)!!.line()

private fun parseCodexConfig(text: String, path: Path): TomlParseResult = Toml.parse(text).also {
  require(!it.hasErrors()) { "Cannot merge Edict settings into $path: ${it.errors().first()}" }
}

private fun sameValue(first: Any?, second: Any?): Boolean = when {
  first is TomlArray && second is TomlArray -> Toml.equals(first, second)
  first is TomlTable && second is TomlTable -> Toml.equals(first, second)
  else -> first == second
}
