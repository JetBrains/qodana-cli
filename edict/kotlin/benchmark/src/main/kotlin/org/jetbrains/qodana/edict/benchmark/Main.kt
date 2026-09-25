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

internal fun compare(benchmarkDir: Path, stateDir: Path, outputDir: Path,
                     analysisSarif: Path = outputDir.resolve("qodana.sarif.json")): BenchmarkReport {
    // Specifications stay in the VCS checkout; reporting reads them only after generation.
    val inputs = loadInputs(benchmarkDir, outputDir)
    val clusters = resolveClusters(inputs, stateDir)
    val statuses = clusterOutcomes(clusters, stateDir)
    val outcomes = clusters.mapValues { (_, members) -> generationOutcome(members, statuses) }
    val successful = inputs.specifications.filter { outcomes[it.ruleId] == "Generated" }
    val gold = readSarif(stateDir.resolve("gold.sarif.json")).findings
    val analysis = readSarif(analysisSarif)
    val generated = scoringInspections(clusters, statuses)
    val generatedIds = generated.associate { it.id to it.rule }
    require(analysis.registeredRules.containsAll(generatedIds.keys)) {
        "Generated inspections missing from analysis SARIF: ${generatedIds.keys - analysis.registeredRules}"
    }
    // Only the namespaced generated rules count; a built-in inspection must never substitute for one.
    val findings = analysis.findings.mapNotNull { finding ->
        generatedIds[finding.ruleId]?.let { finding.copy(ruleId = it) }
    }.groupBy { it.ruleId }.flatMap { (rule, matches) ->
        if (clusters.getValue(rule).size > 1) matches.distinct() else matches
    }
    val metrics = successful.map { calculateMetrics(it, gold, findings) }
    val comparisons = successful.map { compareSpecWithGold(it, gold) }
    val report = BenchmarkReport(metrics, aggregate(metrics), inputs.specifications.size, successful.size,
        specGoldAggregate(comparisons), inputs.revision, outcomes, clusters, statuses)

    outputDir.createDirectories()
    val inspectionsDir = outputDir.resolve("generatedInspections").createDirectories()
    val specGoldDir = outputDir.resolve("specGoldComparisons").createDirectories()
    // Remove only artifacts owned by this task, so a rerun cannot publish stale successful rules.
    inspectionsDir.listDirectoryEntries("*.kts").forEach { it.deleteExisting() }
    specGoldDir.listDirectoryEntries("*.json").forEach { it.deleteExisting() }
    generated.forEach { inspection ->
        val filename = if (clusters.getValue(inspection.rule).size == 1) inspection.rule else "${inspection.rule}--${inspection.cluster}"
        stateDir.resolve("inspections/${inspection.cluster}.inspection.kts")
            .copyTo(inspectionsDir.resolve("$filename.kts"), overwrite = true)
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
        val allowed = setOf("--benchmark-dir", "--state-dir", "--output-dir", "--analysis-sarif", "--project-dir")
        require(args.size % 2 == 0 && args.toList().chunked(2).all { it[0] in allowed }) {
            "Use --benchmark-dir <fixtures> --state-dir <project/.edict> --output-dir <reports> [--analysis-sarif <file>]"
        }
        val options = args.toList().chunked(2).associate { it[0] to Path.of(it[1]).toAbsolutePath().normalize() }
        require(options.size == args.size / 2) { "Duplicate options" }
        val state = options["--state-dir"] ?: error("--state-dir is required")
        val output = options["--output-dir"] ?: error("--output-dir is required")
        val benchmark = options["--benchmark-dir"] ?: error("--benchmark-dir is required")
        options["--project-dir"]?.let { generateSarif(it, benchmark, state, output) }
        val report = compare(benchmark, state, output, options["--analysis-sarif"] ?: output.resolve("qodana.sarif.json"))
        logReport(report)
        options["--project-dir"]?.let { verifyManagedCompletion(state) }
        check(report.successful > 0) { "No inspections generated; report.json contains the recorded generation outcomes" }
    } catch (error: Exception) {
        System.err.println("Benchmark comparison failed: ${error.message}")
        exitProcess(1)
    }
}
