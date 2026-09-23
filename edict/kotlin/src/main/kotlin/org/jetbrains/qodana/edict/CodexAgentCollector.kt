// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict

import kotlinx.serialization.json.*
import java.io.RandomAccessFile
import java.nio.file.Files
import java.nio.file.Path
import java.time.Instant

/** Tails runtime receipts once, including descendants identified by native thread ancestry. */
internal class CodexAgentCollector(private val home: Path, stdout: Path, private val logger: AgentLogger) {
    private val output = JsonlTail(stdout)
    private var root = ""
    private class Session(path: Path) {
        val tail = JsonlTail(path)
        var id = ""
        var parent = ""
        var agentPath = ""
        val messages = mutableListOf<AgentLogger.Message>()
        fun consume(event: JsonObject) {
            val payload = event.obj("payload")
            if (event.text("type") == "session_meta") {
                id = payload.text("id")
                val spawn = payload.obj("source").obj("subagent").obj("thread_spawn")
                parent = spawn.text("parent_thread_id")
                agentPath = spawn.text("agent_path")
            }
            if (event.text("type") != "response_item" || payload.text("type") != "message" ||
                payload.text("role") != "assistant" || payload.text("phase") !in listOf("", "commentary", "final", "final_answer")) return
            val text = payload.array("content").filter { it.text("type") == "output_text" }.joinToString("\n") { it.text("text") }
            if (text.isNotBlank()) messages += AgentLogger.Message(
                Instant.parse(event.text("timestamp")), id, agentPath, phase = payload.text("phase"), text = text,
            )
        }
    }
    private val sessions = mutableMapOf<Path, Session>()

    fun scan(final: Boolean = false) {
        output.read(final) { if (it.text("type") == "thread.started") root = it.text("thread_id") }
        val directory = home.resolve("sessions")
        if (Files.exists(directory)) Files.walk(directory).use { paths ->
            paths.filter { Files.isRegularFile(it) && it.toString().endsWith(".jsonl") }.forEach {
                val session = sessions.getOrPut(it) { Session(it) }
                session.tail.read(final, session::consume)
            }
        }
        val allowed = mutableSetOf<String>()
        if (root.isNotBlank()) allowed += root
        do {
            val size = allowed.size
            sessions.values.filter { it.id.isNotBlank() && it.parent in allowed }.forEach { allowed += it.id }
        } while (size != allowed.size)
        sessions.values.filter { it.id in allowed }.forEach { session ->
            session.messages.forEach { logger.record(it.copy(agentId = session.id, agentPath = session.agentPath, root = session.id == root)) }
            session.messages.clear()
        }
        logger.flush(final)
    }
}

/** Preserve split UTF-8/JSON records until newline; final flush accepts a complete last record without newline. */
private class JsonlTail(private val path: Path) {
    private var offset = 0L
    private var pending = byteArrayOf()
    fun read(final: Boolean, consume: (JsonObject) -> Unit) {
        if (!Files.exists(path)) return
        RandomAccessFile(path.toFile(), "r").use { file ->
            check(file.length() >= offset) { "Runtime trace was truncated: $path" }
            file.seek(offset)
            while (file.filePointer < file.length()) {
                val bytes = ByteArray(minOf(64 * 1024L, file.length() - file.filePointer).toInt())
                file.readFully(bytes)
                offset += bytes.size
                pending += bytes
                var start = 0
                pending.indices.forEach { end ->
                    if (pending[end] == '\n'.code.toByte()) {
                        val line = pending.copyOfRange(start, end).toString(Charsets.UTF_8).trim()
                        if (line.isNotEmpty()) consume(wireJson.parseToJsonElement(line).jsonObject)
                        start = end + 1
                    }
                }
                pending = pending.copyOfRange(start, pending.size)
                check(pending.size <= 32 * 1024 * 1024) { "Runtime record exceeds 32 MiB: $path" }
            }
        }
        if (final && pending.isNotEmpty()) {
            val line = pending.toString(Charsets.UTF_8).trim()
            if (line.isNotEmpty()) consume(wireJson.parseToJsonElement(line).jsonObject)
            pending = byteArrayOf()
        }
    }
}
