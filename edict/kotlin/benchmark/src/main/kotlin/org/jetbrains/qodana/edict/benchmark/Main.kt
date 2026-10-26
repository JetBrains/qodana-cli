// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.benchmark

import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.*
import org.jetbrains.qodana.edict.runtime.CodexPriceAnalyzer
import java.nio.file.Path
import kotlin.io.path.*
import kotlin.system.exitProcess

internal val json = Json { ignoreUnknownKeys = true; prettyPrint = true; encodeDefaults = true }

/** Every benchmark rule and the rule of each Signal, from the gold run's `edictBenchmarkManifest` property. */
internal data class GoldManifest(val rules: List<String>, val ruleBySignal: Map<String, String>)

internal data class Sarif(
    val findings: List<Finding>,
    val registeredRules: Set<String>,
    val manifest: GoldManifest? = null,
    /** The `expectedState` property of each rule that has one: `generated` or `discontinued`. */
    val expectedStates: Map<String, String> = emptyMap(),
)

internal val EXPECTED_STATES = setOf("generated", "discontinued")

private fun JsonObject.obj(name: String): JsonObject? = get(name)?.takeUnless { it is JsonNull }?.jsonObject
private fun JsonObject.array(name: String): JsonArray? = get(name)?.takeUnless { it is JsonNull }?.jsonArray

private fun JsonElement.id(): String = jsonObject.getValue("id").jsonPrimitive.content

private fun goldManifest(manifest: JsonObject, path: Path): GoldManifest {
    val cases = manifest.array("cases") ?: error("edictBenchmarkManifest without cases in $path")
    val signals = cases.flatMap { case -> (case.jsonObject.array("signals") ?: emptyList()).map { it.id() to case.id() } }
    require(signals.distinctBy { it.first }.size == signals.size) { "edictBenchmarkManifest assigns a Signal twice in $path" }
    return GoldManifest(cases.map { it.id() }, signals.toMap())
}

internal fun readSarif(path: Path): Sarif {
    val report = json.parseToJsonElement(path.readText()).jsonObject
    val runs = report.array("runs") ?: error("No SARIF runs in $path")
    require(runs.isNotEmpty()) { "No SARIF runs in $path" }
    val registered = mutableSetOf<String>()
    val expectedStates = mutableMapOf<String, String>()
    val findings = runs.flatMap { element ->
        val run = element.jsonObject
        val tool = run.obj("tool")
        val components = listOfNotNull(tool?.obj("driver")) + (tool?.array("extensions") ?: emptyList())
        components.forEach { component ->
            component.jsonObject.array("rules")?.forEach { rule ->
                val id = rule.jsonObject["id"]?.jsonPrimitive?.contentOrNull ?: return@forEach
                registered.add(id)
                rule.jsonObject.obj("properties")?.get("expectedState")?.jsonPrimitive?.contentOrNull?.let { state ->
                    require(state in EXPECTED_STATES) { "Rule $id has expectedState '$state', expected one of $EXPECTED_STATES in $path" }
                    expectedStates[id] = state
                }
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
                region?.get("charLength")?.jsonPrimitive?.longOrNull,
                value.obj("properties")?.get("edictLabel")?.jsonPrimitive?.contentOrNull)
        }
    }
    val manifest = runs.firstNotNullOfOrNull { it.jsonObject.obj("properties")?.obj("edictBenchmarkManifest") }
    return Sarif(findings, registered, manifest?.let { goldManifest(it, path) }, expectedStates)
}

// Gold without any edictLabel lists only expected findings, so each of its results is a positive.
internal fun readGold(path: Path): Sarif {
    val gold = readSarif(path)
    return if (gold.findings.any { it.label != null }) gold else gold.copy(findings = gold.findings.map { it.copy(label = POSITIVE) })
}

internal fun compare(_benchmarkDir: Path, stateDir: Path, outputDir: Path,
                     analysisSarif: Path = outputDir.resolve("qodana.sarif.json"),
                     generationPriceReport: JsonObject? = null): BenchmarkReport {
    val goldSarif = readGold(stateDir.resolve("gold.sarif.json"))
    val inputs = loadInputs(goldSarif, outputDir)
    val clusters = resolveClusters(inputs, stateDir)
    val statuses = clusterOutcomes(clusters, stateDir)
    val outcomes = clusters.mapValues { (_, members) -> generationOutcome(members, statuses) }
    val successful = inputs.rules.filter { outcomes[it] == "Generated" }
    val gold = goldSarif.findings
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
    val report = BenchmarkReport(metrics, aggregate(metrics), inputs.rules.size, successful.size,
        inputs.revision, outcomes, clusters, statuses, generationPriceReport, inputs.expectedStates,
        inputs.rules.count { meetsExpectedState(inputs.expectedStates.getValue(it), outcomes.getValue(it)) })

    outputDir.createDirectories()
    val inspectionsDir = outputDir.resolve("generatedInspections").createDirectories()
    // Remove only artifacts owned by this task, so a rerun cannot publish stale successful rules.
    inspectionsDir.listDirectoryEntries("*.kts").forEach { it.deleteExisting() }
    deleteTree(outputDir.resolve("specGoldComparisons"))
    generated.forEach { inspection ->
        val filename = if (clusters.getValue(inspection.rule).size == 1) inspection.rule else "${inspection.rule}--${inspection.cluster}"
        stateDir.resolve("inspections/${inspection.cluster}.inspection.kts")
            .copyTo(inspectionsDir.resolve("$filename.kts"), overwrite = true)
    }
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
        val expected = report.expectedStates.getValue(rule)
        teamCity("testStarted", "name" to rule)
        if (!meetsExpectedState(expected, status)) {
            teamCity("testFailed", "name" to rule, "message" to "Expected $expected, got $status")
        }
        teamCity("testFinished", "name" to rule)
        if (metric != null) println("$rule: TP=${metric.truePositives} FP=${metric.falsePositives} FN=${metric.falseNegatives} " +
            "matchedNegative=${metric.matchedNegative} matchedUnlabeled=${metric.matchedUnlabeled} " +
            "R=${metric.recall} P=${metric.precision} F1=${metric.f1Score}")
    }
    teamCity("buildStatisticValue", "key" to "edict.totalInspectionsProcessed", "value" to report.totalInspectionsProcessed)
    teamCity("buildStatisticValue", "key" to "edict.successful", "value" to report.successful)
    teamCity("buildStatisticValue", "key" to "edict.expectedStateMatched", "value" to report.expectedStateMatched)
    teamCity("buildStatisticValue", "key" to "edict.avgF1Score", "value" to report.aggregate.avgF1Score)
    teamCity("buildStatisticValue", "key" to "edict.totalTP", "value" to report.aggregate.totalTP)
    teamCity("buildStatisticValue", "key" to "edict.totalFP", "value" to report.aggregate.totalFP)
    teamCity("buildStatisticValue", "key" to "edict.totalFN", "value" to report.aggregate.totalFN)
    teamCity("buildStatisticValue", "key" to "edict.totalMatchedNegative", "value" to report.aggregate.totalMatchedNegative)
    teamCity("buildStatisticValue", "key" to "edict.totalMatchedUnlabeled", "value" to report.aggregate.totalMatchedUnlabeled)
    report.generationPriceReport?.get("totalPrice")?.jsonObject?.let { total ->
        total["priceUsd"]?.jsonPrimitive?.doubleOrNull?.let {
            teamCity("buildStatisticValue", "key" to "edict.priceUsd", "value" to it)
        }
        total["totalTokens"]?.jsonPrimitive?.longOrNull?.let {
            teamCity("buildStatisticValue", "key" to "edict.totalTokens", "value" to it)
        }
    }
    teamCity("buildStatus", "text" to "${report.successful}/${report.totalInspectionsProcessed} inspections generated, " +
        "${report.expectedStateMatched}/${report.totalInspectionsProcessed} in the expected state")
}

fun main(args: Array<String>) {
    try {
        val allowed = setOf("--benchmark-dir", "--state-dir", "--output-dir", "--analysis-sarif", "--project-dir")
        require(args.size % 2 == 0 && args.toList().chunked(2).all { it[0] in allowed }) {
            "Use --benchmark-dir <fixtures> --state-dir <project/.edict> --output-dir <reports> [--analysis-sarif <file>]"
        }
        val options = args.toList().chunked(2).associate { it[0] to it[1] }
        require(options.size == args.size / 2) { "Duplicate options" }
        fun path(name: String): Path = Path.of(options[name] ?: error("$name is required")).toAbsolutePath().normalize()
        val state = path("--state-dir")
        val output = path("--output-dir")
        val benchmark = path("--benchmark-dir")
        val project = options["--project-dir"]?.let { Path.of(it).toAbsolutePath().normalize() }
        val price = project?.let {
            CodexPriceAnalyzer.analyze(
                output.resolve("codex-home"),
                output.resolve("log/edict/edict-price-report.json"),
            ).also { result -> println(result.rendered) }
        }
        project?.let { generateSarif(it, state, output) }
        val report = compare(
            benchmark,
            state,
            output,
            options["--analysis-sarif"]?.let { Path.of(it).toAbsolutePath().normalize() }
                ?: output.resolve("qodana.sarif.json"),
            price?.report,
        )
        logReport(report)
        project?.let { verifyManagedCompletion(state) }
        check(report.successful > 0) { "No inspections generated; report.json contains the recorded generation outcomes" }
    } catch (error: Exception) {
        System.err.println("Benchmark comparison failed: ${error.message}")
        exitProcess(1)
    }
}
