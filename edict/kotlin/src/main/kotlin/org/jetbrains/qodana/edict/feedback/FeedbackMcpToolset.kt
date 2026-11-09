// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

import io.modelcontextprotocol.kotlin.sdk.server.Server
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.encodeToJsonElement
import org.jetbrains.qodana.edict.edictnext.EdictManagementService
import org.jetbrains.qodana.edict.edictnext.EdictNextJson
import org.jetbrains.qodana.edict.edictnext.property
import org.jetbrains.qodana.edict.edictnext.requireString
import org.jetbrains.qodana.edict.edictnext.stringArguments
import org.jetbrains.qodana.edict.edictnext.toToolResult
import org.jetbrains.qodana.edict.edictnext.toolSchema

/** The managed skill that turns the answers to review comments of Edict into Signals. */
internal const val FEEDBACK_SIGNAL_SKILL = "edict-feedback-signal-analysis"

/** The tools that turn the answers to review comments of Edict into inbox Signals. */
internal class FeedbackMcpToolset(
    private val management: EdictManagementService,
    private val signals: FeedbackSignals,
) {
    fun registerTools(server: Server) {
        server.addTool(
            name = "edict_collect_feedback_answers",
            description = "Return the active feedback records whose review comment got only 👍 (label POSITIVE: the finding " +
                "is a real problem) or only 👎 (label NEGATIVE: it is not), with the comment's URL and replies, and move " +
                "the other records of closed pull requests to history; the rest stay active until they get an answer.",
            inputSchema = stringArguments("token" to "Your managed feedback-analysis task token"),
        ) { request ->
            management.requireSkill(request.requireString("token"), FEEDBACK_SIGNAL_SKILL)
            buildJsonObject { put("answers", EdictNextJson.encodeToJsonElement(signals.collect())) }.toToolResult()
        }

        server.addTool(
            name = "edict_publish_feedback_signal",
            description = "Publish the STRONG SubmittedFeedback Signal of one answer that edict_collect_feedback_answers " +
                "returned to the inbox, and move its record to history. The server takes the label from the comment's " +
                "thumbs, and the code, revision, inspection and replies, as written, from the record and the comment.",
            inputSchema = toolSchema(
                required = listOf("token", "recordId", "description"),
                "token" to property("string", "Your managed feedback-analysis task token"),
                "recordId" to property("string", "The id of a record that edict_collect_feedback_answers returned"),
                "description" to property("string", "One line saying what the record's lines demonstrate, under the label"),
            ),
        ) { request ->
            val token = request.requireString("token")
            management.requireSkill(token, FEEDBACK_SIGNAL_SKILL)
            signals.publish(token, request.requireString("recordId"), request.requireString("description")).toToolResult()
        }
    }
}
