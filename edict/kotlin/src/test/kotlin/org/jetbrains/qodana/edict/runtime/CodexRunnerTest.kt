package org.jetbrains.qodana.edict.runtime

import org.jetbrains.qodana.edict.common.EdictLayout
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import org.tomlj.Toml
import java.nio.file.Files
import java.nio.file.Path
import kotlin.test.*

class CodexRunnerTest {
    @TempDir
    lateinit var directory: Path

    @Test
    fun `runtime keeps only provider and trust globally and installs Edict into the project`() {
        val project = Files.createDirectory(directory.resolve("project"))
        val state = Files.createDirectory(directory.resolve("state"))
        val runner = CodexRunner(
            directory.resolve("output"), EdictLayout(project, state), "http://127.0.0.1:10001/mcp"
        )
        runner.prepare()
        val global = Toml.parse(runner.home.resolve("config.toml"))
        assertFalse(global.hasErrors(), global.errors().toString())
        assertNull(global.get("mcp_servers"))
        assertNull(global.get("permissions"))
        assertEquals("trusted", global.getString(listOf("projects", project.toRealPath().toString(), "trust_level")))
        assertFalse(Files.exists(runner.home.resolve("skills")))

        val local = Toml.parse(project.resolve(".codex/config.toml"))
        assertFalse(local.hasErrors(), local.errors().toString())
        assertEquals(true, local.getBoolean(listOf("features", "multi_agent")))
        assertEquals(true, local.getBoolean(listOf("agents", "enabled")))
        assertEquals(50, local.getLong(listOf("agents", "max_concurrent_threads_per_session")))
        assertEquals(setOf("edict-mcp"), local.getTable("mcp_servers")!!.keySet())
        assertEquals(
            "deny",
            local.getString(listOf("permissions", "edict", "filesystem", runner.trace.toRealPath().toString())),
        )
        assertTrue(Files.exists(project.resolve(".codex/skills/edict_manager/SKILL.md")))
        assertEquals(
            listOf("-c", """mcp_servers.edict-mcp.url="http://127.0.0.1:10001/mcp""""),
            runner.configOverrides(),
        )
    }

    @Test
    fun `runtime passes test-only tools and writable roots as overrides`() {
        val project = Files.createDirectory(directory.resolve("project-next"))
        val state = Files.createDirectory(directory.resolve("state-next"))
        val repository = Files.createDirectory(directory.resolve("repository-next"))
        val runner = CodexRunner(
            directory.resolve("output-next"), EdictLayout(project, state), "http://127.0.0.1:10002/mcp",
            enabledTools = listOf("edict_next_prepare_pipeline", "edict_next_validate_generation"),
            additionalWritableRoots = listOf(repository),
            stateWritable = true,
        )
        val root = repository.toRealPath().toString()
        assertEquals(
            listOf(
                "-c", """mcp_servers.edict-mcp.url="http://127.0.0.1:10002/mcp"""",
                "-c", """mcp_servers.edict-mcp.enabled_tools=["edict_next_prepare_pipeline", "edict_next_validate_generation"]""",
                "-c", """permissions.edict.filesystem={"$root" = "write", "$root/.git" = "write", "${state.toRealPath()}" = "write"}""",
            ),
            runner.configOverrides(),
        )
    }

    @Test
    fun `isolated runtime preserves selected provider without inheriting host tools or permissions`() {
        val file = directory.resolve("config.toml")
        Files.writeString(
            file, """
            model_provider = "local.proxy"
            approval_policy = "on-request"
            [model_providers."local.proxy"]
            name = "Local provider"
            base_url = "http://127.0.0.1:1234/v1"
            wire_api = "responses"
            requires_openai_auth = false
            request_max_retries = 2
            [model_providers."local.proxy".http_headers]
            "X-Routing" = "test-value"
            [mcp_servers.unrelated]
            command = "do-not-run"
        """.trimIndent()
        )
        val runner = CodexRunner(directory, EdictLayout(directory, directory), "http://127.0.0.1/mcp")
        val inherited = assertNotNull(runner.providerConfiguration(file))
        val parsed = Toml.parse(inherited)
        assertFalse(parsed.hasErrors(), parsed.errors().toString())
        assertEquals("local.proxy", parsed.getString("model_provider"))
        assertEquals(
            "test-value",
            parsed.getString(listOf("model_providers", "local.proxy", "http_headers", "X-Routing"))
        )
        assertEquals(false, parsed.getBoolean(listOf("model_providers", "local.proxy", "requires_openai_auth")))
        assertNull(parsed.get("mcp_servers"))
        assertNull(parsed.get("approval_policy"))
    }
}
