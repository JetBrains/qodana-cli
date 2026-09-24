// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.signals

import org.jetbrains.qodana.edict.model.SignalLabel
import java.io.ByteArrayOutputStream

/** Strict ordinary Git unified-diff parsing, without IntelliJ PatchReader. Counts distinguish headers from source. */
object UnifiedDiff {
    data class Changes(val before: Map<String, List<Int>>, val after: Map<String, List<Int>>) {
        fun side(label: SignalLabel): Map<String, List<Int>> = if (label == SignalLabel.POSITIVE) before else after
    }

    private val hunk = Regex("^@@ -(\\d+)(?:,(\\d+))? \\+(\\d+)(?:,(\\d+))? @@.*")

    fun parse(diff: String): Changes {
        val before = linkedMapOf<String, MutableList<Int>>()
        val after = linkedMapOf<String, MutableList<Int>>()
        var beforePath = ""
        var afterPath = ""
        var oldLine = 0
        var newLine = 0
        var oldRemaining = 0
        var newRemaining = 0
        diff.split('\n').forEachIndexed { index, line ->
            fun valid(condition: Boolean) = require(condition) { "Malformed unified diff at line ${index + 1}" }
            if (line == "\\ No newline at end of file") return@forEachIndexed
            if (oldRemaining > 0 || newRemaining > 0) {
                valid(line.isNotEmpty())
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
                }

                line.startsWith('+') || line.startsWith('-') || line.startsWith(' ') -> valid(false)
            }
        }
        require(oldRemaining == 0 && newRemaining == 0) { "Truncated unified diff" }
        require(before.isNotEmpty() || after.isNotEmpty()) { "Unified diff must contain changed lines" }
        return Changes(before, after)
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
