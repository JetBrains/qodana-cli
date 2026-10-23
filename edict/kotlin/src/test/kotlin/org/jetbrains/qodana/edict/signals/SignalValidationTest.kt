package org.jetbrains.qodana.edict.signals

import org.jetbrains.qodana.edict.edictnext.*
import kotlin.test.Test
import kotlin.test.assertContains
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

class SignalValidationTest {
  private fun signal(diff: String) = EdictNextSignal(
    id = stableSignalId("bounded-diff"),
    idempotencyKey = "bounded-diff",
    fileRevision = EdictNextFileRevision("Example.kt", "a".repeat(40), listOf(EdictNextLineRange(10, 10))),
    source = EdictNextSignalSource.FromPR(1, "Fix call", listOf("Use the correct call"), diff, "https://example.test/pr/1"),
    label = EdictNextSignalLabel.POSITIVE,
    description = "Calls the wrong function",
    provenance = EdictNextSignalProvenance("pr-1"),
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
      fileRevision = signal(excerpt).fileRevision.copy(expectedRanges = listOf(EdictNextLineRange(10, 10), EdictNextLineRange(30, 30))),
    )
    assertFailsWith<IllegalArgumentException> { SignalValidation.validate(unrepresented) }
  }
}
