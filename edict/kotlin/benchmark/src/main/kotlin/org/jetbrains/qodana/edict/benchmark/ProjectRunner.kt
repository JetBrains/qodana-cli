// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.benchmark

import kotlinx.serialization.json.*
import java.nio.file.*
import java.nio.file.attribute.BasicFileAttributes
import java.util.concurrent.TimeUnit
import kotlin.io.path.*

internal fun writeJson(path: Path, value: JsonElement) {
    path.parent.createDirectories()
    path.writeText(json.encodeToString(JsonElement.serializer(), value) + "\n")
}

internal fun readObject(path: Path) = json.parseToJsonElement(path.readText()).jsonObject

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
    return ProcessBuilder(listOf("/opt/idea/bin/qodana", "scan", "--project-dir", project.toString(),
        "--results-dir", results.toString(), "--cache-dir", cache.toString(), "--disable-sanity", "--run-promo=false",
        "--save-report=false", "--property=idea.headless.enable.statistics=false") + extra)
        .redirectErrorStream(true).redirectOutput(log.toFile()).apply {
            // The image exports QODANA_CONF=/root/.config/idea. Override the actual CLI input,
            // not idea.config.path: the CLI sorts duplicate JVM properties and the default wins.
            environment()["QODANA_CONF"] = cache.resolve("config").toString()
        }
}

internal fun copySource(source: Path, target: Path) {
    val excluded = setOf(".git", ".edict", ".qodana", "benchmark", "inspections", "target", "qodana.yaml")
    Files.walkFileTree(source, object : SimpleFileVisitor<Path>() {
        override fun preVisitDirectory(dir: Path, attrs: BasicFileAttributes): FileVisitResult {
            if (dir != source && dir.fileName.toString() in excluded) return FileVisitResult.SKIP_SUBTREE
            target.resolve(source.relativize(dir)).createDirectories()
            return FileVisitResult.CONTINUE
        }
        override fun visitFile(file: Path, attrs: BasicFileAttributes): FileVisitResult {
            if (file.fileName.toString() !in excluded) Files.copy(file, target.resolve(source.relativize(file)), LinkOption.NOFOLLOW_LINKS)
            return FileVisitResult.CONTINUE
        }
    })
}

internal fun deleteTree(path: Path) {
    if (!path.exists()) return
    Files.walk(path).use { files -> files.sorted(Comparator.reverseOrder()).forEach(Files::delete) }
}

internal class ProjectRunner(private val source: Path, private val output: Path) {
    private val workspace = output.resolve("scan-workspace")

    @Synchronized
    fun scan(destination: Path, codes: Map<String, String>): JsonObject {
        val project = workspace.resolve("project")
        if (!project.exists()) {
            copySource(source, project)
            // Qodana consults Git while computing source roots. No history or gold fixtures are copied.
            commandOutput("git", "-C", project.toString(), "init", "--quiet")
        }
        deleteTree(project.resolve("inspections"))
        val inspections = project.resolve("inspections").createDirectories()
        codes.forEach { (rule, code) ->
            require(rule.matches(Regex("EdictBenchmark[A-Za-z0-9]+")))
            inspections.resolve("$rule.inspection.kts").writeText(code)
        }
        writeJson(project.resolve("qodana.yaml"), obj("version" to text("1.0"), "profile" to obj("name" to text("empty")),
            "include" to JsonArray(codes.keys.map { obj("name" to text(it)) })))
        val results = destination.resolve("results")
        val log = destination.resolve("analysis.log")
        val process = qodanaProcess(project, results, workspace.resolve("cache"), log).start()
        try {
            if (!process.waitFor(30, TimeUnit.MINUTES)) throw InfrastructureFailure("Project scan timed out; see $log")
            if (process.exitValue() != 0) throw InfrastructureFailure("Project analysis exited ${process.exitValue()}; see $log\n${log.readText().takeLast(2000)}")
            val sarif = results.resolve("qodana.sarif.json")
            val registered = readSarif(sarif).registeredRules
            check(registered.containsAll(codes.keys)) { "Generated inspections were not loaded: ${codes.keys - registered}" }
            return readObject(sarif)
        } finally { stop(process) }
    }
}
