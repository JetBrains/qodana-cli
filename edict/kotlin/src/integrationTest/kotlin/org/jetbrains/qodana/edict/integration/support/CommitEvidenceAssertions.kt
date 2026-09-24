// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.integration.support

import org.jetbrains.qodana.edict.common.json
import org.jetbrains.qodana.edict.git.GitRepository
import org.jetbrains.qodana.edict.model.Plan
import org.jetbrains.qodana.edict.model.SignalLabel
import org.jetbrains.qodana.edict.model.StateFile
import org.jetbrains.qodana.edict.runtime.CodexRunner
import org.jetbrains.qodana.edict.signals.SignalValidation
import java.nio.file.Files
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

internal const val threeCommitProject = "testExtractSignalsFromThreeCommits"
internal const val threeCommitBaseline = "26b38d1203a6697bac3ec38659f25ed6e6f50f05"
internal const val threeCommitEquality = "b375279dbaa2dd53d64dd9047bdb596d0d5160e7"
internal const val threeCommitLocale = "4a448d1e6e30961c43024f9900f80e6d3d088b79"
internal const val threeCommitHead = "19475f69ff6ed87a68712b4ad9d55938f3868b6e"

internal data class CommitExpectation(
    val parent: String,
    val revision: String,
    val path: String,
    val positiveLine: Int = 5,
    val negativeLine: Int = 5,
)

internal fun threeCommitExpectations(): List<CommitExpectation> = listOf(
    Triple(threeCommitBaseline, threeCommitEquality, "RoleMatcher.java"),
    Triple(threeCommitEquality, threeCommitLocale, "KeyNormalizer.java"),
    Triple(threeCommitLocale, threeCommitHead, "TimeoutConverter.java"),
).map { (parent, revision, name) ->
    CommitExpectation(parent, revision, "$threeCommitProject/src/main/java/com/mycompany/app/$name")
}

/** Check the complete result set, including each evidence side and its original Git bytes. */
internal fun verifyCommitSignals(repository: GitRepository, files: List<StateFile>, expected: List<CommitExpectation>) {
    assertEquals(expected.size * 2, files.size, "Every selected commit must retain both evidence sides")
    val signals = files.map { file ->
        val signal = SignalValidation.validate(file.path, file.content)
        // Clustering may attach a generated example; historical evidence must stay byte-for-byte authentic.
        repository.validateEvidence(signal.copy(syntheticExampleId = null))
        assertEquals("${signal.id}.json", file.path.substringAfterLast('/'))
        assertTrue(signal.provenance.workItemId.isNotBlank())
        signal
    }
    assertEquals(signals.size, signals.map { it.id }.distinct().size, "Signal IDs must be unique")
    assertEquals(signals.size, signals.map { it.idempotencyKey }.distinct().size, "Evidence must not be duplicated")
    assertEquals(
        expected.map { it.revision }.toSet(),
        signals.map { it.source.commitRevision }.toSet(),
        "Commit selection differs"
    )
    expected.forEach { commit ->
        val pair = signals.filter { it.source.commitRevision == commit.revision }
        assertEquals(2, pair.size, "Signal count for ${commit.revision}")
        assertEquals(
            SignalLabel.entries.toSet(),
            pair.map { it.label }.toSet(),
            "Evidence labels for ${commit.revision}"
        )
        pair.forEach { signal ->
            assertEquals("FromCommit", signal.source.type)
            assertEquals(commit.parent, signal.source.parentRevision)
            assertEquals(commit.path, signal.fileRevision.path)
            assertEquals(
                repository.diff(commit.parent, commit.revision, listOf(commit.path)),
                signal.source.diffPositiveToNegative
            )
            val positive = signal.label == SignalLabel.POSITIVE
            assertEquals(if (positive) commit.parent else commit.revision, signal.fileRevision.revision)
            val line = if (positive) commit.positiveLine else commit.negativeLine
            assertTrue(
                signal.fileRevision.expectedRanges.any { line in it.start..it.end },
                "${signal.id} must cover corrected line $line"
            )
        }
    }
}

internal fun verifyManagedRun(workspace: IntegrationWorkspace, runtime: CodexRunner, plan: Plan) {
    assertTrue(plan.tasks.isNotEmpty(), "Manager must persist its execution plan")
    plan.tasks.forEach { task ->
        assertEquals("completed", task.status, "Incomplete ${task.skill} task ${task.id}")
        assertTrue(task.agentId.isNotBlank(), "Every managed task needs a native agent")
        assertTrue(task.result.isNotBlank(), "Completed ${task.skill} task must retain its result")
    }
    assertEquals(
        plan.tasks.size,
        plan.tasks.map { it.agentId }.distinct().size,
        "Each task must use a distinct native worker"
    )
    val persisted = json.decodeFromString<Plan>(Files.readString(workspace.state.resolve("plans/${plan.id}.json")))
    assertEquals(plan, persisted, "Completed plan must be persisted without lag")
    val log = Files.readString(workspace.logs.resolve("edict-mcp-system.log"))
    val lifecycle = Regex("] edict_(?:plan_create|delegate|task_[a-z]+) ")
    assertFalse(
        log.lineSequence().any { lifecycle.containsMatchIn(it) && it.contains("\"isError\":true") },
        "Managed lifecycle failed; inspect ${workspace.logs}"
    )
    assertFalse(log.contains("] edict_task_cancel "), "Healthy runs must not hide failed workers behind cancellation")
    assertFalse(log.lineSequence().any {
        it.contains("] edict_task_finish ") && it.substringBefore(" => ").contains("\"status\":\"failed\"")
    }, "Worker reported failure")
    verifyRuntimeWorkers(runtime.home.resolve("sessions"), plan)
    verifyAgentLogs(workspace.logs, plan)
}
