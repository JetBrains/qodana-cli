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
import kotlinx.serialization.json.jsonPrimitive
import org.jetbrains.qodana.edict.common.EdictCIConfiguration
import org.jetbrains.qodana.edict.common.EdictConfiguration
import org.jetbrains.qodana.edict.common.PromotionConfiguration
import org.jetbrains.qodana.edict.common.sha256
import org.jetbrains.qodana.edict.common.wireJson
import org.jetbrains.qodana.edict.edictnext.EdictNextClusterStatus
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState.Plan
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalLabel
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalSource
import org.jetbrains.qodana.edict.edictnext.EdictRepository
import org.jetbrains.qodana.edict.edictnext.EdictRepositoryDirectory
import org.jetbrains.qodana.edict.integration.support.*
import org.jetbrains.qodana.edict.integration.support.inspection.InspectionServer
import org.jetbrains.qodana.edict.integration.support.inspection.stageOrderProblems
import org.jetbrains.qodana.edict.integration.support.reviews.ReviewProviderFixture
import org.jetbrains.qodana.edict.runtime.CodexRunner
import org.junit.jupiter.api.Test

class LivePipelineTest : IntegrationTest() {
    @Test
    fun `short daily request runs the default pipeline`() {
        ReviewProviderFixture("github", workspace.repository).use { reviews ->
            InspectionServer.start(workspace).use { inspection ->
                workspace.withCodex(
                    "Run the daily repository routine.",
                    provider = reviews.client,
                    inspectionServer = inspection,
                    timeoutMinutes = 180,
                    configuration = EdictConfiguration(
                        ci = EdictCIConfiguration("https://github.com/owner/repo"),
                        promotion = PromotionConfiguration("fixture-reviewer"),
                    ),
                ) { store, runtime, _ ->
                    val plan = assertNotNull(store.plan())
                    val required = setOf(
                        "edict-signal-analysis",
                        "edict-pr-signal-analysis",
                        "edict-next-run",
                        "edict-next-distribution",
                        "edict-next-generation",
                        "edict-next-cluster-generation",
                        "edict-next-code-example-overseer",
                        "edict-next-inspection-shallow-review",
                        "edict-next-weak-signal-review",
                        "edict-next-inspection-code-review",
                        "edict-promote",
                    )
                    assertTrue(
                        plan.tasks.map { it.skill }.toSet().containsAll(required),
                        "Pipeline must execute PR extraction, distribution, generation, all three reviews, and publication",
                    )
                    verifyManagedRun(workspace, runtime, plan)
                    verifySequentialStages(plan)
                    verifyProcessedCluster(runtime, inspection, plan)
                    reviews.verifyRequests()
                }
            }
        }
    }

    private fun verifySequentialStages(plan: Plan) {
        val stages = plan.tasks.filter { it.parentId.isEmpty() }
        assertEquals(
            listOf("edict-pr-signal-analysis", "edict-next-run", "edict-promote"),
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

    private fun verifyProcessedCluster(
        runtime: CodexRunner,
        inspection: InspectionServer,
        plan: Plan,
    ) {
        val repository = EdictRepository(EdictRepositoryDirectory(workspace.state))
        val state = runBlocking { repository.loadState() }
        assertTrue(state.inboxSignals.isEmpty(), "Distribution must consume every extracted Signal")
        assertEquals(1, state.clusters.size, "The positive/negative pair must form exactly one cluster")
        val cluster = state.clusters.single()
        assertEquals("Java", cluster.manifest.language.name)
        assertEquals(2, cluster.signals.size, "The review correction must retain both evidence sides")
        assertEquals(EdictNextSignalLabel.entries.toSet(), cluster.signals.map { it.label }.toSet())
        val expectedDiff = workspace.repository.diff(historyBefore, historyCommit, listOf(historyPath))
        val providerDiff = expectedDiff.substring(expectedDiff.indexOf("--- a/"))
        cluster.signals.forEach { signal ->
            val source = signal.source as EdictNextSignalSource.FromPR
            assertEquals(7, source.prNumber)
            assertTrue(source.diffPositiveToNegative in listOf(expectedDiff, providerDiff))
            assertEquals(historyPath, signal.fileRevision.path)
            val positive = signal.label == EdictNextSignalLabel.POSITIVE
            assertEquals(if (positive) historyBefore else historyCommit, signal.fileRevision.revision)
            val expectedLine = if (positive) 5 else 10
            assertTrue(signal.fileRevision.expectedRanges.orEmpty().any { expectedLine in it.start..it.end })
            val sourceText = workspace.repository.fileAt(signal.fileRevision.revision, historyPath)
            val lineCount = sourceText.lineSequence().count()
            assertTrue(signal.fileRevision.expectedRanges.orEmpty().all { it.end <= lineCount })
        }
        assertTrue(cluster.historyPath.readText().isNotBlank())

        if (cluster.manifest.status == EdictNextClusterStatus.Pending) {
            assertEquals(
                listOf("inspections/${cluster.id}.candidate.kts"),
                state.filesByRelativePath.keys.filter { it.startsWith("inspections/") }.sorted(),
                "A rejected evidence-backed attempt must remain as one resumable candidate",
            )
            assertTrue(
                state.filesByRelativePath.getValue("inspections/${cluster.id}.candidate.kts").isNotEmpty(),
                "Pending generation must preserve its candidate",
            )
            assertTrue(
                plan.tasks.any { it.skill == "edict-next-inspection-shallow-review" && it.status == "completed" },
                "Pending is acceptable only after an independent candidate review",
            )
            return
        }

        assertEquals(EdictNextClusterStatus.Generated, cluster.manifest.status)
        assertTrue(
            plan.tasks.any { it.skill == "edict-next-weak-signal-review" && it.status == "completed" },
            "Generated transition requires weak-signal review",
        )

        val code = assertNotNull(state.inspectionCode(cluster.id))
        assertTrue(code.isNotBlank())
        assertEquals(
            listOf("inspections/${cluster.id}.inspection.kts"),
            state.filesByRelativePath.keys.filter { it.startsWith("inspections/") }.sorted(),
            "Only the accepted inspection may remain, with no candidate",
        )
        val contextPath = historyPath.removePrefix("$historyProject/")
        val originalPositive = inspection.run(code, contextPath, workspace.repository.fileAt(historyBefore, historyPath))
        assertContains(originalPositive.problemLines, 5, "Accepted inspection must detect the original problem")
        val originalNegative = inspection.run(code, contextPath, workspace.repository.fileAt(historyCommit, historyPath))
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
