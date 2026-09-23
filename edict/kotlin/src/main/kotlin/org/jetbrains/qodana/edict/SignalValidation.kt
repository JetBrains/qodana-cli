// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.*

private val revisionPattern = Regex("(?:[0-9a-f]{40}|[0-9a-f]{64})")
fun validRevision(value: String): Boolean = revisionPattern.matches(value)
fun validSourcePath(value: String): Boolean = value.isNotEmpty() && !value.startsWith('/') &&
        value.none { it in "\\:\u0000\r\n" } && value.split('/').none { it.isEmpty() || it == "." || it == ".." }

/** Combines the managed Go contract with the standalone parts of repository-validation-utils.kt. */
object SignalValidation {
    fun validate(path: String, content: String): Signal {
        fun field(name: String, condition: Boolean, reason: String) {
            require(condition) { "Invalid signal '$path': $name: $reason" }
        }

        val document = try {
            json.parseToJsonElement(content).jsonObject
        } catch (e: Exception) {
            throw IllegalArgumentException("Invalid signal '$path': JSON: ${e.message}", e)
        }
        val signal = try {
            json.decodeFromJsonElement<Signal>(document)
        } catch (e: Exception) {
            throw IllegalArgumentException("Invalid signal '$path': JSON: ${e.message}", e)
        }
        field("idempotencyKey", signal.idempotencyKey.isNotBlank(), "must not be blank")
        field("id", signal.id == stableSignalId(signal.idempotencyKey), "must equal SHA-256 derived stable signal ID")
        field("id", path.substringAfterLast('/') == "${signal.id}.json", "must match filename")
        field(
            "description",
            signal.description.isNotBlank() && signal.description == signal.description.trim() &&
                    signal.description.length <= 1000 && signal.description.none { it == '\n' || it == '\r' },
            "must be one concise line of at most 1000 characters"
        )
        field(
            "syntheticExampleId",
            !path.startsWith("inbox/") || signal.syntheticExampleId == null,
            "inbox signals cannot reference examples"
        )
        field("provenance.workItemId", signal.provenance.workItemId.isNotBlank(), "must not be blank")
        val file = signal.fileRevision
        field("fileRevision.path", validSourcePath(file.path), "must be a clean repository-relative path")
        field("fileRevision.revision", validRevision(file.revision), "must be a full lowercase Git revision")
        field(
            "fileRevision.expectedRanges",
            file.expectedRanges.isNotEmpty(),
            "must contain ranges with integer start and end"
        )
        file.expectedRanges.forEachIndexed { i, range ->
            val raw = document.getValue("fileRevision").jsonObject.getValue("expectedRanges").jsonArray[i].jsonObject
            field("fileRevision.expectedRanges[$i]", listOf("start", "end").all { key ->
                val number = raw[key] as? JsonPrimitive
                number != null && !number.isString && number.intOrNull != null
            }, "start and end must be JSON integers, not quoted strings")
            field(
                "fileRevision.expectedRanges[$i]",
                range.start >= 1 && range.end >= range.start,
                "requires 1 <= start <= end"
            )
        }
        val source = signal.source
        when (source.type) {
            "FromCommit" -> {
                field(
                    "source.commitRevision",
                    validRevision(source.commitRevision.orEmpty()),
                    "requires full correcting revision"
                )
                field(
                    "source.parentRevision",
                    validRevision(source.parentRevision.orEmpty()),
                    "requires full parent revision"
                )
                field("source.message", !source.message.isNullOrBlank(), "requires complete commit message")
                field(
                    "source.diffPositiveToNegative",
                    source.diffPositiveToNegative.endsWith('\n'),
                    "must preserve canonical Git output including final newline"
                )
                val expected =
                    if (signal.label == SignalLabel.POSITIVE) source.parentRevision else source.commitRevision
                field(
                    "fileRevision.revision",
                    file.revision == expected,
                    "must match ${signal.label} evidence side ($expected)"
                )
            }

            "FromPR" -> field(
                "source", (source.prNumber ?: 0) > 0 && !source.title.isNullOrBlank() &&
                        !source.url.isNullOrBlank() && source.discussionMessages.isNotEmpty() && source.discussionMessages.all(
                    String::isNotBlank
                ),
                "FromPR requires prNumber, title, discussionMessages and url"
            )

            else -> field("source.type", false, "must be FromCommit or FromPR")
        }
        val changes = try {
            UnifiedDiff.parse(source.diffPositiveToNegative).side(signal.label)
        } catch (e: Exception) {
            throw IllegalArgumentException("Invalid signal '$path': source.diffPositiveToNegative: ${e.message}", e)
        }
        field("fileRevision.path", file.path in changes, "must match a changed ${signal.label} path: ${changes.keys}")
        file.expectedRanges.forEachIndexed { i, range ->
            field(
                "fileRevision.expectedRanges[$i]", changes.getValue(file.path).any { it in range.start..range.end },
                "must intersect a changed ${signal.label} line in '${file.path}'"
            )
        }
        return signal
    }
}

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
