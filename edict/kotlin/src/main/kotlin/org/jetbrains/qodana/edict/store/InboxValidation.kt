// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.store

import kotlinx.serialization.Serializable
import org.jetbrains.qodana.edict.common.json
import org.jetbrains.qodana.edict.common.sha256
import org.jetbrains.qodana.edict.model.Signal
import org.jetbrains.qodana.edict.signals.SignalValidation

@Serializable
data class ValidatedInboxFile(
    val relativePath: String,
    val sha256: String,
    val signalId: String,
    val idempotencyKey: String
)

@Serializable
data class ValidationReceipt(val receiptId: String, val validatedFiles: List<ValidatedInboxFile>)

/** Read-only validation of an already-published managed inbox; no IDE or Git checkout is required for state. */
fun validateInboxChanges(store: Store, paths: List<String>): ValidationReceipt {
    require(paths.isNotEmpty() && paths.distinct().size == paths.size) { "Provide distinct inbox paths" }
    val files = paths.map { path ->
        require(path.startsWith("inbox/") && path.count { it == '/' } == 1) { "Expected inbox/<signal-id>.json" }
        val file = store.read(path)
        val signal = SignalValidation.validate(path, file.content)
        ValidatedInboxFile(path, file.hash, signal.id, signal.idempotencyKey)
    }
    require(files.map { it.idempotencyKey }.distinct().size == files.size) { "Duplicate idempotency keys" }
    val keys = files.map { it.idempotencyKey }.toSet()
    store.list("inbox").filterNot { it in paths }.forEach { path ->
        val existing = json.decodeFromString<Signal>(store.read(path).content)
        require(existing.idempotencyKey !in keys) { "Idempotency key already exists in $path" }
    }
    return ValidationReceipt(
        "validation-${sha256(files.joinToString("|") { "${it.relativePath}:${it.sha256}" }).take(24)}",
        files
    )
}
