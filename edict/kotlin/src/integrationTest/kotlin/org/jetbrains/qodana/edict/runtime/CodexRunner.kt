// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.runtime

import kotlinx.serialization.json.JsonPrimitive
import org.jetbrains.qodana.edict.common.EdictLayout
import org.jetbrains.qodana.edict.logging.AgentLogger
import org.jetbrains.qodana.edict.edictnext.EdictNextMcpToolset
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState.Plan
import org.jetbrains.qodana.edict.setup.CodexSetup
import org.jetbrains.qodana.edict.setup.EdictConfig
import org.tomlj.Toml
import org.tomlj.TomlTable
import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.StandardCopyOption
import java.nio.file.attribute.PosixFilePermissions
import java.util.concurrent.TimeUnit

/**
 * Isolated host for managed-skill integration tests. MCP must run outside the agent filesystem sandbox.
 *
 * Like a user, the runner keeps only the provider and project trust in its temporary `CODEX_HOME`; the session settings
 * come from `qodana edict install` in the project. Test-only extras are `codex -c` overrides of that local config.
 */
class CodexRunner internal constructor(
    private val output: Path,
    layout: EdictLayout,
    private val mcpUrl: String,
    // The real Codex: under Gradle, CODEX_BIN is the fake that only answers Edict's trust check.
    private val executable: String = System.getenv("EDICT_REAL_CODEX") ?: System.getenv("CODEX_BIN") ?: "codex",
    val model: String = System.getenv("CODEX_MODEL") ?: "gpt-5.6-sol",
    private val agentLogger: AgentLogger? = null,
    private val enabledTools: List<String>? = null,
    private val additionalWritableRoots: List<Path> = emptyList(),
    private val stateWritable: Boolean = false,
) {
    // Codex and its sandbox see the project by its canonical path, as the server does.
    private val layout = layout.copy(root = layout.root.toRealPath())
    private val project = this.layout.root
    private val state = this.layout.stateDirectory
    val home: Path = output.resolve("codex-home")
    val scratch: Path = output.resolve("scratch")
    val trace: Path = output.resolve("trace")
    private val pricing by lazy { OpenAiPricingLoader.fetch(model) }
    private fun quote(value: String): String = JsonPrimitive(value).toString()

    fun prepare() {
        listOf(output, home, scratch, trace).forEach {
            Files.createDirectories(it)
            if (Files.getFileStore(it).supportsFileAttributeView("posix")) Files.setPosixFilePermissions(
                it,
                PosixFilePermissions.fromString("rwx------")
            )
        }
        val sourceHome = CodexSetup.userCodexHome()
        val auth = sourceHome.resolve("auth.json")
        if (Files.exists(auth)) Files.copy(auth, home.resolve("auth.json"), StandardCopyOption.REPLACE_EXISTING)
        Files.writeString(
            home.resolve("config.toml"),
            providerConfiguration(sourceHome.resolve("config.toml")).orEmpty() + "\n" +
                "[projects.${quote(project.toString())}]\ntrust_level = \"trusted\"\n",
        )
        CodexSetup.install(
            layout,
            listOf(trace, home.resolve("sessions"), home.resolve("log")).map { it.toAbsolutePath() },
            EdictConfig.load(layout),
            home,
        )
    }

    /** Test-only additions to the installed local config. */
    internal fun configOverrides(): List<String> = buildList {
        add("mcp_servers.${EdictNextMcpToolset.SERVER_NAME}.url=${quote(mcpUrl)}")
        enabledTools?.let { tools ->
            add("mcp_servers.${EdictNextMcpToolset.SERVER_NAME}.enabled_tools=[${tools.joinToString(", ") { quote(it) }}]")
        }
        val writable = additionalWritableRoots.map { it.toRealPath() }.flatMap { listOf(it, it.resolve(".git")) } +
            listOfNotNull(state.toRealPath().takeIf { stateWritable })
        // Path keys contain dots, so they go into an inline table; Codex merges it into the installed profile.
        if (writable.isNotEmpty()) {
            add("permissions.edict.filesystem={${writable.joinToString(", ") { "${quote(it.toString())} = \"write\"" }}}")
        }
    }.flatMap { listOf("-c", it) }

    // Inherit only the selected provider, never unrelated hooks, MCP servers, skills or host permissions.
    internal fun providerConfiguration(path: Path): String? {
        if (!Files.exists(path)) return null
        val config = Toml.parse(path)
        require(!config.hasErrors()) { "Cannot parse source Codex provider configuration" }
        val name = config.getString("model_provider") ?: return null
        val provider = config.getTable("model_providers")?.getTable(listOf(name)) ?: return null
        fun value(v: Any): String = when (v) {
            is String -> quote(v)
            is Boolean, is Number -> v.toString()
            is org.tomlj.TomlArray -> (0 until v.size()).joinToString(", ", "[", "]") { value(v.get(it)) }
            else -> error("Unsupported provider configuration value")
        }

        fun table(section: String, table: TomlTable): String = buildString {
            appendLine("[$section]")
            table.keySet().forEach { key ->
                val entry = table.get(listOf(key))!!
                if (entry !is TomlTable) appendLine("${quote(key)} = ${value(entry)}")
            }
            table.keySet().forEach { key ->
                (table.get(listOf(key)) as? TomlTable)?.let { append(table("$section.${quote(key)}", it)) }
            }
        }
        return "model_provider = ${quote(name)}\n" + table("model_providers.${quote(name)}", provider)
    }

    fun run(prompt: String, timeoutMinutes: Long = 20): String {
        val stdout = trace.resolve("stdout.jsonl")
        val stderr = trace.resolve("stderr.log")
        val last = trace.resolve("last-message.txt")
        Files.deleteIfExists(last)
        val process = ProcessBuilder(
            listOf(executable, "exec") + configOverrides() + listOf(
                "--dangerously-bypass-hook-trust", "--json", "--skip-git-repo-check", "--model", model,
                "--output-last-message", last.toString(), prompt,
            )
        )
            .directory(project.toFile()).redirectOutput(stdout.toFile()).redirectError(stderr.toFile())
            .apply { environment()["CODEX_HOME"] = home.toString(); environment()["TMPDIR"] = scratch.toString() }
            .start()
        process.outputStream.close()
        val collector = agentLogger?.let { CodexAgentCollector(home, stdout, it) }
        var failure: Throwable? = null
        try {
            val deadline = System.nanoTime() + TimeUnit.MINUTES.toNanos(timeoutMinutes)
            while (!process.waitFor(250, TimeUnit.MILLISECONDS)) {
                collector?.scan()
                check(System.nanoTime() < deadline) { "Codex timed out after $timeoutMinutes minutes; inspect $trace" }
            }
            check(process.exitValue() == 0) { "Codex exited ${process.exitValue()}; inspect $trace" }
            check(Files.exists(last)) { "Codex returned no final message; inspect $trace" }
            return Files.readString(last)
        } catch (e: Throwable) {
            failure = e
            throw e
        } finally {
            if (process.isAlive) {
                process.descendants().forEach { it.destroyForcibly() }
                process.destroyForcibly()
                process.waitFor(5, TimeUnit.SECONDS)
            }
            try {
                collector?.scan(final = true)
            } catch (e: Exception) {
                if (failure != null) failure.addSuppressed(e) else throw e
            }
        }
    }

    internal fun writePriceReport(plan: Plan): CodexPriceReport =
        CodexPriceReporter.create(home, plan, { model ->
            check(model == pricing.model) { "Unexpected model in Codex session: $model" }
            pricing
        }).also {
            CodexPriceReporter.write(it, layout.processLogDirectory.resolve("edict-price-report.json"))
        }

    fun verifySandbox() {
        val allowed = scratch.resolve("sandbox-write-probe")
        val denied = state.resolve("unmanaged-write-probe")
        val process = ProcessBuilder(
            buildList {
                addAll(listOf(this@CodexRunner.executable, "sandbox"))
                addAll(this@CodexRunner.configOverrides())
                addAll(
                    listOf(
                        "-P",
                        "edict",
                        "-C",
                        this@CodexRunner.project.toString(),
                        "--",
                        "/bin/sh",
                        "-c",
                        "printf allowed > \"\$1\" && printf forbidden > \"\$2\"",
                        "edict-sandbox-probe",
                        allowed.toString(),
                        denied.toString(),
                    )
                )
            }
        )
            .redirectOutput(trace.resolve("sandbox.stdout").toFile())
            .redirectError(trace.resolve("sandbox.stderr").toFile())
            .apply { environment()["CODEX_HOME"] = home.toString(); environment()["TMPDIR"] = scratch.toString() }
            .start()
        process.outputStream.close()
        try {
            check(process.waitFor(30, TimeUnit.SECONDS)) { "Sandbox probe timed out" }
            val stateAccessIsCorrect = if (stateWritable) {
                process.exitValue() == 0 && Files.exists(denied) && Files.readString(denied) == "forbidden"
            }
            else process.exitValue() != 0 && !Files.exists(denied)
            check(Files.exists(allowed) && Files.readString(allowed) == "allowed" && stateAccessIsCorrect) {
                "Codex sandbox must permit scratch writes and enforce configured state access; inspect $trace"
            }
        } finally {
            if (process.isAlive) process.destroyForcibly()
            Files.deleteIfExists(allowed)
            Files.deleteIfExists(denied)
        }
    }
}
