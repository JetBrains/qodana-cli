// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.benchmark

import kotlinx.serialization.json.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import kotlin.io.path.*
import kotlin.test.*

class ComparisonTest {
    @TempDir lateinit var root: Path
    private fun finding(line: Long, offset: Long? = null, path: String? = "X.java", rule: String = "Rule") =
        Finding(rule, path, line, offset, offset?.let { 4L })

    @Test fun `nearby TP still leaves exact FN as in reference`() {
        val metrics = calculateMetrics("Rule", listOf(finding(10, 100)), listOf(finding(12, 200), finding(20, 300)))
        assertEquals(1, metrics.truePositives)
        assertEquals(1, metrics.falsePositives)
        assertEquals(1, metrics.falseNegatives)
        assertEquals(1.0, metrics.recall)
        assertEquals(0.5, metrics.precision)
    }

    @Test fun `exact intersection is inclusive and missing locations never match`() {
        assertTrue(matchExact(finding(1, 10), finding(9, 14)))
        assertFalse(matchExact(finding(1, 10), finding(1, 15)))
        assertFalse(matchExact(finding(1, 10), finding(1, 10, "Other.java")))
        assertFalse(matchExact(finding(1, 10, null), finding(1, 10, null)))
        assertFalse(matchLenient(finding(1, path = null), finding(1, path = null)))
        assertFalse(matchExact(finding(1), finding(1)))
        assertTrue(matchLenient(finding(1), finding(3)))
        assertFalse(matchLenient(finding(1), finding(4)))
    }

    @Test fun `aggregate defaults and median are stable`() {
        assertEquals(0.0, aggregate(emptyList()).avgRecall)
        assertEquals(2.5, listOf(4.0, 1.0, 3.0, 2.0).median())
        assertEquals(2.0, listOf(3.0, 1.0, 2.0).median())
    }

    private fun write(path: String, content: String) = root.resolve(path).also {
        it.parent.createDirectories()
        it.writeText(content)
    }

    private fun sarif(rule: String, line: Int = 10, offset: Int = 100) = """
        {"version":"2.1.0","runs":[{"tool":{"driver":{"name":"test"},"extensions":[{"rules":[{"id":"$rule"}]}]},
          "results":[{"ruleId":"$rule","locations":[{"physicalLocation":{"artifactLocation":{"uri":"X.java"},
            "region":{"startLine":$line,"charOffset":$offset,"charLength":4}}}]}]}]}
    """.trimIndent()

    private fun fixture() {
        write("reports/source-revision.txt", "revision\n")
        write("project/.edict/clusters/rule/description.json", """{"status":"Generated"}""")
        write("project/.edict/clusters/pending/description.json", """{"status":"Pending"}""")
        write("project/.edict/clusters/rule/signals/s-rule.json", """{"source":{"type":"SubmittedFeedback","suggestionId":"benchmark/Rule/specification.json#positiveExamples/0"}}""")
        write("project/.edict/clusters/pending/signals/s-pending.json", """{"provenance":{"workItemId":"benchmark/Pending/specification.json#/positiveExamples/0"}}""")
        write("project/.edict/inspections/rule.inspection.kts", "// fixture inspection")
        val goldRuns = listOf("Rule", "Pending")
            .flatMap { json.parseToJsonElement(sarif(it)).jsonObject.getValue("runs").jsonArray }
        writeJson(root.resolve("project/.edict/gold.sarif.json"), obj("runs" to JsonArray(goldRuns)))
        write("reports/qodana.sarif.json", sarif("EdictBenchmarkRule"))
    }

    @Test fun `comparison writes generated artifacts and aggregates only generated inspections`() {
        fixture()
        write("reports/generatedInspections/Stale.kts", "stale")
        write("reports/specGoldComparisons/Stale.json", "{}")
        val report = compare(root.resolve("benchmark"), root.resolve("project/.edict"), root.resolve("reports"))
        assertEquals(2, report.totalInspectionsProcessed)
        assertEquals(1, report.successful)
        assertEquals(1, report.aggregate.totalTP)
        assertEquals("Pending", report.generationOutcomes["Pending"])
        assertEquals(report, json.decodeFromString<BenchmarkReport>(root.resolve("reports/report.json").readText()))
        assertEquals("// fixture inspection", root.resolve("reports/generatedInspections/Rule.kts").readText())
        assertTrue(root.resolve("reports/qodana.sarif.json").exists())
        assertFalse(root.resolve("reports/specGoldComparisons").exists())
        assertFalse(root.resolve("reports/generatedInspections/Stale.kts").exists())
        // Reporting leaves the gold SARIF and state store in place.
        assertEquals(setOf("Rule", "Pending"), readSarif(root.resolve("project/.edict/gold.sarif.json")).findings.map { it.ruleId }.toSet())
        assertFalse(root.resolve("reports/state").exists())
    }

    @Test fun `built in rules cannot stand in for generated inspections`() {
        fixture()
        write("reports/qodana.sarif.json", sarif("Rule"))
        val error = assertFailsWith<IllegalArgumentException> {
            compare(root.resolve("benchmark"), root.resolve("project/.edict"), root.resolve("reports"))
        }
        assertContains(error.message.orEmpty(), "EdictBenchmarkRule")
    }

    @Test fun `optional null SARIF fields and multiple runs are supported`() {
        val path = write("nullable.sarif.json", """{"runs":[
          {"tool":null,"results":null},
          {"tool":{"driver":{"rules":null},"extensions":null},"results":[{"ruleId":"Rule","locations":null}]}
        ]}""")
        val report = readSarif(path)
        assertEquals(listOf(Finding("Rule", null)), report.findings)
        assertTrue(report.registeredRules.isEmpty())
    }

    @Test fun `zero generated inspections still produce a report`() {
        fixture()
        write("project/.edict/clusters/rule/description.json", """{"status":"Invalid"}""")
        write("reports/qodana.sarif.json", """{"runs":[{"results":[]}]}""")
        val report = compare(root.resolve("benchmark"), root.resolve("project/.edict"), root.resolve("reports"))
        assertEquals(0, report.successful)
        assertTrue(root.resolve("reports/report.json").exists())
    }

    @Test fun `no clusters produces an explicit outcome and empty SARIF`() {
        fixture()
        deleteTree(root.resolve("project/.edict/clusters"))
        generateSarif(root.resolve("unused-project"), root.resolve("project/.edict"), root.resolve("reports"))
        val report = compare(root.resolve("benchmark"), root.resolve("project/.edict"), root.resolve("reports"))
        assertEquals(mapOf("Rule" to "NotClustered", "Pending" to "NotClustered"), report.generationOutcomes)
        assertEquals(0, report.successful)
        assertTrue(readSarif(root.resolve("reports/qodana.sarif.json")).findings.isEmpty())
        assertFalse(root.resolve("project/.edict/clusters").exists())
    }

    @Test fun `split and merged clusters are scored for their source rules`() {
        fixture()
        write("project/.edict/clusters/pending/description.json", """{"status":"Generated"}""")
        write("project/.edict/clusters/pending/signals/s-rule-second.json", """{"provenance":{"workItemId":"benchmark/Rule/specification.json#/positiveExamples/1"}}""")
        write("project/.edict/inspections/pending.inspection.kts", "// shared inspection")
        val runs = listOf(sarif("EdictBenchmarkRule_Cluster1"), sarif("EdictBenchmarkRule_Cluster2"),
            sarif("EdictBenchmarkPending", line = 100, offset = 1000))
            .flatMap { json.parseToJsonElement(it).jsonObject.getValue("runs").jsonArray }
        writeJson(root.resolve("reports/qodana.sarif.json"), obj("runs" to JsonArray(runs)))
        val report = compare(root.resolve("benchmark"), root.resolve("project/.edict"), root.resolve("reports"))
        assertEquals(2, report.successful)
        assertEquals(listOf("pending", "rule"), report.clustersByRule["Rule"])
        assertEquals(listOf("pending"), report.clustersByRule["Pending"])
        // The same finding from two clusters counts once for this specification.
        assertEquals(1, report.inspectionMetrics.single { it.ruleId == "Rule" }.truePositives)
        assertEquals(1, report.inspectionMetrics.single { it.ruleId == "Pending" }.falsePositives)
        assertTrue(root.resolve("reports/generatedInspections/Rule--rule.kts").exists())
        assertTrue(root.resolve("reports/generatedInspections/Rule--pending.kts").exists())
        assertTrue(root.resolve("reports/generatedInspections/Pending.kts").exists())
    }
}
