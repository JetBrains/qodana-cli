// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.integration

import java.nio.file.Files
import java.nio.file.Path
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonPrimitive
import org.jetbrains.qodana.edict.common.flag
import org.jetbrains.qodana.edict.common.obj
import org.jetbrains.qodana.edict.common.text
import org.jetbrains.qodana.edict.common.wireJson
import org.jetbrains.qodana.edict.edictnext.*
import org.jetbrains.qodana.edict.integration.support.IntegrationTest
import org.jetbrains.qodana.edict.integration.support.verifyManagedRun
import org.jetbrains.qodana.edict.signals.SignalValidation
import org.junit.jupiter.api.Test
import kotlin.io.path.exists
import kotlin.io.path.readText
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotEquals
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

class LiveEnrichmentTest : IntegrationTest() {
    override val fixtureRevision = "ef6c6e2061172498387c7c98cd1b10b7b65d1b09"
    override val fixtureProject = "testEnrichSignalFromGitHistory"
    override val fixtureStatePath = "$fixtureProject/.edict"

    @Test
    fun `enrich a cluster within server bounds without generating inspections`() {
        val specification = wireJson.parseToJsonElement(workspace.project.resolve("spec.json").readText()).jsonObject
        val enrichmentCluster = specification.text("clusterId")
        val enrichmentCorrection = specification.text("correctionCommit")
        val repository = EdictRepository(EdictRepositoryDirectory(workspace.state))
        val original = repository.loadCluster(enrichmentCluster)
        val seed = original.signals.single()
        SignalValidation.validate(seed)
        workspace.repository.validateEvidence(seed)
        val enrichmentSeedCommit = (seed.source as EdictNextSignalSource.FromCommit).commitRevision
        // Older pinned fixtures may contain legacy coverage. Enrichment must neither read nor update it.
        val legacyCoverage = original.directory.root.resolve("history-analysis.json")
        val legacyCoverageBefore = legacyCoverage.takeIf { it.exists() }?.readText()
        val seedBytes = original.directory.signalPath(seed.id).readText()
        val manifestBytes = original.directory.manifestPath.readText()

        // The server owns search bounds; the skill obtains evidence through the common cluster read tool.
        workspace.withEdictNextCodex("Enrich cluster $enrichmentCluster with historical evidence.") { store, runtime, _ ->
            val plan = assertNotNull(store.plan())
            val coordinator = plan.tasks.single()
            assertEquals("edict-git-history-signal-enrichment", coordinator.skill)
            assertTrue(enrichmentCluster in coordinator.prompt)

            val state = runBlocking { repository.loadState() }
            assertTrue(state.inboxSignals.isEmpty(), "Cluster enrichment must not need redistribution")
            val cluster = state.clusters.single()
            assertEquals(enrichmentCluster, cluster.id)
            assertEquals(seedBytes, cluster.directory.signalPath(seed.id).readText())
            assertEquals(manifestBytes, cluster.directory.manifestPath.readText())
            val enriched = cluster.signals.filter { it.id != seed.id }
            assertTrue(enriched.isNotEmpty())
            assertTrue(enriched.any { (it.source as? EdictNextSignalSource.FromCommit)?.commitRevision == enrichmentCorrection })
            enriched.forEach {
                SignalValidation.validate(it)
                workspace.repository.validateEvidence(it)
                assertEquals(EdictNextSignalStrength.STRONG, it.strength)
                assertEquals(null, it.syntheticExampleId)
                assertNotEquals((it.source as EdictNextSignalSource.FromCommit).commitRevision, enrichmentSeedCommit)
            }

            assertEquals(legacyCoverageBefore, legacyCoverage.takeIf { it.exists() }?.readText())
            assertEquals(EdictNextClusterStatus.Pending, cluster.manifest.status)
            assertTrue(cluster.examples.isEmpty())
            assertFalse(cluster.candidateInspectionPath.exists())
            assertFalse(repository.paths.inspectionPath(cluster.id).exists())
            assertFalse(cluster.directory.evaluationPath.exists())

            val calls = recordedMcpCalls(runtime.home.resolve("sessions"))
            assertTrue(calls.any { it.text("tool") == "edict_next_get_history_context" })
            assertTrue(calls.any { it.text("tool") == "edict_publish_signal_to_cluster" && it.obj("result").obj("structuredContent").flag("created") == true })
            assertTrue(calls.any { it.text("tool") == "edict_get_cluster_signals" })
            calls.filter { it.text("tool") in setOf("edict_next_get_history_context", "edict_publish_signal_to_cluster", "edict_get_cluster_signals") }
                .forEach { call -> assertEquals(enrichmentCluster, call.obj("arguments").text("clusterId")) }
            assertFalse(calls.any { it.text("tool") == "edict_next_record_history_analysis" })
            val context = calls.first { it.text("tool") == "edict_next_get_history_context" }
                .obj("result").obj("structuredContent")
            assertEquals(enrichmentCluster, context.text("clusterId"))
            assertFalse("signals" in context || "coverage" in context)
            val revisions = context.obj("range").getValue("commitRevisions").jsonArray.map { it.jsonPrimitive.content }
            assertTrue(enrichmentCorrection in revisions)
            assertTrue(enriched.all { (it.source as EdictNextSignalSource.FromCommit).commitRevision in revisions })
            val allowedDomainTools = setOf(
                "edict_next_get_history_context", "edict_publish_signal_to_cluster", "edict_get_cluster_signals",
            )
            assertFalse(calls.any { it.text("tool").startsWith("edict_next_") && it.text("tool") !in allowedDomainTools })
            assertFalse(calls.any { it.text("tool") in setOf(
                "edict_publish_signal", "compile_inspection_kts", "run_inspection_kts", "generate_psi_tree",
                "generate_inspection_kts_api", "generate_inspection_kts_examples", "edict_start_intellij",
            ) })
            verifyManagedRun(workspace, runtime, plan)
        }
    }

    /** Actual MCP receipts, not tool names merely included in assignments/results. */
    private fun recordedMcpCalls(sessions: Path): List<JsonObject> = Files.walk(sessions).use { paths ->
        paths.filter { it.toString().endsWith(".jsonl") }.toList()
            .flatMap { Files.readAllLines(it).filter(String::isNotBlank).map { line -> wireJson.parseToJsonElement(line).jsonObject } }
            .filter { event ->
                event.text("type") == "event_msg" && event.obj("payload").text("type") == "item_completed" &&
                    event.obj("payload").obj("item").text("server") == "edict-mcp"
            }
            .map { it.obj("payload").obj("item") }
    }
}
