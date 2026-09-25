// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.signals

import kotlinx.serialization.json.*
import org.jetbrains.qodana.edict.common.json
import org.jetbrains.qodana.edict.model.Signal
import org.jetbrains.qodana.edict.model.SignalLabel
import org.jetbrains.qodana.edict.model.stableSignalId

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
        val suppliedFeedback = signal.source.type == "SubmittedFeedback" && signal.source.suggestionId != null
        if (suppliedFeedback && signal.idempotencyKey.isEmpty()) {
            field("id", signal.id.matches(Regex("s-[0-9a-f]{12}")), "expected the supplied feedback's 12-digit hexadecimal ID")
        } else {
            field("idempotencyKey", signal.idempotencyKey.isNotBlank(), "must not be blank")
            field("id", signal.id == stableSignalId(signal.idempotencyKey), "must equal SHA-256 derived stable signal ID")
        }
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
        field("provenance.workItemId", suppliedFeedback || signal.provenance.workItemId.isNotBlank(), "must not be blank")
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
            "SubmittedFeedback" -> {
                if (suppliedFeedback) {
                    field("source.suggestionId", !source.suggestionId.isNullOrBlank(), "requires the original suggestion reference")
                    field("source.inspectionName", !source.inspectionName.isNullOrBlank(), "requires the supplied inspection name")
                    field("source.inspectionDescription", !source.inspectionDescription.isNullOrBlank(), "requires the original feedback description")
                    field("source.codeSnippet", !source.codeSnippet.isNullOrBlank(), "requires the supplied source snippet")
                    field("source.reason", !source.reason.isNullOrBlank(), "requires the original feedback reason")
                } else {
                    field("source.message", !source.message.isNullOrBlank(), "requires the original feedback")
                    field("source.url", !source.url.isNullOrBlank(), "requires the feedback source reference")
                }
                field("source", source.diffPositiveToNegative.isEmpty() && source.commitRevision == null &&
                        source.parentRevision == null && source.prNumber == null && source.title == null &&
                        source.discussionMessages.isEmpty(), "feedback must not fabricate commit or PR evidence")
                // Feedback labels an exact source location; it has no correcting diff or before/after sides.
                return signal
            }

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

            else -> field("source.type", false, "must be FromCommit, FromPR or SubmittedFeedback")
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
