// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.integration

import java.nio.file.Files
import kotlin.io.path.readText
import kotlin.test.assertContains
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertTrue
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.jetbrains.qodana.edict.common.sha256
import org.jetbrains.qodana.edict.common.wireJson
import org.jetbrains.qodana.edict.edictnext.EdictNextClusterStatus
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState.Plan
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalLabel
import org.jetbrains.qodana.edict.edictnext.EdictRepository
import org.jetbrains.qodana.edict.edictnext.EdictRepositoryDirectory
import org.jetbrains.qodana.edict.integration.support.*
import org.jetbrains.qodana.edict.integration.support.inspection.InspectionServer
import org.jetbrains.qodana.edict.runtime.CodexRunner
import org.junit.jupiter.api.Test

class LivePipelineTest : IntegrationTest() {
    override val fixtureRevision = threeCommitHead
    override val fixtureProject = threeCommitProject

    @Test
    fun `managed skills extract cluster and generate an inspection from Distillery evidence`() {
        val expected = threeCommitExpectations().last()
        InspectionServer.start(workspace).use { inspection ->
            workspace.withCodex(
                """
                Use edict_manager and execute exactly three top-level tasks in this order:
                1. edict-batch-signal-analysis: extract Signals from exactly $threeCommitHead^! with commit limit 1.
                2. edict-next-distribution: distribute every extracted inbox Signal.
                3. edict-next-generation: generate and validate inspections for every Pending cluster.

                Source checkout: ${workspace.repository.root}
                Inspected IntelliJ project: ${workspace.project}
                Generation scratch root: ${workspace.output.resolve("scratch/pipeline-generation")}
                Publish extracted Signals only through edict_publish_signal. Follow the managed protocol for every worker.
                """.trimIndent(),
                inspectionServer = inspection,
                timeoutMinutes = 180,
            ) { store, runtime, _ ->
                val plan = assertNotNull(store.plan())
                val required = setOf(
                    "edict-batch-signal-analysis",
                    "edict-signal-analysis",
                    "edict-next-distribution",
                    "edict-next-generation",
                    "edict-next-cluster-generation",
                    "edict-next-code-example-overseer",
                    "edict-next-inspection-shallow-review",
                    "edict-next-weak-signal-review",
                    "edict-next-inspection-code-review",
                )
                assertTrue(
                    plan.tasks.map { it.skill }.toSet().containsAll(required),
                    "Pipeline must execute extraction, distribution, generation, example reconciliation and all three reviews",
                )
                verifyManagedRun(workspace, runtime, plan)
                verifySequentialStages(plan)
                verifyGeneratedCluster(runtime, inspection, expected)
            }
        }
    }

    private fun verifySequentialStages(plan: Plan) {
        val stages = plan.tasks.filter { it.parentId.isEmpty() }
        assertEquals(
            listOf("edict-batch-signal-analysis", "edict-next-distribution", "edict-next-generation"),
            stages.map { it.skill },
        )
        val ids = stages.map { it.id }
        // Lifecycle calls return compact acknowledgements, so the task log is the record of stage order.
        val events = Files.readAllLines(workspace.layout.tasksLogPath)
        ids.forEachIndexed { index, id ->
            val started = events.indexOfFirst { it.startsWith("[-:$id] ") && it.endsWith(" started") }
            val finished = events.indexOfFirst { it.startsWith("[-:$id] ") && it.endsWith(" finished") }
            assertTrue(started >= 0, "Stage $id has no recorded running state")
            assertTrue(finished > started, "Stage $id did not finish after it started")
            if (index > 0) {
                val previous = ids[index - 1]
                val previousFinished = events.indexOfFirst { it.startsWith("[-:$previous] ") && it.endsWith(" finished") }
                assertTrue(started > previousFinished, "Stage $id started before previous stage $previous finished")
            }
        }
    }

    private fun verifyGeneratedCluster(
        runtime: CodexRunner,
        inspection: InspectionServer,
        expected: CommitExpectation,
    ) {
        val repository = EdictRepository(EdictRepositoryDirectory(workspace.state))
        val state = runBlocking { repository.loadState() }
        assertTrue(state.inboxSignals.isEmpty(), "Distribution must consume every extracted Signal")
        assertEquals(1, state.clusters.size, "The positive/negative pair must form exactly one cluster")
        val cluster = state.clusters.single()
        assertEquals(EdictNextClusterStatus.Generated, cluster.manifest.status)
        assertEquals("Java", cluster.manifest.language.name)
        verifyCommitSignals(workspace.repository, signalFiles(cluster.directory.signalsDirectory), listOf(expected))

        val code = assertNotNull(state.inspectionCode(cluster.id))
        assertTrue(code.isNotBlank())
        assertEquals(
            listOf("inspections/${cluster.id}.inspection.kts"),
            state.filesByRelativePath.keys.filter { it.startsWith("inspections/") }.sorted(),
            "Only the accepted inspection may remain, with no candidate",
        )
        assertTrue(cluster.historyPath.readText().isNotBlank())

        val contextPath = expected.path.removePrefix("$threeCommitProject/")
        val originalPositive = inspection.run(code, contextPath, workspace.repository.fileAt(expected.parent, expected.path))
        assertContains(originalPositive.problemLines, expected.positiveLine, "Accepted inspection must detect the original problem")
        val originalNegative = inspection.run(code, contextPath, workspace.repository.fileAt(expected.revision, expected.path))
        assertTrue(originalNegative.problemLines.isEmpty(), "Accepted inspection must not flag the corrected revision")

        cluster.signals.forEach { signal ->
            val exampleId = assertNotNull(signal.syntheticExampleId, "Generation must assign every Signal a measured example")
            val example = cluster.examples.single { it.metadata.id == exampleId }
            val measured = inspection.run(code, contextPath, example.code)
            when (signal.label) {
                EdictNextSignalLabel.POSITIVE -> {
                    val ranges = assertNotNull(example.metadata.expectedRanges)
                    assertEquals(1, ranges.size)
                    assertEquals(1, measured.problemLines.size, "Positive example must contain exactly one detected problem")
                    assertTrue(measured.problemLines.single() in ranges.single().start..ranges.single().end)
                }
                EdictNextSignalLabel.NEGATIVE -> {
                    assertTrue(example.metadata.expectedRanges.isNullOrEmpty())
                    assertTrue(measured.problemLines.isEmpty(), "Negative example must not be reported")
                }
            }
        }
        verifyGenerationEvidence(cluster.directory.evaluationPath, code)
    }

    private fun verifyGenerationEvidence(evaluationPath: java.nio.file.Path, code: String) {
        val evaluation = wireJson.parseToJsonElement(evaluationPath.readText()).jsonObject
        assertEquals(
            sha256(code),
            evaluation["inspectionHash"]?.jsonPrimitive?.content,
            "The final evaluation must score the exact persisted inspection",
        )
        val calls = Files.readAllLines(workspace.output.resolve("log/inspection-mcp.jsonl"))
            .map { wireJson.parseToJsonElement(it).jsonObject }
        assertTrue(calls.any { it["tool"]?.toString()?.contains("run_inspection_kts_examples") == true })
    }
}
