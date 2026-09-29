package org.jetbrains.qodana.edict.skills

import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Files
import java.nio.file.Path
import kotlin.test.*

class SkillsTest {
    @TempDir
    lateinit var directory: Path

    @Test
    fun `install managed skills with matching registry and metadata`() {
        val installed = Skills.install(directory)
        assertEquals(13, installed.size)
        assertEquals(Registry.policies.map { it.name }.sorted(), installed)
        Registry.policies.forEach { policy ->
            val name = policy.name
            val text = Files.readString(directory.resolve("$name/SKILL.md"))
            assertContains(text, "name: $name\n")
            assertFalse(text.contains("edict-next-"))
            val invocation = Files.readString(directory.resolve("$name/agents/openai.yaml"))
            assertContains(invocation, "allow_implicit_invocation: ${name == "edict_manager"}")
        }
        assertTrue(Files.exists(directory.resolve("edict_manager/references/protocol.md")))
        val preparation = Files.readString(directory.resolve("edict-prepare/SKILL.md"))
        assertContains(preparation, "supplied `codeSnippet` is found verbatim at the cited revision")
        assertContains(preparation, "does not require the feedback prose itself to say that the range is")
        val generation = Files.readString(directory.resolve("edict-generation/SKILL.md"))
        assertContains(generation, "Review findings alone must not produce a Pending outcome")
        val clusterGeneration = Files.readString(directory.resolve("edict-cluster-generation/SKILL.md"))
        assertContains(clusterGeneration, "Review findings alone never justify this transition")
        assertContains(clusterGeneration, "MAJOR findings are publishable limitations")
        assertContains(clusterGeneration, "`review` (`code` or `weak-signal`)")
        assertFalse(clusterGeneration.contains("Delegate `edict-inspection-value-review`"))
        assertFalse(Files.exists(directory.resolve("edict-next-run/SKILL.md")))
        assertFailsWith<IllegalArgumentException> { Skills.install(directory, "../edict_manager") }
        assertFailsWith<IllegalArgumentException> { Skills.install(directory, "edict-next-run") }
    }

    @Test
    fun `registry separates delegation from execution`() {
        assertTrue(Registry["edict_manager"].writes.isEmpty())
        assertTrue(Registry["edict-signal-analysis"].operations.isEmpty())
        Registry.policies.forEach { policy ->
            assertTrue(policy.operations.containsAll(policy.writes))
            policy.delegates.forEach { Registry[it] }
        }
    }
}
