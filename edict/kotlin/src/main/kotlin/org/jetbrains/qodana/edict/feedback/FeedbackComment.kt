// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

/** The marker that names [recordId] in its review comment, hidden from readers, so that an answer leads back to the record. */
internal fun feedbackMarker(recordId: String): String = "<!-- edict-feedback:$recordId -->"

/**
 * The review comment of [record]: that it is a test finding of Edict, its problem, how to assess it, and the inspection's
 * description folded away.
 */
internal fun feedbackComment(record: FeedbackRecord): String = buildString {
    appendLine("**Edict test finding.** This inspection was generated from the project's reviews and is not established yet.")
    appendLine("Your assessment decides whether it becomes one; this finding does not affect the analysis result.")
    appendLine()
    record.message.lines().forEach { appendLine("> $it") }
    appendLine()
    appendLine("**Is this a real problem?** React with 👍 if it is, or with 👎 if it is not.")
    appendLine("A reply that explains why is welcome too: it is kept as context of your answer.")
    appendLine()
    appendLine("<details>")
    appendLine("<summary>${record.inspectionName}</summary>")
    appendLine()
    appendLine(record.inspectionDescription)
    appendLine()
    appendLine("</details>")
    appendLine()
    append(feedbackMarker(record.id))
}
