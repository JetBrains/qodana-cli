// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.runtime

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject
import org.jetbrains.qodana.edict.common.json
import org.jetbrains.qodana.edict.common.number
import org.jetbrains.qodana.edict.common.obj
import org.jetbrains.qodana.edict.common.text
import org.jetbrains.qodana.edict.common.wireJson
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState.Plan
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState.Task
import java.net.URI
import java.net.http.HttpClient
import java.net.http.HttpRequest
import java.net.http.HttpResponse
import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.StandardOpenOption.CREATE
import java.nio.file.StandardOpenOption.TRUNCATE_EXISTING
import java.time.Duration
import java.time.Instant

@Serializable
internal data class CodexTokenPrice(
    val uncachedInputTokens: Long = 0,
    val cachedInputTokens: Long = 0,
    val cacheWriteInputTokens: Long = 0,
    val outputTokens: Long = 0,
    val reasoningOutputTokens: Long = 0,
    val totalTokens: Long = 0,
    val uncachedInputPriceUsd: Double = 0.0,
    val cachedInputPriceUsd: Double = 0.0,
    val cacheWritePriceUsd: Double = 0.0,
    val outputPriceUsd: Double = 0.0,
    val priceUsd: Double = 0.0,
) {
    operator fun plus(other: CodexTokenPrice) = CodexTokenPrice(
        uncachedInputTokens + other.uncachedInputTokens,
        cachedInputTokens + other.cachedInputTokens,
        cacheWriteInputTokens + other.cacheWriteInputTokens,
        outputTokens + other.outputTokens,
        reasoningOutputTokens + other.reasoningOutputTokens,
        totalTokens + other.totalTokens,
        uncachedInputPriceUsd + other.uncachedInputPriceUsd,
        cachedInputPriceUsd + other.cachedInputPriceUsd,
        cacheWritePriceUsd + other.cacheWritePriceUsd,
        outputPriceUsd + other.outputPriceUsd,
        priceUsd + other.priceUsd,
    )
}

@Serializable
internal data class OpenAiTokenRates(
    val uncachedInputUsdPerMillion: Double,
    val cachedInputUsdPerMillion: Double,
    val cacheWriteUsdPerMillion: Double,
    val outputUsdPerMillion: Double,
)

@Serializable
internal data class OpenAiPricing(
    val model: String,
    val tier: String = "standard",
    val sourceUrl: String = OPENAI_PRICING_URL,
    val retrievedAt: String,
    val shortContextMaxInputTokens: Long = 272_000,
    val shortContextRates: OpenAiTokenRates,
    val longContextRates: OpenAiTokenRates? = null,
) {
    fun price(tokens: CodexTokenPrice, requestInputTokens: Long): CodexTokenPrice {
        val rates = if (requestInputTokens > shortContextMaxInputTokens) {
            checkNotNull(longContextRates) { "Official pricing has no long-context rates for $model" }
        } else shortContextRates
        val uncached = tokens.uncachedInputTokens * rates.uncachedInputUsdPerMillion / 1_000_000.0
        val cached = tokens.cachedInputTokens * rates.cachedInputUsdPerMillion / 1_000_000.0
        val cacheWrite = tokens.cacheWriteInputTokens * rates.cacheWriteUsdPerMillion / 1_000_000.0
        val output = tokens.outputTokens * rates.outputUsdPerMillion / 1_000_000.0
        return tokens.copy(
            uncachedInputPriceUsd = uncached,
            cachedInputPriceUsd = cached,
            cacheWritePriceUsd = cacheWrite,
            outputPriceUsd = output,
            priceUsd = uncached + cached + cacheWrite + output,
        )
    }
}

@Serializable
internal data class CodexStagePrice(
    val taskId: String,
    val skill: String,
    val title: String,
    val ownPrice: CodexTokenPrice,
    /** Own price plus every descendant task, recursively. */
    val inclusivePrice: CodexTokenPrice,
)

@Serializable
internal data class CodexClusterPrice(
    val taskId: String,
    val title: String,
    val ownPrice: CodexTokenPrice,
    val topSubstages: List<CodexStagePrice>,
    /** Cluster worker plus every substage and nested worker. */
    val inclusivePrice: CodexTokenPrice,
)

@Serializable
internal data class CodexPriceReport(
    val model: String,
    val currency: String = "usd",
    val pricing: OpenAiPricing,
    val managerOwnPrice: CodexTokenPrice,
    val topStages: List<CodexStagePrice>,
    val clusterGenerations: List<CodexClusterPrice>,
    val unattributedPrice: CodexTokenPrice,
    /** Manager, all managed tasks, and any unattributed runtime sessions exactly once. */
    val totalPrice: CodexTokenPrice,
) {
    fun render(): String = buildString {
        appendLine("Price report ($model; official OpenAI ${pricing.tier} list prices; inclusive prices contain every descendant):")
        appendLine("Pricing source: ${pricing.sourceUrl} (retrieved ${pricing.retrievedAt})")
        appendLine("Rates per 1M tokens: ${rates(pricing.shortContextRates)} (input <= ${"%,d".format(pricing.shortContextMaxInputTokens)})")
        pricing.longContextRates?.let { appendLine("Long-context rates: ${rates(it)}") }
        appendLine("Top stages:")
        topStages.forEach { appendLine("- ${it.skill}: ${format(it.inclusivePrice)}") }
        clusterGenerations.forEach { cluster ->
            appendLine("Cluster generation ${cluster.title}:")
            appendLine("- own generation/coordination: ${format(cluster.ownPrice)}")
            cluster.topSubstages.forEach { appendLine("- ${it.skill}: ${format(it.inclusivePrice)}") }
            appendLine("- cluster total: ${format(cluster.inclusivePrice)}")
        }
        appendLine("Manager own: ${format(managerOwnPrice)}")
        if (unattributedPrice.totalTokens > 0) appendLine("Unattributed sessions: ${format(unattributedPrice)}")
        append("Total price: ${format(totalPrice)}")
    }

    private fun rates(value: OpenAiTokenRates): String =
        "input=$${money(value.uncachedInputUsdPerMillion)}, cached=$${money(value.cachedInputUsdPerMillion)}, " +
            "cache-write=$${money(value.cacheWriteUsdPerMillion)}, output=$${money(value.outputUsdPerMillion)}"

    private fun format(value: CodexTokenPrice): String =
        "$${money(value.priceUsd)} (uncached-input=${"%,d".format(value.uncachedInputTokens)}/$${money(value.uncachedInputPriceUsd)}, " +
            "cached-input=${"%,d".format(value.cachedInputTokens)}/$${money(value.cachedInputPriceUsd)}, " +
            "cache-write=${"%,d".format(value.cacheWriteInputTokens)}/$${money(value.cacheWritePriceUsd)}, " +
            "output=${"%,d".format(value.outputTokens)}/$${money(value.outputPriceUsd)}, " +
            "reasoning-subset=${"%,d".format(value.reasoningOutputTokens)}, total=${"%,d".format(value.totalTokens)} tokens)"

    private fun money(value: Double): String = "%.6f".format(java.util.Locale.ROOT, value)
}

internal object CodexPriceReporter {
    fun create(home: Path, plan: Plan, pricing: OpenAiPricing): CodexPriceReport {
        val sessions = readSessions(home.resolve("sessions"), pricing)
        val tasksById = plan.tasks.associateBy(Task::id)
        val sessionsByAgent = buildMap {
            sessions.forEach { session ->
                put(session.id, session)
                session.agentPath.takeIf(String::isNotBlank)?.let { put(it, session) }
            }
        }
        val taskAgents = plan.tasks.map(Task::agentId).filter(String::isNotBlank).toSet()

        fun own(task: Task): CodexTokenPrice = sessionsByAgent[task.agentId]?.price ?: CodexTokenPrice()
        fun isDescendant(task: Task, ancestorId: String): Boolean {
            var parentId = task.parentId
            while (parentId.isNotEmpty()) {
                if (parentId == ancestorId) return true
                parentId = tasksById[parentId]?.parentId.orEmpty()
            }
            return false
        }
        fun inclusive(task: Task): CodexTokenPrice = plan.tasks.asSequence()
            .filter { it.id == task.id || isDescendant(it, task.id) }
            .fold(CodexTokenPrice()) { price, nested -> price + own(nested) }
        fun stage(task: Task) = CodexStagePrice(task.id, task.skill, task.title, own(task), inclusive(task))

        val roots = plan.tasks.filter { it.parentId.isEmpty() }.map(::stage)
        val clusters = plan.tasks.filter { it.skill == "edict-next-cluster-generation" }.map { cluster ->
            CodexClusterPrice(
                cluster.id,
                cluster.title,
                own(cluster),
                plan.tasks.filter { it.parentId == cluster.id }.map(::stage),
                inclusive(cluster),
            )
        }
        val manager = sessions.filter(SessionPrice::root)
            .fold(CodexTokenPrice()) { price, session -> price + session.price }
        val unattributed = sessions.filter { session ->
            !session.root && session.id !in taskAgents && session.agentPath !in taskAgents
        }
            .fold(CodexTokenPrice()) { price, session -> price + session.price }
        val total = sessions.fold(CodexTokenPrice()) { price, session -> price + session.price }
        return CodexPriceReport(pricing.model, pricing = pricing, managerOwnPrice = manager, topStages = roots, clusterGenerations = clusters,
            unattributedPrice = unattributed, totalPrice = total)
    }

    fun write(report: CodexPriceReport, path: Path) {
        Files.createDirectories(path.parent)
        Files.writeString(path, json.encodeToString(CodexPriceReport.serializer(), report) + "\n", CREATE, TRUNCATE_EXISTING)
    }

    private fun readSessions(directory: Path, pricing: OpenAiPricing): List<SessionPrice> {
        if (!Files.isDirectory(directory)) return emptyList()
        val result = mutableListOf<SessionPrice>()
        Files.walk(directory).use { paths -> paths.filter { Files.isRegularFile(it) && it.toString().endsWith(".jsonl") }
            .forEach { file -> readSession(file, pricing)?.let(result::add) } }
        return result
    }

    private fun readSession(path: Path, pricing: OpenAiPricing): SessionPrice? {
        var id = ""
        var agentPath = ""
        var root = false
        var price = CodexTokenPrice()
        var requestCosts = CodexTokenPrice()
        var requestPricesSeen = false
        Files.newBufferedReader(path).useLines { lines ->
            lines.filter(String::isNotBlank).forEach { line ->
                val event = runCatching { wireJson.parseToJsonElement(line).jsonObject }.getOrNull() ?: return@forEach
                val payload = event.obj("payload")
                if (event.text("type") == "session_meta") {
                    id = payload.text("id")
                    agentPath = payload.obj("source").obj("subagent").obj("thread_spawn").text("agent_path")
                    root = payload.text("parent_thread_id").isEmpty() &&
                        payload.obj("source").obj("subagent").obj("thread_spawn").text("parent_thread_id").isEmpty()
                }
                val usage: JsonObject? = when {
                    event.text("type") == "token_usage_record" -> {
                        val requestUsage = payload.obj("usage").toPrice()
                        requestCosts += pricing.price(requestUsage, requestUsage.inputTokens())
                        requestPricesSeen = true
                        payload.obj("thread_token_usage")
                    }
                    event.text("type") == "event_msg" && payload.text("type") == "token_count" ->
                        payload.obj("info").obj("total_token_usage")
                    else -> null
                }
                if (usage != null) price = usage.toPrice()
            }
        }
        val costs = if (requestPricesSeen) requestCosts else pricing.price(price, price.inputTokens())
        price = price.copy(
            uncachedInputPriceUsd = costs.uncachedInputPriceUsd,
            cachedInputPriceUsd = costs.cachedInputPriceUsd,
            cacheWritePriceUsd = costs.cacheWritePriceUsd,
            outputPriceUsd = costs.outputPriceUsd,
            priceUsd = costs.priceUsd,
        )
        return id.takeIf(String::isNotEmpty)?.let { SessionPrice(it, agentPath, root, price) }
    }

    private fun JsonObject.toPrice(): CodexTokenPrice {
        val input = number("input_tokens")
        val cached = number("cached_input_tokens")
        val cacheWrite = number("cache_write_input_tokens")
        require(input >= cached + cacheWrite) { "Input token categories overlap unexpectedly" }
        return CodexTokenPrice(
            uncachedInputTokens = input - cached - cacheWrite,
            cachedInputTokens = cached,
            cacheWriteInputTokens = cacheWrite,
            outputTokens = number("output_tokens"),
            reasoningOutputTokens = number("reasoning_output_tokens"),
            totalTokens = number("total_tokens").takeIf { it > 0 } ?: input + number("output_tokens"),
        )
    }

    private fun CodexTokenPrice.inputTokens() = uncachedInputTokens + cachedInputTokens + cacheWriteInputTokens

    private data class SessionPrice(val id: String, val agentPath: String, val root: Boolean, val price: CodexTokenPrice)
}

internal object OpenAiPricingLoader {
    fun fetch(model: String): OpenAiPricing {
        val request = HttpRequest.newBuilder(URI.create(OPENAI_PRICING_URL))
            .timeout(Duration.ofSeconds(30))
            .header("User-Agent", "qodana-edict-price-reporter")
            .GET().build()
        val response = HttpClient.newBuilder()
            .connectTimeout(Duration.ofSeconds(10))
            .followRedirects(HttpClient.Redirect.NORMAL)
            .build()
            .send(request, HttpResponse.BodyHandlers.ofString())
        check(response.statusCode() == 200) { "Official OpenAI pricing returned HTTP ${response.statusCode()}" }
        return parse(model, response.body(), Instant.now().toString())
    }

    internal fun parse(model: String, html: String, retrievedAt: String): OpenAiPricing {
        val decoded = html.replace("&quot;", "\"").replace("&#x2F;", "/")
        val number = Regex("""\[0,([0-9]+(?:\.[0-9]+)?)\]""")
        val grouped = Regex(""""model":\[0,"${Regex.escape(model)}"\]""").findAll(decoded).map { marker ->
            val end = (marker.range.last + 1_000).coerceAtMost(decoded.length)
            number.findAll(decoded.substring(marker.range.last + 1, end)).take(8)
                .map { it.groupValues[1].toDouble() }.toList()
        }.firstOrNull { it.size == 8 }
        val inline = Regex("""\[0,"${Regex.escape(model)}"\]((?:,\[0,[0-9]+(?:\.[0-9]+)?\]){4})""")
            .find(decoded)?.groupValues?.get(1)?.let { suffix ->
                number.findAll(suffix).map { it.groupValues[1].toDouble() }.toList()
            }
        val values = grouped ?: inline
        checkNotNull(values) { "No official OpenAI pricing found for model $model" }
        fun rates(offset: Int) = OpenAiTokenRates(
            values[offset],
            values[offset + 1],
            values[offset + 2],
            values[offset + 3],
        )
        return OpenAiPricing(
            model = model,
            retrievedAt = retrievedAt,
            shortContextRates = rates(0),
            longContextRates = values.takeIf { it.size == 8 }?.let { rates(4) },
        )
    }
}

internal const val OPENAI_PRICING_URL = "https://developers.openai.com/api/docs/pricing"
