package org.jetbrains.qodana.edict.integration.support

import java.nio.file.Files
import java.nio.file.Path
import kotlin.test.*
import kotlinx.serialization.json.*
import org.jetbrains.qodana.edict.common.flag
import org.jetbrains.qodana.edict.common.obj
import org.jetbrains.qodana.edict.common.text
import org.jetbrains.qodana.edict.common.wireJson
import org.jetbrains.qodana.edict.model.Plan
import org.jetbrains.qodana.edict.model.Task
import org.jetbrains.qodana.edict.model.TaskAssignment
import org.jetbrains.qodana.edict.skills.Registry

internal fun verifyAgentLogs(logs: Path, plan: Plan) {
    val full = Files.readString(logs.resolve("edict-agents.log"))
    val short = Files.readString(logs.resolve("edict-agent-short.log"))
    listOf(full, short).forEach { text ->
        assertContains(text, "[edict_manager/-] final:")
        assertFalse(text.contains("[unassigned/-]"), "A worker exited before its managed task started")
        text.lineSequence().forEach { assertTrue(it.codePointCount(0, it.length) <= 120, "Log must wrap at 120 characters") }
        plan.tasks.forEach { task ->
            val prefix = "[${task.skill}/${task.id.take(8)}] "
            assertContains(text, prefix + "final:")
            assertContains(text, prefix + "mcp: Started task")
        }
    }
    val unwrapped = full.replace("\n    ", "")
    plan.tasks.forEach { task ->
        val prefix = "[${task.skill}/${task.id.take(8)}] mcp: "
        assertContains(unwrapped, prefix + "Task prompt assigned by ")
        assertContains(unwrapped, task.prompt.replace("\t", "    ").replace("\n", ""))
        assertContains(full, prefix + "edict_task_get response:")
        assertContains(full, prefix + "edict_task_finish response:")
        assertFalse(short.contains(prefix + "edict_task_get response:"))
        assertFalse(short.contains(prefix + "edict_task_finish response:"))
    }
}

/** Inspect actual runtime receipts rather than trusting task declarations or text in parent prompts. */
internal fun verifyRuntimeWorkers(sessions: Path, plan: Plan) {
    data class Worker(val skillsAtStart: Set<String>, val assignment: TaskAssignment?, val skills: Set<String>)
    val workers = mutableMapOf<String, Worker>()
    val spawned = mutableSetOf<String>()
    val skillPath = Regex("(edict_manager|managed-edict-[a-z-]+)/SKILL\\.md")
    Files.walk(sessions).use { paths -> paths.filter { it.toString().endsWith(".jsonl") }.forEach { file ->
        var id = ""
        var agentPath = ""
        var started = false
        var assignment: TaskAssignment? = null
        var assignmentAtStart: TaskAssignment? = null
        var skillsAtStart = emptySet<String>()
        val skills = mutableSetOf<String>()
        val calls = mutableMapOf<String, List<String>>()
        val spawnCalls = mutableSetOf<String>()
        Files.readAllLines(file).filter(String::isNotBlank).forEach { line ->
            val event = wireJson.parseToJsonElement(line).jsonObject
            val payload = event.obj("payload")
            if (event.text("type") == "session_meta") {
                id = payload.text("id")
                agentPath = payload.obj("source").obj("subagent").obj("thread_spawn").text("agent_path")
            }
            if (event.text("type") == "event_msg" && payload.text("type") == "item_completed") {
                val item = payload.obj("item")
                if (item["exit_code"] == JsonPrimitive(0)) {
                    skillPath.findAll(item["command"].toString()).forEach { match ->
                        if (item.text("aggregated_output").contains("name: ${match.groupValues[1]}")) skills += match.groupValues[1]
                    }
                }
                if (item.text("type").contains("collab", true) && item.text("tool") == "spawn_agent") {
                    (item["receiver_thread_ids"] as? JsonArray)?.forEach { spawned += it.jsonPrimitive.content }
                }
                if (item.text("server") == "edict-mcp") {
                    if (item.text("tool") == "edict_task_get" && !started && item.obj("result").flag("isError") != true)
                        assignment = wireJson.decodeFromJsonElement<TaskAssignment>(item.obj("result").getValue("structuredContent"))
                    if (item.text("tool") == "edict_task_start" && !started) {
                        started = true; skillsAtStart = skills.toSet(); assignmentAtStart = assignment
                    }
                }
            }
            if (event.text("type") == "response_item") when (payload.text("type")) {
                "function_call", "custom_tool_call" -> {
                    val text = payload.text("input") + payload.text("arguments")
                    calls[payload.text("call_id")] = skillPath.findAll(text).map { it.groupValues[1] }.toList()
                    if (payload.text("name").substringAfterLast('.') == "spawn_agent") spawnCalls += payload.text("call_id")
                }
                "function_call_output", "custom_tool_call_output" -> {
                    calls[payload.text("call_id")].orEmpty().forEach { skill -> if (payload["output"].toString().contains("name: $skill")) skills += skill }
                    if (payload.text("call_id") in spawnCalls) {
                        val output = runCatching { wireJson.parseToJsonElement(payload.text("output")).jsonObject }.getOrNull()
                        listOf("agent_id", "task_name").mapNotNull { output?.text(it)?.takeIf(String::isNotEmpty) }.forEach { spawned += it }
                    }
                }
            }
        }
        if (id.isNotEmpty()) workers[id] = Worker(skillsAtStart, assignmentAtStart, skills)
        if (agentPath.isNotEmpty()) workers[agentPath] = Worker(skillsAtStart, assignmentAtStart, skills)
    } }
    plan.tasks.forEach { task ->
        assertTrue(task.agentId in spawned, "Task ${task.id} did not use a native spawned worker")
        val worker = assertNotNull(workers[task.agentId], "Missing runtime session for ${task.skill}")
        assertTrue(Registry.installedName(task.skill) in worker.skillsAtStart, "Worker must read its assigned skill before startup")
        assertFalse("edict_manager" in worker.skills, "Worker must not execute the root manager skill")
        assertEquals(TaskAssignment(task.id, task.skill, Registry.skillPath(task.skill), task.prompt), worker.assignment, "Worker must fetch its exact assignment in its own session before startup")
    }
}
