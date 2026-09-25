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
        assertEquals(code.replace("id = \"Original\"", "id = \"EdictBenchmarkRule\""), scoringCode(code, "Rule"))
        assertContains(scoringCode("listOf(InspectionKts(\"Original\", implementation))", "Rule"), "\"EdictBenchmarkRule\"")
        assertFailsWith<IllegalArgumentException> { scoringCode("val id = \"Rule\"", "Rule") }
        assertFailsWith<IllegalArgumentException> { scoringCode("$code\n$code", "Rule") }
    }
    @Test fun `comparison follows distributed feedback provenance and rejects ambiguous mappings`() {
        val signal = root.resolve("state/clusters/chosen-name/signals/s-signal.json")
        signal.parent.createDirectories()
        signal.writeText("""{"provenance":{"workItemId":"benchmark/Rule/specification.json#/positiveExamples/0"}}""")
        val inputs = BenchmarkInputs("revision", mapOf("rule" to "Rule"), listOf(Specification("Rule", "Description", "Java")))
        assertEquals(mapOf("chosen-name" to "Rule"), resolveClusters(inputs, root))
        val duplicate = root.resolve("state/clusters/another/signals/s-signal.json")
        duplicate.parent.createDirectories()
        signal.copyTo(duplicate)
        assertFailsWith<IllegalArgumentException> { resolveClusters(inputs, root) }
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
