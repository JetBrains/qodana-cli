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
        assertEquals(17, installed.size)
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
                "edict-next-inspection-code-review",
                "edict-next-weak-signal-review",
            ),
            Registry["edict-next-cluster-generation"].delegates,
        )
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
    fun `inspection review is read only and validation follows acceptance`() {
        val generation = Skills.read("edict-next-cluster-generation")
        val compactGeneration = generation.replace(Regex("\\s+"), " ")
        val review = Skills.read("edict-next-inspection-code-review")
        val compactReview = review.replace(Regex("\\s+"), " ")

        assertContains(compactGeneration, "After acceptance, call `edict_next_validate_inspection(clusterId)`")
        assertContains(review, "Do not edit the candidate, cluster, examples, inspected project, or repository")
        assertContains(review, "Do not stop after finding enough defects to reject")
        assertContains(review, "report all independently established BLOCKER and MAJOR findings in the same review")
        assertContains(compactReview, "contrived composition of multiple wrappers/operators or rare constants")
        assertContains(compactReview, "Do not require exhaustive closure over all composable Java syntax")
        assertContains(review, "\"status\": \"ACCEPT|REJECT\"")
        assertContains(review, "\"category\": \"OBSERVABILITY|PRECISION|IMPLEMENTATION|COVERAGE|PERFORMANCE|DIAGNOSTIC\"")
        assertFalse(compactReview.contains("append one focused example"))
        assertFalse(review.contains("EXAMPLES_ADDED"))
        assertFalse(review.contains("addedExampleIds"))
    }
}
