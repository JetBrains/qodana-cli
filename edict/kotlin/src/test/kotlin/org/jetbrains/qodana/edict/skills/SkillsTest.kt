package org.jetbrains.qodana.edict.skills

import org.jetbrains.qodana.edict.skills.managed.Registry
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
        assertEquals(18, installed.size)
        assertEquals(Registry.policies.map { it.name }.sorted(), installed)
        Registry.policies.forEach { policy ->
            val name = policy.name
            val text = Files.readString(directory.resolve("$name/SKILL.md"))
            assertContains(text, "name: $name\n")
            if (name != "edict_manager") {
                assertContains(text, "description: Managed ")
                assertContains(text, "[the manager protocol](../edict_manager/references/protocol.md)")
                assertContains(text, "Run only as a delegated managed subagent.")
            }
        }
        assertTrue(Files.exists(directory.resolve("edict_manager/references/protocol.md")))
        assertTrue(Files.exists(directory.resolve("edict-next-run/SKILL.md")))
        assertTrue(Files.exists(directory.resolve("edict-next-generation/SKILL.md")))
        assertFalse(Files.exists(directory.resolve("generation")))
        assertFalse(Files.exists(directory.resolve("signals-extraction")))
        assertFalse(Files.exists(directory.resolve("edict-run/SKILL.md")))
        assertFalse(Files.exists(directory.resolve("edict-prepare/SKILL.md")))
        assertFalse(Files.exists(directory.resolve("edict-inspection-value-review/SKILL.md")))
        assertFailsWith<IllegalArgumentException> { Skills.install(directory, "../edict_manager") }
        assertFailsWith<IllegalArgumentException> { Skills.install(directory, "edict-prepare") }
    }

    @Test
    fun `registry contains only installed skills and valid delegation edges`() {
        assertEquals(
            listOf("edict-next-distribution", "edict-next-generation"),
            Registry["edict-next-run"].delegates,
        )
        assertTrue("edict-git-history-signal-analysis" in Registry["edict_manager"].delegates)
        assertTrue("edict-retrospective-signal-analysis" in Registry["edict_manager"].delegates)
        assertTrue("edict-promote" in Registry["edict_manager"].delegates)
        assertTrue("ecict-check-promotion" in Registry["edict_manager"].delegates)
        assertEquals(listOf("edict-promotion-decision"), Registry["ecict-check-promotion"].delegates)
        listOf("edict-promote", "ecict-check-promotion", "edict-promotion-decision").forEach {
            assertFalse("worktree" in Skills.read(it).lowercase())
        }
        assertEquals(
            listOf("edict-signal-analysis"),
            Registry["edict-git-history-signal-analysis"].delegates,
        )
        assertEquals(
            listOf("edict-signal-analysis"),
            Registry["edict-retrospective-signal-analysis"].delegates,
        )
        assertEquals(
            listOf(
                "edict-next-code-example-overseer",
                "edict-next-inspection-shallow-review",
                "edict-next-weak-signal-review",
                "edict-next-inspection-code-review",
            ),
            Registry["edict-next-cluster-generation"].delegates,
        )
        // Evidence reviews write their examples themselves.
        assertEquals(emptyList(), Registry["edict-next-weak-signal-review"].delegates)
        assertEquals(emptyList(), Registry["edict-next-inspection-code-review"].delegates)
        Registry.policies.forEach { policy ->
            policy.delegates.forEach { Registry[it] }
        }
        listOf(
            "edict-run",
            "edict-prepare",
            "edict-distribution",
            "edict-generation",
            "edict-cluster-generation",
            "edict-code-example",
            "edict-inspection-code-review",
            "edict-inspection-value-review",
            "edict-weak-signal-review",
        ).forEach { name -> assertFailsWith<IllegalStateException> { Registry[name] } }
    }

    @Test
    fun `grouped resources install as flat discoverable skills`() {
        val generation = directory.resolve("generation-only")
        assertEquals(listOf("edict-next-generation"), Skills.install(generation, "edict-next-generation"))
        assertTrue(Files.exists(generation.resolve("edict-next-generation/SKILL.md")))
        assertFalse(Files.exists(generation.resolve("generation")))
        assertContains(Skills.read("edict-next-generation"), "name: edict-next-generation\n")

        val extraction = directory.resolve("extraction-only")
        assertEquals(
            listOf("edict-signal-analysis"),
            Skills.install(extraction, "edict-signal-analysis"),
        )
        assertTrue(Files.exists(extraction.resolve("edict-signal-analysis/SKILL.md")))
        assertTrue(Files.exists(extraction.resolve("edict-signal-analysis/agents/openai.yaml")))
        assertFalse(Files.exists(extraction.resolve("signals-extraction")))
        assertContains(Skills.read("edict-signal-analysis"), "name: edict-signal-analysis\n")
    }

    @Test
    fun `shallow review gates and evidence reviews only add weak examples before the final evaluation`() {
        val compactGeneration = Skills.read("edict-next-cluster-generation").replace(Regex("\\s+"), " ")
        val shallow = Skills.read("edict-next-inspection-shallow-review")
        val compactShallow = shallow.replace(Regex("\\s+"), " ")
        val weak = Skills.read("edict-next-weak-signal-review").replace(Regex("\\s+"), " ")
        val review = Skills.read("edict-next-inspection-code-review").replace(Regex("\\s+"), " ")

        val validation = compactGeneration.indexOf("Submit the complete candidate with `edict_next_save_candidate_inspection`")
        val shallowReview = compactGeneration.indexOf("Launch a fresh shallow review worker")
        assertTrue(validation in 0..<shallowReview)
        assertContains(compactGeneration, "Weak failures never block a cycle or the Generated transition")
        assertContains(compactGeneration, "its `remainingProjectAnalyses` says how many remain")
        assertContains(compactGeneration, "call `edict_next_record_evaluation(token, clusterId)` as the last step")
        assertContains(shallow, "Do not edit the candidate, inspected project, or repository")
        assertContains(compactShallow, "Read only the candidate")
        assertContains(compactShallow, "It does not decide whether the inspection is correct")
        assertContains(shallow, "\"status\": \"ACCEPT|REJECT\"")
        assertContains(shallow, "\"category\": \"HARDCODED|PERFORMANCE|IMPLEMENTATION|METADATA\"")
        assertFalse(shallow.contains("COVERAGE"))
        assertContains(weak, "Do not launch workers: write every example yourself")
        assertContains(weak, "Do not read the candidate's implementation")
        assertContains(weak, "Strong evidence always wins")
        assertContains(review, "The output of this review is evidence")
        assertContains(review, "keep at most five")
        assertFalse(review.contains("\"status\": \"ACCEPT|REJECT\""))
    }
}
