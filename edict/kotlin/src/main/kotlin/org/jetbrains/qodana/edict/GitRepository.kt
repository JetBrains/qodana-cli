// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict

import kotlinx.serialization.Serializable
import java.nio.file.Path
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit

/** Argument-array execution only: repository evidence is never interpolated into a shell. */
internal fun runProcess(directory: Path, arguments: List<String>, timeoutSeconds: Long = 60, maxBytes: Int = 16 * 1024 * 1024): String {
    val process = ProcessBuilder(arguments).directory(directory.toFile()).start()
    process.outputStream.close()
    val executor = Executors.newFixedThreadPool(2)
    try {
        fun read(stream: java.io.InputStream) = stream.use {
            val bytes = it.readNBytes(maxBytes + 1)
            if (bytes.size > maxBytes) { process.destroyForcibly(); error("Process output exceeded $maxBytes bytes; refusing truncated evidence") }
            bytes.toString(Charsets.UTF_8)
        }
        val out = executor.submit<String> { read(process.inputStream) }
        val err = executor.submit<String> { read(process.errorStream) }
        check(process.waitFor(timeoutSeconds, TimeUnit.SECONDS)) { "Process timed out after $timeoutSeconds seconds" }
        val stdout = out.get()
        val stderr = err.get()
        check(process.exitValue() == 0) { "${arguments.first()} exited ${process.exitValue()}: ${stderr.take(2000)}" }
        return stdout
    } finally {
        if (process.isAlive) {
            process.descendants().forEach { it.destroyForcibly() }
            process.destroyForcibly()
        }
        executor.shutdownNow()
    }
}

@Serializable data class CommitInput(val commitRevision: String, val parentRevision: String, val message: String, val diff: String) {
    val workItemId: String get() = "commit-${commitRevision.take(16)}"
}

class GitRepository(directory: Path) {
    val root: Path = Path.of(runProcess(directory, listOf("git", "rev-parse", "--show-toplevel")).trim())
    fun git(vararg arguments: String): String = runProcess(root, listOf("git", "--no-pager", "--literal-pathspecs") + arguments)

    fun resolve(revision: String): String {
        require(revision.isNotBlank() && revision.none { it in "\u0000\r\n" }) { "Revision must be one nonblank expression" }
        return git("rev-parse", "--verify", "--end-of-options", "$revision^{commit}").trim().also { require(validRevision(it)) }
    }

    fun fileAt(revision: String, path: String): String {
        require(validRevision(revision) && validSourcePath(path)) { "Requires exact revision and repository-relative path" }
        return git("show", "$revision:$path")
    }

    fun diff(before: String, after: String, paths: List<String> = emptyList()): String {
        require(validRevision(before) && validRevision(after) && paths.all(::validSourcePath))
        return git("diff", "--no-color", "--no-ext-diff", "--no-textconv", "--unified=200", before, after, "--", *paths.toTypedArray())
    }

    fun commit(revision: String): CommitInput {
        val full = resolve(revision)
        val parents = git("rev-list", "--parents", "-n", "1", full, "--").trim().split(' ').drop(1)
        require(parents.size == 1) { "Commit must have exactly one parent; root and merge commits are excluded" }
        return CommitInput(full, parents.single(), git("show", "-s", "--format=%B", full, "--").trimEnd('\n'), diff(parents.single(), full))
    }

    fun commits(range: String, limit: Int): List<CommitInput> {
        require(limit in 1..1000) { "Commit limit must be 1..1000" }
        require(range.isNotBlank() && !range.startsWith('-') && range.none { it in "\u0000\r\n" }) { "Invalid commit range" }
        return git("rev-list", "--reverse", "--max-count=$limit", range, "--").lineSequence().filter(String::isNotBlank).mapNotNull { ref ->
            val parentCount = git("rev-list", "--parents", "-n", "1", ref, "--").trim().split(' ').size - 1
            if (parentCount != 1) null else commit(ref).takeUnless {
                it.message.startsWith("Revert ") || Regex("""(?i)^(?:\[bot]|dependabot|automated|auto-generated)""").containsMatchIn(it.message)
            }
        }.filter { runCatching { UnifiedDiff.parse(it.diff) }.isSuccess }.toList()
    }

    /** Validate source authenticity as well as the structural validation performed on every state write. */
    fun validateEvidence(signal: Signal) {
        SignalValidation.validate("inbox/${signal.id}.json", json.encodeToString(Signal.serializer(), signal))
        require(signal.source.type == "FromCommit") { "Expected commit signal" }
        val commit = commit(signal.source.commitRevision!!)
        require(signal.source.parentRevision == commit.parentRevision && signal.source.message?.trimEnd('\n') == commit.message) { "Commit metadata differs from repository" }
        val changes = UnifiedDiff.parse(signal.source.diffPositiveToNegative)
        val paths = (changes.before.keys + changes.after.keys).distinct()
        require(signal.source.diffPositiveToNegative == commit.diff ||
            signal.source.diffPositiveToNegative == diff(commit.parentRevision, commit.commitRevision, paths)) { "Signal diff is not canonical Git evidence" }
        val source = fileAt(signal.fileRevision.revision, signal.fileRevision.path)
        val lineCount = source.count { it == '\n' } + if (source.isNotEmpty() && !source.endsWith('\n')) 1 else 0
        require(signal.fileRevision.expectedRanges.all { it.end <= lineCount }) { "Evidence range exceeds historical source" }
    }
}

@Serializable data class SignalFinding(val label: SignalLabel, val path: String, val ranges: List<SignalRange>, val description: String)

/** Analyzers make semantic decisions; the host binds their findings to exact Git evidence and stable identities. */
fun interface CommitAnalyzer { fun analyze(commit: CommitInput, repository: GitRepository): List<SignalFinding> }

class CommitSignalExtractor(private val repository: GitRepository, private val analyzer: CommitAnalyzer) {
    fun extract(revision: String): List<Signal> {
        val commit = repository.commit(revision)
        val findings = analyzer.analyze(commit, repository)
        require(findings.distinct().size == findings.size) { "Duplicate findings" }
        return findings.mapIndexed { index, finding ->
            val evidenceRevision = if (finding.label == SignalLabel.POSITIVE) commit.parentRevision else commit.commitRevision
            val key = listOf(commit.workItemId, index, finding.label, finding.path, evidenceRevision,
                finding.ranges.joinToString(",") { "${it.start}:${it.end}" }).joinToString("|")
            Signal(stableSignalId(key), key, FileRevision(finding.path, evidenceRevision, finding.ranges),
                SignalSource("FromCommit", commit.diff, commit.commitRevision, commit.parentRevision, commit.message),
                finding.label, finding.description, provenance = Provenance(commit.workItemId)).also(repository::validateEvidence)
        }
    }
}
