// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.integration

import java.nio.file.Files
import kotlin.io.path.readText
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertTrue
import org.jetbrains.qodana.edict.common.json
import org.jetbrains.qodana.edict.integration.support.IntegrationTest
import org.jetbrains.qodana.edict.integration.support.signalFiles
import org.jetbrains.qodana.edict.integration.support.verifyManagedRun
import org.jetbrains.qodana.edict.edictnext.EdictNextFileRevision
import org.jetbrains.qodana.edict.edictnext.EdictNextLineRange
import org.jetbrains.qodana.edict.edictnext.EdictNextSignal
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalLabel
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalProvenance
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalSource
import org.jetbrains.qodana.edict.edictnext.stableSignalId
import org.jetbrains.qodana.edict.signals.SignalValidation
import org.junit.jupiter.api.Test

private const val historicalFixtureHead = "a7fa96f48394b6e1e0d2d46e45403f0b3e3e18c3"
private const val historicalFixtureSource = "ssh://git@git.jetbrains.team/sa/distillery-test.git"
private const val historicalOrigin = "fedd984a1a5ab941bbcce0a5f14b01415d01918e"
private const val historicalOriginParent = "2ea6da536647ff8dae5e4a0b3f4e999b0e2e345c"
private const val historicalOriginPath =
    "testExtractRulesFromSleepRemoveCommit/src/test/java/com/mycompany/app/AppTest.java"
private const val historicalCorrection = "1033043161d5df7fb7006309af1b4b21af1bacb9"

class LiveGitHistorySignalAnalysisTest : IntegrationTest() {
    override val fixtureRevision = historicalFixtureHead
    override val fixtureProject = "."
    override val fixtureSource = historicalFixtureSource

    @Test
    fun `managed history skill finds an independent correction with git grep`() {
        val key = "history-seed|$historicalOrigin|$historicalOriginPath|40"
        val seed = EdictNextSignal(
            id = stableSignalId(key),
            idempotencyKey = key,
            fileRevision = EdictNextFileRevision(
                historicalOriginPath,
                historicalOriginParent,
                listOf(EdictNextLineRange(40, 40)),
            ),
            source = EdictNextSignalSource.FromCommit(
                commitRevision = historicalOrigin,
            ),
            label = EdictNextSignalLabel.POSITIVE,
            description = "Thread.sleep must not be used to synchronize asynchronous work in tests",
            provenance = EdictNextSignalProvenance("commit-${historicalOrigin.take(16)}"),
        )
        val inbox = workspace.state.resolve("inbox")
        Files.createDirectories(inbox)
        val seedPath = inbox.resolve("${seed.id}.json")
        Files.writeString(seedPath, json.encodeToString(seed) + "\n")
        val seedBytes = seedPath.readText()

        workspace.withEdictNextCodex(
            """
            Find additional historical evidence for one selected Signal.
            Signal ID: ${seed.id}
            """.trimIndent(),
        ) { store, runtime, _ ->
            val plan = assertNotNull(store.plan())
            val coordinator = plan.tasks.single { it.skill == "edict-git-history-signal-analysis" }
            val workers = plan.tasks.filter { it.skill == "edict-signal-analysis" }
            assertTrue(workers.isNotEmpty(), "Historical candidates must run in evidence workers")
            assertTrue(workers.all { it.parentId == coordinator.id })
            assertTrue(workers.any { historicalCorrection in it.prompt }, "Known independent correction was not inspected")
            assertTrue(runtime.trace.resolve("stdout.jsonl").readText().contains("git grep"), "Skill did not execute git grep")

            assertEquals(seedBytes, seedPath.readText(), "Selected seed Signal must remain unchanged")
            val signals = signalFiles(inbox).map { file ->
                SignalValidation.validate("inbox/${file.fileName}", file.readText())
            }
            val independent = signals.filter { it.id != seed.id }
            assertTrue(independent.isNotEmpty(), "Bounded search must publish independent corrective evidence")
            assertTrue(EdictNextSignalLabel.POSITIVE in independent.map { it.label })
            independent.forEach(workspace.repository::validateEvidence)
            assertTrue(signals.none {
                it.id != seed.id && (it.source as? EdictNextSignalSource.FromCommit)?.commitRevision == historicalOrigin
            })
            verifyManagedRun(workspace, runtime, plan)
        }
    }
}
