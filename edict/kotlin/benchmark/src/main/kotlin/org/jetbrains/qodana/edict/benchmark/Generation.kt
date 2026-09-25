// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.benchmark

import kotlinx.serialization.json.*
import org.jetbrains.qodana.edict.mcp.McpServer
import org.jetbrains.qodana.edict.runtime.CodexRunner
import org.jetbrains.qodana.edict.store.Store
import org.jetbrains.qodana.edict.git.GitRepository
import java.io.RandomAccessFile
import java.nio.file.Path
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit
import kotlin.io.path.*
import kotlin.system.exitProcess

internal fun prepareGeneration(project: Path, output: Path, limit: Int, rules: Set<String>): Map<String, String> {
    require(limit >= 0)
    val specs = project.resolve("benchmark").listDirectoryEntries().sorted().mapNotNull { directory ->
        directory.resolve("specification.json").takeIf { it.isRegularFile() }?.let(::readObject)
    }
    require(specs.map { it.string("ruleId") }.containsAll(rules)) { "Unknown benchmark rule: ${rules - specs.map { it.string("ruleId") }.toSet()}" }
    val selected = specs.filter { rules.isEmpty() || it.string("ruleId") in rules }.let { if (limit > 0) it.take(limit) else it }
    require(selected.isNotEmpty()) { "No benchmark specifications" }
    require(selected.map { it.string("ruleId") }.distinct().size == selected.size) { "Duplicate rule IDs" }
    val repository = GitRepository(project)
    val revision = repository.resolve("HEAD")
    val mapping = selected.associate { spec ->
        val rule = spec.string("ruleId")
        require(rule.matches(Regex("[A-Za-z0-9]+"))) { "Unsupported rule ID: $rule" }
        val cluster = rule.lowercase()
        val directory = output.resolve("state/clusters/$cluster")
        writeJson(directory.resolve("description.json"), obj("id" to text(cluster), "description" to spec.getValue("description"),
            "language" to spec.getValue("language"), "status" to text("Pending"),
            "benchmarkSpecification" to JsonObject(spec.filterKeys { !it.startsWith("optional") })))
        directory.resolve("history.md").writeText("# Benchmark input\n\nImported unchanged from benchmark/$rule/specification.json at $revision.\n" +
            "Required labelled examples are SubmittedFeedback in the inbox, awaiting managed distribution. " +
            "No commit/PR signals were fabricated. Optional examples are held out for scoring.\n")
        seedBenchmarkInbox(repository, output, spec, revision)
        cluster to rule
    }
    require(mapping.size == selected.size) { "Rule IDs collide after lowercasing" }
    writeJson(output.resolve("inputs.json"), obj("revision" to text(revision), "clusterToRule" to JsonObject(mapping.mapValues { text(it.value) }),
        "specifications" to JsonArray(selected)))
    output.resolve("prompt.txt").writeText("""
        ${'$'}edict_manager
        Process the existing inbox and generate inspection rules using the normal Kotlin managed workflow.
        Source project: $project. State is managed exclusively through edict-mcp. Scratch: ${output.resolve("scratch")}.
        Delegate edict-run for preparation, inbox distribution, and rule generation. No history extraction is needed.
        The selected benchmark clusters are ${mapping.entries.joinToString { "${it.key} (${it.value})" }}.
        Their empty Pending descriptions reserve stable IDs for scoring. Each inbox record is SubmittedFeedback from
        the specification named in source.url and provenance.workItemId. Distribute it to that specification's cluster,
        verify the durable signal copy before deleting its inbox entry, then generate rules for the affected clusters.
        Preserve cluster IDs, benchmarkSpecification metadata, and every required label and range.
        Process clusters and review children sequentially, closing completed native agents so the workflow fits six slots.

        Pass these benchmark constraints to every worker and reviewer:
        Feedback is original labelled source evidence, with exact revisions in fileRevision; never fabricate commit/PR
        provenance. Use the normal persisted-signal example assignment and review workflow. Conflicting required labels
        at identical source locations must retain their evidence and a valid domain outcome; never silently relabel them.
        Optional examples and gold SARIF are held-out scoring data; never use them to guide generation.
        Follow ordinary compilation, independent code review, weak-signal review and value-review requirements.
        Use run_project_inspection for complete findings, sending exact stored candidate bytes.
        Return exactly one InspectionKts descriptor with ID EdictBenchmark<OriginalRuleId>, e.g. EdictBenchmarkOctalLiteral,
        to avoid collisions with built-in inspections. Do not invoke legacy Edict inspection-server tools.
        Preserve original specification metadata. Finish every managed task and report persisted statuses and accepted paths.
    """.trimIndent() + "\n")
    return mapping
}

private fun runManaged(project: Path, output: Path, proxy: InspectionProxy, model: String, minutes: Long, preflight: Boolean) {
    Store(output.resolve("state")).use { store ->
        val server = McpServer(store, logs = output.resolve("log/edict"))
        server.serveHttp(0).use { transport ->
            val runner = CodexRunner(output, project, store.root, transport.url, "codex", model, server.agents, mapOf("inspection" to proxy.url))
            runner.prepare()
            val config = runner.home.resolve("config.toml")
            val denied = listOf(project.resolve("benchmark"), project.resolve(".edict"), output.resolve("inputs.json"))
                .joinToString("", postfix = "") { "${JsonPrimitive(it.toString())} = \"deny\"\n" }
            config.writeText(config.readText().replace("[permissions.edict-test.workspace_roots]", denied + "[permissions.edict-test.workspace_roots]")
                .replace("tool_timeout_sec = 300", "tool_timeout_sec = 1800"))
            runner.verifySandbox()
            teamCity("message", "text" to "Kotlin managed skills ready; both MCP servers live; sandbox verified")
            if (preflight) return
            val executor = Executors.newSingleThreadExecutor()
            val future = executor.submit<String> { runner.run(output.resolve("prompt.txt").readText(), minutes) }
            val log = output.resolve("log/edict/edict-agent-short.log")
            var offset = 0L
            try {
                do {
                    proxy.failure?.let { future.cancel(true); throw InfrastructureFailure(it.message.orEmpty()) }
                    if (log.exists()) RandomAccessFile(log.toFile(), "r").use { file ->
                        file.seek(offset)
                        while (true) {
                            val line = file.readLine() ?: break
                            teamCity("message", "text" to String(line.toByteArray(Charsets.ISO_8859_1), Charsets.UTF_8))
                        }
                        offset = file.filePointer
                    }
                    if (!future.isDone) Thread.sleep(1000)
                } while (!future.isDone)
                output.resolve("last-message.txt").writeText(store.redact(future.get()))
                val plan = store.plan()
                check(plan != null && plan.tasks.isNotEmpty() && plan.tasks.all { it.status == "completed" }) {
                    "Managed benchmark plan has missing, failed or unfinished tasks"
                }
                check(setOf("edict-prepare", "edict-distribution", "edict-generation").all { skill -> plan.tasks.any { it.skill == skill } }) {
                    "Managed benchmark must prepare and process the inbox before generating rules"
                }
                check(store.list("inbox").isEmpty()) { "Managed benchmark left unprocessed inbox signals" }
            } finally {
                future.cancel(true)
                executor.shutdownNow()
                executor.awaitTermination(30, TimeUnit.SECONDS)
            }
        }
    }
}

fun main(args: Array<String>) {
    try {
        val allowed = setOf("--project", "--output", "--model", "--minutes", "--limit", "--rules", "--preflight")
        require(args.size % 2 == 0 && args.toList().chunked(2).all { it[0] in allowed }) { "Options require --name value" }
        val options = args.toList().chunked(2).associate { it[0] to it[1] }
        require(options.size == args.size / 2) { "Duplicate options" }
        val project = Path.of(options.getValue("--project")).toRealPath()
        val output = Path.of(options.getValue("--output")).toAbsolutePath().normalize().createDirectories()
        val minutes = options.getValue("--minutes").toLong().also { require(it > 0) }
        val preflight = options.getValue("--preflight").toBooleanStrict()
        check(!output.resolve("state").exists()) { "Use a fresh output directory" }
        val mapping = prepareGeneration(project, output, options.getValue("--limit").toInt(),
            options.getValue("--rules").split(',').filter { it.isNotBlank() }.toSet())
        val scans = ProjectRunner(project, output)
        var generationError: Exception? = null
        try {
            InspectionServer(project, output).use { inspection ->
                val (client, tools) = inspection.connect()
                InspectionProxy(client, tools, project, output, scans).use { proxy ->
                    proxy.preflight()
                    try { runManaged(project, output, proxy, options.getValue("--model"), minutes, preflight) }
                    catch (error: Exception) { generationError = error }
                }
            }
            if (!preflight) {
                val codes = mapping.filter { (cluster, _) -> readObject(output.resolve("state/clusters/$cluster/description.json")).string("status") == "Generated" }
                    .map { (cluster, rule) -> "EdictBenchmark$rule" to output.resolve("state/inspections/$cluster.inspection.kts").readText() }.toMap()
                val sarif = if (codes.isEmpty()) obj("runs" to JsonArray(listOf(obj("results" to JsonArray(emptyList())))))
                    else scans.scan(output.resolve("evaluation"), codes)
                writeJson(output.resolve("qodana.sarif.json"), sarif)
                teamCity("message", "text" to "Generation artifacts ready: ${codes.size}/${mapping.size} inspections; Kotlin comparison runs next")
            }
            generationError?.let { throw it }
        } catch (error: Exception) {
            generationError = error
            throw error
        } finally {
            writeJson(output.resolve("progress.json"), obj("generationError" to (generationError?.let { text(it.message.orEmpty()) } ?: JsonNull),
                "clusters" to JsonObject(mapping.map { (cluster, rule) -> rule to readObject(output.resolve("state/clusters/$cluster/description.json")).getValue("status") }.toMap())))
        }
    } catch (error: Exception) {
        System.err.println("Kotlin benchmark generation failed: ${error.message}")
        error.printStackTrace()
        exitProcess(1)
    }
}
