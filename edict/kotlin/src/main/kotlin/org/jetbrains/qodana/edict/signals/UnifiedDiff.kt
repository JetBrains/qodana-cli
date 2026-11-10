// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.signals

import org.eclipse.jgit.diff.DiffAlgorithm
import org.eclipse.jgit.diff.DiffFormatter
import org.eclipse.jgit.diff.RawText
import org.eclipse.jgit.diff.RawTextComparator
import org.eclipse.jgit.util.QuotedString
import org.jetbrains.qodana.edict.core.EdictLineRange
import org.jetbrains.qodana.edict.core.EdictSignalLabel
import java.io.ByteArrayOutputStream

/** Strict ordinary Git unified-diff parsing, without IntelliJ PatchReader. Counts distinguish headers from source. */
object UnifiedDiff {
    data class Changes(val before: Map<String, List<Int>>, val after: Map<String, List<Int>>) {
        fun side(label: EdictSignalLabel): Map<String, List<Int>> =
            if (label == EdictSignalLabel.POSITIVE) before else after
    }

    private val hunk = Regex("^@@ -(\\d+)(?:,(\\d+))? \\+(\\d+)(?:,(\\d+))? @@.*")

    private data class Line(val before: Int, val after: Int, val content: String)
    private data class Hunk(val beforePath: String, val afterPath: String, val lines: MutableList<Line> = mutableListOf())
    private data class ParsedDiff(val changes: Changes, val hunks: List<Hunk>)

    fun parse(diff: String): Changes = parseDiff(diff).changes

    /** The lines that [hunks], the hunks of one file without its file headers, as GitHub shows a patch, add on its new side. */
    fun addedLines(hunks: String): List<Int> = parseDiff("--- a/$HUNKS_PATH\n+++ b/$HUNKS_PATH\n$hunks").changes.after[HUNKS_PATH].orEmpty()

    /** The path [addedLines] gives the hunks it parses, which name none. */
    private const val HUNKS_PATH = "file"

    /**
     * The Git-format diff of one file from [before] to [after], with [context] lines around each change, or an empty
     * string when nothing changed. A null side is an absent file; paths are repository-relative and appear as `a/` and
     * `b/`, or `/dev/null` for an absent side.
     */
    fun between(before: String?, after: String?, beforePath: String, afterPath: String, context: Int): String {
        require(before != null || after != null) { "File absent at both revisions" }
        val old = RawText(before.orEmpty().toByteArray())
        val new = RawText(after.orEmpty().toByteArray())
        require(!RawText.isBinary(old.rawContent) && !RawText.isBinary(new.rawContent)) { "Binary files have no unified diff" }
        // Git's default algorithm.
        val edits = DiffAlgorithm.getAlgorithm(DiffAlgorithm.SupportedAlgorithm.MYERS).diff(RawTextComparator.DEFAULT, old, new)
        if (edits.isEmpty()) return ""
        val hunks = ByteArrayOutputStream()
        DiffFormatter(hunks).use {
            it.setContext(context)
            it.format(edits, old, new)
        }
        return "--- ${QuotedString.GIT_PATH.quote(if (before == null) "/dev/null" else "a/$beforePath")}\n" +
            "+++ ${QuotedString.GIT_PATH.quote(if (after == null) "/dev/null" else "b/$afterPath")}\n" + hunks.toString(Charsets.UTF_8)
    }

    /** Allow omitted hunks/context, including split hunks, but never edited lines, paths, or shifted coordinates. */
    fun validateExcerpt(excerpt: String, canonicalDiff: String) {
        val selected = parseDiff(excerpt)
        val canonical = parseDiff(canonicalDiff)
        selected.hunks.forEach { selectedHunk ->
            require(canonical.hunks.any { original ->
                original.beforePath == selectedHunk.beforePath && original.afterPath == selectedHunk.afterPath &&
                    original.lines.indices.any { start ->
                        val end = start + selectedHunk.lines.size
                        end <= original.lines.size && original.lines.subList(start, end) == selectedHunk.lines
                    }
            }) { "Diff excerpt must preserve canonical paths, source lines and line coordinates" }
        }
    }

    /**
     * Cuts [diff] to the Signal's [ranges] on its [label]'s side, the other changes in the file, the lines the worker
     * named as related ([relatedBefore], [relatedAfter]) and context around them, narrowing until it fits [maxChars]:
     * fewer changes, then less context, then without the related lines.
     */
    fun excerpt(
        diff: String,
        label: EdictSignalLabel,
        ranges: List<EdictLineRange>,
        maxChars: Int,
        relatedBefore: List<EdictLineRange> = emptyList(),
        relatedAfter: List<EdictLineRange> = emptyList(),
    ): String? {
        val header = diff.lineSequence().takeWhile { !it.startsWith("@@") }.joinToString("") { "$it\n" }
        val lines = parseDiff(diff).hunks.flatMapIndexed { hunk, it -> it.lines.map { line -> hunk to line } }
        val positive = label == EdictSignalLabel.POSITIVE
        val focus = lines.indices.filter { i ->
            lines[i].second.isOnSide(positive) && ranges.any { lines[i].second.number(positive) in it.start..it.end }
        }
        val related = lines.indices.filter { i ->
            val line = lines[i].second
            line.content[0] in " -" && relatedBefore.any { line.before in it.start..it.end } ||
                line.content[0] in " +" && relatedAfter.any { line.after in it.start..it.end }
        }
        val selections = if (related.isEmpty()) listOf(focus) else listOf(focus + related, focus)
        return selections.flatMap { selected -> EXCERPT_LEVELS.map { selected to it } }.firstNotNullOfOrNull { (selected, level) ->
            val (context, radius) = level
            val near = lines.indices.filter { i ->
                lines[i].second.isChange &&
                    (focus.isEmpty() || radius == null || ranges.any { lines[i].second.number(positive) in (it.start - radius)..(it.end + radius) })
            }
            val kept = BooleanArray(lines.size)
            (selected + near).forEach { i ->
                for (j in (i - context).coerceAtLeast(0)..(i + context).coerceAtMost(lines.lastIndex)) {
                    if (lines[j].first == lines[i].first) kept[j] = true
                }
            }
            // A missing-newline marker belongs to the line before it.
            lines.indices.filter { lines[it].second.content.startsWith("\\") }.forEach { kept[it] = kept[it - 1] }
            buildString {
                append(header)
                var start = 0
                while (start < lines.size) {
                    if (!kept[start]) { start++; continue }
                    var end = start
                    while (end + 1 < lines.size && kept[end + 1] && lines[end + 1].first == lines[start].first) end++
                    val run = lines.subList(start, end + 1).map { it.second }
                    val beforeCount = run.count { it.content[0] == ' ' || it.content[0] == '-' }
                    val afterCount = run.count { it.content[0] == ' ' || it.content[0] == '+' }
                    // A zero-length side names the line preceding the insertion/deletion, as parsing expects.
                    val beforeStart = run.first().before - if (beforeCount == 0) 1 else 0
                    val afterStart = run.first().after - if (afterCount == 0) 1 else 0
                    append("@@ -$beforeStart,$beforeCount +$afterStart,$afterCount @@\n")
                    run.forEach { append(it.content).append('\n') }
                    start = end + 1
                }
            }.takeIf { it.length <= maxChars && it.length > header.length }
        }
    }

    /** Whether this line is on the side of a [positive] or a negative example: unchanged, or removed or added. */
    private fun Line.isOnSide(positive: Boolean) = content[0] == ' ' || content[0] == (if (positive) '-' else '+')

    /** This line's number on the side of a [positive] or a negative example: before or after the change. */
    private fun Line.number(positive: Boolean) = if (positive) before else after

    private val Line.isChange get() = content[0] == '-' || content[0] == '+'

    private fun requireWellFormed(index: Int, condition: Boolean) =
        require(condition) { "Malformed unified diff at line ${index + 1}" }

    /**
     * Context lines and the radius around the ranges for other changes, from the widest excerpt to the narrowest; a null
     * radius keeps every change in the file.
     */
    private val EXCERPT_LEVELS = listOf(3 to null, 3 to 20, 3 to 0, 1 to 0, 0 to 0)

    private fun parseDiff(diff: String): ParsedDiff {
        val before = linkedMapOf<String, MutableList<Int>>()
        val after = linkedMapOf<String, MutableList<Int>>()
        val hunks = mutableListOf<Hunk>()
        var beforePath = ""
        var afterPath = ""
        var oldLine = 0
        var newLine = 0
        var oldRemaining = 0
        var newRemaining = 0
        diff.split('\n').forEachIndexed { index, line ->
            if (line == "\\ No newline at end of file") {
                requireWellFormed(index, hunks.isNotEmpty() && hunks.last().lines.isNotEmpty())
                hunks.last().lines += Line(oldLine, newLine, line)
                return@forEachIndexed
            }
            if (oldRemaining > 0 || newRemaining > 0) {
                requireWellFormed(index, line.isNotEmpty())
                hunks.last().lines += Line(oldLine, newLine, line)
                when (line[0]) {
                    ' ' -> {
                        oldRemaining--; newRemaining--; oldLine++; newLine++
                    }

                    '-' -> {
                        if (beforePath != "/dev/null") before.getOrPut(beforePath, ::mutableListOf).add(oldLine)
                        oldRemaining--; oldLine++
                    }

                    '+' -> {
                        if (afterPath != "/dev/null") after.getOrPut(afterPath, ::mutableListOf).add(newLine)
                        newRemaining--; newLine++
                    }

                    else -> requireWellFormed(index, false)
                }
                requireWellFormed(index, oldRemaining >= 0 && newRemaining >= 0)
            } else when {
                line.startsWith("diff --git ") -> {
                    beforePath = ""; afterPath = ""
                }

                line.startsWith("--- ") -> beforePath =
                    unquotePath(line.substring(4).removeSuffix("\t")).removePrefix("a/")

                line.startsWith("+++ ") -> afterPath =
                    unquotePath(line.substring(4).removeSuffix("\t")).removePrefix("b/")

                line.startsWith("@@") -> {
                    val match = hunk.matchEntire(line)
                    requireWellFormed(index, match != null && beforePath.isNotEmpty() && afterPath.isNotEmpty())
                    val values = match!!.groupValues
                    oldLine = values[1].toInt(); oldRemaining = values[2].ifEmpty { "1" }.toInt()
                    newLine = values[3].toInt(); newRemaining = values[4].ifEmpty { "1" }.toInt()
                    requireWellFormed(index, (oldRemaining == 0 || oldLine > 0) && (newRemaining == 0 || newLine > 0))
                    // A zero-length side names the line preceding the insertion/deletion.
                    if (oldRemaining == 0) oldLine++
                    if (newRemaining == 0) newLine++
                    hunks += Hunk(beforePath, afterPath)
                }

                line.startsWith('+') || line.startsWith('-') || line.startsWith(' ') -> requireWellFormed(index, false)
            }
        }
        require(oldRemaining == 0 && newRemaining == 0) { "Truncated unified diff" }
        require(before.isNotEmpty() || after.isNotEmpty()) { "Unified diff must contain changed lines" }
        return ParsedDiff(Changes(before, after), hunks)
    }

    /** A header path as Git writes it: quoted, with C-style and octal escapes, when it has special or non-ASCII bytes. */
    private fun unquotePath(value: String): String {
        require(!value.startsWith('"') || value.length >= 2 && value.endsWith('"')) { "Malformed quoted Git path" }
        return QuotedString.GIT_PATH.dequote(value)
    }
}

