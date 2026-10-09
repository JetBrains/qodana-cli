// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.runtime

import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.encodeToJsonElement
import kotlinx.serialization.json.jsonObject
import org.jetbrains.qodana.edict.common.json
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState.Plan
import java.nio.file.Files
import java.nio.file.Path
import java.time.Instant

data class CodexPriceAnalysis(
    val report: JsonObject,
    val rendered: String,
    val totalPriceUsd: Double,
    val totalTokens: Long,
)

object CodexPriceAnalyzer {
    fun analyze(codexHome: Path, output: Path): CodexPriceAnalysis =
        analyze(codexHome, Plan(request = "Offline price analysis", tasks = emptyList()), output)

    internal fun analyze(
        codexHome: Path,
        plan: Plan,
        output: Path,
        since: Instant? = null,
    ): CodexPriceAnalysis = analyze(codexHome, plan, output, OpenAiPricingLoader::fetch, since)

    internal fun analyze(
        codexHome: Path,
        plan: Plan,
        output: Path,
        loadPricing: (String) -> OpenAiPricing,
        since: Instant? = null,
    ): CodexPriceAnalysis {
        val sessions = codexHome.resolve("sessions")
        require(Files.isDirectory(sessions)) { "Codex sessions directory does not exist: $sessions" }
        val report = CodexPriceReporter.create(codexHome, plan, loadPricing, since)
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
