package org.jetbrains.qodana.edict.signals

import org.jetbrains.qodana.edict.core.*
import kotlin.test.Test
import kotlin.test.assertContains
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

class SignalValidationTest {
  private fun signal(diff: String) = EdictSignal(
    id = stableSignalId("bounded-diff"),
    idempotencyKey = "bounded-diff",
    fileRevision = EdictFileRevision("Example.kt", "a".repeat(40), listOf(EdictLineRange(10, 10))),
    source = EdictSignalSource.FromPR(1, "Fix call", listOf("Use the correct call"), diff, "https://example.test/pr/1"),
    label = EdictSignalLabel.POSITIVE,
    description = "Calls the wrong function",
    provenance = EdictSignalProvenance("pr-1"),
  )

  @Test
  fun `accepts exactly 4000 characters and rejects larger diffs without clipping them`() {
    val excerpt = "--- a/Example.kt\n+++ b/Example.kt\n@@ -10 +10 @@\n-bad()\n+good()\n"
    val full = excerpt.replace("bad()", "bad()" + "x".repeat(SignalValidation.MAX_DIFF_CHARS - excerpt.length))
    assertEquals(4000, full.length)
    assertEquals(signal(full), SignalValidation.validate(signal(full)))
    val failure = assertFailsWith<IllegalArgumentException> { SignalValidation.validate(signal(full + "\n")) }
    assertContains(failure.message.orEmpty(), "at most 4000 characters")
  }

  @Test
  fun `excerpt still has to cover every expected range on its label side`() {
    val excerpt = "--- a/Example.kt\n+++ b/Example.kt\n@@ -10 +10 @@\n-bad()\n+good()\n"
    val unrepresented = signal(excerpt).copy(
      fileRevision = signal(excerpt).fileRevision.copy(expectedRanges = listOf(EdictLineRange(10, 10), EdictLineRange(30, 30))),
    )
    assertFailsWith<IllegalArgumentException> { SignalValidation.validate(unrepresented) }
  }

  @Test
  fun `a finding users rated on GitHub needs the comment it was rated on`() {
    val rated = signal("").copy(
      source = EdictSignalSource.GitHubEvaluatedFinding(
        "Rule", "Reports the rule.", "Breaks the rule", url = "", replies = emptyList(), inspectionDigest = "a".repeat(64),
      ),
    )

    val failure = assertFailsWith<IllegalArgumentException> { SignalValidation.validate(rated) }

    assertEquals(
      "Invalid signal 'inbox/${rated.id}.json': source: GitHubEvaluatedFinding requires inspectionName, problemMessage and url",
      failure.message,
    )
  }

  @Test
  fun `a finding users rated on GitHub needs the digest of the inspection bytes that reported it`() {
    val rated = signal("").copy(
      source = EdictSignalSource.GitHubEvaluatedFinding(
        "Rule", "Reports the rule.", "Breaks the rule", "https://github.com/owner/project/pull/7#discussion_r1", emptyList(), "rule",
      ),
    )

    val failure = assertFailsWith<IllegalArgumentException> { SignalValidation.validate(rated) }

    assertEquals(
      "Invalid signal 'inbox/${rated.id}.json': source.inspectionDigest: must be the SHA-256 hex of the inspection",
      failure.message,
    )
  }
}
