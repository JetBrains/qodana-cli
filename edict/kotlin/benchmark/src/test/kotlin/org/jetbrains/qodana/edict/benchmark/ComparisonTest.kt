// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.benchmark

import kotlinx.serialization.encodeToString
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
    private fun spec(vararg positives: FileRevision) = Specification("Rule", "Example rule", "Java", positives.toList())

    @Test fun `nearby TP still leaves exact FN as in reference`() {
        val metrics = calculateMetrics(spec(), listOf(finding(10, 100)), listOf(finding(12, 200), finding(20, 300)))
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

    @Test fun `positive requires every range and negative ignores unrelated ranges and rules`() {
        val example = FileRevision("X.java", expectedProblemRanges = listOf(LineRange(10, 12), LineRange(20, 22)))
        assertFalse(specSatisfaction(spec(example), listOf(finding(11))).satisfiesSpec)
        assertTrue(specSatisfaction(spec(example), listOf(finding(11), finding(22))).satisfiesSpec)
        val negative = spec().copy(negativeExamples = listOf(example))
        assertTrue(specSatisfaction(negative, listOf(finding(50), finding(11, rule = "Other"))).satisfiesSpec)
        assertFalse(specSatisfaction(negative, listOf(finding(22))).satisfiesSpec)
    }

    @Test fun `contradictory labels cannot both pass`() {
        val example = FileRevision("X.java", expectedProblemRanges = listOf(LineRange(10, 12)))
        val contradictory = spec(example).copy(negativeExamples = listOf(example))
        assertFalse(specSatisfaction(contradictory, emptyList()).satisfiesSpec)
        assertFalse(specSatisfaction(contradictory, listOf(finding(11))).satisfiesSpec)
    }

    @Test fun `optional thresholds and empty defaults match reference`() {
        val optional = spec().copy(optionalPositiveExamples = (1..10).map { FileRevision("$it.java") })
        assertTrue(specSatisfaction(optional, (1..7).map { finding(1, path = "$it.java") }).satisfiesSpec)
        assertFalse(specSatisfaction(optional, (1..6).map { finding(1, path = "$it.java") }).satisfiesSpec)
        assertFalse(specSatisfaction(optional.copy(optionalRecallThreshold = 0.8),
            (1..7).map { finding(1, path = "$it.java") }).satisfiesSpec)
        val empty = specSatisfaction(spec(), emptyList())
        assertEquals(1.0, empty.optionalPrecision)
        assertEquals(1.0, empty.optionalRecall)
        assertTrue(empty.satisfiesSpec)
        assertEquals(0.0, aggregate(emptyList()).avgRecall)
        assertEquals(2.5, listOf(4.0, 1.0, 3.0, 2.0).median())
        assertEquals(2.0, listOf(3.0, 1.0, 2.0).median())
    }

    @Test fun `spec gold classification preserves reference FP and FN meanings`() {
        val positive = listOf(FileRevision("X.java"), FileRevision("Missing.java"))
        val negative = listOf(FileRevision("Clean.java"), FileRevision("X.java"))
        val classified = compareSpecWithGold(spec().copy(positiveExamples = positive, negativeExamples = negative), listOf(finding(10)))
        assertEquals(listOf(ExampleClassification.TP, ExampleClassification.FP), classified.positiveExamples.map { it.classification })
        assertEquals(listOf(ExampleClassification.TN, ExampleClassification.FN), classified.negativeExamples.map { it.classification })
        assertEquals(SpecGoldAggregateMetrics(0.5, 0.5, 1, 1, 1, 1), specGoldAggregate(listOf(classified)))
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
        val input = BenchmarkInputs("revision",
            listOf(spec(FileRevision("X.java")), Specification("Pending", "Unfinished", "Java")))
        write("generation/inputs.json", json.encodeToString(input))
        write("generation/state/clusters/rule/description.json", """{"status":"Generated"}""")
        write("generation/state/clusters/pending/description.json", """{"status":"Pending"}""")
        write("generation/state/clusters/rule/signals/s-rule.json", """{"provenance":{"workItemId":"benchmark/Rule/specification.json#/positiveExamples/0"}}""")
        write("generation/state/clusters/pending/signals/s-pending.json", """{"provenance":{"workItemId":"benchmark/Pending/specification.json#/positiveExamples/0"}}""")
        write("generation/state/inspections/rule.inspection.kts", "// fixture inspection")
        write("benchmark/gold.sarif.json", sarif("Rule"))
        write("generation/qodana.sarif.json", sarif("EdictBenchmarkRule"))
    }

    @Test fun `comparison writes reference artifacts and aggregates only generated inspections`() {
        fixture()
        write("reports/generatedInspections/Stale.kts", "stale")
        write("reports/specGoldComparisons/Stale.json", "{}")
        val report = compare(root.resolve("benchmark"), root.resolve("generation"), root.resolve("reports"))
        assertEquals(2, report.totalInspectionsProcessed)
        assertEquals(1, report.successful)
        assertEquals(1, report.aggregate.totalTP)
        assertEquals(1.0, report.aggregate.specSatisfiedRate)
        assertEquals("Pending", report.generationOutcomes["Pending"])
        assertEquals(report, json.decodeFromString<BenchmarkReport>(root.resolve("reports/report.json").readText()))
        assertEquals("// fixture inspection", root.resolve("reports/generatedInspections/Rule.kts").readText())
        assertTrue(root.resolve("reports/specGoldComparisons/Rule.json").exists())
        assertTrue(root.resolve("reports/qodana.sarif.json").exists())
        assertFalse(root.resolve("reports/generatedInspections/Stale.kts").exists())
        assertFalse(root.resolve("reports/specGoldComparisons/Stale.json").exists())
        // Held-out snapshot remains untouched.
        assertEquals(2, json.decodeFromString<BenchmarkInputs>(root.resolve("generation/inputs.json").readText()).specifications.size)
    }

    @Test fun `built in rules cannot stand in for generated inspections`() {
        fixture()
        write("generation/qodana.sarif.json", sarif("Rule"))
        val error = assertFailsWith<IllegalArgumentException> {
            compare(root.resolve("benchmark"), root.resolve("generation"), root.resolve("reports"))
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
        write("generation/state/clusters/rule/description.json", """{"status":"Invalid"}""")
        write("generation/qodana.sarif.json", """{"runs":[{"results":[]}]}""")
        val report = compare(root.resolve("benchmark"), root.resolve("generation"), root.resolve("reports"))
        assertEquals(0, report.successful)
        assertEquals(0.0, report.aggregate.specSatisfiedRate)
        assertTrue(root.resolve("reports/report.json").exists())
    }

    @Test fun `no clusters produces an explicit outcome and empty SARIF`() {
        fixture()
        deleteTree(root.resolve("generation/state/clusters"))
        generateSarif(root.resolve("unused-project"), root.resolve("generation"))
        val report = compare(root.resolve("benchmark"), root.resolve("generation"), root.resolve("reports"))
        assertEquals(mapOf("Rule" to "NotClustered", "Pending" to "NotClustered"), report.generationOutcomes)
        assertEquals(0, report.successful)
        assertTrue(readSarif(root.resolve("reports/qodana.sarif.json")).findings.isEmpty())
        assertFalse(root.resolve("generation/state/clusters").exists())
    }

    @Test fun `split and merged clusters are scored against their source specifications`() {
        fixture()
        write("generation/state/clusters/pending/description.json", """{"status":"Generated"}""")
        write("generation/state/clusters/pending/signals/s-rule-second.json", """{"provenance":{"workItemId":"benchmark/Rule/specification.json#/positiveExamples/1"}}""")
        write("generation/state/inspections/pending.inspection.kts", "// shared inspection")
        val runs = listOf("EdictBenchmarkRule_Cluster1", "EdictBenchmarkRule_Cluster2", "EdictBenchmarkPending")
            .flatMap { json.parseToJsonElement(sarif(it)).jsonObject.getValue("runs").jsonArray }
        writeJson(root.resolve("generation/qodana.sarif.json"), obj("runs" to JsonArray(runs)))
        val report = compare(root.resolve("benchmark"), root.resolve("generation"), root.resolve("reports"))
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
