// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.benchmark

import kotlinx.serialization.json.*
import java.nio.file.Path
import kotlin.io.path.*

// A gold manifest names every rule, including one without a confirmed positive; otherwise the gold findings do.
internal fun loadInputs(gold: Sarif, output: Path): BenchmarkInputs {
    val rules = gold.manifest?.rules?.sorted() ?: gold.findings.map { it.ruleId }.distinct().sorted()
    return BenchmarkInputs(output.resolve("source-revision.txt").readText().trim(), rules, gold.manifest?.ruleBySignal,
        rules.associateWith { gold.expectedStates[it] ?: "generated" })
}

// A rule meets its expected state when its clusters end Generated or Discontinued as the gold rule expects.
internal fun meetsExpectedState(expected: String, outcome: String): Boolean = outcome.equals(expected, ignoreCase = true)

private val SPECIFICATION_REFERENCE = Regex("^benchmark/([A-Za-z0-9]+)/specification\\.json#/?")

// With a gold manifest, a Signal belongs to the rule listed for its id; otherwise to its specification reference.
private fun signalRule(signal: JsonObject, ruleBySignal: Map<String, String>?): String? {
    if (ruleBySignal != null) return signal["id"]?.jsonPrimitive?.contentOrNull?.let(ruleBySignal::get)
    val origin = signal["source"]?.jsonObject?.get("suggestionId")?.jsonPrimitive?.contentOrNull
        ?: signal["provenance"]?.jsonObject?.get("workItemId")?.jsonPrimitive?.contentOrNull
    return origin?.let { SPECIFICATION_REFERENCE.find(it)?.groupValues?.get(1) }
}

// Discover only clusters created by Edict. A rule may span several clusters,
// and a cluster may contain feedback for several rules.
internal fun resolveClusters(inputs: BenchmarkInputs, state: Path): Map<String, List<String>> {
    val rules = inputs.rules
    require(rules.isNotEmpty() && rules.distinct().size == rules.size && rules.all { it.matches(Regex("[A-Za-z0-9]+(-[A-Za-z0-9]+)*")) }) {
        "Expected distinct, safe benchmark rule IDs"
    }
    val root = state.resolve("clusters")
    val directories = if (root.exists()) root.listDirectoryEntries().filter { it.isDirectory() }.sortedBy { it.name } else emptyList()
    val memberships = directories.associate { directory ->
        require(directory.name.matches(Regex("[a-z0-9][a-z0-9-]*"))) { "Unsafe cluster ID: ${directory.name}" }
        val signals = directory.resolve("signals")
        directory.name to if (!signals.exists()) emptySet() else signals.listDirectoryEntries("*.json").mapNotNull {
            signalRule(readObject(it), inputs.ruleBySignal)
        }.toSet()
    }
    return rules.associateWith { rule -> memberships.filterValues { rule in it }.keys.toList() }
}

internal fun clusterOutcomes(clusters: Map<String, List<String>>, state: Path): Map<String, String> =
    clusters.values.flatten().distinct().associateWith { cluster ->
        val directory = state.resolve("clusters/$cluster")
        val manifest = directory.resolve("cluster.json").takeIf { it.exists() }
            ?: directory.resolve("description.json")
        readObject(manifest).string("status")
    }

internal fun generationOutcome(clusters: List<String>, statuses: Map<String, String>): String {
    val outcomes = clusters.map { statuses.getValue(it) }.distinct()
    return when {
        outcomes.isEmpty() -> "NotClustered"
        outcomes.size == 1 -> outcomes.single()
        "Generated" in outcomes -> "PartiallyGenerated"
        else -> outcomes.sorted().joinToString("+")
    }
}

internal data class ScoringInspection(val rule: String, val cluster: String, val id: String)

// Kebab-case rule ids become PascalCase, so every scoring id stays a plain identifier.
internal fun scoringInspections(clusters: Map<String, List<String>>, statuses: Map<String, String>): List<ScoringInspection> =
    clusters.flatMap { (rule, members) ->
        val id = "EdictBenchmark" + rule.split('-').joinToString("") { it.replaceFirstChar(Char::uppercaseChar) }
        members.mapIndexedNotNull { index, cluster ->
            if (statuses.getValue(cluster) != "Generated") null else ScoringInspection(rule, cluster,
                id + if (members.size == 1) "" else "_Cluster${index + 1}")
        }
    }.also { inspections ->
        require(inspections.distinctBy { it.id }.size == inspections.size) { "Benchmark rule IDs collide as scoring IDs" }
    }

// Only descriptor metadata changes in the scoring copy; the accepted source stays untouched.
// Fail closed for unsupported descriptor forms instead of accidentally scoring a built-in rule.
internal fun scoringCode(code: String, id: String): String {
    require(id.matches(Regex("EdictBenchmark[A-Za-z0-9_]+")))
    val descriptor = Regex("""\bInspectionKts\s*\(\s*(?:id\s*=\s*)?"[^"\\]*"""")
    val matches = descriptor.findAll(code).toList()
    require(matches.size == 1) { "Expected one InspectionKts descriptor with a literal first ID for $id" }
    val match = matches.single()
    return code.replaceRange(match.range, match.value.substringBefore('"') + "\"$id\"")
}

internal fun generateSarif(project: Path, state: Path, output: Path) {
    val inputs = loadInputs(readGold(state.resolve("gold.sarif.json")), output)
    val clusters = resolveClusters(inputs, state)
    val codes = scoringInspections(clusters, clusterOutcomes(clusters, state)).associate { inspection ->
        inspection.id to scoringCode(state.resolve("inspections/${inspection.cluster}.inspection.kts").readText(), inspection.id)
    }
    val sarif = if (codes.isEmpty()) obj("runs" to JsonArray(listOf(obj("results" to JsonArray(emptyList())))))
        else ProjectRunner(project, output).scan(output.resolve("evaluation"), codes)
    writeJson(output.resolve("qodana.sarif.json"), sarif)
}

internal fun verifyManagedCompletion(state: Path) {
    require(state.resolve("inbox").listDirectoryEntries("*.json").isEmpty()) { "Codex left unprocessed inbox signals" }
}
