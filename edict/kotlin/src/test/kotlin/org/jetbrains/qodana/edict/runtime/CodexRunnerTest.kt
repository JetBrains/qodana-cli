package org.jetbrains.qodana.edict.runtime

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
    fun `runtime exposes exactly one Edict MCP server`() {
        val project = Files.createDirectory(directory.resolve("project"))
        val state = Files.createDirectory(directory.resolve("state"))
        val runner = CodexRunner(
            directory.resolve("output"), project, state, "http://127.0.0.1:10001/mcp"
        )
        runner.prepare()
        val parsed = Toml.parse(runner.home.resolve("config.toml"))
        assertFalse(parsed.hasErrors(), parsed.errors().toString())
        assertEquals(true, parsed.getBoolean(listOf("features", "multi_agent")))
        assertNull(parsed.get(listOf("features", "multi_agent_v2")))
        assertEquals(true, parsed.getBoolean(listOf("agents", "enabled")))
        assertEquals(50, parsed.getLong(listOf("agents", "max_concurrent_threads_per_session")))
        assertEquals(setOf("edict-mcp"), parsed.getTable("mcp_servers")!!.keySet())
        assertEquals("http://127.0.0.1:10001/mcp", parsed.getString(listOf("mcp_servers", "edict-mcp", "url")))
    }

    @Test
    fun `runtime installs managed skills and configures command backed Edict MCP`() {
        val project = Files.createDirectory(directory.resolve("project-next"))
        val state = Files.createDirectory(directory.resolve("state-next"))
        val repository = Files.createDirectory(directory.resolve("repository-next"))
        val runner = CodexRunner(
            directory.resolve("output-next"), project, state, "",
            primaryMcpCommand = listOf("/opt/edict", "edict-mcp-next", "--state-dir", state.toString()),
            primaryMcpEnabledTools = listOf("edict_next_prepare_pipeline", "edict_next_validate_generation"),
            additionalWritableRoots = listOf(repository),
        )
        runner.prepare()

        val parsed = Toml.parse(runner.home.resolve("config.toml"))
        assertFalse(parsed.hasErrors(), parsed.errors().toString())
        assertEquals(setOf("edict-mcp"), parsed.getTable("mcp_servers")!!.keySet())
        assertEquals("/opt/edict", parsed.getString(listOf("mcp_servers", "edict-mcp", "command")))
        assertEquals(true, parsed.getBoolean(listOf("mcp_servers", "edict-mcp", "required")))
        assertEquals(
            listOf("edict-mcp-next", "--state-dir", state.toString()),
            parsed.getArray(listOf("mcp_servers", "edict-mcp", "args"))!!.toList(),
        )
        assertEquals(
            listOf("edict_next_prepare_pipeline", "edict_next_validate_generation"),
            parsed.getArray(listOf("mcp_servers", "edict-mcp", "enabled_tools"))!!.toList(),
        )
        assertEquals(
            listOf("code_mode", "deferred"),
            parsed.getArray(listOf("mcp_servers", "edict-mcp", "omit_tools_from"))!!.toList(),
        )
        assertTrue(Files.exists(runner.home.resolve("skills/edict-next-run/SKILL.md")))
        assertTrue(Files.exists(runner.home.resolve("skills/edict_manager/SKILL.md")))
        assertFalse(Files.exists(runner.home.resolve("skills/edict-run/SKILL.md")))
        assertEquals(
            "write",
            parsed.getString(listOf("permissions", "edict-test", "filesystem", repository.toRealPath().resolve(".git").toString())),
        )
        assertEquals(true, parsed.getBoolean(listOf("permissions", "edict-test", "workspace_roots", repository.toRealPath().toString())))
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
        val runner = CodexRunner(directory, directory, directory, "http://127.0.0.1/mcp")
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
