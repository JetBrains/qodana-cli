// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.signals

import org.jetbrains.qodana.edict.edictnext.EdictNextLineRange
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalLabel
import java.io.ByteArrayOutputStream

/** Strict ordinary Git unified-diff parsing, without IntelliJ PatchReader. Counts distinguish headers from source. */
object UnifiedDiff {
    data class Changes(val before: Map<String, List<Int>>, val after: Map<String, List<Int>>) {
        fun side(label: EdictNextSignalLabel): Map<String, List<Int>> =
            if (label == EdictNextSignalLabel.POSITIVE) before else after
    }

    private val hunk = Regex("^@@ -(\\d+)(?:,(\\d+))? \\+(\\d+)(?:,(\\d+))? @@.*")

    private data class Line(val before: Int, val after: Int, val content: String)
    private data class Hunk(val beforePath: String, val afterPath: String, val lines: MutableList<Line> = mutableListOf())
    private data class ParsedDiff(val changes: Changes, val hunks: List<Hunk>)

    fun parse(diff: String): Changes = parseDiff(diff).changes

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
        label: EdictNextSignalLabel,
        ranges: List<EdictNextLineRange>,
        maxChars: Int,
        relatedBefore: List<EdictNextLineRange> = emptyList(),
        relatedAfter: List<EdictNextLineRange> = emptyList(),
    ): String? {
        val header = diff.lineSequence().takeWhile { !it.startsWith("@@") }.joinToString("") { "$it\n" }
        val lines = parseDiff(diff).hunks.flatMapIndexed { hunk, it -> it.lines.map { line -> hunk to line } }
        val positive = label == EdictNextSignalLabel.POSITIVE
        fun onSide(line: Line) = line.content[0] == ' ' || line.content[0] == (if (positive) '-' else '+')
        fun number(line: Line) = if (positive) line.before else line.after
        fun changed(line: Line) = line.content[0] == '-' || line.content[0] == '+'
        val focus = lines.indices.filter { i -> onSide(lines[i].second) && ranges.any { number(lines[i].second) in it.start..it.end } }
        val related = lines.indices.filter { i ->
            val line = lines[i].second
            line.content[0] in " -" && relatedBefore.any { line.before in it.start..it.end } ||
                line.content[0] in " +" && relatedAfter.any { line.after in it.start..it.end }
        }
        val selections = if (related.isEmpty()) listOf(focus) else listOf(focus + related, focus)
        return selections.flatMap { selected -> EXCERPT_LEVELS.map { selected to it } }.firstNotNullOfOrNull { (selected, level) ->
            val (context, radius) = level
            val near = lines.indices.filter { i ->
                changed(lines[i].second) &&
                    (focus.isEmpty() || radius == null || ranges.any { number(lines[i].second) in (it.start - radius)..(it.end + radius) })
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
            fun valid(condition: Boolean) = require(condition) { "Malformed unified diff at line ${index + 1}" }
            if (line == "\\ No newline at end of file") {
                valid(hunks.isNotEmpty() && hunks.last().lines.isNotEmpty())
                hunks.last().lines += Line(oldLine, newLine, line)
                return@forEachIndexed
            }
            if (oldRemaining > 0 || newRemaining > 0) {
                valid(line.isNotEmpty())
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

                    else -> valid(false)
                }
                valid(oldRemaining >= 0 && newRemaining >= 0)
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
                    valid(match != null && beforePath.isNotEmpty() && afterPath.isNotEmpty())
                    val values = match!!.groupValues
                    oldLine = values[1].toInt(); oldRemaining = values[2].ifEmpty { "1" }.toInt()
                    newLine = values[3].toInt(); newRemaining = values[4].ifEmpty { "1" }.toInt()
                    valid((oldRemaining == 0 || oldLine > 0) && (newRemaining == 0 || newLine > 0))
                    // A zero-length side names the line preceding the insertion/deletion.
                    if (oldRemaining == 0) oldLine++
                    if (newRemaining == 0) newLine++
                    hunks += Hunk(beforePath, afterPath)
                }

                line.startsWith('+') || line.startsWith('-') || line.startsWith(' ') -> valid(false)
            }
        }
        require(oldRemaining == 0 && newRemaining == 0) { "Truncated unified diff" }
        require(before.isNotEmpty() || after.isNotEmpty()) { "Unified diff must contain changed lines" }
        return ParsedDiff(Changes(before, after), hunks)
    }

    // Git core.quotePath encodes non-ASCII UTF-8 bytes as three-digit octal escapes.
    private fun unquotePath(value: String): String {
        if (!value.startsWith('"')) return value
        require(value.endsWith('"')) { "Malformed quoted Git path" }
        val bytes = ByteArrayOutputStream()
        var i = 1
        while (i < value.lastIndex) {
            val c = value[i++]
            if (c != '\\') {
                bytes.write(c.toString().toByteArray(Charsets.UTF_8)); continue
            }
            require(i < value.lastIndex) { "Incomplete Git path escape" }
            val escaped = value[i++]
            if (escaped in '0'..'7') {
                require(i + 1 < value.lastIndex && value[i] in '0'..'7' && value[i + 1] in '0'..'7') { "Invalid Git octal escape" }
                bytes.write("$escaped${value[i++]}${value[i++]}".toInt(8))
            } else bytes.write(
                when (escaped) {
                    '\\' -> 92; '"' -> 34; 't' -> 9; 'n' -> 10; 'r' -> 13; 'a' -> 7; 'b' -> 8; 'f' -> 12; 'v' -> 11
                    else -> error("Invalid Git path escape")
                }
            )
        }
        return bytes.toString(Charsets.UTF_8)
    }
}
