package org.jetbrains.qodana.edict.store

import org.jetbrains.qodana.edict.common.json
import org.jetbrains.qodana.edict.model.Step
import org.jetbrains.qodana.edict.skills.Registry
import org.jetbrains.qodana.edict.support.fixtureSignals
import org.jetbrains.qodana.edict.support.gitFixture
import org.jetbrains.qodana.edict.support.launch
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class StateValidationTest {
    @TempDir
    lateinit var directory: Path

    @Test
    fun `managed validators preserve distribution bytes and validate generation structure`() {
        val signal = fixtureSignals(gitFixture(directory.resolve("source"))).first()
        Store(directory.resolve("state")).use { store ->
            val created = store.createPlan(
                "Extract distribute generate",
                listOf(
                    Step("edict-batch-signal-analysis", "Extract"),
                    Step("edict-distribution", "Distribute"),
                    Step("edict-generation", "Generate")
                )
            )
            val extraction = store.launch(
                created.token,
                created.plan.tasks[0].id,
                "edict-batch-signal-analysis",
                listOf("inbox.write"),
                listOf("inbox")
            )
            val inbox = store.write(extraction.token, "inbox/${signal.id}.json", json.encodeToString(signal), "")
            val snapshot = prepareDistributionValidation(store, listOf(inbox.path))
            store.finishTask(extraction.token, "completed", "Extracted")

            val distribution = store.launch(
                created.token,
                created.plan.tasks[1].id,
                "edict-distribution",
                Registry["edict-distribution"].operations,
                listOf(inbox.path, "clusters/equality")
            )
            val cluster = "clusters/equality"
            store.write(
                distribution.token,
                "$cluster/description.json",
                """{"id":"equality","description":"Compare strings by value","language":"Java","status":"Pending","knownProblems":[]}""",
                ""
            )
            store.write(distribution.token, "$cluster/history.md", "Distributed ${signal.id}\n", "")
            var member = store.write(distribution.token, "$cluster/signals/${signal.id}.json", inbox.content, "")
            store.delete(distribution.token, inbox.path, inbox.hash)
            assertTrue(validateDistributionState(store, snapshot).success)

            member = store.write(
                distribution.token,
                member.path,
                json.encodeToString(signal.copy(description = "Tampered during distribution")),
                member.hash
            )
            assertFalse(validateDistributionState(store, snapshot).success)
            member = store.write(distribution.token, member.path, inbox.content, member.hash)
            assertTrue(validateDistributionState(store, snapshot).success)
            store.finishTask(distribution.token, "completed", "Distributed")

            val generation = store.launch(
                created.token,
                created.plan.tasks[2].id,
                "edict-generation",
                Registry["edict-generation"].operations,
                listOf(cluster, "inspections")
            )
            val clusterTask = store.addTask(generation.token, "edict-cluster-generation", "Generate equality")
            val worker = store.launch(
                generation.token,
                clusterTask.id,
                clusterTask.skill,
                Registry[clusterTask.skill].operations,
                listOf(cluster, "inspections")
            )
            val exampleTask = store.addTask(worker.token, "edict-code-example", "Positive example")
            val example = store.launch(
                worker.token,
                exampleTask.id,
                exampleTask.skill,
                Registry[exampleTask.skill].operations,
                listOf(cluster)
            )
            val exampleRoot = "$cluster/synthetic-examples/positive"
            val metadataPath = "$exampleRoot/metadata.json"
            var metadata = store.write(
                example.token,
                metadataPath,
                """{"id":"positive","fileName":"Example.java","label":"POSITIVE","expectedRanges":[{"start":3,"end":3}]}""",
                ""
            )
            store.write(example.token, "$exampleRoot/project/Example.java", "class Example {}\n", "")
            assertFalse(validateCodeExampleState(store, "equality", "positive").success)
            metadata = store.write(
                example.token,
                metadataPath,
                """{"id":"positive","fileName":"Example.java","label":"POSITIVE","expectedRanges":[{"start":1,"end":1}]}""",
                metadata.hash
            )
            assertTrue(metadata.hash.isNotBlank())
            val linked = store.write(
                example.token,
                member.path,
                json.encodeToString(signal.copy(syntheticExampleId = "positive")),
                member.hash
            )
            assertTrue(linked.hash.isNotBlank())
            assertTrue(validateCodeExampleState(store, "equality", "positive").success)
            assertTrue(validateClusterExamplesState(store, "equality").success)
            store.finishTask(example.token, "completed", "Validated positive example")

            store.write(worker.token, "inspections/equality.inspection.kts", "listOf(localInspection { })\n", "")
            val description = store.read("$cluster/description.json")
            store.write(
                worker.token,
                description.path,
                """{"id":"equality","description":"Compare strings by value","language":"Java","status":"Generated","knownProblems":[]}""",
                description.hash
            )
            assertTrue(validateGenerationState(store, listOf("equality")).success)
        }
    }
}
