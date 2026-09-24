// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.logging

import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.StandardOpenOption.*
import java.nio.file.attribute.PosixFilePermissions
import java.time.Instant
import kotlinx.serialization.json.*
import org.jetbrains.qodana.edict.common.flag
import org.jetbrains.qodana.edict.common.json
import org.jetbrains.qodana.edict.common.obj
import org.jetbrains.qodana.edict.common.text
import org.jetbrains.qodana.edict.model.Task
import org.jetbrains.qodana.edict.store.Store

/** Human-readable assistant output and attributed MCP activity; private runtime events are excluded. */
class AgentLogger(private val store: Store, val directory: Path) {
    data class Message(
        val time: Instant, val agentId: String, val agentPath: String = "", val root: Boolean = false,
        val phase: String = "message", val text: String,
    )
    private data class Identity(val skill: String, val task: String = "")
    private val pending = mutableListOf<Message>()
    private val identities = mutableMapOf<String, Identity>()
    private val full = directory.resolve("edict-agents.log")
    private val short = directory.resolve("edict-agent-short.log")

    init {
        Files.createDirectories(directory)
        if (Files.getFileStore(directory).supportsFileAttributeView("posix"))
            Files.setPosixFilePermissions(directory, PosixFilePermissions.fromString("rwx------"))
        listOf(full, short).forEach {
            Files.writeString(it, "", CREATE, APPEND)
            if (Files.getFileStore(it).supportsFileAttributeView("posix"))
                Files.setPosixFilePermissions(it, PosixFilePermissions.fromString("rw-------"))
        }
    }

    @Synchronized fun record(message: Message) { pending += message }

    /** Worker commentary can arrive before task_start provides the runtime identity. */
    @Synchronized fun flush(final: Boolean = false) {
        store.plan()?.tasks?.filter { it.agentId.isNotBlank() }?.forEach {
            identities[it.agentId] = Identity(it.skill, it.id)
        }
        val iterator = pending.sortedBy { it.time }.iterator()
        val retained = mutableListOf<Message>()
        while (iterator.hasNext()) {
            val message = iterator.next()
            val identity = if (message.root) Identity("edict_manager")
                else identities[message.agentId] ?: identities[message.agentPath]
            if (identity == null && !final) { retained += message; continue }
            val phase = when (message.phase) { "final_answer" -> "final"; "" -> "message"; else -> message.phase }
            write(message.time, identity ?: Identity("unassigned"), phase, message.text, message.text)
        }
        pending.clear()
        pending += retained
    }

    @Synchronized internal fun mcp(caller: String, tool: String, arguments: JsonObject, response: JsonObject) {
        val at = Instant.now()
        val result = response.obj("structuredContent")
        val ok = response.flag("isError") != true
        val callerIdentity = if (caller.startsWith("edict_manager/") || tool == "edict_plan_create" && ok)
            Identity("edict_manager") else Identity(caller.substringBefore('/'), caller.substringAfter('/', ""))
        val target = if (tool == "edict_delegate" && ok) Identity(result.text("skill"), result.text("taskId")) else callerIdentity
        val summary = if (!ok) "$tool failed" else when (tool) {
            "edict_task_start" -> "Started task; assignment fetched and skill verified"
            "edict_task_finish" -> "Finished task: ${arguments.text("status")}"
            "edict_task_cancel" -> "Cancelled task ${arguments.text("taskId").take(8)}"
            "edict_delegate" -> "Task delegated by ${callerIdentity.skill}/${callerIdentity.task.take(8).ifEmpty { "-" }}"
            "edict_state_write" -> "Wrote ${arguments.text("path")}"
            "edict_state_delete" -> "Deleted ${arguments.text("path")}"
            else -> "$tool ok"
        }
        val detail = if (tool == "edict_delegate" && ok)
            "Task prompt assigned by ${callerIdentity.skill}/${callerIdentity.task.take(8).ifEmpty { "-" }}:\n${arguments.text("prompt")}" else summary
        write(at, target, "mcp", detail, summary)
        val payload = response["structuredContent"] ?: response["content"] ?: JsonNull
        write(at, target, "mcp", "$tool response:\n${json.encodeToString(JsonElement.serializer(), payload)}", null)
        flush()
    }

    private fun write(at: Instant, identity: Identity, kind: String, text: String, shortText: String?) {
        val prefix = "$at [${identity.skill}/${identity.task.take(8).ifEmpty { "-" }}] $kind: "
        Files.writeString(full, format(prefix, store.redact(text)), APPEND)
        if (shortText != null) Files.writeString(short, format(prefix, store.redact(shortText)), APPEND)
    }

    private fun format(initialPrefix: String, text: String): String = buildString {
        var prefix = initialPrefix
        for (line in text.replace("\t", "    ").trimEnd('\r', '\n').split('\n')) {
            val points = line.trimEnd('\r').codePoints().toArray()
            var offset = 0
            do {
                val length = minOf(points.size - offset, 120 - prefix.codePointCount(0, prefix.length))
                append(prefix).append(String(points, offset, length)).append('\n')
                offset += length
                prefix = "    "
            } while (offset < points.size)
        }
    }
}
