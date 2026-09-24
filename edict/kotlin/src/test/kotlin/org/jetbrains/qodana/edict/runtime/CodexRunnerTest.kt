package org.jetbrains.qodana.edict.runtime

import java.nio.file.Files
import java.nio.file.Path
import kotlin.test.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import org.tomlj.Toml

class CodexRunnerTest {
    @TempDir lateinit var directory: Path

    @Test fun `external compiler tools are explicit and cannot replace managed state authority`() {
        val project = Files.createDirectory(directory.resolve("project"))
        val state = Files.createDirectory(directory.resolve("state"))
        val runner = CodexRunner(directory.resolve("output"), project, state, "http://127.0.0.1:10001/mcp",
            additionalMcpServers = mapOf("inspection.compiler" to "http://127.0.0.1:10002/mcp"))
        runner.prepare()
        val parsed = Toml.parse(runner.home.resolve("config.toml"))
        assertFalse(parsed.hasErrors(), parsed.errors().toString())
        assertEquals(setOf("edict-mcp", "inspection.compiler"), parsed.getTable("mcp_servers")!!.keySet())
        assertEquals("http://127.0.0.1:10001/mcp", parsed.getString(listOf("mcp_servers", "edict-mcp", "url")))
        assertEquals("http://127.0.0.1:10002/mcp", parsed.getString(listOf("mcp_servers", "inspection.compiler", "url")))
        assertEquals(300, parsed.getLong(listOf("mcp_servers", "inspection.compiler", "tool_timeout_sec")))
        assertFailsWith<IllegalArgumentException> {
            CodexRunner(directory.resolve("invalid"), project, state, "http://127.0.0.1:10001/mcp",
                additionalMcpServers = mapOf("edict-mcp" to "http://127.0.0.1:10002/mcp")).prepare()
        }
    }

    @Test fun `isolated runtime preserves selected provider without inheriting host tools or permissions`() {
        val file = directory.resolve("config.toml")
        Files.writeString(file, """
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
        """.trimIndent())
        val runner = CodexRunner(directory, directory, directory, "http://127.0.0.1/mcp")
        val inherited = assertNotNull(runner.providerConfiguration(file))
        val parsed = Toml.parse(inherited)
        assertFalse(parsed.hasErrors(), parsed.errors().toString())
        assertEquals("local.proxy", parsed.getString("model_provider"))
        assertEquals("test-value", parsed.getString(listOf("model_providers", "local.proxy", "http_headers", "X-Routing")))
        assertEquals(false, parsed.getBoolean(listOf("model_providers", "local.proxy", "requires_openai_auth")))
        assertNull(parsed.get("mcp_servers"))
        assertNull(parsed.get("approval_policy"))
    }
}
