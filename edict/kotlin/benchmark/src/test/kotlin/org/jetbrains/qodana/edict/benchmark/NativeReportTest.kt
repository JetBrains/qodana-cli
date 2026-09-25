// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.benchmark

import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import kotlin.io.path.*
import kotlin.test.*

class NativeReportTest {
    @TempDir lateinit var root: Path
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
        val inputs = BenchmarkInputs("revision", listOf(Specification("Rule", "Description", "Java"), Specification("Other", "Other description", "Java")))
        assertEquals(mapOf("Rule" to listOf("chosen-name"), "Other" to emptyList()), resolveClusters(inputs, root))
        val split = root.resolve("clusters/another/signals/s-second.json")
        split.parent.createDirectories()
        split.writeText("""{"provenance":{"workItemId":"benchmark/Rule/specification.json#/positiveExamples/1"}}""")
        signal.resolveSibling("s-other.json").writeText("""{"provenance":{"workItemId":"benchmark/Other/specification.json#/positiveExamples/0"}}""")
        assertEquals(mapOf("Rule" to listOf("another", "chosen-name"), "Other" to listOf("chosen-name")), resolveClusters(inputs, root))
    }
    @Test fun `missing clusters never fall back to a predefined inspection name`() {
        val inputs = BenchmarkInputs("revision", listOf(Specification("Rule", "Description", "Java")))
        assertEquals(mapOf("Rule" to emptyList()), resolveClusters(inputs, root))
        root.resolve("clusters/rule").createDirectories().resolve("description.json").writeText("""{"status":"Generated"}""")
        assertEquals(mapOf("Rule" to emptyList()), resolveClusters(inputs, root))
        assertEquals("NotClustered", generationOutcome(emptyList(), emptyMap()))
        assertEquals("PartiallyGenerated", generationOutcome(listOf("a", "b"), mapOf("a" to "Generated", "b" to "Pending")))
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
