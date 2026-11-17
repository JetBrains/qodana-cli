// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.extraction.git

import kotlinx.serialization.Serializable
import org.jetbrains.qodana.edict.common.CommitInput
import org.jetbrains.qodana.edict.common.GitRepository
import org.jetbrains.qodana.edict.core.EdictFileRevision
import org.jetbrains.qodana.edict.core.EdictLineRange
import org.jetbrains.qodana.edict.core.EdictSignal
import org.jetbrains.qodana.edict.core.EdictSignalLabel
import org.jetbrains.qodana.edict.core.EdictSignalProvenance
import org.jetbrains.qodana.edict.core.EdictSignalSource
import org.jetbrains.qodana.edict.core.stableSignalId

@Serializable
data class SignalFinding(
    val label: EdictSignalLabel,
    val path: String,
    val ranges: List<EdictLineRange>,
    val description: String
)

/** Analyzers make semantic decisions; the host binds their findings to exact Git evidence and stable identities. */
fun interface CommitAnalyzer {
    fun analyze(commit: CommitInput, repository: GitRepository): List<SignalFinding>
}

class CommitSignalExtractor(private val repository: GitRepository, private val analyzer: CommitAnalyzer) {
    fun extract(revision: String): List<EdictSignal> {
        val commit = repository.commit(revision)
        val findings = analyzer.analyze(commit, repository)
        require(findings.distinct().size == findings.size) { "Duplicate findings" }
        return findings.mapIndexed { index, finding ->
            val evidenceRevision =
                if (finding.label == EdictSignalLabel.POSITIVE) commit.parentRevision else commit.commitRevision
            val key = listOf(
                commit.workItemId, index, finding.label, finding.path, evidenceRevision,
                finding.ranges.joinToString(",") { "${it.start}:${it.end}" }).joinToString("|")
            EdictSignal(
                id = stableSignalId(key),
                idempotencyKey = key,
                fileRevision = EdictFileRevision(finding.path, evidenceRevision, finding.ranges),
                source = EdictSignalSource.FromCommit(
                    commitRevision = commit.commitRevision,
                ),
                label = finding.label,
                description = finding.description,
                provenance = EdictSignalProvenance(commit.workItemId),
            ).also(repository::validateEvidence)
        }
    }
}
