package org.jetbrains.qodana.edict.signals

import org.jetbrains.qodana.edict.edictnext.EdictNextLineRange
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalLabel
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNull

class UnifiedDiffTest {
  private val header = "--- a/old.kt\n+++ b/new.kt\n"
  private val canonical = header + "@@ -10,5 +20,5 @@\n context\n-before\n+after\n more\n-last\n+fixed\n end\n"

  @Test
  fun `excerpt keeps relevant changes inside a split hunk with original coordinates`() {
    val excerpt = header + "@@ -13 +23 @@\n-last\n+fixed\n"
    UnifiedDiff.validateExcerpt(excerpt, canonical)
    assertEquals(mapOf("old.kt" to listOf(13)), UnifiedDiff.parse(excerpt).before)
    assertEquals(mapOf("new.kt" to listOf(23)), UnifiedDiff.parse(excerpt).after)
  }

  @Test
  fun `excerpt can retain several separated slices and omit unrelated files`() {
    val excerpt = header + "@@ -11 +21 @@\n-before\n+after\n@@ -13 +23 @@\n-last\n+fixed\n"
    val other = "--- a/other.kt\n+++ b/other.kt\n@@ -1 +1 @@\n-old\n+new\n"
    UnifiedDiff.validateExcerpt(excerpt, other + canonical)
  }

  @Test
  fun `excerpt handles zero length sides and missing final newline`() {
    UnifiedDiff.validateExcerpt(header + "@@ -11 +20,0 @@\n-before\n", canonical)
    UnifiedDiff.validateExcerpt(header + "@@ -11,0 +21 @@\n+after\n", canonical)
    val deletion = header + "@@ -1,1 +0,0 @@\n-gone\n\\ No newline at end of file\n"
    val addition = header + "@@ -0,0 +1,1 @@\n+new\n\\ No newline at end of file\n"
    UnifiedDiff.validateExcerpt(deletion, deletion)
    UnifiedDiff.validateExcerpt(addition, addition)
    assertEquals(listOf(1), UnifiedDiff.parse(addition).side(EdictNextSignalLabel.NEGATIVE)["new.kt"])
  }

  @Test
  fun `verifier rejects edited paths lines prefixes and coordinates`() {
    val excerpt = header + "@@ -11 +21 @@\n-before\n+after\n"
    listOf(
      excerpt.replace("old.kt", "invented.kt"),
      excerpt.replace("+after", "+invented"),
      excerpt.replace("-11 +21", "-12 +22"),
      excerpt.replace("-before\n+after", "+before\n-after"),
      header + "@@ -11 +21 @@\n-last\n+fixed\n",
    ).forEach { invalid ->
      assertFailsWith<IllegalArgumentException>(invalid) { UnifiedDiff.validateExcerpt(invalid, canonical) }
    }
  }

  @Test
  fun `verifier rejects character cutoffs and omitted source inside a hunk`() {
    assertFailsWith<IllegalArgumentException> { UnifiedDiff.validateExcerpt(canonical.take(canonical.length - 10), canonical) }
    assertFailsWith<IllegalArgumentException> {
      UnifiedDiff.validateExcerpt(header + "@@ -11,2 +21,2 @@\n-before\n+after\n-last\n+fixed\n", canonical)
    }
  }

  @Test
  fun `server excerpt keeps the changes near a Signal's lines with original coordinates`() {
    val large = header + "@@ -1,200 +1,201 @@\n" + (1..200).joinToString("\n", postfix = "\n") { line ->
      when (line) {
        100 -> "-old 100\n+new 100"
        150 -> " line 150\n+inserted"
        180 -> "-old 180\n+new 180"
        else -> " line $line"
      }
    }
    val near = { line: Int -> (line - 3..line + 3).filter { it != line }.map { " line $it" } }

    val positive = UnifiedDiff.excerpt(large, EdictNextSignalLabel.POSITIVE, listOf(EdictNextLineRange(100, 100)), 4000)!!
    assertEquals(
      header + "@@ -97,7 +97,7 @@\n" + (near(100).take(3) + listOf("-old 100", "+new 100") + near(100).drop(3)).joinToString("") { "$it\n" },
      positive,
    )
    UnifiedDiff.validateExcerpt(positive, large)
    val negative = UnifiedDiff.excerpt(large, EdictNextSignalLabel.NEGATIVE, listOf(EdictNextLineRange(151, 151)), 4000)!!
    assertEquals(header + "@@ -148,6 +148,7 @@\n", negative.lines().take(3).joinToString("\n") + "\n")
    UnifiedDiff.validateExcerpt(negative, large)
    assertEquals(mapOf("new.kt" to listOf(151)), UnifiedDiff.parse(negative).after)
    assertNull(UnifiedDiff.excerpt(large, EdictNextSignalLabel.POSITIVE, listOf(EdictNextLineRange(100, 100)), header.length + 20))
  }
}
