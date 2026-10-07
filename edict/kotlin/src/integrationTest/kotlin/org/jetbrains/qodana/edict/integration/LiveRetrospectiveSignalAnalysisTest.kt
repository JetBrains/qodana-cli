// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.integration

import java.nio.file.Files
import java.nio.file.Path
import kotlin.io.path.isRegularFile
import kotlin.io.path.readText
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertTrue
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.jetbrains.qodana.edict.common.json
import org.jetbrains.qodana.edict.integration.support.CommitExpectation
import org.jetbrains.qodana.edict.integration.support.IntegrationTest
import org.jetbrains.qodana.edict.integration.support.historyBefore
import org.jetbrains.qodana.edict.integration.support.historyCommit
import org.jetbrains.qodana.edict.integration.support.historyPath
import org.jetbrains.qodana.edict.integration.support.signalFiles
import org.jetbrains.qodana.edict.integration.support.verifyCommitSignals
import org.jetbrains.qodana.edict.integration.support.verifyManagedRun
import org.junit.jupiter.api.Assumptions.assumeTrue
import org.junit.jupiter.api.Test

private const val retrospectiveFixtureHead = "a7fa96f48394b6e1e0d2d46e45403f0b3e3e18c3"
private const val retrospectiveFixtureSource = "ssh://git@git.jetbrains.team/sa/distillery-test.git"
private const val retrospectiveCluster = "avoid-thread-sleep-for-synchronization"
private const val retrospectiveInspection = "AvoidThreadSleepForSynchronization"

class LiveRetrospectiveSignalAnalysisTest : IntegrationTest() {
    override val fixtureRevision = retrospectiveFixtureHead
    override val fixtureSource = retrospectiveFixtureSource

    @Test
    fun `managed retrospective skill extracts a fix from real Qodana absent results`() {
        val ultimate = System.getenv("ULTIMATE_EDICT_REPO")?.takeIf(String::isNotBlank)?.let(Path::of)
        assumeTrue(ultimate != null && Files.isRegularFile(ultimate.resolve("bazel.cmd")),
            "ULTIMATE_EDICT_REPO with bazel.cmd is required")
        ultimate!!

        val legacyState = workspace.project.resolve(".edict")
        val sourceCluster = legacyState.resolve("clusters/$retrospectiveCluster/description.json")
        val sourceInspection = legacyState.resolve("inspections/$retrospectiveCluster.inspection.kts")
        assertTrue(sourceCluster.isRegularFile(), "Missing retrospective cluster fixture")
        assertTrue(sourceInspection.isRegularFile(), "Missing retrospective inspection fixture")
        val targetCluster = workspace.state.resolve("clusters/$retrospectiveCluster/description.json")
        val targetInspection = workspace.state.resolve("inspections/$retrospectiveCluster.inspection.kts")
        Files.createDirectories(targetCluster.parent)
        Files.createDirectories(targetInspection.parent)
        Files.copy(sourceCluster, targetCluster)
        Files.copy(sourceInspection, targetInspection)
        val clusterBytes = targetCluster.readText()
        val inspectionBytes = targetInspection.readText()

        val runner = workspace.output.resolve("scratch/qodana-history-runner")
        Files.createDirectories(runner.parent)
        Files.writeString(runner, qodanaRunnerScript())
        runner.toFile().setExecutable(true, true)

        val cache = Path.of(System.getProperty("user.home"), "Library/Caches/JetBrains/MonorepoBazel")
        val diskCache = Path.of(System.getProperty("user.home"), "Library/Caches/JetBrains/monorepo-bazel-cache")
        val devData = Path.of(System.getProperty("user.home"), "Library/Caches/JetBrains/MonorepoDevData")
        val daemon = Path.of(System.getProperty("user.home"), "Library/Application Support/kotlin/daemon")
        val writable = listOf(workspace.repository.root, ultimate, cache, diskCache, devData, daemon).filter(Files::exists)
        workspace.withEdictNextCodex(
            prompt = """
                Use edict_manager and the managed protocol to perform one retrospective Signal analysis.
                Create exactly one top-level edict-retrospective-signal-analysis task.
                Source checkout: ${workspace.repository.root}
                Analyzed project path relative to each temporary worktree: ${workspace.repository.root.relativize(workspace.project)}
                Edict state root: ${workspace.state}
                Cluster ID: $retrospectiveCluster
                End date: 2026-09-13
                Lookback period: use the skill default of three calendar months
                Qodana runner executable: $runner
                Runner contract:
                  snapshot: $runner --project <project> --results <empty-result-directory>
                  comparison: $runner --project <project> --results <empty-result-directory> --baseline <snapshot-sarif> --include-absent
                The runner prints the absolute qodana.sarif.json path after a successful run. Use the captured inspection
                in both temporary worktrees. Do not change the caller's checkout or managed Edict inputs. Publish every
                independently verified Signal through edict_publish_signal.
            """.trimIndent(),
            timeoutMinutes = 30,
            additionalWritableRoots = writable,
        ) { store, runtime, _ ->
            val plan = assertNotNull(store.plan())
            val coordinator = plan.tasks.single { it.skill == "edict-retrospective-signal-analysis" }
            val workers = plan.tasks.filter { it.skill == "edict-signal-analysis" }
            assertTrue(workers.any { historyCommit in it.prompt }, "Known fixing commit was not inspected")
            assertTrue(workers.all { it.parentId == coordinator.id })

            val trace = runtime.trace.resolve("stdout.jsonl").readText()
            assertTrue(trace.contains(runner.toString()), "Qodana runner was not invoked")
            assertTrue(trace.contains("--include-absent"), "HEAD comparison did not request absent results")
            assertEquals(clusterBytes, targetCluster.readText(), "Managed cluster input changed")
            assertEquals(inspectionBytes, targetInspection.readText(), "Managed inspection input changed")

            val reports = Files.walk(runtime.scratch).use { paths ->
                paths.filter { it.fileName.toString() == "qodana.sarif.json" && it.isRegularFile() }.toList()
            }
            assertTrue(reports.size >= 2, "Expected snapshot and comparison SARIF reports")
            val absent = reports.sumOf(::absentInspectionResults)
            assertEquals(1, absent, "Expected exactly one absent fixture result")
            verifyCommitSignals(
                workspace.repository,
                signalFiles(workspace.state.resolve("inbox")),
                listOf(CommitExpectation(historyBefore, historyCommit, historyPath, 5, 10)),
            )
            verifyManagedRun(workspace, runtime, plan)
        }
    }

    private fun absentInspectionResults(path: Path): Int =
        json.parseToJsonElement(path.readText()).jsonObject["runs"]!!.jsonArray.sumOf { run ->
            run.jsonObject["results"]?.jsonArray.orEmpty().count { result ->
                val row = result.jsonObject
                row["ruleId"]?.jsonPrimitive?.content == retrospectiveInspection &&
                    row["baselineState"]?.jsonPrimitive?.content == "absent"
            }
        }

    private fun qodanaRunnerScript(): String = """
        #!/bin/sh
        set -eu
        project=
        results=
        baseline=
        include_absent=false
        while [ "${'$'}#" -gt 0 ]; do
          case "${'$'}1" in
            --project) project=${'$'}2; shift 2 ;;
            --results) results=${'$'}2; shift 2 ;;
            --baseline) baseline=${'$'}2; shift 2 ;;
            --include-absent) include_absent=true; shift ;;
            *) echo "unexpected argument: ${'$'}1" >&2; exit 2 ;;
          esac
        done
        test -n "${'$'}project"
        test -n "${'$'}results"
        mkdir -p "${'$'}results"
        cd "${'$'}ULTIMATE_EDICT_REPO"
        run_qodana() {
          IJ_PRIVATE_PACKAGES_AUTHORIZER_SKIP=true /bin/sh ./bazel.cmd run --config=ci \
            --action_env=TMPDIR="${'$'}TMPDIR" --host_action_env=TMPDIR="${'$'}TMPDIR" \
            //build:qodana_for_jvm -- qodana "${'$'}@"
        }
        if [ -n "${'$'}baseline" ] && [ "${'$'}include_absent" = true ]; then
          run_qodana --baseline "${'$'}baseline" --baseline-include-absent "${'$'}project" "${'$'}results"
        else
          test -z "${'$'}baseline"
          run_qodana "${'$'}project" "${'$'}results"
        fi
        report="${'$'}results/qodana.sarif.json"
        test -f "${'$'}report"
        printf '%s\n' "${'$'}report"
    """.trimIndent() + "\n"
}
