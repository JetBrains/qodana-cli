package org.jetbrains.qodana.edict.store

import java.nio.file.Path
import kotlinx.serialization.encodeToString
import org.jetbrains.qodana.edict.common.json
import org.jetbrains.qodana.edict.model.Step
import org.jetbrains.qodana.edict.skills.Registry
import org.jetbrains.qodana.edict.support.fixtureSignals
import org.jetbrains.qodana.edict.support.gitFixture
import org.jetbrains.qodana.edict.support.launch
import kotlin.io.path.createTempDirectory
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFails
import kotlin.test.assertTrue

class DistributionTest {
    private val directory = createTempDirectory("edict-distribution-test")

    @Test
    fun `task cursor promotes an inbox neighbour to a cluster and requires its context`() {
        val signals = fixtureSignals(gitFixture(directory.resolve("source"))).sortedBy { it.id }
        Store(directory.resolve("state")).use { store ->
            val created = store.createPlan(
                "Extract and distribute",
                listOf(Step("edict-batch-signal-analysis", "Extract"), Step("edict-distribution", "Distribute"))
            )
            val extraction = store.launch(
                created.token,
                created.plan.tasks[0].id,
                "edict-batch-signal-analysis",
                Registry["edict-batch-signal-analysis"].operations,
                listOf("inbox")
            )
            val inbox = signals.map { signal ->
                store.write(extraction.token, "inbox/${signal.id}.json", json.encodeToString(signal), "")
            }
            val snapshot = prepareDistributionValidation(store, inbox.map { it.path })
            store.finishTask(extraction.token, "completed", "Extracted")
            val distribution = store.launch(
                created.token,
                created.plan.tasks[1].id,
                "edict-distribution",
                Registry["edict-distribution"].operations,
                inbox.map { it.path } + "clusters"
            )
            val neighbourMap = mapOf(
                signals[0].id to SignalNeighbours(
                    signals[0].id,
                    listOf(DistributionNeighbour(signals[1].id, 0.1))
                ),
                signals[1].id to SignalNeighbours(
                    signals[1].id,
                    listOf(DistributionNeighbour(signals[0].id, 0.1))
                ),
            )
            val service = DistributionService(store, mapOf(snapshot.receipt.receiptId to snapshot)) { corpus, ids, cache ->
                assertEquals(signals.map { it.id }.toSet(), corpus.map { it.id }.toSet())
                assertEquals(signals.map { it.id }, ids)
                assertEquals(store.root.resolve("embeddings"), cache)
                neighbourMap
            }

            val first = service.nextSignal(distribution.token, snapshot.receipt)
            assertEquals(signals[0].id, first.signalId)
            assertEquals("signal", first.candidates.single().kind)
            assertFails {
                store.write(distribution.token, "clusters/manual/history.md", "bypass", "")
            }
            assertTrue(service.addSignal(distribution.token, signals[0].id, "string-value-equality").added)

            val second = service.nextSignal(distribution.token, snapshot.receipt)
            assertEquals(signals[1].id, second.signalId)
            assertEquals("string-value-equality", second.candidates.single().clusterId)
            assertFails { service.addSignal(distribution.token, signals[1].id, "string-value-equality") }
            val context = service.context(distribution.token, "cluster", "string-value-equality")
            assertEquals(listOf(signals[0].id), context.signals.map { it.id })
            assertTrue(service.addSignal(distribution.token, signals[1].id, "string-value-equality").added)

            val done = service.nextSignal(distribution.token, snapshot.receipt)
            assertEquals("STOP_DISTRIBUTION", done.nextAction)
            assertTrue(validateDistributionState(store, snapshot).success)
            assertTrue(store.list("inbox").isEmpty())
            assertEquals(
                signals.map { it.id },
                store.list("clusters/string-value-equality/signals").map { Path.of(it).fileName.toString().removeSuffix(".json") }
            )
        }
    }
}
