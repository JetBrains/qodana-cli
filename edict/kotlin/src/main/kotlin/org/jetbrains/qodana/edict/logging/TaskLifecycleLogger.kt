// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.logging

import org.jetbrains.qodana.edict.common.EdictLayout
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState.Plan
import java.io.PrintWriter
import java.nio.file.Files
import java.nio.file.StandardOpenOption.APPEND
import java.nio.file.StandardOpenOption.CREATE

/** MCP task transitions, also streamed by CI while the coordinating agent waits. */
internal class TaskLifecycleLogger(
    private val store: EdictNextRepositoryState,
    layout: EdictLayout,
    private val output: PrintWriter,
) {
    private val file = layout.tasksLogPath.also {
        Files.createDirectories(it.parent)
        Files.writeString(it, "", CREATE, APPEND)
    }

    // Called with the store locked, so parallel workers cannot reorder transitions.
    fun record(before: Plan?, after: Plan) {
        val previous = before?.tasks?.associateBy { it.id }.orEmpty()
        for (task in after.tasks) {
            val oldStatus = previous[task.id]?.status
            val event = when {
                task.status == oldStatus -> continue
                task.status == "running" -> "started"
                task.status in CLOSED && oldStatus !in CLOSED -> "finished"
                else -> continue
            }
            val status = if (event == "finished") " [${task.status}]" else ""
            val header = singleLine(store.redact("[-:${task.id.take(DISPLAYED_ID_LENGTH)}] ${task.title} $event$status"))
            val message = if (event == "finished") {
                "$header\n  ${singleLine(store.redact(task.result))}"
            }
            else header
            Files.writeString(file, "$message\n", APPEND)
            // stdout belongs to JSON-RPC when the MCP transport is stdio.
            output.println(message)
            output.flush()
        }
    }
}

private val CLOSED = setOf("completed", "failed", "cancelled")
private const val DISPLAYED_ID_LENGTH = 8
private fun singleLine(value: String): String = value.replace(Regex("[\\p{Cntrl}\\s]+"), " ").trim()
