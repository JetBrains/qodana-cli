package org.jetbrains.qodana.edict

import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Files
import java.nio.file.Path
import kotlin.test.*

class SkillsTest {
    @TempDir lateinit var directory: Path

    @Test fun `install only managed skills with matching registry and metadata`() {
        assertEquals(13, Skills.install(directory).size)
        Registry.policies.forEach { policy ->
            val name = Registry.installedName(policy.name)
            val text = Files.readString(directory.resolve("$name/SKILL.md"))
            assertContains(text, "name: $name\n")
            assertFalse(text.contains("edict-next-"))
            val invocation = Files.readString(directory.resolve("$name/agents/openai.yaml"))
            assertContains(invocation, "allow_implicit_invocation: ${name == "edict_manager"}")
        }
        assertTrue(Files.exists(directory.resolve("edict_manager/references/protocol.md")))
        assertFailsWith<IllegalArgumentException> { Skills.install(directory, "../edict_manager") }
    }

    @Test fun `registry separates delegation from execution`() {
        assertTrue(Registry["edict_manager"].writes.isEmpty())
        assertTrue(Registry["edict-signal-analysis"].operations.isEmpty())
        Registry.policies.forEach { policy ->
            assertTrue(policy.operations.containsAll(policy.writes))
            policy.delegates.forEach { Registry[it] }
        }
    }
}
