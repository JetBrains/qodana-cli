package org.jetbrains.qodana.edict.logging

import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.StandardOpenOption.*
import java.time.Instant
import kotlin.test.*
import kotlinx.serialization.json.*
import org.jetbrains.qodana.edict.common.flag
import org.jetbrains.qodana.edict.common.json
import org.jetbrains.qodana.edict.common.obj
import org.jetbrains.qodana.edict.common.text
import org.jetbrains.qodana.edict.common.wireJson
import org.jetbrains.qodana.edict.mcp.McpServer
import org.jetbrains.qodana.edict.model.Step
import org.jetbrains.qodana.edict.runtime.CodexAgentCollector
import org.jetbrains.qodana.edict.runtime.CodexRunner
import org.jetbrains.qodana.edict.store.Store
import org.jetbrains.qodana.edict.support.batch
import org.jetbrains.qodana.edict.support.launch
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir

class AgentLoggingTest {
    @TempDir lateinit var directory: Path

    @Test fun `collector attributes nested output defers early workers and redacts revoked tokens`() {
        Store(directory.resolve("state")).use { store ->
            val created = store.createPlan("Extract", listOf(Step("edict-batch-signal-analysis", "Commit")))
            val batch = store.launch(created.token, created.plan.tasks.single().id, "edict-batch-signal-analysis", emptyList(), emptyList())
            val child = store.addTask(batch.token, "edict-signal-analysis", "Inspect")
            val leaf = store.delegate(batch.token, child.id, emptyList(), emptyList(), "\$managed-edict-signal-analysis\nRead /skills/managed-edict-signal-analysis/SKILL.md")
            val home = directory.resolve("home")
            val sessions = home.resolve("sessions")
            Files.createDirectories(sessions)
            val stdout = directory.resolve("stdout.jsonl")
            Files.writeString(stdout, """{"type":"thread.started","thread_id":"root-thread"}""" + "\n")
            val logs = directory.resolve("log")
            val collector = CodexAgentCollector(home, stdout, AgentLogger(store, logs))
            fun append(file: String, value: JsonObject) = Files.writeString(sessions.resolve(file), wireJson.encodeToString(JsonObject.serializer(), value) + "\n", CREATE, APPEND)
            fun meta(file: String, id: String, parent: String = "", path: String = "") = append(file, buildJsonObject {
                put("type", "session_meta")
                putJsonObject("payload") {
                    put("id", id)
                    if (parent.isEmpty()) put("source", "exec") else putJsonObject("source") {
                        putJsonObject("subagent") { putJsonObject("thread_spawn") { put("parent_thread_id", parent); put("agent_path", path) } }
                    }
                }
            })
            fun message(text: String, role: String = "assistant", phase: String = "commentary") = buildJsonObject {
                put("type", "response_item"); put("timestamp", Instant.now().toString())
                putJsonObject("payload") {
                    put("type", "message"); put("role", role); put("phase", phase)
                    putJsonArray("content") { add(buildJsonObject { put("type", "output_text"); put("text", text) }) }
                }
            }
            meta("root.jsonl", "root-thread")
            meta("batch.jsonl", store.plan()!!.tasks.first().agentId, "root-thread")
            meta("leaf.jsonl", "leaf-thread", store.plan()!!.tasks.first().agentId, "/root/batch/leaf")
            meta("unrelated.jsonl", "other-thread")
            append("root.jsonl", message("Manager commentary"))
            append("batch.jsonl", message("Batch commentary"))
            append("leaf.jsonl", message("Early leaf output"))
            append("unrelated.jsonl", message("Unrelated output"))
            append("root.jsonl", message("Private prompt", role = "user"))
            append("root.jsonl", message("Private reasoning", phase = "analysis"))
            collector.scan()
            assertFalse(Files.readString(logs.resolve("edict-agents.log")).contains("Early leaf output"))
            store.readTask(leaf.token)
            store.startTask(leaf.token, "/root/batch/leaf", leaf.skill)
            collector.scan()
            store.finishTask(leaf.token, "completed", "Done")
            val digest = "f".repeat(64)
            append("leaf.jsonl", message("Leaf finished $digest ${leaf.token} ${created.token}\n" + "Unicode 世界😀 ".repeat(50), phase = "final_answer"))
            val final = wireJson.encodeToString(JsonObject.serializer(), message("Manager final", phase = "final_answer"))
            Files.writeString(sessions.resolve("root.jsonl"), final.take(final.length / 2), APPEND)
            collector.scan()
            assertFalse(Files.readString(logs.resolve("edict-agents.log")).contains("Manager final"))
            Files.writeString(sessions.resolve("root.jsonl"), final.drop(final.length / 2), APPEND)
            collector.scan(final = true)
            collector.scan(final = true)
            val full = Files.readString(logs.resolve("edict-agents.log"))
            val unwrapped = full.replace("\n    ", "")
            assertContains(unwrapped, "[edict-signal-analysis/${leaf.taskId.take(8)}] final: Leaf finished $digest [REDACTED] [REDACTED]")
            listOf("Manager commentary", "Batch commentary", "Early leaf output", "Manager final").forEach {
                assertEquals(1, Regex(Regex.escape(it)).findAll(full).count())
            }
            listOf("Unrelated output", "Private prompt", "Private reasoning", leaf.token, created.token).forEach { assertFalse(full.contains(it)) }
            full.lineSequence().forEach { assertTrue(it.codePointCount(0, it.length) <= 120) }
            assertEquals(full, Files.readString(logs.resolve("edict-agent-short.log")))
        }
    }

    @Test fun `MCP logs contain complete assignments and summaries without capability leaks`() {
        Store(directory.resolve("state")).use { store ->
            val logs = directory.resolve("log")
            val server = McpServer(store, logs = logs)
            fun call(name: String, arguments: JsonObject): JsonObject {
                val result = server.call(name, arguments)
                assertEquals(false, result.flag("isError"))
                return result.obj("structuredContent")
            }
            val created = call("edict_plan_create", buildJsonObject {
                put("request", "Extract"); put("steps", json.encodeToJsonElement(listOf(Step("edict-batch-signal-analysis", "Commit"))))
            })
            val task = store.plan()!!.tasks.single()
            val prompt = "\$managed-edict-batch-signal-analysis\nRead /skills/managed-edict-batch-signal-analysis/SKILL.md and inspect HEAD."
            val delegated = call("edict_delegate", buildJsonObject { put("token", created.text("token")); put("taskId", task.id); put("prompt", prompt) })
            val token = delegated.text("token")
            call("edict_task_get", buildJsonObject { put("token", token) })
            call("edict_task_start", buildJsonObject { put("token", token); put("agentId", "worker"); put("skill", task.skill) })
            call("edict_task_finish", buildJsonObject { put("token", token); put("status", "completed"); put("result", "Done") })
            val full = Files.readString(logs.resolve("edict-agents.log"))
            val short = Files.readString(logs.resolve("edict-agent-short.log"))
            assertContains(full.replace("\n    ", ""), prompt.replace("\n", ""))
            assertContains(full, "[${task.skill}/${task.id.take(8)}] mcp: edict_task_finish response:")
            assertFalse(short.contains("response:"))
            assertContains(short, "Started task")
            listOf(full, short).forEach { assertFalse(it.contains(token)); assertFalse(it.contains(created.text("token"))) }
        }
    }

    @Test fun `runtime failure still flushes agent messages`() {
        Store(directory.resolve("state")).use { store ->
            val executable = directory.resolve("codex-stub")
            Files.writeString(executable, """
                #!/bin/sh
                mkdir -p "${'$'}CODEX_HOME/sessions"
                printf '%s\n' '{"type":"thread.started","thread_id":"failed-root"}'
                printf '%s\n' '{"type":"session_meta","payload":{"id":"failed-root","source":"exec"}}' '{"timestamp":"2026-09-23T16:00:00Z","type":"response_item","payload":{"type":"message","role":"assistant","phase":"commentary","content":[{"type":"output_text","text":"Partial output before failure"}]}}' > "${'$'}CODEX_HOME/sessions/root.jsonl"
                exit 7
            """.trimIndent())
            executable.toFile().setExecutable(true)
            val logs = directory.resolve("log")
            val runtime = CodexRunner(directory, directory, store.root, "http://127.0.0.1/mcp", executable.toString(), "test", AgentLogger(store, logs))
            runtime.prepare()
            assertContains(assertFailsWith<IllegalStateException> { runtime.run("test") }.message!!, "Codex exited 7")
            assertContains(Files.readString(logs.resolve("edict-agents.log")), "[edict_manager/-] commentary: Partial output before failure")
        }
    }
}
