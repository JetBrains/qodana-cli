// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.runtime

import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.StandardCopyOption
import java.nio.file.attribute.PosixFilePermissions
import java.util.concurrent.TimeUnit
import kotlinx.serialization.json.JsonPrimitive
import org.jetbrains.qodana.edict.logging.AgentLogger
import org.jetbrains.qodana.edict.skills.Skills
import org.tomlj.Toml
import org.tomlj.TomlTable

/** Isolated host for managed-skill tests and integrations. MCP must run outside the agent filesystem sandbox. */
class CodexRunner(
    private val output: Path,
    private val project: Path,
    private val state: Path,
    private val mcpUrl: String,
    private val executable: String = System.getenv("CODEX_BIN") ?: "codex",
    val model: String = System.getenv("CODEX_MODEL") ?: "gpt-5.6-sol",
    private val agentLogger: AgentLogger? = null,
    private val additionalMcpServers: Map<String, String> = emptyMap(),
) {
    val home: Path = output.resolve("codex-home")
    val scratch: Path = output.resolve("scratch")
    val trace: Path = output.resolve("trace")
    private fun quote(value: String): String = JsonPrimitive(value).toString()

    fun prepare() {
        require("edict-mcp" !in additionalMcpServers) { "Additional tools must not replace the managed Edict server" }
        listOf(output, home, scratch, trace).forEach {
            Files.createDirectories(it)
            if (Files.getFileStore(it).supportsFileAttributeView("posix")) Files.setPosixFilePermissions(it, PosixFilePermissions.fromString("rwx------"))
        }
        Skills.install(home.resolve("skills"))
        val sourceHome = Path.of(System.getenv("CODEX_HOME") ?: Path.of(System.getProperty("user.home"), ".codex").toString())
        val inherited = providerConfiguration(sourceHome.resolve("config.toml"))
        val litellm = inherited == null && !System.getenv("LITELLM_API_KEY").isNullOrBlank()
        val provider = inherited ?: if (litellm) """
            model_provider = "litellm"
            [model_providers.litellm]
            name = "LiteLLM"
            base_url = "https://litellm.labs.jb.gg/openai"
            env_key = "LITELLM_API_KEY"
            wire_api = "responses"
        """.trimIndent() else ""
        if (!litellm) {
            val auth = sourceHome.resolve("auth.json")
            if (Files.exists(auth)) Files.copy(auth, home.resolve("auth.json"), StandardCopyOption.REPLACE_EXISTING)
        }
        Files.writeString(home.resolve("config.toml"), """
            approval_policy = "never"
            default_permissions = "edict-test"
            model_reasoning_effort = "high"
            $provider

            [permissions.edict-test]
            extends = ":read-only"
            [permissions.edict-test.filesystem]
            ":tmpdir" = "write"
            ${quote(project.toRealPath().toString())} = "read"
            ${quote(state.toRealPath().toString())} = "read"
            ${quote(trace.toAbsolutePath().toString())} = "deny"
            ${quote(home.resolve("sessions").toAbsolutePath().toString())} = "deny"
            ${quote(home.resolve("log").toAbsolutePath().toString())} = "deny"
            ${quote(output.resolve("log").toAbsolutePath().toString())} = "deny"
            [permissions.edict-test.workspace_roots]
            ${quote(scratch.toAbsolutePath().toString())} = true
            [permissions.edict-test.filesystem.":workspace_roots"]
            "." = "write"
            [permissions.edict-test.network]
            enabled = true
            mode = "full"

            [features]
            multi_agent = true
            [agents]
            max_depth = 5
            max_concurrent_threads_per_session = 6
            [mcp_servers.edict-mcp]
            url = ${quote(mcpUrl)}
            default_tools_approval_mode = "approve"
        """.trimIndent() + "\n" + additionalMcpServers.entries.joinToString("\n") { (name, url) ->
            """
                [mcp_servers.${quote(name)}]
                url = ${quote(url)}
                default_tools_approval_mode = "approve"
                tool_timeout_sec = 300
            """.trimIndent() + "\n"
        })
    }

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
        val process = ProcessBuilder(executable, "exec", "--dangerously-bypass-hook-trust", "--json", "--skip-git-repo-check", "--model", model,
            "--output-last-message", last.toString(), prompt)
            .directory(project.toFile()).redirectOutput(stdout.toFile()).redirectError(stderr.toFile())
            .apply { environment()["CODEX_HOME"] = home.toString(); environment()["TMPDIR"] = scratch.toString() }.start()
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
            try { collector?.scan(final = true) }
            catch (e: Exception) { if (failure != null) failure.addSuppressed(e) else throw e }
        }
    }

    fun verifySandbox() {
        val allowed = scratch.resolve("sandbox-write-probe")
        val denied = state.resolve("unmanaged-write-probe")
        val process = ProcessBuilder(executable, "sandbox", "-P", "edict-test", "-C", project.toString(), "--", "/bin/sh", "-c",
            "printf allowed > \"\$1\" && printf forbidden > \"\$2\"", "edict-sandbox-probe", allowed.toString(), denied.toString())
            .redirectOutput(trace.resolve("sandbox.stdout").toFile()).redirectError(trace.resolve("sandbox.stderr").toFile())
            .apply { environment()["CODEX_HOME"] = home.toString(); environment()["TMPDIR"] = scratch.toString() }.start()
        process.outputStream.close()
        try {
            check(process.waitFor(30, TimeUnit.SECONDS)) { "Sandbox probe timed out" }
            check(process.exitValue() != 0 && Files.exists(allowed) && Files.readString(allowed) == "allowed" && !Files.exists(denied)) {
                "Codex sandbox must permit scratch writes and deny direct state writes; inspect $trace"
            }
        } finally { if (process.isAlive) process.destroyForcibly(); Files.deleteIfExists(allowed) }
    }
}
