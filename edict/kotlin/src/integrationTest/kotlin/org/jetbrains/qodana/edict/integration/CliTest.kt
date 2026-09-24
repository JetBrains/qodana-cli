package org.jetbrains.qodana.edict.integration

import java.nio.file.Files
import java.util.concurrent.TimeUnit
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlinx.serialization.json.jsonObject
import org.jetbrains.qodana.edict.common.array
import org.jetbrains.qodana.edict.common.obj
import org.jetbrains.qodana.edict.common.runProcess
import org.jetbrains.qodana.edict.common.wireJson
import org.jetbrains.qodana.edict.integration.support.IntegrationTest
import org.jetbrains.qodana.edict.store.Store
import org.junit.jupiter.api.Test

class CliTest : IntegrationTest() {

    @Test
    fun `installed application serves protocol-only stdio and releases state lock on EOF`() {
        val stdout = directory.resolve("stdout")
        val stderr = directory.resolve("stderr")
        val state = workspace.state
        val process = ProcessBuilder(
            System.getProperty("edict.executable"),
            "mcp",
            "--project-dir",
            workspace.project.toString(),
            "--state-dir",
            state.toString(),
            "--log-dir",
            directory.resolve("log").toString()
        )
            .directory(workspace.project.toFile()).redirectOutput(stdout.toFile()).redirectError(stderr.toFile())
            .start()
        try {
            process.outputStream.bufferedWriter().use {
                it.appendLine("""{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26"}}""")
                it.appendLine("""{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"edict_registry","arguments":{}}}""")
            }
            assertTrue(process.waitFor(30, TimeUnit.SECONDS), "CLI did not terminate on EOF")
            assertEquals(0, process.exitValue(), Files.readString(stderr))
            val responses = Files.readAllLines(stdout).map { wireJson.parseToJsonElement(it).jsonObject }
            assertEquals(2, responses.size)
            assertEquals(13, responses.last().obj("result").obj("structuredContent").array("skills").size)
            Store(state).use { assertNull(it.plan()) }
        } finally {
            if (process.isAlive) process.destroyForcibly()
        }
    }

    @Test
    fun `packaged distribution installs complete skill resources`() {
        runProcess(
            workspace.project,
            listOf(
                System.getProperty("edict.executable"),
                "install-skills",
                "--directory",
                directory.resolve("skills").toString()
            )
        )
        assertTrue(Files.exists(directory.resolve("skills/edict_manager/references/signals.md")))
        assertTrue(Files.exists(directory.resolve("skills/managed-edict-signal-analysis/agents/openai.yaml")))
    }
}
