// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.integration

import java.nio.file.Files
import kotlin.io.path.readText
import kotlin.test.assertContains
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertTrue
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.decodeFromJsonElement
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject
import org.jetbrains.qodana.edict.common.sha256
import org.jetbrains.qodana.edict.common.wireJson
import org.jetbrains.qodana.edict.edictnext.EdictNextClusterStatus
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState.Plan
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalLabel
import org.jetbrains.qodana.edict.edictnext.EdictRepository
import org.jetbrains.qodana.edict.edictnext.EdictRepositoryDirectory
import org.jetbrains.qodana.edict.integration.support.*
import org.jetbrains.qodana.edict.integration.support.inspection.InspectionServer
import org.jetbrains.qodana.edict.integration.support.inspection.acceptedCandidateReviews
import org.jetbrains.qodana.edict.integration.support.inspection.stageOrderProblems
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
                State repository: ${workspace.state}
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
                    "edict-next-inspection-code-review",
                    "edict-next-weak-signal-review",
                )
                assertTrue(
                    plan.tasks.map { it.skill }.toSet().containsAll(required),
                    "Pipeline must execute extraction, distribution, generation, example reconciliation and both reviews",
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
        val snapshots = Files.readAllLines(workspace.layout.mcpSystemLogPath).mapNotNull { line ->
            val encoded = line.substringAfter(" => ", missingDelimiterValue = "")
            if (encoded.isEmpty()) return@mapNotNull null
            val result = runCatching { wireJson.parseToJsonElement(encoded).jsonObject["structuredContent"] as? JsonObject }.getOrNull()
            (result?.get("plan") as? JsonObject ?: result)?.takeIf { it["tasks"] is JsonArray }
                ?.let { wireJson.decodeFromJsonElement<Plan>(it) }
        }
        val problems = stageOrderProblems(snapshots, ids)
        assertTrue(problems.isEmpty(), problems.joinToString("\n"))
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
        verifyGenerationEvidence(runtime, code)
    }

    private fun verifyGenerationEvidence(runtime: CodexRunner, code: String) {
        val acceptedReviews = acceptedCandidateReviews(runtime.scratch, sha256(code))
        assertTrue(acceptedReviews.isNotEmpty(), "Code review must accept the exact persisted inspection hash")
        val calls = Files.readAllLines(workspace.output.resolve("log/inspection-mcp.jsonl"))
            .map { wireJson.parseToJsonElement(it).jsonObject }
        assertTrue(calls.any { it["tool"]?.toString()?.contains("run_inspection_kts_examples") == true })
    }
}
