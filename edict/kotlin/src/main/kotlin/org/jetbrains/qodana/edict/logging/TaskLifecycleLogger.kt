// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.logging

import org.jetbrains.qodana.edict.model.Plan
import org.jetbrains.qodana.edict.store.Store
import java.io.PrintWriter
import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.StandardOpenOption.APPEND
import java.nio.file.StandardOpenOption.CREATE

/** MCP task transitions, also streamed by CI while the coordinating agent waits. */
internal class TaskLifecycleLogger(
    private val store: Store,
    directory: Path?,
    private val output: PrintWriter,
) {
    private val file = directory?.resolve("edict-tasks.log")?.also {
        Files.createDirectories(it.parent)
        Files.writeString(it, "", CREATE, APPEND)
    }

    // Called with the store locked, so parallel workers cannot reorder transitions.
    fun record(before: Plan?, after: Plan) {
        val previous = before?.tasks?.associateBy { it.id }.orEmpty()
        val tasks = after.tasks.associateBy { it.id }
        for (task in after.tasks) {
            val oldStatus = previous[task.id]?.status
            val event = when {
                task.status == oldStatus -> continue
                task.status == "running" -> "started"
                task.status in listOf("completed", "failed") && oldStatus !in listOf("completed", "failed") -> "finished"
                else -> continue
            }
            // Reviewers and example workers inherit their cluster from their generation ancestor.
            val clusterTask = generateSequence(task) { tasks[it.parentId] }
                .firstOrNull { it.skill == "edict-cluster-generation" }
            val cluster = clusterTask?.scope?.firstNotNullOfOrNull { path ->
                path.removePrefix("clusters/").substringBefore('/').takeIf {
                    path.startsWith("clusters/") && it.isNotEmpty()
                }
            } ?: "-"
            val message = store.redact("[$cluster:${task.id}] ${task.title} $event")
                .replace(Regex("[\\p{Cntrl}\\s]+"), " ")
            file?.let { Files.writeString(it, "$message\n", APPEND) }
            // stdout belongs to JSON-RPC when the MCP transport is stdio.
            output.println(message)
            output.flush()
        }
    }
}
