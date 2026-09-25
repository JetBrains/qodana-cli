// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.benchmark

import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.*
import java.nio.file.Path
import kotlin.io.path.*
import kotlin.system.exitProcess

internal val json = Json { ignoreUnknownKeys = true; prettyPrint = true; encodeDefaults = true }

internal data class Sarif(val findings: List<Finding>, val registeredRules: Set<String>)

private fun JsonObject.obj(name: String): JsonObject? = get(name)?.takeUnless { it is JsonNull }?.jsonObject
private fun JsonObject.array(name: String): JsonArray? = get(name)?.takeUnless { it is JsonNull }?.jsonArray

internal fun readSarif(path: Path): Sarif {
    val report = json.parseToJsonElement(path.readText()).jsonObject
    val runs = report.array("runs") ?: error("No SARIF runs in $path")
    require(runs.isNotEmpty()) { "No SARIF runs in $path" }
    val registered = mutableSetOf<String>()
    val findings = runs.flatMap { element ->
        val run = element.jsonObject
        val tool = run.obj("tool")
        val components = listOfNotNull(tool?.obj("driver")) + (tool?.array("extensions") ?: emptyList())
        components.forEach { component ->
            component.jsonObject.array("rules")?.forEach { rule ->
                rule.jsonObject["id"]?.jsonPrimitive?.contentOrNull?.let(registered::add)
            }
        }
        (run.array("results") ?: emptyList()).map { result ->
            val value = result.jsonObject
            val location = value.array("locations")?.firstOrNull()?.jsonObject?.obj("physicalLocation")
            val region = location?.obj("region")
            Finding(value["ruleId"]?.jsonPrimitive?.contentOrNull ?: error("SARIF result without ruleId in $path"),
                location?.obj("artifactLocation")?.get("uri")?.jsonPrimitive?.contentOrNull,
                region?.get("startLine")?.jsonPrimitive?.longOrNull,
                region?.get("charOffset")?.jsonPrimitive?.longOrNull,
                region?.get("charLength")?.jsonPrimitive?.longOrNull)
        }
    }
    return Sarif(findings, registered)
}

internal fun compare(benchmarkDir: Path, generationDir: Path, outputDir: Path,
                     analysisSarif: Path = generationDir.resolve("qodana.sarif.json")): BenchmarkReport {
    // Use the immutable snapshot selected before generation, including held-out optional examples.
    val inputs = json.decodeFromString<BenchmarkInputs>(generationDir.resolve("inputs.json").readText())
    require(inputs.specifications.isNotEmpty()) { "No benchmark specifications" }
    val rules = inputs.specifications.map { it.ruleId }
    require(rules.distinct().size == rules.size && inputs.clusterToRule.values.toSet() == rules.toSet() &&
        inputs.clusterToRule.size == rules.size) { "Specifications and cluster mapping must be one-to-one" }
    require(inputs.clusterToRule.all { (cluster, rule) ->
        cluster.matches(Regex("[a-z0-9][a-z0-9-]*")) && rule.matches(Regex("[A-Za-z0-9]+"))
    }) { "Unsafe cluster or rule ID" }

    val outcomes = inputs.clusterToRule.map { (cluster, rule) ->
        val description = json.parseToJsonElement(
            generationDir.resolve("state/clusters/$cluster/description.json").readText()).jsonObject
        rule to description.getValue("status").jsonPrimitive.content
    }.toMap()
    val successful = inputs.specifications.filter { outcomes[it.ruleId] == "Generated" }
    val gold = readSarif(benchmarkDir.resolve("gold.sarif.json")).findings
    val analysis = readSarif(analysisSarif)
    val generatedIds = successful.associate { "EdictBenchmark${it.ruleId}" to it.ruleId }
    require(analysis.registeredRules.containsAll(generatedIds.keys)) {
        "Generated inspections missing from analysis SARIF: ${generatedIds.keys - analysis.registeredRules}"
    }
    // Only the namespaced generated rules count; a built-in inspection must never substitute for one.
    val findings = analysis.findings.mapNotNull { finding ->
        generatedIds[finding.ruleId]?.let { finding.copy(ruleId = it) }
    }
    val metrics = successful.map { calculateMetrics(it, gold, findings) }
    val comparisons = successful.map { compareSpecWithGold(it, gold) }
    val report = BenchmarkReport(metrics, aggregate(metrics), inputs.specifications.size, successful.size,
        specGoldAggregate(comparisons), inputs.revision, outcomes)

    outputDir.createDirectories()
    val inspectionsDir = outputDir.resolve("generatedInspections").createDirectories()
    val specGoldDir = outputDir.resolve("specGoldComparisons").createDirectories()
    // Remove only artifacts owned by this task, so a rerun cannot publish stale successful rules.
    inspectionsDir.listDirectoryEntries("*.kts").forEach { it.deleteExisting() }
    specGoldDir.listDirectoryEntries("*.json").forEach { it.deleteExisting() }
    val clusters = inputs.clusterToRule.entries.associate { it.value to it.key }
    successful.forEach { spec ->
        generationDir.resolve("state/inspections/${clusters.getValue(spec.ruleId)}.inspection.kts")
            .copyTo(inspectionsDir.resolve("${spec.ruleId}.kts"), overwrite = true)
    }
    comparisons.forEach { specGoldDir.resolve("${it.ruleId}.json").writeText(json.encodeToString(it) + "\n") }
    val destinationSarif = outputDir.resolve("qodana.sarif.json")
    if (analysisSarif.toAbsolutePath().normalize() != destinationSarif.toAbsolutePath().normalize()) {
        analysisSarif.copyTo(destinationSarif, overwrite = true)
    }
    outputDir.resolve("report.json").writeText(json.encodeToString(report) + "\n")
    return report
}

internal fun teamCity(kind: String, vararg attributes: Pair<String, Any>) {
    fun escape(value: Any) = value.toString().replace("|", "||").replace("'", "|'")
        .replace("\n", "|n").replace("\r", "|r").replace("[", "|[").replace("]", "|]")
    println("##teamcity[$kind ${attributes.joinToString(" ") { "${it.first}='${escape(it.second)}'" }}]")
}

internal fun logReport(report: BenchmarkReport) {
    val byRule = report.inspectionMetrics.associateBy { it.ruleId }
    report.generationOutcomes.forEach { (rule, status) ->
        val metric = byRule[rule]
        teamCity("testStarted", "name" to rule)
        if (metric == null || !metric.satisfiesSpec) {
            teamCity("testFailed", "name" to rule, "message" to "$status; specification satisfied: ${metric?.satisfiesSpec ?: false}")
        }
        teamCity("testFinished", "name" to rule)
        if (metric != null) println("$rule: TP=${metric.truePositives} FP=${metric.falsePositives} FN=${metric.falseNegatives} " +
            "R=${metric.recall} P=${metric.precision} F1=${metric.f1Score} Spec=${metric.satisfiesSpec}")
    }
    teamCity("buildStatisticValue", "key" to "edict.totalInspectionsProcessed", "value" to report.totalInspectionsProcessed)
    teamCity("buildStatisticValue", "key" to "edict.successful", "value" to report.successful)
    teamCity("buildStatisticValue", "key" to "edict.specSatisfiedCount", "value" to report.aggregate.specSatisfiedCount)
    teamCity("buildStatisticValue", "key" to "edict.avgF1Score", "value" to report.aggregate.avgF1Score)
    teamCity("buildStatus", "text" to "${report.successful}/${report.totalInspectionsProcessed} inspections generated, " +
        "${report.aggregate.specSatisfiedCount}/${report.successful} specs satisfied")
}

fun main(args: Array<String>) {
    try {
        val allowed = setOf("--benchmark-dir", "--generation-dir", "--output-dir", "--analysis-sarif")
        require(args.size % 2 == 0 && args.toList().chunked(2).all { it[0] in allowed }) {
            "Use --benchmark-dir <fixtures> --generation-dir <generation artifacts> [--output-dir <reports>] [--analysis-sarif <file>]"
        }
        val options = args.toList().chunked(2).associate { it[0] to Path.of(it[1]).toAbsolutePath().normalize() }
        require(options.size == args.size / 2) { "Duplicate options" }
        val generation = options["--generation-dir"] ?: error("--generation-dir is required")
        val report = compare(options["--benchmark-dir"] ?: error("--benchmark-dir is required"), generation,
            options["--output-dir"] ?: generation, options["--analysis-sarif"] ?: generation.resolve("qodana.sarif.json"))
        logReport(report)
        check(report.successful > 0) { "No inspections generated; report.json contains the recorded generation outcomes" }
    } catch (error: Exception) {
        System.err.println("Benchmark comparison failed: ${error.message}")
        exitProcess(1)
    }
}
