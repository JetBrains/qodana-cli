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
import java.nio.file.Files
import java.nio.file.Path
import kotlin.test.assertContains
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

class CodexPriceReportTest {
    @TempDir
    lateinit var directory: Path

    @Test
    fun `prices top stages and cluster substages inclusively without double counting total`() {
        val sessions = directory.resolve("sessions")
        fun session(id: String, parent: String, agentPath: String = "", tokens: Long) {
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
            Files.writeString(sessions.resolve("$id.jsonl"), wireJson.encodeToString(meta) + "\n" + wireJson.encodeToString(usageRecord) + "\n")
        }

        session("manager", "", tokens = 10)
        session("extract-thread", "manager", "/root/extract", 20)
        session("generation-thread", "manager", "/root/generation", 30)
        session("cluster-thread", "generation-thread", "/root/generation/cluster", 40)
        session("review-thread", "cluster-thread", "/root/generation/cluster/review", 50)
        val extract = Task(id = "extract-task", skill = "edict-batch-signal-analysis", title = "Extract", agentId = "/root/extract")
        val generation = Task(id = "generation-task", skill = "edict-next-generation", title = "Generate", agentId = "/root/generation")
        val cluster = Task(id = "cluster-task", parentId = generation.id, skill = "edict-next-cluster-generation", title = "Rule", agentId = "/root/generation/cluster")
        val review = Task(id = "review-task", parentId = cluster.id, skill = "edict-next-inspection-code-review", title = "Review", agentId = "/root/generation/cluster/review")
        val pricing = pricing()
        val plan = Plan(request = "test", tasks = listOf(extract, generation, cluster, review))
        val report = CodexPriceReporter.create(directory, plan, pricing)

        assertEquals(20, report.topStages.single { it.taskId == extract.id }.inclusivePrice.totalTokens)
        assertEquals(120, report.topStages.single { it.taskId == generation.id }.inclusivePrice.totalTokens)
        assertEquals(90, report.clusterGenerations.single().inclusivePrice.totalTokens)
        assertEquals(50, report.clusterGenerations.single().topSubstages.single().inclusivePrice.totalTokens)
        assertEquals(0, report.unattributedPrice.totalTokens)
        assertEquals(150, report.totalPrice.totalTokens)
        assertEquals(0.0, report.totalPrice.uncachedInputPriceUsd)
        assertEquals(0.000054, report.totalPrice.cachedInputPriceUsd, absoluteTolerance = 1e-12)
        assertEquals(0.000050, report.totalPrice.cacheWritePriceUsd, absoluteTolerance = 1e-12)
        assertEquals(0.000100, report.totalPrice.outputPriceUsd, absoluteTolerance = 1e-12)
        assertEquals(0.000204, report.totalPrice.priceUsd, absoluteTolerance = 1e-12)
        assertContains(report.render(), "Total price: $0.000204")
        assertContains(report.render(), "uncached-input=0")

        val plans = directory.resolve("state/plans")
        Files.createDirectories(plans)
        Files.writeString(plans.resolve("plan.json"), wireJson.encodeToString(Plan.serializer(), plan))
        val output = directory.resolve("price.json")
        val analysis = CodexPriceAnalyzer.analyze(directory, directory.resolve("state"), output, pricing)
        assertEquals(150, analysis.totalTokens)
        assertEquals(0.000204, analysis.totalPriceUsd, absoluteTolerance = 1e-12)
        assertEquals(150, analysis.report.getValue("totalPrice").jsonObject
            .getValue("totalTokens").jsonPrimitive.long)
        assertTrue(Files.size(output) > 0)
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

    private fun pricing() = OpenAiPricing(
        model = "gpt-5.6-sol",
        retrievedAt = "2026-09-29T00:00:00Z",
        shortContextRates = OpenAiTokenRates(4.0, 0.4, 5.0, 20.0),
        longContextRates = OpenAiTokenRates(8.0, 0.8, 10.0, 30.0),
    )
}
