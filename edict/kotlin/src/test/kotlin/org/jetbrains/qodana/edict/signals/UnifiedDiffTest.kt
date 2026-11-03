// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.signals

import org.jetbrains.qodana.edict.edictnext.EdictNextLineRange
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalLabel.NEGATIVE
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalLabel.POSITIVE
import org.jetbrains.qodana.edict.signals.UnifiedDiff.Changes
import org.jetbrains.qodana.edict.signals.support.ADDED_FILE
import org.jetbrains.qodana.edict.signals.support.CANONICAL
import org.jetbrains.qodana.edict.signals.support.CHANGE_WITHOUT_NEWLINE
import org.jetbrains.qodana.edict.signals.support.CONTEXT_ONLY
import org.jetbrains.qodana.edict.signals.support.CUT_CANONICAL
import org.jetbrains.qodana.edict.signals.support.CanonicalExcerpt
import org.jetbrains.qodana.edict.signals.support.DELETION_SLICE
import org.jetbrains.qodana.edict.signals.support.DELETION_WITHOUT_NEWLINE
import org.jetbrains.qodana.edict.signals.support.ForgedExcerpt
import org.jetbrains.qodana.edict.signals.support.LARGE
import org.jetbrains.qodana.edict.signals.support.MalformedDiff
import org.jetbrains.qodana.edict.signals.support.NEARBY_CHANGES
import org.jetbrains.qodana.edict.signals.support.OTHER_FILE_AND_CANONICAL
import org.jetbrains.qodana.edict.signals.support.SINGLE_LINE_HUNK
import org.jetbrains.qodana.edict.signals.support.UNCLOSED_QUOTE
import org.jetbrains.qodana.edict.signals.support.diff
import org.jetbrains.qodana.edict.signals.support.onLine
import org.jetbrains.qodana.edict.support.afterSource
import org.jetbrains.qodana.edict.support.beforeSource
import org.jetbrains.qodana.edict.support.fixturePath
import org.junit.jupiter.params.ParameterizedTest
import org.junit.jupiter.params.provider.EnumSource
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNull

internal class UnifiedDiffTest {
  @Test
  fun `each side's changed lines are numbered by its file`() {
    assertEquals(Changes(mapOf("old.kt" to listOf(11, 13)), mapOf("new.kt" to listOf(21, 23))), UnifiedDiff.parse(CANONICAL))
  }

  @Test
  fun `a hunk without counts has one line on each side`() {
    assertEquals(Changes(mapOf("old.kt" to listOf(13)), mapOf("new.kt" to listOf(23))), UnifiedDiff.parse(SINGLE_LINE_HUNK))
  }

  @Test
  fun `a zero-length side names the line before the change, so a deletion has no changed line after`() {
    assertEquals(Changes(mapOf("old.kt" to listOf(11)), emptyMap()), UnifiedDiff.parse(DELETION_SLICE))
  }

  @Test
  fun `an added file has no changed line before`() {
    assertEquals(Changes(emptyMap(), mapOf("new.kt" to listOf(1, 2))), UnifiedDiff.parse(ADDED_FILE))
  }

  @Test
  fun `a missing-newline marker is not a line of either side`() {
    assertEquals(Changes(mapOf("old.kt" to listOf(3)), mapOf("new.kt" to listOf(3))), UnifiedDiff.parse(CHANGE_WITHOUT_NEWLINE))
  }

  @Test
  fun `a quoted path is unquoted with its octal escapes as UTF-8`() {
    // A one-line change; the a/ and b/ paths are Git-quoted, with octal escapes for the "é" of "café".
    assertEquals(
      Changes(mapOf("src/café 1.kt" to listOf(1)), mapOf("src/café 2.kt" to listOf(1))),
      UnifiedDiff.parse(diff("quoted-paths")),
    )
  }

  @Test
  fun `the files of a Git diff are parsed in order, ignoring its extended headers`() {
    // Git diff of A.kt (with an index header, change on line 2) and B.kt (hunk at line 5, change on line 6).
    assertEquals(
      Changes(mapOf("A.kt" to listOf(2), "B.kt" to listOf(6)), mapOf("A.kt" to listOf(2), "B.kt" to listOf(6))),
      UnifiedDiff.parse(diff("two-files")),
    )
  }

  @Test
  fun `the Signal's label picks its side of the changes`() {
    val changes = UnifiedDiff.parse(CANONICAL)

    assertEquals(mapOf("old.kt" to listOf(11, 13)), changes.side(POSITIVE))
    assertEquals(mapOf("new.kt" to listOf(21, 23)), changes.side(NEGATIVE))
  }

  @ParameterizedTest
  @EnumSource(MalformedDiff::class)
  fun `a line that breaks the diff's structure is reported by number`(malformed: MalformedDiff) {
    val error = assertFailsWith<IllegalArgumentException> { UnifiedDiff.parse(malformed.diff) }

    assertEquals("Malformed unified diff at line ${malformed.line}", error.message)
  }

  @Test
  fun `a diff cut inside a hunk's last line is truncated`() {
    val error = assertFailsWith<IllegalArgumentException> { UnifiedDiff.parse(CUT_CANONICAL) }

    assertEquals("Truncated unified diff", error.message)
  }

  @Test
  fun `a diff of context lines only is refused`() {
    val error = assertFailsWith<IllegalArgumentException> { UnifiedDiff.parse(CONTEXT_ONLY) }

    assertEquals("Unified diff must contain changed lines", error.message)
  }

  @Test
  fun `a quoted path without its closing quote is refused`() {
    val error = assertFailsWith<IllegalArgumentException> { UnifiedDiff.parse(UNCLOSED_QUOTE) }

    assertEquals("Malformed quoted Git path", error.message)
  }

  @ParameterizedTest
  @EnumSource(CanonicalExcerpt::class)
  fun `an excerpt of whole hunks or of slices recounted at their coordinates is valid`(excerpt: CanonicalExcerpt) {
    UnifiedDiff.validateExcerpt(excerpt.diff, CANONICAL)
  }

  @Test
  fun `an excerpt may leave out the other files of the diff`() {
    UnifiedDiff.validateExcerpt(CanonicalExcerpt.TWO_SLICES.diff, OTHER_FILE_AND_CANONICAL)
  }

  @Test
  fun `an excerpt keeps the missing-newline marker of its line`() {
    UnifiedDiff.validateExcerpt(DELETION_WITHOUT_NEWLINE, DELETION_WITHOUT_NEWLINE)
  }

  @ParameterizedTest
  @EnumSource(ForgedExcerpt::class)
  fun `an excerpt that changes a path, a line or a coordinate, or skips lines inside a hunk, is refused`(forged: ForgedExcerpt) {
    val error = assertFailsWith<IllegalArgumentException> { UnifiedDiff.validateExcerpt(forged.diff, CANONICAL) }

    assertEquals("Diff excerpt must preserve canonical paths, source lines and line coordinates", error.message)
  }

  @Test
  fun `an excerpt cut off by characters is refused`() {
    val error = assertFailsWith<IllegalArgumentException> { UnifiedDiff.validateExcerpt(CUT_CANONICAL, CANONICAL) }

    assertEquals("Truncated unified diff", error.message)
  }

  @Test
  fun `a positive excerpt keeps the change at its line with three context lines`() {
    // The hunk around line 100 of LARGE, with three context lines, and the file headers.
    assertEquals(diff("large-positive-100"), UnifiedDiff.excerpt(LARGE, POSITIVE, onLine(100), 4000))
  }

  @Test
  fun `a negative excerpt keeps the insertion at its line with three context lines`() {
    // The hunk around the insertion at line 151 of LARGE, with three context lines either side.
    assertEquals(diff("large-negative-151"), UnifiedDiff.excerpt(LARGE, NEGATIVE, onLine(151), 4000))
  }

  @Test
  fun `an excerpt keeps the changes within 20 lines of its line while they fit`() {
    // Two hunks, for the changes of lines 10 and 20, each with three context lines.
    assertEquals(diff("nearby-changes-with-neighbour"), UnifiedDiff.excerpt(NEARBY_CHANGES, POSITIVE, onLine(10), 197))
  }

  @Test
  fun `an excerpt that cannot fit the nearby changes keeps its line's change and three diff lines either side`() {
    // One hunk for the change of line 10, three diff lines either side of its removed line: three context lines before,
    // the added line and two context lines after. The change of line 20 is dropped.
    assertEquals(diff("nearby-changes-three-context-lines"), UnifiedDiff.excerpt(NEARBY_CHANGES, POSITIVE, onLine(10), 196))
  }

  @Test
  fun `an excerpt that cannot fit three context lines keeps one`() {
    // One hunk for the change of line 10 with a single context line, before it.
    assertEquals(diff("nearby-changes-one-context-line"), UnifiedDiff.excerpt(NEARBY_CHANGES, POSITIVE, onLine(10), 99))
  }

  @Test
  fun `an excerpt that cannot fit one context line keeps only its line`() {
    // One hunk holding only the removed line 10, with no context lines.
    assertEquals(diff("nearby-changes-only-line"), UnifiedDiff.excerpt(NEARBY_CHANGES, POSITIVE, onLine(10), 65))
  }

  @Test
  fun `an excerpt that cannot fit even its line is null`() {
    assertNull(UnifiedDiff.excerpt(NEARBY_CHANGES, POSITIVE, onLine(10), 50))
  }

  @Test
  fun `an excerpt keeps the missing-newline marker with its line`() {
    // The deletion of line 3 followed by the "No newline at end of file" marker.
    assertEquals(diff("change-without-newline-only-line"), UnifiedDiff.excerpt(CHANGE_WITHOUT_NEWLINE, POSITIVE, onLine(3), 75))
  }

  @Test
  fun `an excerpt whose lines are outside every hunk keeps every change`() {
    // Three hunks, for the change of line 100, the insertion at line 151 and the change of line 180, each with three
    // context lines.
    assertEquals(diff("large-every-change"), UnifiedDiff.excerpt(LARGE, POSITIVE, listOf(EdictNextLineRange(500, 600)), 4000))
  }

  @Test
  fun `a diff keeps the requested number of context lines around the change`() {
    // The hunk of the change on line 3 with one context line either side.
    assertEquals(diff("between-one-context-line"), UnifiedDiff.between(beforeSource, afterSource, fixturePath, fixturePath, 1))
  }

  @Test
  fun `an unchanged file's diff is empty`() {
    assertEquals("", UnifiedDiff.between(beforeSource, beforeSource, fixturePath, fixturePath, 3))
  }

  @Test
  fun `an added file is diffed from dev null`() {
    // A new file: headers from /dev/null and one hunk of five added lines.
    assertEquals(diff("between-added"), UnifiedDiff.between(null, afterSource, fixturePath, fixturePath, 3))
  }

  @Test
  fun `a deleted file is diffed to dev null`() {
    // A deleted file: headers to /dev/null and one hunk of five removed lines.
    assertEquals(diff("between-deleted"), UnifiedDiff.between(beforeSource, null, fixturePath, fixturePath, 3))
  }

  @Test
  fun `a moved file is diffed between its two paths`() {
    // Headers name Equality.java before and Strings.java after; one hunk with the change on line 3.
    assertEquals(
      diff("between-renamed"),
      UnifiedDiff.between(beforeSource, afterSource, "module/src/Equality.java", "module/src/Strings.java", 3),
    )
  }

  @Test
  fun `a path with non-ASCII or control characters is quoted as Git quotes it`() {
    // Git-quoted headers, "src/caf\303\251 1.kt" and "src/tab\there.kt"; one hunk with the change on line 3.
    assertEquals(diff("between-quoted-paths"), UnifiedDiff.between(beforeSource, afterSource, "src/café 1.kt", "src/tab\there.kt", 3))
  }

  @Test
  fun `a file absent at both revisions is refused`() {
    val error = assertFailsWith<IllegalArgumentException> { UnifiedDiff.between(null, null, fixturePath, fixturePath, 3) }

    assertEquals("File absent at both revisions", error.message)
  }

  @Test
  fun `a binary file is refused`() {
    val error = assertFailsWith<IllegalArgumentException> { UnifiedDiff.between("a\u0000b", "a", fixturePath, fixturePath, 3) }

    assertEquals("Binary files have no unified diff", error.message)
  }
}
