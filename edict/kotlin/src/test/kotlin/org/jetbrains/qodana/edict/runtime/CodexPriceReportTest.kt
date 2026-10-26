// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.runtime

import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.long
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonObject
import org.jetbrains.qodana.edict.common.wireJson
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState.Plan
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState.Task
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import org.junitpioneer.jupiter.DefaultLocale
import java.nio.file.Files
import java.nio.file.Path
import kotlin.test.assertContains
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

class CodexPriceReportTest {
    @TempDir
    lateinit var directory: Path

    @Test
    fun `prices pipeline steps and groups cluster branches without double counting total`() {
        val sessions = directory.resolve("sessions")
        fun session(
            id: String,
            parent: String,
            agentPath: String = "",
            tokens: Long,
            model: String = "gpt-5.6-sol",
        ) {
            Files.createDirectories(sessions)
            val meta = buildJsonObject {
                put("type", "session_meta")
                putJsonObject("payload") {
                    put("id", id)
                    if (parent.isNotEmpty()) put("parent_thread_id", parent)
                    if (agentPath.isNotEmpty()) putJsonObject("source") { putJsonObject("subagent") {
                        putJsonObject("thread_spawn") { put("parent_thread_id", parent); put("agent_path", agentPath) }
                    } }
                }
            }
            fun usage() = buildJsonObject {
                put("input_tokens", tokens - 1); put("cached_input_tokens", tokens - 3)
                put("cache_write_input_tokens", 2); put("output_tokens", 1)
                put("reasoning_output_tokens", 1); put("total_tokens", tokens)
            }
            val usageRecord = buildJsonObject {
                put("type", "token_usage_record")
                putJsonObject("payload") {
                    put("usage", usage())
                    put("thread_token_usage", usage())
                }
            }
            val turn = buildJsonObject {
                put("type", "turn_context")
                putJsonObject("payload") { put("model", model) }
            }
            Files.writeString(
                sessions.resolve("$id.jsonl"),
                listOf(meta, turn, usageRecord).joinToString("\n") { wireJson.encodeToString(it) } + "\n",
            )
        }

        session("manager", "", tokens = 40)
        session("extract-thread", "manager", "/root/extract", 20)
        session("enrich-one-thread", "manager", "/root/enrich-one", 25)
        session("enrich-two-thread", "manager", "/root/enrich-two", 35)
        session("cluster-thread", "manager", "/root/cluster", 40)
        session("review-thread", "cluster-thread", "/root/cluster/review", 50, "gpt-5.6-luna")
        session("second-cluster-thread", "manager", "/root/second-cluster", 30)
        session("unrelated-manager", "", tokens = 1_000)
        val extract = Task(id = "extract-task", skill = "edict-batch-signal-analysis", title = "Extract", agentId = "/root/extract")
        val enrichOne = Task(id = "enrich-one-task", skill = "edict-git-history-signal-enrichment", title = "Enrich first-rule", agentId = "/root/enrich-one")
        val enrichTwo = Task(id = "enrich-two-task", skill = "edict-git-history-signal-enrichment", title = "Enrich second-rule", agentId = "/root/enrich-two")
        val cluster = Task(id = "cluster-task", skill = "edict-next-cluster-generation", title = "Generate first-rule", agentId = "/root/cluster")
        val review = Task(id = "review-task", parentId = cluster.id, skill = "edict-next-inspection-code-review", title = "Review", agentId = "/root/cluster/review")
        val secondCluster = Task(id = "second-cluster-task", skill = "edict-next-cluster-generation", title = "Generate second-rule", agentId = "/root/second-cluster")
        val plan = Plan(request = "test", tasks = listOf(extract, enrichOne, enrichTwo, cluster, review, secondCluster))
        val report = CodexPriceReporter.create(directory, plan, { pricing(it) })

        assertEquals(20, report.pipelineSteps.single { it.skill == extract.skill }.inclusivePrice.totalTokens)
        val enrichment = report.pipelineSteps.single { it.skill == enrichOne.skill }
        assertEquals(60, enrichment.inclusivePrice.totalTokens)
        assertEquals(listOf(25L, 35L), enrichment.tasks.map { it.inclusivePrice.totalTokens })
        val generation = report.pipelineSteps.single { it.skill == cluster.skill }
        assertEquals(120, generation.inclusivePrice.totalTokens)
        assertEquals(90, generation.tasks.single { it.taskId == cluster.id }.inclusivePrice.totalTokens)
        assertEquals(50, generation.tasks.single { it.taskId == cluster.id }.subtasks.single().inclusivePrice.totalTokens)
        assertEquals(0, report.unattributedPrice.totalTokens)
        assertEquals(240, report.totalPrice.totalTokens)
        assertEquals(0.0, report.totalPrice.uncachedInputPriceUsd)
        assertEquals(0.0000876, report.totalPrice.cachedInputPriceUsd, absoluteTolerance = 1e-12)
        assertEquals(0.000070, report.totalPrice.cacheWritePriceUsd, absoluteTolerance = 1e-12)
        assertEquals(0.000140, report.totalPrice.outputPriceUsd, absoluteTolerance = 1e-12)
        assertEquals(0.0002976, report.totalPrice.priceUsd, absoluteTolerance = 1e-12)
        assertEquals(setOf("gpt-5.6-luna", "gpt-5.6-sol"), report.models.map { it.pricing.model }.toSet())
        assertEquals(50, report.models.single { it.pricing.model == "gpt-5.6-luna" }.price.totalTokens)
        assertEquals(190, report.models.single { it.pricing.model == "gpt-5.6-sol" }.price.totalTokens)
        assertContains(report.render(), "- edict-git-history-signal-enrichment: $0.0001 (60 tokens)")
        assertContains(report.render(), "  - Enrich first-rule: $0.0000 (25 tokens)")
        assertContains(report.render(), "- edict-next-cluster-generation: $0.0001 (120 tokens)")
        assertContains(report.render(), "  - Generate first-rule: $0.0001 (90 tokens)")
        assertContains(report.render(), "    - Review [edict-next-inspection-code-review]: $0.0000 (50 tokens)")
        assertContains(report.render(), "Total price: $0.0003 (240 tokens)")
        assertFalse(report.render().contains("uncached-input="))

        val output = directory.resolve("price.json")
        val analysis = CodexPriceAnalyzer.analyze(directory, plan, output, { pricing(it) })
        assertEquals(240, analysis.totalTokens)
        assertEquals(0.0002976, analysis.totalPriceUsd, absoluteTolerance = 1e-12)
        assertEquals(240, analysis.report.getValue("totalPrice").jsonObject
            .getValue("totalTokens").jsonPrimitive.long)
        assertTrue(Files.size(output) > 0)
    }

    @Test
    @DefaultLocale("de-DE")
    fun `compact report retains every cluster and nested substage with locale independent totals`() {
        val own = CodexTokenPrice(totalTokens = 1_000, priceUsd = 1.25)
        val review = CodexTaskPrice("review", "edict-next-inspection-code-review", "Review", own, emptyList(), own)
        val examples = CodexTaskPrice("examples", "edict-next-code-example-overseer", "Examples", own, listOf(review), own + own)
        val first = CodexTaskPrice("first", "edict-next-cluster-generation", "Generate first-rule", own,
            listOf(examples), own + own + own)
        val second = CodexTaskPrice("second", first.skill, "Generate\nsecond-rule", own, emptyList(), own)
        val report = CodexPriceReport(
            models = emptyList(),
            managerOwnPrice = own,
            pipelineSteps = listOf(CodexPipelineStepPrice(first.skill, listOf(first, second), own + own + own + own)),
            unattributedPrice = own,
            totalPrice = own + own + own + own + own + own,
        )

        assertEquals(
            """
            Pipeline steps:
            - edict-next-cluster-generation: $5.0000 (4,000 tokens)
              - Generate first-rule: $3.7500 (3,000 tokens)
                - own: $1.2500 (1,000 tokens)
                - Examples [edict-next-code-example-overseer]: $2.5000 (2,000 tokens)
                  - own: $1.2500 (1,000 tokens)
                  - Review [edict-next-inspection-code-review]: $1.2500 (1,000 tokens)
              - Generate second-rule: $1.2500 (1,000 tokens)
            Manager own: $1.2500 (1,000 tokens)
            Unattributed sessions: $1.2500 (1,000 tokens)
            Total price: $7.5000 (6,000 tokens)
            """.trimIndent(),
            report.render().substringAfter("\n"),
        )
    }

    @Test
    fun `prices each usage record with its preceding turn model`() {
        val sessions = directory.resolve("sessions")
        Files.createDirectories(sessions)
        fun turn(model: String) = buildJsonObject {
            put("type", "turn_context")
            putJsonObject("payload") { put("model", model) }
        }
        fun usage(tokens: Long) = buildJsonObject {
            put("type", "token_usage_record")
            putJsonObject("payload") {
                putJsonObject("usage") {
                    put("input_tokens", tokens - 1)
                    put("cached_input_tokens", tokens - 1)
                    put("output_tokens", 1)
                    put("total_tokens", tokens)
                }
                putJsonObject("thread_token_usage") { put("total_tokens", tokens) }
            }
        }
        val meta = buildJsonObject {
            put("type", "session_meta")
            putJsonObject("payload") { put("id", "manager") }
        }
        val events = listOf(meta, turn("gpt-5.6-sol"), usage(10), turn("gpt-5.6-luna"), usage(20))
        Files.writeString(
            sessions.resolve("manager.jsonl"),
            events.joinToString("\n") { wireJson.encodeToString(it) } + "\n",
        )

        val report = CodexPriceReporter.create(directory, Plan(request = "test", tasks = emptyList()), { pricing(it) })

        assertEquals(30, report.totalPrice.totalTokens)
        assertEquals(10, report.models.single { it.pricing.model == "gpt-5.6-sol" }.price.totalTokens)
        assertEquals(20, report.models.single { it.pricing.model == "gpt-5.6-luna" }.price.totalTokens)
    }

    @Test
    fun `parses current official pricing shape and selects long context rates per request`() {
        val html = """
            props="{&quot;rows&quot;:[1,[[1,[[0,&quot;gpt-5.6-sol&quot;],[0,4],[0,0.4],[0,5],[0,20]]]]] }"
            props="{&quot;groups&quot;:[1,[[0,{&quot;model&quot;:[0,&quot;gpt-5.6-sol&quot;],&quot;rows&quot;:[1,[[1,[[0,4],[0,0.4],[0,5],[0,20],[0,8],[0,0.8],[0,10],[0,30]]]]]}]]] }"
        """.trimIndent()
        val pricing = OpenAiPricingLoader.parse("gpt-5.6-sol", html, "2026-09-29T00:00:00Z")

        assertEquals(4.0, pricing.shortContextRates.uncachedInputUsdPerMillion)
        assertEquals(0.4, pricing.shortContextRates.cachedInputUsdPerMillion)
        assertEquals(5.0, pricing.shortContextRates.cacheWriteUsdPerMillion)
        assertEquals(20.0, pricing.shortContextRates.outputUsdPerMillion)
        assertEquals(8.0, assertNotNull(pricing.longContextRates).uncachedInputUsdPerMillion)
        val usage = CodexTokenPrice(uncachedInputTokens = 1, cachedInputTokens = 1,
            cacheWriteInputTokens = 1, outputTokens = 1, totalTokens = 4)
        assertEquals(29.4 / 1_000_000, pricing.price(usage, 272_000).priceUsd, absoluteTolerance = 1e-12)
        assertEquals(48.8 / 1_000_000, pricing.price(usage, 272_001).priceUsd, absoluteTolerance = 1e-12)
    }

    private fun pricing(model: String = "gpt-5.6-sol") = OpenAiPricing(
        model = model,
        retrievedAt = "2026-09-29T00:00:00Z",
        shortContextRates = OpenAiTokenRates(4.0, 0.4, 5.0, 20.0),
        longContextRates = OpenAiTokenRates(8.0, 0.8, 10.0, 30.0),
    )
}
