// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import java.security.MessageDigest
import java.security.SecureRandom

val json = Json { encodeDefaults = true; prettyPrint = true; explicitNulls = false }
internal val wireJson = Json { encodeDefaults = true; explicitNulls = false }
fun sha256(value: String): String =
    MessageDigest.getInstance("SHA-256").digest(value.toByteArray(Charsets.UTF_8)).toHexString()

fun stableSignalId(key: String): String = "s-${sha256(key).take(10)}"
internal fun randomId(): String = ByteArray(32).also(SecureRandom()::nextBytes).toHexString()

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

@Serializable
enum class SignalLabel { POSITIVE, NEGATIVE }

@Serializable
data class SignalRange(val start: Int, val end: Int)

@Serializable
data class FileRevision(val path: String, val revision: String, val expectedRanges: List<SignalRange>)

@Serializable
data class SignalSource(
    val type: String,
    val diffPositiveToNegative: String,
    val commitRevision: String? = null,
    val parentRevision: String? = null,
    val message: String? = null,
    val prNumber: Int? = null,
    val title: String? = null,
    val discussionMessages: List<String> = emptyList(),
    val url: String? = null,
)

@Serializable
data class Provenance(val workItemId: String, val analysisBatchId: String? = null)

@Serializable
data class Signal(
    val id: String,
    val idempotencyKey: String,
    val fileRevision: FileRevision,
    val source: SignalSource,
    val label: SignalLabel,
    val description: String,
    val syntheticExampleId: String? = null,
    val provenance: Provenance,
)
