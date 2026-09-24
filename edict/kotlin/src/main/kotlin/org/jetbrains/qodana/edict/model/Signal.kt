// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.model

import kotlinx.serialization.Serializable
import org.jetbrains.qodana.edict.common.sha256

fun stableSignalId(key: String): String = "s-${sha256(key).take(10)}"

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
