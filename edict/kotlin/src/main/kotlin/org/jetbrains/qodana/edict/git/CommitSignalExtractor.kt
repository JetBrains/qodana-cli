// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.git

import kotlinx.serialization.Serializable
import org.jetbrains.qodana.edict.model.*

@Serializable
data class SignalFinding(
    val label: SignalLabel,
    val path: String,
    val ranges: List<SignalRange>,
    val description: String
)

/** Analyzers make semantic decisions; the host binds their findings to exact Git evidence and stable identities. */
fun interface CommitAnalyzer {
    fun analyze(commit: CommitInput, repository: GitRepository): List<SignalFinding>
}

class CommitSignalExtractor(private val repository: GitRepository, private val analyzer: CommitAnalyzer) {
    fun extract(revision: String): List<Signal> {
        val commit = repository.commit(revision)
        val findings = analyzer.analyze(commit, repository)
        require(findings.distinct().size == findings.size) { "Duplicate findings" }
        return findings.mapIndexed { index, finding ->
            val evidenceRevision =
                if (finding.label == SignalLabel.POSITIVE) commit.parentRevision else commit.commitRevision
            val key = listOf(
                commit.workItemId, index, finding.label, finding.path, evidenceRevision,
                finding.ranges.joinToString(",") { "${it.start}:${it.end}" }).joinToString("|")
            Signal(
                stableSignalId(key), key, FileRevision(finding.path, evidenceRevision, finding.ranges),
                SignalSource("FromCommit", commit.diff, commit.commitRevision, commit.parentRevision, commit.message),
                finding.label, finding.description, provenance = Provenance(commit.workItemId)
            ).also(repository::validateEvidence)
        }
    }
}
