// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.runtime

import kotlinx.serialization.decodeFromString
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.encodeToJsonElement
import kotlinx.serialization.json.jsonObject
import org.jetbrains.qodana.edict.common.json
import org.jetbrains.qodana.edict.edictnext.EdictNextJson
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState.Plan
import java.nio.file.Files
import java.nio.file.Path

data class CodexPriceAnalysis(
    val report: JsonObject,
    val rendered: String,
    val totalPriceUsd: Double,
    val totalTokens: Long,
)

object CodexPriceAnalyzer {
    fun analyze(codexHome: Path, stateDirectory: Path, model: String, output: Path): CodexPriceAnalysis =
        analyze(codexHome, stateDirectory, output, OpenAiPricingLoader.fetch(model))

    internal fun analyze(
        codexHome: Path,
        stateDirectory: Path,
        output: Path,
        pricing: OpenAiPricing,
    ): CodexPriceAnalysis {
        val sessions = codexHome.resolve("sessions")
        require(Files.isDirectory(sessions)) { "Codex sessions directory does not exist: $sessions" }
        val plans = stateDirectory.resolve("plans")
        require(Files.isDirectory(plans)) { "Edict plans directory does not exist: $plans" }
        val planFiles = Files.list(plans).use { paths ->
            paths.filter { Files.isRegularFile(it) && it.fileName.toString().endsWith(".json") }.toList()
        }
        val planPath = planFiles.maxByOrNull { Files.getLastModifiedTime(it).toMillis() }
            ?: error("No managed plan found in $plans")
        val plan = EdictNextJson.decodeFromString<Plan>(Files.readString(planPath))
        val report = CodexPriceReporter.create(codexHome, plan, pricing)
        check(report.totalPrice.totalTokens > 0) { "No Codex token usage found in $sessions" }
        CodexPriceReporter.write(report, output)
        return CodexPriceAnalysis(
            json.encodeToJsonElement(CodexPriceReport.serializer(), report).jsonObject,
            report.render(),
            report.totalPrice.priceUsd,
            report.totalPrice.totalTokens,
        )
    }
}
