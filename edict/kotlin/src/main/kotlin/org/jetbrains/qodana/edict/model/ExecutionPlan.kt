// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.model

import kotlinx.serialization.Serializable
import org.jetbrains.qodana.edict.common.randomId
import org.jetbrains.qodana.edict.common.sha256

@Serializable
data class Step(val skill: String, val title: String)
@Serializable
data class Task(
    val id: String = randomId(), val parentId: String = "", val skill: String, val title: String,
    val prompt: String = "", val status: String = "pending", val agentId: String = "", val result: String = "",
    val operations: List<String> = emptyList(), val scope: List<String> = emptyList(),
)

@Serializable
data class Plan(val id: String = randomId(), val request: String, val revision: Int = 0, val tasks: List<Task>)
@Serializable
data class PlanCreation(val plan: Plan, val token: String)
@Serializable
data class StateFile(val path: String, val content: String, val hash: String = sha256(content))
@Serializable
data class Delegation(
    val token: String,
    val taskId: String,
    val skill: String,
    val skillPath: String,
    val prompt: String
)

@Serializable
data class TaskAssignment(val taskId: String, val skill: String, val skillPath: String, val prompt: String)
