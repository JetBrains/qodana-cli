// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.benchmark

import kotlinx.serialization.json.*
import java.nio.file.Files
import java.nio.file.Path
import java.util.Base64
import kotlin.io.path.*

private const val job = "StaticAnalysis_Edict_Benchmarks_JenkinsKotlinSkills"
private const val image = "registry.jetbrains.team/p/sa/containers/qodana-jvm:263.SNAPSHOT.149"
private fun properties(values: Map<String, String>) = obj("property" to JsonArray(values.map { (key, value) -> obj("name" to text(key), "value" to text(value)) }))
private fun api(path: String, value: JsonObject, method: String = "PUT") {
    val payload = Files.createTempFile("edict-teamcity-", ".json")
    try {
        payload.writeText(value.toString())
        check(ProcessBuilder("teamcity", "api", path, "-X", method, "--input", payload.toString(), "--silent").inheritIO().start().waitFor() == 0) {
            "TeamCity update failed: $path"
        }
    } finally { payload.deleteExisting() }
}

private fun step(id: String, name: String, script: String, mode: String = "default") = obj("id" to text(id), "name" to text(name),
    "type" to text("simpleRunner"), "properties" to properties(mapOf("script.content" to script, "use.custom.script" to "true", "teamcity.step.mode" to mode)))

fun main(args: Array<String>) {
    require(args.size == 2) { "Expected scripts directory and full published commit SHA" }
    val scripts = Path.of(args[0])
    val revision = args[1].also { require(it.matches(Regex("[0-9a-f]{40}"))) }
    val unpack = listOf("prepare.sh", "generate.sh", "run.sh", "compare.sh").joinToString("\n") { name ->
        val encoded = Base64.getEncoder().encodeToString(scripts.resolve(name).readBytes())
        "base64 --decode > benchmark-scripts/$name <<'EDICT_SCRIPT'\n$encoded\nEDICT_SCRIPT"
    }
    val prepare = "#!/bin/bash\nset -euo pipefail\nmkdir -p benchmark-scripts benchmark-output\n$unpack\nbash benchmark-scripts/prepare.sh\n"
    api("/app/rest/buildTypes/id:$job/steps", obj("step" to JsonArray(listOf(
        step("PREPARE_KOTLIN", "Build Kotlin benchmark runner and pull assembled image", prepare),
        step("GENERATE_KOTLIN", "Start inspections MCP and generate with Kotlin managed skills",
            "#!/bin/bash\nset -euo pipefail\nexport BENCHMARK_CONTAINER=edict-benchmark-%teamcity.build.id%\nbash benchmark-scripts/generate.sh\n"),
        step("STOP_BENCHMARK", "Stop benchmark container",
            "#!/bin/bash\ndocker rm -f edict-benchmark-%teamcity.build.id% >/dev/null 2>&1 || true\n", "execute_always"),
        step("COMPARE_KOTLIN", "Checkout comparison sources and run Gradle :benchmark:compare",
            "#!/bin/bash\nset -euo pipefail\nbash benchmark-scripts/compare.sh\n", "execute_always"),
    ))))
    api("/app/rest/buildTypes/id:$job/snapshot-dependencies", obj("snapshot-dependency" to JsonArray(emptyList())))
    api("/app/rest/buildTypes/id:$job/artifact-dependencies", obj("artifact-dependency" to JsonArray(emptyList())))
    val parameters = mapOf("benchmark.image" to image, "env.BENCHMARK_IMAGE" to "%benchmark.image%",
        "env.BENCHMARK_SOURCE_REVISION" to revision, "env.BENCHMARK_COMPARISON_REVISION" to revision,
        "env.LITELLM_API_KEY" to "%liteLLMToken%", "env.BENCHMARK_MODEL" to "gpt-5.6-sol", "env.BENCHMARK_MINUTES" to "240",
        "env.BENCHMARK_LIMIT" to "0", "env.BENCHMARK_RULES" to "", "env.BENCHMARK_PREFLIGHT" to "false", "env.BENCHMARK_CODEX_VERSION" to "0.155.1")
    parameters.forEach { (name, value) -> api("/app/rest/buildTypes/id:$job/parameters", obj("name" to text(name), "value" to text(value)), "POST") }
    val artifacts = listOf("report.json", "progress.json", "qodana.sarif.json", "inputs.json", "last-message.txt", "inspection-tools.json",
        "image-digests.json", "source-revision.txt", "runner-revision.txt", "runner-jar.sha256", "comparison-revision.txt")
        .map { "benchmark-output/$it" } + listOf("benchmark-output/generatedInspections => generatedInspections.zip",
        "benchmark-output/specGoldComparisons => specGoldComparisons.zip", "benchmark-output/state => state.zip", "benchmark-output/log => logs.zip",
        "benchmark-output/trace/sandbox.stderr", "benchmark-output/trace/sandbox.stdout", "benchmark-output/mcp-results/log => inspection-ide-logs.zip",
        "benchmark-output/evaluation/results => evaluation.zip", "benchmark-output/project-runs/**/analysis.log => project-analysis-logs.zip",
        "benchmark-output/evaluation/analysis.log", "benchmark-scripts => runner-scripts.zip", "benchmark-runtime/benchmark-runner.jar")
    mapOf("executionTimeoutMin" to "300", "maximumNumberOfBuilds" to "1", "cleanBuild" to "true",
        "publishArtifactCondition" to "ALWAYS", "artifactRules" to artifacts.joinToString("\n")).forEach { (name, value) ->
        api("/app/rest/buildTypes/id:$job/settings/$name", obj("name" to text(name), "value" to text(value)))
    }
    println("https://buildserver.labs.intellij.net/buildConfiguration/$job")
}
