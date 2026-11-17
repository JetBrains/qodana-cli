// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.benchmark

import kotlinx.serialization.json.*
import java.nio.file.*
import java.util.concurrent.TimeUnit
import kotlin.io.path.*

internal fun obj(vararg values: Pair<String, JsonElement>) = JsonObject(mapOf(*values))
internal fun text(value: String) = JsonPrimitive(value)
internal fun JsonObject.string(name: String) = getValue(name).jsonPrimitive.content

internal fun writeJson(path: Path, value: JsonElement) {
    path.parent.createDirectories()
    path.writeText(json.encodeToString(JsonElement.serializer(), value) + "\n")
}

internal fun readObject(path: Path) = json.parseToJsonElement(path.readText()).jsonObject

// Edict evaluates inspections on every content file, test sources included. The empty base drops the recommended
// profile's exclusions (test sources, tests/, tools/, build/, .gitignore and others); the FlexInspect group loads the
// generated scripts, which an empty profile does not contain.
internal fun projectConfiguration(rules: Set<String>) = obj(
    "version" to text("1.0"),
    "profile" to obj(
        "base" to obj("name" to text("empty")),
        "inspections" to JsonArray(listOf(obj("group" to text("category:FlexInspect"), "enabled" to JsonPrimitive(true)))),
    ),
    "include" to JsonArray(rules.map { obj("name" to text(it)) }),
)

internal fun stop(process: Process) {
    val descendants = process.descendants().toList()
    if (process.isAlive) process.destroy()
    if (!process.waitFor(20, TimeUnit.SECONDS)) process.destroyForcibly()
    descendants.filter { it.isAlive }.forEach { it.destroyForcibly() }
    process.waitFor(5, TimeUnit.SECONDS)
}

internal fun commandOutput(vararg command: String): String {
    val process = ProcessBuilder(*command).redirectError(ProcessBuilder.Redirect.INHERIT).start()
    val result = process.inputStream.bufferedReader().readText().trim()
    check(process.waitFor() == 0) { "Command failed: ${command.first()}" }
    return result
}

internal fun qodanaProcess(project: Path, results: Path, cache: Path, log: Path, vararg extra: String): ProcessBuilder {
    results.createDirectories(); cache.createDirectories(); log.parent.createDirectories()
    val distribution = System.getenv("QODANA_DIST") ?: error("QODANA_DIST must point to the native distribution")
    return ProcessBuilder(listOf(System.getenv("QODANA_CLI") ?: "qodana", "scan", "--within-docker=false",
        "--ide", distribution, "--project-dir", project.toString(),
        "--results-dir", results.toString(), "--cache-dir", cache.toString(), "--disable-sanity", "--run-promo=false",
        "--save-report=false", "--property=idea.headless.enable.statistics=false") + extra)
        .redirectErrorStream(true).redirectOutput(log.toFile()).apply {
            // Every IDE process needs its own configuration directory to avoid lock collisions.
            environment()["QODANA_CONF"] = cache.resolve("config").toString()
        }
}

// Scoring copies go beside the project's own scripts in the inspections directory that FlexInspect loads. Only files with
// the scoring prefix are replaced or removed, and the directory once it is empty.
internal fun removeScoringInspections(inspections: Path) {
    if (!inspections.exists()) return
    inspections.listDirectoryEntries("EdictBenchmark*.inspection.kts").forEach { it.deleteExisting() }
    if (inspections.listDirectoryEntries().isEmpty()) inspections.deleteExisting()
}

internal fun deleteTree(path: Path) {
    if (!path.exists()) return
    Files.walk(path).use { files -> files.sorted(Comparator.reverseOrder()).forEach(Files::delete) }
}

// The limit includes opening the project, which alone takes several minutes for Ultimate.
private fun scanTimeoutMinutes(): Long {
    val configured = System.getenv("BENCHMARK_SCAN_TIMEOUT_MINUTES") ?: return 30
    return configured.toLongOrNull()?.takeIf { it > 0 }
        ?: error("BENCHMARK_SCAN_TIMEOUT_MINUTES must be a positive number of minutes, got '$configured'")
}

// Scans the checked-out project in place: the Edict MCP server has stopped by now, so this IDE opens the project alone.
internal class ProjectRunner(private val project: Path, private val output: Path) {
    @Synchronized
    fun scan(destination: Path, codes: Map<String, String>): JsonObject {
        val inspections = project.resolve("inspections")
        removeScoringInspections(inspections)
        inspections.createDirectories()
        codes.forEach { (rule, code) ->
            require(rule.matches(Regex("EdictBenchmark[A-Za-z0-9_]+")))
            inspections.resolve("$rule.inspection.kts").writeText(code)
        }
        // The project's own qodana.yaml stays untouched: the scan reads this configuration instead.
        val configuration = destination.resolve("qodana.yaml")
        writeJson(configuration, projectConfiguration(codes.keys))
        val results = destination.resolve("results")
        val log = destination.resolve("analysis.log")
        val timeout = scanTimeoutMinutes()
        // Scoring rules inspect Java and Kotlin files only, so injected code only costs time: in Ultimate the
        // JSON Schema injector alone dominated the scan.
        val process = qodanaProcess(project, results, output.resolve("scan-workspace/cache"), log,
            "--config", configuration.toString(), "--property=idea.batch.inspections.inspect.injected.psi=false").start()
        try {
            if (!process.waitFor(timeout, TimeUnit.MINUTES)) {
                throw IllegalStateException("Project scan timed out after $timeout minutes; see $log")
            }
            if (process.exitValue() != 0) throw IllegalStateException("Project analysis exited ${process.exitValue()}; see $log\n${log.readText().takeLast(2000)}")
            val sarif = results.resolve("qodana.sarif.json")
            val registered = readSarif(sarif).registeredRules
            check(registered.containsAll(codes.keys)) { "Generated inspections were not loaded: ${codes.keys - registered}" }
            return readObject(sarif)
        } finally {
            stop(process)
            removeScoringInspections(inspections)
        }
    }
}
