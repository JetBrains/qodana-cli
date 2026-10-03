// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.common

import kotlinx.serialization.Serializable
import org.jetbrains.qodana.edict.edictnext.EdictNextSignal
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalLabel
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalSource
import org.jetbrains.qodana.edict.signals.UnifiedDiff
import org.jetbrains.qodana.edict.signals.validRevision
import org.jetbrains.qodana.edict.signals.validSourcePath
import java.nio.file.Path

@Serializable
data class CommitInput(val commitRevision: String, val parentRevision: String, val message: String, val diff: String) {
    val workItemId: String get() = "commit-${commitRevision.take(16)}"
}

class GitRepository(directory: Path) {
    val root: Path = Path.of(runProcess(directory, listOf("git", "rev-parse", "--show-toplevel")).trim())
    fun git(vararg arguments: String): String =
        runProcess(root, listOf("git", "--no-pager", "--literal-pathspecs") + arguments)

    fun resolve(revision: String): String {
        require(revision.isNotBlank() && revision.none { it in "\u0000\r\n" }) { "Revision must be one nonblank expression" }
        return git("rev-parse", "--verify", "--end-of-options", "$revision^{commit}").trim()
            .also { require(validRevision(it)) }
    }

    fun fileAt(revision: String, path: String): String {
        require(validRevision(revision) && validSourcePath(path)) { "Requires exact revision and repository-relative path" }
        return git("show", "$revision:$path")
    }

    fun diff(before: String, after: String, paths: List<String> = emptyList()): String {
        require(validRevision(before) && validRevision(after) && paths.all(::validSourcePath))
        return git(
            "diff",
            "--no-color",
            "--no-ext-diff",
            "--no-textconv",
            "--unified=200",
            before,
            after,
            "--",
            *paths.toTypedArray()
        )
    }

    fun commit(revision: String): CommitInput {
        val full = resolve(revision)
        val parents = git("rev-list", "--parents", "-n", "1", full, "--").trim().split(' ').drop(1)
        require(parents.size == 1) { "Commit must have exactly one parent; root and merge commits are excluded" }
        return CommitInput(
            full,
            parents.single(),
            git("show", "-s", "--format=%B", full, "--").trimEnd('\n'),
            diff(parents.single(), full)
        )
    }

    fun commits(range: String, limit: Int): List<CommitInput> {
        require(limit in 1..1000) { "Commit limit must be 1..1000" }
        require(range.isNotBlank() && !range.startsWith('-') && range.none { it in "\u0000\r\n" }) { "Invalid commit range" }
        return git("rev-list", "--reverse", "--max-count=$limit", range, "--").lineSequence().filter(String::isNotBlank)
            .mapNotNull { ref ->
                val parentCount = git("rev-list", "--parents", "-n", "1", ref, "--").trim().split(' ').size - 1
                if (parentCount != 1) null else commit(ref).takeUnless {
                    it.message.startsWith("Revert ") || Regex("""(?i)^(?:\[bot]|dependabot|automated|auto-generated)""").containsMatchIn(
                        it.message
                    )
                }
            }.filter { runCatching { UnifiedDiff.parse(it.diff) }.isSuccess }.toList()
    }

    /** Validate that commit Signal evidence still matches the source repository exactly. */
    fun validateEvidence(signal: EdictNextSignal) {
        val signalSource = signal.source
        require(signalSource is EdictNextSignalSource.FromCommit) { "Expected commit signal" }
        val commit = commit(signalSource.commitRevision)
        val expectedRevision = when (signal.label) {
            EdictNextSignalLabel.POSITIVE -> commit.parentRevision
            EdictNextSignalLabel.NEGATIVE -> commit.commitRevision
        }
        require(signal.fileRevision.revision == expectedRevision) {
            "Signal revision does not match its ${signal.label} commit side"
        }
        val changes = UnifiedDiff.parse(commit.diff).side(signal.label)
        val changedLines = changes[signal.fileRevision.path]
        require(changedLines != null) { "Signal path is not changed on its ${signal.label} commit side" }
        val ranges = signal.fileRevision.expectedRanges.orEmpty()
        require(ranges.isNotEmpty() && ranges.all { it.start >= 1 && it.end >= it.start }) {
            "Signal requires valid nonempty evidence ranges"
        }
        require(ranges.all { range -> changedLines.any { it in range.start..range.end } }) {
            "Signal evidence range does not intersect its ${signal.label} commit diff"
        }
        val source = fileAt(signal.fileRevision.revision, signal.fileRevision.path)
        val lineCount = source.count { it == '\n' } + if (source.isNotEmpty() && !source.endsWith('\n')) 1 else 0
        require(ranges.all { it.end <= lineCount }) {
            "Evidence range exceeds historical source"
        }
    }
}
