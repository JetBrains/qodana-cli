// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.integration

import java.nio.file.Files
import kotlin.test.*
import kotlinx.serialization.json.*
import org.jetbrains.qodana.edict.common.json
import org.jetbrains.qodana.edict.common.obj
import org.jetbrains.qodana.edict.common.sha256
import org.jetbrains.qodana.edict.common.text
import org.jetbrains.qodana.edict.common.wireJson
import org.jetbrains.qodana.edict.integration.support.CommitExpectation
import org.jetbrains.qodana.edict.integration.support.IntegrationTest
import org.jetbrains.qodana.edict.integration.support.inspection.InspectionServer
import org.jetbrains.qodana.edict.integration.support.inspection.acceptedCandidateReviews
import org.jetbrains.qodana.edict.integration.support.inspection.decodeInspectionResult
import org.jetbrains.qodana.edict.integration.support.inspection.stageOrderProblems
import org.jetbrains.qodana.edict.integration.support.threeCommitExpectations
import org.jetbrains.qodana.edict.integration.support.threeCommitHead
import org.jetbrains.qodana.edict.integration.support.threeCommitProject
import org.jetbrains.qodana.edict.integration.support.verifyCommitSignals
import org.jetbrains.qodana.edict.integration.support.verifyManagedRun
import org.jetbrains.qodana.edict.model.Plan
import org.jetbrains.qodana.edict.model.Signal
import org.jetbrains.qodana.edict.model.SignalLabel
import org.jetbrains.qodana.edict.model.SignalRange
import org.jetbrains.qodana.edict.runtime.CodexRunner
import org.jetbrains.qodana.edict.store.Store
import org.jetbrains.qodana.edict.support.batch
import org.junit.jupiter.api.Test

class LivePipelineTest : IntegrationTest() {
    override val fixtureRevision = threeCommitHead
    override val fixtureProject = threeCommitProject

    @Test
    fun `managed skills extract cluster and generate an inspection from Distillery evidence`() {
        val expected = threeCommitExpectations().last()
        InspectionServer.start(workspace).use { inspection ->
            workspace.withCodex(
                "Run three tasks in order: extract signals from the latest commit, cluster them, then generate inspections.",
                inspectionUrl = inspection.url, timeoutMinutes = 60,
            ) { store, runtime, _ ->
                val plan = assertNotNull(store.plan())
                val required = setOf("edict-batch-signal-analysis", "edict-signal-analysis", "edict-distribution", "edict-generation",
                    "edict-cluster-generation", "edict-code-example", "edict-inspection-code-review", "edict-weak-signal-review", "edict-inspection-value-review")
                assertTrue(plan.tasks.map { it.skill }.toSet().containsAll(required), "Pipeline must execute extraction, distribution, generation and all native review/example workers")
                verifyManagedRun(workspace, runtime, plan)
                verifySequentialStages(plan)
                verifyGeneratedCluster(store, runtime, inspection, expected)
            }
        }
    }

    private fun verifySequentialStages(plan: Plan) {
        val stages = plan.tasks.filter { it.parentId.isEmpty() }
        assertEquals(listOf("edict-batch-signal-analysis", "edict-distribution", "edict-generation"), stages.map { it.skill })
        val ids = stages.map { it.id }
        val snapshots = Files.readAllLines(workspace.logs.resolve("edict-mcp-system.log")).mapNotNull { line ->
            val result = wireJson.parseToJsonElement(line.substringAfter(" => ")).jsonObject["structuredContent"] as? JsonObject
            (result?.get("plan") as? JsonObject ?: result)?.takeIf { it["tasks"] is JsonArray }
                ?.let { wireJson.decodeFromJsonElement<Plan>(it) }
        }
        val problems = stageOrderProblems(snapshots, ids)
        assertTrue(problems.isEmpty(), problems.joinToString("\n"))
    }

    private fun verifyGeneratedCluster(store: Store, runtime: CodexRunner, inspection: InspectionServer, expected: CommitExpectation) {
        assertTrue(store.list("inbox").isEmpty(), "Clustering must consume every extracted signal")
        val descriptions = store.list("clusters").filter { it.endsWith("/description.json") }
        assertEquals(1, descriptions.size, "The positive/negative pair must form exactly one cluster")
        val descriptionPath = descriptions.single()
        val clusterPath = descriptionPath.substringBeforeLast('/')
        val cluster = clusterPath.substringAfterLast('/')
        val description = wireJson.parseToJsonElement(store.read(descriptionPath).content).jsonObject
        assertEquals(cluster, description.text("id"))
        assertEquals("Java", description.text("language"))
        assertEquals("Generated", description.text("status"), "Generation must produce an accepted inspection; inspect cluster history and agent logs")
        assertTrue(description["predecessorId"] == null || description["predecessorId"] == JsonNull)
        val signalFiles = store.list("$clusterPath/signals").map(store::read)
        verifyCommitSignals(workspace.repository, signalFiles, listOf(expected))
        val accepted = "inspections/$cluster.inspection.kts"
        val code = store.read(accepted).content
        assertTrue(code.isNotBlank())
        assertEquals(listOf(accepted), store.list("inspections"), "Only the accepted inspection may remain, with no candidate")
        assertTrue(store.read("$clusterPath/history.md").content.isNotBlank())

        // Independently compile the actual persisted bytes against both original Git revisions.
        val contextPath = expected.path.removePrefix("$threeCommitProject/")
        val originalPositive = inspection.run(code, contextPath, workspace.repository.fileAt(expected.parent, expected.path))
        assertContains(originalPositive.problemLines, expected.positiveLine, "Accepted inspection must detect the original integer overflow")
        val originalNegative = inspection.run(code, contextPath, workspace.repository.fileAt(expected.revision, expected.path))
        assertTrue(originalNegative.problemLines.isEmpty(), "Accepted inspection must not flag the corrected revision")

        signalFiles.forEach { file ->
            val signal = json.decodeFromString<Signal>(file.content)
            val example = assertNotNull(signal.syntheticExampleId, "Generation must assign every signal a measured example")
            val directory = "$clusterPath/synthetic-examples/$example"
            val metadata = wireJson.parseToJsonElement(store.read("$directory/metadata.json").content).jsonObject
            assertEquals(example, metadata.text("id"))
            assertEquals(signal.label.name, metadata.text("label"))
            val filename = metadata.text("fileName")
            assertTrue(filename.isNotBlank() && '/' !in filename && '\\' !in filename, "Example must use a single source file")
            val source = store.read("$directory/project/$filename").content
            val measured = inspection.run(code, contextPath, source)
            val ranges = (metadata["expectedRanges"] as? JsonArray).orEmpty().map { json.decodeFromJsonElement<SignalRange>(it) }
            if (signal.label == SignalLabel.POSITIVE) {
                assertEquals(1, ranges.size)
                assertEquals(1, measured.problemLines.size, "Positive example must contain exactly one detected problem")
                assertTrue(measured.problemLines.single() in ranges.single().start..ranges.single().end)
            } else {
                assertTrue(ranges.isEmpty())
                assertTrue(measured.problemLines.isEmpty(), "Negative example must not be reported")
            }
        }
        verifyGenerationEvidence(runtime, code)
    }

    private fun verifyGenerationEvidence(runtime: CodexRunner, code: String) {
        val hash = sha256(code)
        val acceptedReviews = acceptedCandidateReviews(runtime.scratch, hash)
        assertTrue(acceptedReviews.size >= 2, "Code and value reviews must accept the exact persisted inspection hash")
        val measured = Files.readAllLines(workspace.output.resolve("log/inspection-mcp.jsonl")).map { wireJson.parseToJsonElement(it).jsonObject }
            .count { call -> call.text("tool") == "run_inspection_kts" && call.obj("arguments").text("inspectionKtsCode") == code &&
                runCatching { decodeInspectionResult(call.obj("result")).compilationSuccess }.getOrDefault(false) }
        assertTrue(measured >= 3, "Agent must compile and measure its final candidate on examples and project source")
    }
}

/** Review skills require a JSON object at the assigned output path, without prescribing a file extension. */
