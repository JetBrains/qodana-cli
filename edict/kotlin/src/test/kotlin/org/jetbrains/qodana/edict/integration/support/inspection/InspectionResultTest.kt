// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.integration.support.inspection

import java.nio.file.Files
import java.nio.file.Path
import kotlin.test.*
import kotlinx.serialization.json.*
import org.jetbrains.qodana.edict.common.sha256
import org.jetbrains.qodana.edict.common.wireJson
import org.jetbrains.qodana.edict.model.Plan
import org.jetbrains.qodana.edict.model.Task
import org.junit.jupiter.api.io.TempDir

class InspectionResultTest {
    @TempDir lateinit var directory: Path

    @Test fun `review receipts use their JSON content regardless of filename extension`() {
        val hash = sha256("persisted accepted inspection")
        fun receipt(path: String, candidateHash: String = hash, status: String = "ACCEPT") = directory.resolve(path).also {
            Files.writeString(it, """{"candidateHash":"$candidateHash","status":"$status","findings":[],"summary":"review"}""")
        }
        val code = receipt("code-review.md")
        val value = receipt("value-review")
        receipt("rejected.json", status = "REJECT")
        receipt("previous-candidate.md", candidateHash = sha256("previous inspection"))
        Files.writeString(directory.resolve("invalid.json"), "not a JSON receipt")
        Files.writeString(directory.resolve("narrative.md"), "```json\n${Files.readString(code)}\n```")
        assertEquals(setOf(code, value), acceptedCandidateReviews(directory, hash).toSet())
    }

    @Test fun `decode compiler responses from text and structured MCP content`() {
        val output = """{"compilationSuccess":true,"foundProblems":[{"lineNumber":5}]}"""
        val envelopes = listOf(
            buildJsonObject { putJsonArray("content") { add(buildJsonObject { put("type", "text"); put("text", output) }) } },
            buildJsonObject { put("structuredContent", wireJson.parseToJsonElement(output)) },
        )
        envelopes.forEach {
            val result = decodeInspectionResult(it)
            assertTrue(result.compilationSuccess)
            assertEquals(listOf(5), result.problemLines)
        }
        assertFailsWith<IllegalStateException> { decodeInspectionResult(buildJsonObject { put("isError", true) }) }
        assertFailsWith<IllegalStateException> { decodeInspectionResult(JsonObject(emptyMap())) }
    }

    @Test fun `completed final plan cannot conceal invalid stage execution`() {
        val ids = listOf("extract", "cluster", "generate")
        fun snapshot(vararg states: String) = Plan(request = "pipeline", tasks = states.mapIndexed { index, status ->
            Task(id = ids[index], skill = ids[index], title = ids[index], status = status)
        })
        val finished = snapshot("completed", "completed", "completed")
        val invalid = listOf(
            listOf(snapshot("running", "running", "pending"), snapshot("completed", "completed", "running"), finished),
            listOf(snapshot("pending", "running", "pending"), snapshot("running", "completed", "pending"), snapshot("completed", "completed", "running"), finished),
            listOf(finished),
        )
        invalid.forEach { assertTrue(stageOrderProblems(it, ids).isNotEmpty()) }
        assertTrue(stageOrderProblems(listOf(snapshot("running", "pending", "pending"), snapshot("completed", "running", "pending"),
            snapshot("completed", "completed", "running"), finished), ids).isEmpty())
    }
}
