// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.benchmark

import kotlinx.serialization.json.*
import java.nio.file.Files
import kotlin.io.path.*

private const val job = "StaticAnalysis_Edict_Benchmarks_JenkinsKotlinSkills"
private const val project = "StaticAnalysis_Edict_Benchmarks"
private const val sourceRoot = "StaticAnalysis_Edict_Benchmarks_QodanaCliEdictMaster"
private const val fixtureRoot = "StaticAnalysis_Edict_Benchmarks_HttpsGithubComQodanaEdictJenkinsGit"
private fun properties(values: Map<String, String>) = obj("property" to JsonArray(values.map { (key, value) -> obj("name" to text(key), "value" to text(value)) }))
private fun api(path: String, value: JsonObject = obj(), method: String = "PUT") {
    val payload = Files.createTempFile("edict-teamcity-", ".json")
    try {
        payload.writeText(value.toString())
        check(ProcessBuilder("teamcity", "api", path, "-X", method, "--input", payload.toString(), "--silent").inheritIO().start().waitFor() == 0) {
            "TeamCity update failed: $path"
        }
    } finally { payload.deleteExisting() }
}

private fun bashStep(id: String, name: String, script: String, mode: String = "default") = obj("id" to text(id), "name" to text(name),
    "type" to text("simpleRunner"), "properties" to properties(mapOf("script.content" to "#!/bin/bash\nset -euo pipefail\nbash qodana-cli/scripts/edict-benchmark/$script\n",
        "use.custom.script" to "true", "teamcity.step.mode" to mode)))

private fun artifact(source: String, build: String, paths: String) = obj("type" to text("artifact_dependency"),
    "source-buildType" to obj("id" to text(source)), "properties" to properties(mapOf(
        "revisionName" to "buildId", "revisionValue" to build, "cleanDestinationDirectory" to "true", "pathRules" to paths)))

fun main() {
    val roots = json.parseToJsonElement(commandOutput("teamcity", "api", "/app/rest/vcs-roots?locator=project:(id:$project)&fields=vcs-root(id)")).jsonObject
    val exists = roots["vcs-root"]?.jsonArray.orEmpty().any { it.jsonObject.string("id") == sourceRoot }
    val root = obj("id" to text(sourceRoot), "name" to text("qodana-cli / avafanasev/edict-master"),
        "vcsName" to text("jetbrains.git"), "project" to obj("id" to text(project)), "properties" to properties(mapOf(
            "url" to "https://github.com/JetBrains/qodana-cli.git", "branch" to "refs/heads/avafanasev/edict-master",
            "authMethod" to "ANONYMOUS", "submoduleCheckout" to "IGNORE", "agentCleanPolicy" to "ALWAYS", "agentCleanFilesPolicy" to "ALL_UNTRACKED")))
    if (exists) api("/app/rest/vcs-roots/id:$sourceRoot/properties", root.getValue("properties").jsonObject)
    else api("/app/rest/vcs-roots", root, "POST")
    api("/app/rest/buildTypes/id:$job/vcs-root-entries", obj("vcs-root-entry" to JsonArray(listOf(
        obj("vcs-root" to obj("id" to text(fixtureRoot)), "checkout-rules" to text("+:. => project")),
        obj("vcs-root" to obj("id" to text(sourceRoot)), "checkout-rules" to text("+:. => qodana-cli"))
    ))))
    api("/app/rest/buildTypes/id:$job/steps", obj("step" to JsonArray(listOf(
        bashStep("INSTALL_CODEX", "Install Codex and configure LiteLLM", "install-codex.sh"),
        bashStep("START_MCP", "Install Kotlin skills and start native MCP servers", "prepare.sh"),
        bashStep("EXECUTE_CODEX", "Process inbox and generate new rules", "generate.sh"),
        obj("id" to text("REPORT"), "name" to text("Generate SARIF and compare with gold"), "type" to text("gradle-runner"),
            "properties" to properties(mapOf("teamcity.step.mode" to "execute_if_failed", "teamcity.build.workingDir" to "qodana-cli/edict/kotlin",
                "target.jdk.home" to "%env.JDK_21_0%", "ui.gradleRunner.gradle.wrapper.useWrapper" to "true",
                "ui.gradleRunner.gradle.tasks.names" to ":benchmark:report",
                "ui.gradleRunner.additional.gradle.cmd.params" to "--no-daemon --console=plain -PbenchmarkDir=%teamcity.build.checkoutDir%/project/benchmark " +
                    "-PgenerationDir=%teamcity.build.checkoutDir%/benchmark-output -PsourceProjectDir=%teamcity.build.checkoutDir%/project")))
    ))))
    api("/app/rest/buildTypes/id:$job/snapshot-dependencies", obj("snapshot-dependency" to JsonArray(emptyList())))
    api("/app/rest/buildTypes/id:$job/artifact-dependencies", obj("artifact-dependency" to JsonArray(listOf(
        artifact("ijplatform_master_QodanaJvmEdict", "1070981529", "qodana-QDJVM-263.SNAPSHOT.150-aarch64.tar.gz* => native-artifacts"),
        artifact("ijplatform_master_QodanaCliAll", "1070964192", "cli_linux_arm64_v8.0/qodana => native-cli")
    ))))
    val features = json.parseToJsonElement(commandOutput("teamcity", "api", "/app/rest/buildTypes/id:$job/features")).jsonObject
    features["feature"]?.jsonArray.orEmpty().filter { it.jsonObject.string("type") == "DockerSupport" }.forEach {
        api("/app/rest/buildTypes/id:$job/features/${it.jsonObject.string("id")}", method = "DELETE")
    }
    features["feature"]?.jsonArray.orEmpty().filter { it.jsonObject.string("type") == "jetbrains.agent.free.space" }.forEach {
        api("/app/rest/buildTypes/id:$job/features/${it.jsonObject.string("id")}/parameters",
            properties(mapOf("free-space-fail-start" to "false", "free-space-work" to "20gb")))
    }
    val parameters = mapOf("env.QODANA_DIST" to "%teamcity.build.checkoutDir%/native-dist",
        "env.QODANA_CLI" to "%teamcity.build.checkoutDir%/benchmark-output/tooling/bin/qodana",
        "env.EDICT_PROJECT_DIR" to "%teamcity.build.checkoutDir%/project",
        "env.LITELLM_API_KEY" to "%liteLLMToken%", "env.BENCHMARK_MODEL" to "gpt-5.6-sol", "env.BENCHMARK_MINUTES" to "240",
        "env.BENCHMARK_LIMIT" to "0", "env.BENCHMARK_RULES" to "", "env.BENCHMARK_CODEX_VERSION" to "0.155.1")
    parameters.forEach { (name, value) -> api("/app/rest/buildTypes/id:$job/parameters", obj("name" to text(name), "value" to text(value)), "POST") }
    val obsolete = setOf("benchmark.image", "env.BENCHMARK_IMAGE", "env.BENCHMARK_SOURCE_REVISION", "env.BENCHMARK_COMPARISON_REVISION", "env.BENCHMARK_PREFLIGHT")
    // Read names only: secure parameter values are never needed by this configuration tool.
    val names = json.parseToJsonElement(commandOutput("teamcity", "api", "/app/rest/buildTypes/id:$job/parameters?fields=property(name)")).jsonObject
    names["property"]?.jsonArray.orEmpty().map { it.jsonObject.string("name") }.filter { it in obsolete }.forEach {
        api("/app/rest/buildTypes/id:$job/parameters/$it", method = "DELETE")
    }
    val artifacts = listOf("report.json", "qodana.sarif.json", "inputs.json", "prompt.txt", "inspection-tools.json", "source-revision.txt", "runner-revision.txt")
        .map { "benchmark-output/$it" } + listOf("benchmark-output/generatedInspections => generatedInspections.zip",
        "benchmark-output/specGoldComparisons => specGoldComparisons.zip", "benchmark-output/state => state.zip", "benchmark-output/log => logs.zip",
        "benchmark-output/trace/sandbox.stderr", "benchmark-output/trace/sandbox.stdout", "benchmark-output/mcp-results/log => inspection-ide-logs.zip",
        "benchmark-output/evaluation => evaluation.zip", "qodana-cli/scripts/edict-benchmark => runner-scripts.zip")
    mapOf("executionTimeoutMin" to "300", "maximumNumberOfBuilds" to "1", "cleanBuild" to "true", "checkoutMode" to "ON_AGENT",
        "publishArtifactCondition" to "ALWAYS", "artifactRules" to artifacts.joinToString("\n")).forEach { (name, value) ->
        api("/app/rest/buildTypes/id:$job/settings/$name", obj("name" to text(name), "value" to text(value)))
    }
    println("https://buildserver.labs.intellij.net/buildConfiguration/$job")
}
