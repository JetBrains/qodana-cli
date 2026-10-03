// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.signals

import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.decodeFromJsonElement
import org.jetbrains.qodana.edict.common.json
import org.jetbrains.qodana.edict.edictnext.EdictNextSignal
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalSource
import org.jetbrains.qodana.edict.edictnext.stableSignalId

/** Validates extraction records at the trusted state boundary before Edict Next consumes them. */
object SignalValidation {
  fun validate(signal: EdictNextSignal): EdictNextSignal =
    validate("inbox/${signal.id}.json", json.encodeToString(signal))

  fun validate(path: String, content: String): EdictNextSignal {
    fun field(name: String, condition: Boolean, reason: String) {
      require(condition) { "Invalid signal '$path': $name: $reason" }
    }

    val document = try {
      json.parseToJsonElement(content).jsonObject
    }
    catch (e: Exception) {
      throw IllegalArgumentException("Invalid signal '$path': JSON: ${e.message}", e)
    }
    val signal = try {
      json.decodeFromJsonElement<EdictNextSignal>(document)
    }
    catch (e: Exception) {
      throw IllegalArgumentException("Invalid signal '$path': JSON: ${e.message}", e)
    }
    val source = signal.source
    val suppliedFeedback = source is EdictNextSignalSource.SubmittedFeedback && source.suggestionId != null
    if (suppliedFeedback && signal.idempotencyKey.isEmpty()) {
      field("id", signal.id.matches(Regex("s-[0-9a-f]{12}")), "expected the supplied feedback's 12-digit hexadecimal ID")
    }
    else {
      field("idempotencyKey", signal.idempotencyKey.isNotBlank(), "must not be blank")
      field("id", signal.id == stableSignalId(signal.idempotencyKey), "must equal SHA-256 derived stable signal ID")
    }
    field("id", path.substringAfterLast('/') == "${signal.id}.json", "must match filename")
    field(
      "description",
      signal.description.isNotBlank() && signal.description == signal.description.trim() &&
        signal.description.length <= 1000 && signal.description.none { it == '\n' || it == '\r' },
      "must be one concise line of at most 1000 characters",
    )
    field("syntheticExampleId", !path.startsWith("inbox/") || signal.syntheticExampleId == null, "inbox signals cannot reference examples")
    field("provenance.workItemId", suppliedFeedback || signal.provenance.workItemId.isNotBlank(), "must not be blank")
    val file = signal.fileRevision
    field("fileRevision.path", validSourcePath(file.path), "must be a clean repository-relative path")
    field("fileRevision.revision", validRevision(file.revision), "must be a full lowercase Git revision")
    field("fileRevision.expectedRanges", !file.expectedRanges.isNullOrEmpty(), "must contain ranges with integer start and end")
    file.expectedRanges.orEmpty().forEachIndexed { index, range ->
      val raw = document.getValue("fileRevision").jsonObject.getValue("expectedRanges").jsonArray[index].jsonObject
      field(
        "fileRevision.expectedRanges[$index]",
        listOf("start", "end").all { key ->
          val number = raw[key] as? JsonPrimitive
          number != null && !number.isString && number.intOrNull != null
        },
        "start and end must be JSON integers, not quoted strings",
      )
      field("fileRevision.expectedRanges[$index]", range.start >= 1 && range.end >= range.start, "requires 1 <= start <= end")
    }

    val diffPositiveToNegative = when (source) {
      is EdictNextSignalSource.SubmittedFeedback -> return signal
      is EdictNextSignalSource.FromCommit -> {
        field("source.commitRevision", validRevision(source.commitRevision.orEmpty()), "requires full correcting revision")
        null
      }
      is EdictNextSignalSource.FromPR -> {
        field(
          "source",
          source.prNumber > 0 && source.title.isNotBlank() && source.url.isNotBlank() &&
            source.discussionMessages.isNotEmpty() && source.discussionMessages.all(String::isNotBlank),
          "FromPR requires prNumber, title, discussionMessages and url",
        )
        source.diffPositiveToNegative
      }
      is EdictNextSignalSource.Generated -> {
        field("source.type", false, "must be FromCommit, FromPR or SubmittedFeedback")
        null
      }
    }
    if (diffPositiveToNegative == null) return signal
    val changes = try {
      UnifiedDiff.parse(diffPositiveToNegative).side(signal.label)
    }
    catch (e: Exception) {
      throw IllegalArgumentException("Invalid signal '$path': source.diffPositiveToNegative: ${e.message}", e)
    }
    field("fileRevision.path", file.path in changes, "must match a changed ${signal.label} path: ${changes.keys}")
    file.expectedRanges.orEmpty().forEachIndexed { index, range ->
      field(
        "fileRevision.expectedRanges[$index]",
        changes.getValue(file.path).any { it in range.start..range.end },
        "must intersect a changed ${signal.label} line in '${file.path}'",
      )
    }
    return signal
  }
}
