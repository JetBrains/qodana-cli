// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.benchmark

import kotlinx.serialization.json.*
import java.nio.file.Path
import kotlin.io.path.*

// Distribution may choose a different cluster ID. Match the original feedback provenance,
// without prescribing clustering or exposing the held-out specifications to the agent.
internal fun resolveClusters(inputs: BenchmarkInputs, generation: Path): Map<String, String> {
    val root = generation.resolve("state/clusters")
    val memberships = root.listDirectoryEntries().filter { it.isDirectory() }.associate { directory ->
        val signals = directory.resolve("signals")
        directory.name to if (!signals.exists()) emptySet() else signals.listDirectoryEntries("*.json").mapNotNull {
            readObject(it)["provenance"]?.jsonObject?.get("workItemId")?.jsonPrimitive?.content
                ?.let { origin -> Regex("^benchmark/([A-Za-z0-9]+)/specification\\.json#/").find(origin)?.groupValues?.get(1) }
        }.toSet()
    }
    val result = inputs.clusterToRule.map { (reserved, rule) ->
        val matches = memberships.filterValues { rule in it }.keys
        require(matches.size <= 1) { "Feedback for $rule was split across $matches; cannot assign a single inspection" }
        (matches.singleOrNull() ?: reserved) to rule
    }
    require(result.map { it.first }.distinct().size == result.size) { "Several specifications were merged into one cluster; cannot attribute per-rule metrics" }
    return result.toMap()
}

// Only descriptor metadata changes in the scoring copy; the accepted source stays untouched.
// Fail closed for unsupported descriptor forms instead of accidentally scoring a built-in rule.
internal fun scoringCode(code: String, rule: String): String {
    require(rule.matches(Regex("[A-Za-z0-9]+")))
    val descriptor = Regex("""\bInspectionKts\s*\(\s*(?:id\s*=\s*)?"[^"\\]*"""")
    val matches = descriptor.findAll(code).toList()
    require(matches.size == 1) { "Expected one InspectionKts descriptor with a literal first ID for $rule" }
    val match = matches.single()
    return code.replaceRange(match.range, match.value.substringBefore('"') + "\"EdictBenchmark$rule\"")
}

internal fun generateSarif(project: Path, generation: Path) {
    val inputs = json.decodeFromString<BenchmarkInputs>(generation.resolve("inputs.json").readText())
    val codes = resolveClusters(inputs, generation).mapNotNull { (cluster, rule) ->
        val status = readObject(generation.resolve("state/clusters/$cluster/description.json")).string("status")
        if (status != "Generated") null else "EdictBenchmark$rule" to scoringCode(
            generation.resolve("state/inspections/$cluster.inspection.kts").readText(), rule)
    }.toMap()
    val sarif = if (codes.isEmpty()) obj("runs" to JsonArray(listOf(obj("results" to JsonArray(emptyList())))))
        else ProjectRunner(project, generation).scan(generation.resolve("evaluation"), codes)
    writeJson(generation.resolve("qodana.sarif.json"), sarif)
}

internal fun verifyManagedCompletion(generation: Path) {
    require(generation.resolve("state/inbox").listDirectoryEntries("*.json").isEmpty()) { "Codex left unprocessed inbox signals" }
    val plans = generation.resolve("state/plans").listDirectoryEntries("*.json")
    require(plans.isNotEmpty()) { "No managed Edict plan was created" }
    val tasks = plans.flatMap { readObject(it).getValue("tasks").jsonArray }
    require(tasks.isNotEmpty() && tasks.all { it.jsonObject.string("status") == "completed" }) { "Managed Edict tasks are unfinished" }
    require(setOf("edict-prepare", "edict-distribution", "edict-generation").all { skill ->
        tasks.any { it.jsonObject.string("skill") == skill }
    }) { "Managed Edict did not complete preparation, distribution and generation" }
}
