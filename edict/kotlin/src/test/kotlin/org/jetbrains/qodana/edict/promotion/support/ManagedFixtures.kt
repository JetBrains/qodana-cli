// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.promotion.support

import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import org.jetbrains.qodana.edict.ci.ReviewApi
import org.jetbrains.qodana.edict.common.flag
import org.jetbrains.qodana.edict.core.EdictManagementService
import org.jetbrains.qodana.edict.core.EdictJson
import org.jetbrains.qodana.edict.core.EdictRepositoryState
import org.jetbrains.qodana.edict.core.EdictRepositoryState.Delegation
import org.jetbrains.qodana.edict.core.EdictSignal
import org.jetbrains.qodana.edict.core.EdictRepository
import org.jetbrains.qodana.edict.common.EdictLayout
import org.jetbrains.qodana.edict.support.withTools
import org.jetbrains.qodana.edict.support.launch

/** Fetches the batch of PR [prNumber] as the PR-analysis coordinator of the run in [state] does through the MCP server. */
internal fun EdictManagementService.fetchPrBatch(state: EdictRepositoryState, prNumber: Int) =
  callOrFail("edict_fetch_pr_batch", buildJsonObject {
    put("token", state.worker("edict-pr-signal-analysis").token)
    put("prNumbers", JsonArray(listOf(JsonPrimitive(prNumber))))
  })

/** Publishes [signal] to the inbox as the weak-signal promotion checker does through the MCP server. */
internal fun EdictRepository.publishToInbox(signal: EdictSignal) = withManagement(null) { state, management ->
  management.callOrFail("edict_publish_signal", buildJsonObject {
    put("token", state.worker("edict-check-weak-signal-promotion").token)
    put("signal", EdictJson.encodeToJsonElement(EdictSignal.serializer(), signal))
  })
}

/** Runs [action] with the state of a run on this repository and its management service, reading reviews from [reviews]. */
internal fun <T> EdictRepository.withManagement(
  reviews: ReviewApi?,
  action: (EdictRepositoryState, EdictManagementService) -> T,
): T = EdictRepositoryState.open(paths.root).use { state ->
  val layout = EdictLayout(paths.root.parent, paths.root)
  action(state, EdictManagementService(state, layout, ciReviews = reviews).withTools(layout))
}

private fun EdictManagementService.callOrFail(name: String, arguments: JsonObject) {
  val response = call(name, arguments)
  check(response.flag("isError") != true) { "$name failed: $response" }
}

/** A worker of the only task of a new plan for [skill]. */
private fun EdictRepositoryState.worker(skill: String): Delegation {
  val plan = createPlan("Run $skill", listOf(EdictRepositoryState.Step(skill, "Run $skill")))
  return launch(plan.token, plan.plan.tasks.single().id, skill)
}
