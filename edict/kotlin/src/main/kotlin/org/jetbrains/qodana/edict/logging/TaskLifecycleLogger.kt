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
            val message = store.redact("[-:${task.id}] ${task.title} $event")
                .replace(Regex("[\\p{Cntrl}\\s]+"), " ")
            Files.writeString(file, "$message\n", APPEND)
            // stdout belongs to JSON-RPC when the MCP transport is stdio.
            output.println(message)
            output.flush()
        }
    }
}

private val CLOSED = setOf("completed", "failed", "cancelled")
