// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.integration

import kotlinx.serialization.json.jsonObject
import org.jetbrains.qodana.edict.common.array
import org.jetbrains.qodana.edict.common.flag
import org.jetbrains.qodana.edict.common.text
import org.jetbrains.qodana.edict.common.wireJson
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalLabel
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalSource
import org.jetbrains.qodana.edict.integration.support.IntegrationTest
import org.jetbrains.qodana.edict.integration.support.historyBefore
import org.jetbrains.qodana.edict.integration.support.historyCommit
import org.jetbrains.qodana.edict.integration.support.historyPath
import org.jetbrains.qodana.edict.integration.support.reviews.ReviewProviderFixture
import org.jetbrains.qodana.edict.integration.support.signalFiles
import org.jetbrains.qodana.edict.integration.support.verifyManagedRun
import org.jetbrains.qodana.edict.signals.SignalValidation
import org.junit.jupiter.api.Test
import java.nio.file.Files
import kotlin.io.path.readText
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

class LivePrExtractionTest : IntegrationTest() {
    @Test
    fun `managed skills extract a Space review evidence pair with real native workers`() {
        ReviewProviderFixture("space", workspace.repository).use { fixture ->
            workspace.withCodex(
                """
                Use edict_manager and the managed protocol to extract Signals from Space review #7.
                Provider: space
                Project key: owner
                Repository: repo
                PR limit: 1
                Source checkout: ${workspace.repository.root}
                Edict state root: ${workspace.state}
                Publish every supported Signal through edict_publish_signal.
                """.trimIndent(),
                provider = fixture.client,
            ) { store, runtime, _ ->
                val plan = assertNotNull(store.plan())
                assertEquals(2, plan.tasks.size, "One PR coordinator and one discussion worker required")
                val coordinator = plan.tasks.single { it.skill == "edict-pr-signal-analysis" }
                val worker = plan.tasks.single { it.skill == "edict-signal-analysis" }
                assertEquals("", coordinator.parentId)
                assertEquals(coordinator.id, worker.parentId)
                verifyReviewSignals(fixture)
                fixture.verifyRequests()
                verifyReviewCalls()
                verifyManagedRun(workspace, runtime, plan)
            }
        }
    }

    private fun verifyReviewSignals(fixture: ReviewProviderFixture) {
        val files = signalFiles(workspace.state.resolve("inbox"))
        assertEquals(2, files.size, "Review correction must retain both evidence sides")
        val diff = workspace.repository.diff(historyBefore, historyCommit, listOf(historyPath))
        val providerDiff = diff.substring(diff.indexOf("--- a/"))
        val signals = files.map { file ->
            val signal = SignalValidation.validate("inbox/${file.fileName}", file.readText())
            val signalSource = signal.source as EdictNextSignalSource.FromPR
            assertEquals(7, signalSource.prNumber)
            assertEquals(fixture.title, signalSource.title)
            assertEquals(listOf(fixture.message), signalSource.discussionMessages)
            assertEquals(fixture.discussionUrl, signalSource.url)
            assertTrue(
                signalSource.diffPositiveToNegative in listOf(diff, providerDiff),
                "PR diff must match canonical historical Git bytes",
            )
            assertEquals(historyPath, signal.fileRevision.path)
            assertTrue(signal.provenance.workItemId.startsWith("pr-7-"))
            assertFalse(signal.provenance.analysisBatchId.isNullOrBlank())
            val positive = signal.label == EdictNextSignalLabel.POSITIVE
            val revision = if (positive) historyBefore else historyCommit
            val line = if (positive) 5 else 10
            assertEquals(revision, signal.fileRevision.revision)
            val source = fixture.source.getValue(revision)
            val lines = source.count { it == '\n' } + if (source.isNotEmpty() && !source.endsWith('\n')) 1 else 0
            signal.fileRevision.expectedRanges.orEmpty().forEach {
                assertTrue(it.end <= lines, "Signal range exceeds historical source")
            }
            assertTrue(
                signal.fileRevision.expectedRanges.orEmpty().any { line in it.start..it.end },
                "Evidence must cover correction at line $line",
            )
            signal
        }
        assertEquals(EdictNextSignalLabel.entries.toSet(), signals.map { it.label }.toSet())
        assertEquals(1, signals.map { it.provenance.workItemId }.distinct().size)
        assertEquals(1, signals.map { it.provenance.analysisBatchId }.distinct().size)
        assertEquals(2, signals.map { it.idempotencyKey }.distinct().size)
    }

    private fun verifyReviewCalls() {
        val calls = mutableSetOf<String>()
        val record = Regex("^\\S+ \\[[^]]+] (edict_[a-z_]+) (\\{.*}) => (\\{.*})$")
        Files.readAllLines(workspace.logs.resolve("edict-mcp-system.log")).filter(String::isNotBlank).forEach { line ->
            val match = assertNotNull(record.matchEntire(line), "Invalid MCP system-log entry")
            val name = match.groupValues[1]
            val arguments = wireJson.parseToJsonElement(match.groupValues[2]).jsonObject
            val response = wireJson.parseToJsonElement(match.groupValues[3]).jsonObject
            calls += name
            if (response.flag("isError") == true) {
                val detail = response.array("content").joinToString("\n") { it.text("text") }
                val path = arguments.text("path")
                val missingOutput = name == "edict_read" && path.startsWith("inbox/") &&
                    detail == workspace.state.resolve(path).toString()
                assertTrue(missingOutput, "$name failed: $detail; inspect ${workspace.logs}")
            }
        }
        listOf(
            "edict_fetch_pr_batch",
            "edict_list_pr_analysis_items",
            "edict_get_pr_analysis_item",
            "edict_validate_pr_signals",
        ).forEach { assertTrue(it in calls, "Missing real $it call") }
    }
}
