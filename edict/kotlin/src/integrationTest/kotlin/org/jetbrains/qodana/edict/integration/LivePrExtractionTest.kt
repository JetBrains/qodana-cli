// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.integration

import kotlinx.serialization.json.jsonObject
import org.jetbrains.qodana.edict.common.array
import org.jetbrains.qodana.edict.common.flag
import org.jetbrains.qodana.edict.common.text
import org.jetbrains.qodana.edict.common.wireJson
import org.jetbrains.qodana.edict.integration.support.*
import org.jetbrains.qodana.edict.integration.support.reviews.ReviewProviderFixture
import org.jetbrains.qodana.edict.model.SignalLabel
import org.jetbrains.qodana.edict.signals.SignalValidation
import org.jetbrains.qodana.edict.store.Store
import org.junit.jupiter.api.Test
import java.nio.file.Files
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

class LivePrExtractionTest : IntegrationTest() {
    @Test
    fun `managed skills extract a GitHub review evidence pair with real native workers`() = extractReview("github")

    @Test
    fun `managed skills extract a Space review evidence pair with real native workers`() = extractReview("space")

    private fun extractReview(provider: String) {
        ReviewProviderFixture(provider, workspace.repository).use { fixture ->
            workspace.withCodex(
                "Extract signals from $provider review #7 in owner/repo.",
                provider = fixture.client
            ) { store, runtime, _ ->
                val plan = assertNotNull(store.plan())
                assertEquals(2, plan.tasks.size, "One PR coordinator and one discussion worker required")
                val coordinator = plan.tasks.single { it.skill == "edict-pr-signal-analysis" }
                val worker = plan.tasks.single { it.skill == "edict-signal-analysis" }
                assertEquals("", coordinator.parentId)
                assertEquals(coordinator.id, worker.parentId)
                verifyReviewSignals(store, fixture)
                fixture.verifyRequests()
                verifyReviewCalls()
                verifyManagedRun(workspace, runtime, plan)
            }
        }
    }

    private fun verifyReviewSignals(store: Store, fixture: ReviewProviderFixture) {
        val paths = store.list("inbox")
        assertEquals(2, paths.size, "Review correction must retain both evidence sides")
        val diff = workspace.repository.diff(historyBefore, historyCommit, listOf(historyPath))
        val providerDiff = diff.substring(diff.indexOf("--- a/"))
        val signals = paths.map { path ->
            val signal = SignalValidation.validate(path, store.read(path).content)
            assertEquals("inbox/${signal.id}.json", path)
            assertEquals("FromPR", signal.source.type)
            assertEquals(7, signal.source.prNumber)
            assertEquals(fixture.title, signal.source.title)
            assertEquals(listOf(fixture.message), signal.source.discussionMessages)
            assertEquals(fixture.discussionUrl, signal.source.url)
            assertTrue(
                signal.source.diffPositiveToNegative in listOf(diff, providerDiff),
                "PR diff must match canonical historical Git bytes"
            )
            assertEquals(historyPath, signal.fileRevision.path)
            assertTrue(signal.provenance.workItemId.startsWith("pr-7-"))
            assertFalse(signal.provenance.analysisBatchId.isNullOrBlank())
            val positive = signal.label == SignalLabel.POSITIVE
            val revision = if (positive) historyBefore else historyCommit
            val line = if (positive) 5 else 10
            assertEquals(revision, signal.fileRevision.revision)
            val source = fixture.source.getValue(revision)
            val lines = source.count { it == '\n' } + if (source.isNotEmpty() && !source.endsWith('\n')) 1 else 0
            signal.fileRevision.expectedRanges.forEach {
                assertTrue(
                    it.end <= lines,
                    "Signal range exceeds historical source"
                )
            }
            assertTrue(
                signal.fileRevision.expectedRanges.any { line in it.start..it.end },
                "Evidence must cover correction at line $line"
            )
            signal
        }
        assertEquals(SignalLabel.entries.toSet(), signals.map { it.label }.toSet())
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
                // An existence check before publishing an idempotent inbox record can return ENOENT.
                val missingOutput = name == "edict_read" && path.startsWith("inbox/") &&
                        detail == workspace.state.resolve(path).toString()
                assertTrue(missingOutput, "$name failed: $detail; inspect ${workspace.logs}")
            }
        }
        listOf(
            "edict_prepare_pr_analysis",
            "edict_list_pr_analysis_items",
            "edict_get_pr_analysis_item",
            "edict_validate_pr_signals"
        )
            .forEach { assertTrue(it in calls, "Missing real $it call") }
    }
}
