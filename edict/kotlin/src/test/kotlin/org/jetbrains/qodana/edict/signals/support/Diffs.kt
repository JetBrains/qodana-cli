// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.signals.support

import org.jetbrains.qodana.edict.core.EdictLineRange
import org.jetbrains.qodana.edict.support.resourceText

/** The unified diff `diffs/<name>.diff` of the test resources, for diffs whose exact text is the case. */
internal fun diff(name: String): String = resourceText("diffs/$name.diff")

/** The ranges of a Signal on the one line [line]. */
internal fun onLine(line: Int): List<EdictLineRange> = listOf(EdictLineRange(line, line))

/** A unified diff of `old.kt` to `new.kt`: its two path headers, then [lines] (hunk headers and hunk lines). */
internal fun oldToNew(vararg lines: String): String = (listOf("--- a/old.kt", "+++ b/new.kt") + lines).joinToString("") { "$it\n" }

/** The line Git writes after a last line without a newline. */
internal const val NO_NEWLINE = "\\ No newline at end of file"

/** One hunk of two changes, at lines 10–14 before and 20–24 after: the canonical diff the excerpts are cut from. */
internal val CANONICAL = oldToNew("@@ -10,5 +20,5 @@", " context", "-before", "+after", " more", "-last", "+fixed", " end")

/** The second change of [CANONICAL] as a hunk without counts. */
internal val SINGLE_LINE_HUNK = oldToNew("@@ -13 +23 @@", "-last", "+fixed")

/** The deletion of the first change of [CANONICAL] alone, with the zero-length after side naming line 20. */
internal val DELETION_SLICE = oldToNew("@@ -11 +20,0 @@", "-before")

/** [CANONICAL] cut inside its last changed line. */
internal val CUT_CANONICAL = CANONICAL.substringBefore("ixed\n")

/** A diff of another file, then [CANONICAL]. */
internal val OTHER_FILE_AND_CANONICAL = "--- a/other.kt\n+++ b/other.kt\n@@ -1 +1 @@\n-old\n+new\n$CANONICAL"

/** A diff that adds `new.kt` of two lines. */
internal const val ADDED_FILE = "--- /dev/null\n+++ b/new.kt\n@@ -0,0 +1,2 @@\n+first\n+second\n"

/** A change of the last line, which has no newline on either side. */
internal val CHANGE_WITHOUT_NEWLINE = oldToNew("@@ -1,3 +1,3 @@", " a", " b", "-c", NO_NEWLINE, "+d", NO_NEWLINE)

/** The deletion of a file's only line, which has no newline. */
internal val DELETION_WITHOUT_NEWLINE = oldToNew("@@ -1,1 +0,0 @@", "-gone", NO_NEWLINE)

/** A hunk of one context line and no change. */
internal val CONTEXT_ONLY = oldToNew("@@ -1 +1 @@", " same")

/** A diff whose old path opens a quote it never closes. */
internal const val UNCLOSED_QUOTE = "--- \"a/old.kt\n+++ b/new.kt\n@@ -1 +1 @@\n-old\n+new\n"

/** Excerpts of [CANONICAL] that keep its paths, lines and coordinates. */
internal enum class CanonicalExcerpt(val diff: String) {
  WHOLE_HUNK(CANONICAL),
  TWO_SLICES(oldToNew("@@ -11 +21 @@", "-before", "+after", "@@ -13 +23 @@", "-last", "+fixed")),
  DELETION(DELETION_SLICE),
  INSERTION(oldToNew("@@ -11,0 +21 @@", "+after")),
}

/** Would-be excerpts of [CANONICAL] that change it. */
internal enum class ForgedExcerpt(val diff: String) {
  EDITED_PATH(oldToNew("@@ -11 +21 @@", "-before", "+after").replace("a/old.kt", "a/invented.kt")),
  EDITED_LINE(oldToNew("@@ -11 +21 @@", "-before", "+invented")),
  SHIFTED_COORDINATES(oldToNew("@@ -12 +22 @@", "-before", "+after")),
  GAP_IN_HUNK(oldToNew("@@ -11,2 +21,2 @@", "-before", "+after", "-last", "+fixed")),
}

/** Diffs whose structure breaks at [line]. */
internal enum class MalformedDiff(val line: Int, val diff: String) {
  HUNK_WITHOUT_PATHS(1, "@@ -1 +1 @@\n-old\n+new\n"),
  HUNK_AFTER_FILE_HEADER(7, oldToNew("@@ -1 +1 @@", "-old", "+new", "diff --git a/B.kt b/B.kt", "@@ -1 +1 @@", "-old", "+new")),
  MARKER_BEFORE_LINES(3, oldToNew(NO_NEWLINE, "@@ -1 +1 @@", "-old", "+new")),
  LINE_ZERO_WITH_COUNT(3, oldToNew("@@ -0,1 +1 @@", "-old", "+new")),
  UNKNOWN_LINE_PREFIX(5, oldToNew("@@ -1,2 +1,2 @@", "-old", "*new")),
  EMPTY_LINE_IN_HUNK(5, oldToNew("@@ -1,2 +1,2 @@", "-old", "", "+new")),
  OVERLONG_HUNK(5, oldToNew("@@ -1 +1 @@", "-old", "-older", "+new")),
  LINE_AFTER_HUNK(6, oldToNew("@@ -1 +1 @@", "-old", "+new", "+extra")),
}

/**
 * A one-hunk diff of `old.kt` to `new.kt` over [lines] context lines `line <n>`, in which each line of [replaced]
 * reads `old <n>` before and `new <n>` after, and a line `inserted` follows each line of [insertedAfter].
 */
internal fun numberedDiff(lines: Int, replaced: Set<Int>, insertedAfter: Set<Int> = emptySet()): String =
  oldToNew(
    "@@ -1,$lines +1,${lines + insertedAfter.size} @@",
    *(1..lines).flatMap { n ->
      (if (n in replaced) listOf("-old $n", "+new $n") else listOf(" line $n")) + if (n in insertedAfter) listOf("+inserted") else emptyList()
    }.toTypedArray(),
  )

/** Lines 1–200, with changes at lines 100 and 180 and an insertion after line 150. */
internal val LARGE = numberedDiff(200, replaced = setOf(100, 180), insertedAfter = setOf(150))

/** Lines 1–40, with changes at lines 10 and 20. */
internal val NEARBY_CHANGES = numberedDiff(40, replaced = setOf(10, 20))
