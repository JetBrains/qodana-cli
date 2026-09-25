// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.benchmark

import kotlinx.serialization.json.*
import java.nio.file.Path
import kotlin.io.path.*

// Discover only clusters created by Edict. A specification may span several clusters,
// and a cluster may contain feedback from several specifications.
internal fun resolveClusters(inputs: BenchmarkInputs, generation: Path): Map<String, List<String>> {
    val rules = inputs.specifications.map { it.ruleId }
    require(rules.isNotEmpty() && rules.distinct().size == rules.size && rules.all { it.matches(Regex("[A-Za-z0-9]+")) }) {
        "Expected distinct, safe benchmark rule IDs"
    }
    val root = generation.resolve("state/clusters")
    val directories = if (root.exists()) root.listDirectoryEntries().filter { it.isDirectory() }.sortedBy { it.name } else emptyList()
    val memberships = directories.associate { directory ->
        require(directory.name.matches(Regex("[a-z0-9][a-z0-9-]*"))) { "Unsafe cluster ID: ${directory.name}" }
        val signals = directory.resolve("signals")
        directory.name to if (!signals.exists()) emptySet() else signals.listDirectoryEntries("*.json").mapNotNull {
            readObject(it)["provenance"]?.jsonObject?.get("workItemId")?.jsonPrimitive?.content
                ?.let { origin -> Regex("^benchmark/([A-Za-z0-9]+)/specification\\.json#/").find(origin)?.groupValues?.get(1) }
        }.toSet()
    }
    return rules.associateWith { rule -> memberships.filterValues { rule in it }.keys.toList() }
}

internal fun clusterOutcomes(clusters: Map<String, List<String>>, generation: Path): Map<String, String> =
    clusters.values.flatten().distinct().associateWith { cluster ->
        readObject(generation.resolve("state/clusters/$cluster/description.json")).string("status")
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

internal fun scoringInspections(clusters: Map<String, List<String>>, statuses: Map<String, String>): List<ScoringInspection> =
    clusters.flatMap { (rule, members) ->
        members.mapIndexedNotNull { index, cluster ->
            if (statuses.getValue(cluster) != "Generated") null else ScoringInspection(rule, cluster,
                "EdictBenchmark$rule" + if (members.size == 1) "" else "_Cluster${index + 1}")
        }
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

internal fun generateSarif(project: Path, generation: Path) {
    val inputs = json.decodeFromString<BenchmarkInputs>(generation.resolve("inputs.json").readText())
    val clusters = resolveClusters(inputs, generation)
    val codes = scoringInspections(clusters, clusterOutcomes(clusters, generation)).associate { inspection ->
        inspection.id to scoringCode(generation.resolve("state/inspections/${inspection.cluster}.inspection.kts").readText(), inspection.id)
    }
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
