// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.integration.support.inspection

import java.nio.file.Files
import java.nio.file.Path
import kotlinx.serialization.json.jsonObject
import org.jetbrains.qodana.edict.common.text
import org.jetbrains.qodana.edict.common.wireJson
import org.jetbrains.qodana.edict.model.Plan

internal fun acceptedCandidateReviews(scratch: Path, candidateHash: String): List<Path> =
    Files.walk(scratch).use { files -> files.filter { Files.isRegularFile(it) }.filter { path ->
        val review = runCatching { wireJson.parseToJsonElement(Files.readString(path)).jsonObject }.getOrNull()
        review?.text("candidateHash") == candidateHash && review.text("status") == "ACCEPT"
    }.toList() }

/** A completed final plan alone must not conceal skipped execution or overlapping pipeline stages. */
internal fun stageOrderProblems(snapshots: List<Plan>, stages: List<String>): List<String> = buildList {
    val started = mutableSetOf<String>()
    snapshots.forEachIndexed { index, snapshot ->
        val states = snapshot.tasks.associate { it.id to it.status }
        snapshot.tasks.filter { it.status == "running" }.forEach { started += it.id }
        stages.forEachIndexed { stageIndex, id ->
            if (states[id] != null && states[id] != "pending") stages.take(stageIndex).forEach { previous ->
                if (states[previous] != "completed") add("Snapshot $index: stage $id is ${states[id]} before $previous completed (${states[previous]})")
            }
        }
    }
    stages.filter { it !in started }.forEach { add("Stage $it has no recorded running state") }
}
