// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.benchmark

import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonObject
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import kotlin.io.path.*
import kotlin.test.*

class NativeReportTest {
    @TempDir lateinit var root: Path
    @Test fun `project scan disables all inspections without naming an empty profile`() {
        val configuration = projectConfiguration(linkedSetOf("EdictBenchmarkRule"))
        assertNull(configuration["inspections"])
        assertEquals(JsonArray(listOf(obj("group" to text("ALL"), "enabled" to JsonPrimitive(false)))),
            configuration.getValue("profile").jsonObject["inspections"])
        assertEquals(JsonArray(listOf(obj("name" to text("EdictBenchmarkRule")))), configuration["include"])
    }
    @Test fun `scoring renames descriptor metadata without changing the implementation`() {
        val code = """val implementation = localInspection { file, inspection -> println("Original") }
            listOf(InspectionKts(id = "Original", localTool = implementation, name = "Original"))"""
        assertEquals(code.replace("id = \"Original\"", "id = \"EdictBenchmarkRule\""), scoringCode(code, "EdictBenchmarkRule"))
        assertContains(scoringCode("listOf(InspectionKts(\"Original\", implementation))", "EdictBenchmarkRule_Cluster1"), "\"EdictBenchmarkRule_Cluster1\"")
        assertFailsWith<IllegalArgumentException> { scoringCode("val id = \"Rule\"", "EdictBenchmarkRule") }
        assertFailsWith<IllegalArgumentException> { scoringCode("$code\n$code", "EdictBenchmarkRule") }
    }
    @Test fun `comparison follows Edict cluster membership including splits and merges`() {
        val signal = root.resolve("clusters/chosen-name/signals/s-signal.json")
        signal.parent.createDirectories()
        signal.writeText("""{"source":{"type":"SubmittedFeedback","inspectionName":"Rule","suggestionId":"benchmark/Rule/specification.json#positiveExamples/0"}}""")
        val inputs = BenchmarkInputs("revision", listOf("Rule", "Other"))
        assertEquals(mapOf("Rule" to listOf("chosen-name"), "Other" to emptyList()), resolveClusters(inputs, root))
        val split = root.resolve("clusters/another/signals/s-second.json")
        split.parent.createDirectories()
        split.writeText("""{"provenance":{"workItemId":"benchmark/Rule/specification.json#/positiveExamples/1"}}""")
        signal.resolveSibling("s-other.json").writeText("""{"provenance":{"workItemId":"benchmark/Other/specification.json#/positiveExamples/0"}}""")
        assertEquals(mapOf("Rule" to listOf("another", "chosen-name"), "Other" to listOf("chosen-name")), resolveClusters(inputs, root))
    }
    @Test fun `a gold manifest assigns Signals by id instead of their source reference`() {
        val gold = root.resolve("gold.sarif.json")
        gold.writeText("""{"runs":[{"properties":{"edictBenchmarkManifest":{"format":"intellij-v1","cases":[
          {"id":"use-set-contains","signals":[{"id":"s-000000000001","label":"POSITIVE"}]},
          {"id":"no-sleep-on-edt","signals":[{"id":"s-000000000002","label":"NEGATIVE"},{"id":"s-000000000003"}]}]}},
          "results":[{"ruleId":"use-set-contains","properties":{"edictLabel":"POSITIVE"}}]}]}""")
        val sarif = readSarif(gold)
        assertEquals(POSITIVE, sarif.findings.single().label)
        root.resolve("reports").createDirectories().resolve("source-revision.txt").writeText("revision\n")
        val inputs = loadInputs(sarif, root.resolve("reports"))
        // A rule without any gold result still takes part.
        assertEquals(listOf("no-sleep-on-edt", "use-set-contains"), inputs.rules)
        for ((cluster, signal) in listOf("chosen" to "s-000000000001", "chosen" to "s-000000000002", "split" to "s-000000000003")) {
            root.resolve("clusters/$cluster/signals").createDirectories().resolve("$signal.json").writeText("""{"id":"$signal",
              "source":{"type":"SubmittedFeedback","suggestionId":"benchmark/Other/specification.json#positiveExamples/0"}}""")
        }
        val clusters = resolveClusters(inputs, root)
        assertEquals(mapOf("no-sleep-on-edt" to listOf("chosen", "split"), "use-set-contains" to listOf("chosen")), clusters)
        val statuses = mapOf("chosen" to "Generated", "split" to "Generated")
        assertEquals(setOf("EdictBenchmarkNoSleepOnEdt_Cluster1", "EdictBenchmarkNoSleepOnEdt_Cluster2", "EdictBenchmarkUseSetContains"),
            scoringInspections(clusters, statuses).map { it.id }.toSet())
    }
    @Test fun `missing clusters never fall back to a predefined inspection name`() {
        val inputs = BenchmarkInputs("revision", listOf("Rule"))
        assertEquals(mapOf("Rule" to emptyList()), resolveClusters(inputs, root))
        root.resolve("clusters/rule").createDirectories().resolve("description.json").writeText("""{"status":"Generated"}""")
        assertEquals(mapOf("Rule" to emptyList()), resolveClusters(inputs, root))
        assertEquals("NotClustered", generationOutcome(emptyList(), emptyMap()))
        assertEquals("PartiallyGenerated", generationOutcome(listOf("a", "b"), mapOf("a" to "Generated", "b" to "Pending")))
    }
    @Test fun `cluster outcomes read Edict Next manifests and legacy descriptions`() {
        root.resolve("clusters/next").createDirectories().resolve("cluster.json").writeText("""{"status":"Generated"}""")
        root.resolve("clusters/legacy").createDirectories().resolve("description.json").writeText("""{"status":"Pending"}""")
        val clusters = mapOf("Rule" to listOf("next", "legacy"))
        assertEquals(mapOf("next" to "Generated", "legacy" to "Pending"), clusterOutcomes(clusters, root))
    }
    @Test fun `scan source copy excludes state gold and previous candidates`() {
        val source = root.resolve("source").createDirectories()
        for (path in listOf("src/X.java", "benchmark/gold.sarif.json", "inspections/Old.inspection.kts", ".edict/state.json", ".git/config", "qodana.yaml", "AGENTS.md")) {
            source.resolve(path).apply { parent.createDirectories(); writeText(path) }
        }
        val target = root.resolve("target")
        copySource(source, target)
        assertEquals("src/X.java", target.resolve("src/X.java").readText())
        assertEquals(listOf("src"), target.listDirectoryEntries().map { it.fileName.toString() })
    }
}
