package org.jetbrains.qodana.edict.signals

import org.jetbrains.qodana.edict.common.json
import org.jetbrains.qodana.edict.model.SignalLabel
import org.jetbrains.qodana.edict.model.SignalRange
import org.jetbrains.qodana.edict.model.SignalSource
import org.jetbrains.qodana.edict.support.fixtureSignals
import org.jetbrains.qodana.edict.support.gitFixture
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import kotlin.test.assertEquals
import kotlin.test.assertFails
import kotlin.test.assertTrue

class SignalValidationTest {
    @TempDir
    lateinit var directory: Path
    @Test
    fun `feedback requires source attribution and cannot masquerade as a correcting change`() {
        val original = fixtureSignals(gitFixture(directory)).first()
        val signal = original.copy(source = SignalSource("SubmittedFeedback", "", message = "Original labelled feedback",
            url = "git:${original.fileRevision.revision}:benchmark/Rule/specification.json"))
        fun validate(source: SignalSource) = SignalValidation.validate("inbox/${signal.id}.json", json.encodeToString(signal.copy(source = source)))
        assertEquals(signal, validate(signal.source))
        listOf(signal.source.copy(message = null), signal.source.copy(url = ""),
            signal.source.copy(diffPositiveToNegative = original.source.diffPositiveToNegative),
            signal.source.copy(commitRevision = original.source.commitRevision), signal.source.copy(prNumber = 1))
            .forEach { assertFails { validate(it) } }
    }

    @Test
    fun `validate both sides of real commit and reject forged source evidence`() {
        val repository = gitFixture(directory)
        val signals = fixtureSignals(repository)
        assertEquals(setOf(SignalLabel.POSITIVE, SignalLabel.NEGATIVE), signals.map { it.label }.toSet())
        signals.forEach(repository::validateEvidence)
        val signal = signals.first()
        val invalid = listOf(
            signal.copy(id = "s-0000000000"), signal.copy(description = "multi\nline"),
            signal.copy(fileRevision = signal.fileRevision.copy(path = "src/Equality.java")),
            signal.copy(fileRevision = signal.fileRevision.copy(revision = signals.last().fileRevision.revision)),
            signal.copy(fileRevision = signal.fileRevision.copy(expectedRanges = listOf(SignalRange(1, 1)))),
            signal.copy(fileRevision = signal.fileRevision.copy(expectedRanges = listOf(SignalRange(0, 3)))),
            signal.copy(source = signal.source.copy(diffPositiveToNegative = signal.source.diffPositiveToNegative.trimEnd())),
        )
        invalid.forEach { assertFails { SignalValidation.validate("inbox/${it.id}.json", json.encodeToString(it)) } }
        val quotedRange = json.encodeToString(signal).replace("\"start\": 3", "\"start\": \"3\"")
        assertFails { SignalValidation.validate("inbox/${signal.id}.json", quotedRange) }
        assertFails { repository.validateEvidence(signal.copy(source = signal.source.copy(message = "Invented"))) }
        assertFails {
            repository.validateEvidence(
                signal.copy(
                    fileRevision = signal.fileRevision.copy(
                        expectedRanges = listOf(
                            SignalRange(3, 999)
                        )
                    )
                )
            )
        }
    }

    @Test
    fun `diff parser validates counts and sides for renames quoted paths additions and deletions`() {
        val diff =
            "diff --git a/old b/new\n--- a/old\n+++ b/new\n@@ -1,2 +1,2 @@\n--- source text\n+++ source text\n context\n"
        val changes = UnifiedDiff.parse(diff)
        assertEquals(mapOf("old" to listOf(1)), changes.before)
        assertEquals(mapOf("new" to listOf(1)), changes.after)
        val quoted =
            UnifiedDiff.parse("--- \"a/caf\\303\\251.java\"\n+++ \"b/caf\\303\\251.java\"\n@@ -1 +1 @@\n-old\n+new\n")
        assertEquals(setOf("café.java"), quoted.before.keys)
        val add = UnifiedDiff.parse("--- /dev/null\n+++ b/added\n@@ -0,0 +1 @@\n+new\n\\ No newline at end of file\n")
        assertTrue(add.before.isEmpty()); assertEquals(listOf(1), add.after["added"])
        val delete = UnifiedDiff.parse("--- a/deleted\n+++ /dev/null\n@@ -1 +0,0 @@\n-old\n")
        assertTrue(delete.after.isEmpty())
        listOf(
            "not a diff",
            diff.replace("-1,2", "-1,3"),
            diff + "+excess\n",
            diff.replace(" context\n", ""),
            "@@ -1 +1 @@\n-a\n+b\n"
        ).forEach {
            assertFails("Accepted malformed diff $it") { UnifiedDiff.parse(it) }
        }
    }
}
