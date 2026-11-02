// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.promotion.support

import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import org.jetbrains.qodana.edict.ci.ReviewApi
import org.jetbrains.qodana.edict.common.flag
import org.jetbrains.qodana.edict.edictnext.EdictManagementService
import org.jetbrains.qodana.edict.edictnext.EdictNextJson
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState.Delegation
import org.jetbrains.qodana.edict.edictnext.EdictNextSignal
import org.jetbrains.qodana.edict.edictnext.EdictRepository
import org.jetbrains.qodana.edict.common.EdictLayout
import org.jetbrains.qodana.edict.support.withTools
import org.jetbrains.qodana.edict.support.launch

/** Fetches the batch of PR [prNumber] from [reviews] as the PR-analysis coordinator does through the MCP server. */
internal fun EdictRepository.fetchPrBatch(reviews: ReviewApi, prNumber: Int) = withManagement(reviews) { state, management ->
  management.callOrFail("edict_fetch_pr_batch", buildJsonObject {
    put("token", state.worker("edict-pr-signal-analysis").token)
    put("prNumbers", JsonArray(listOf(JsonPrimitive(prNumber))))
  })
}

/** Publishes [signal] to the inbox as the weak-signal promotion checker does through the MCP server. */
internal fun EdictRepository.publishToInbox(signal: EdictNextSignal) = withManagement(null) { state, management ->
  management.callOrFail("edict_publish_signal", buildJsonObject {
    put("token", state.worker("edict-check-weak-signal-promotion").token)
    put("signal", EdictNextJson.encodeToJsonElement(EdictNextSignal.serializer(), signal))
  })
}

private fun <T> EdictRepository.withManagement(
  reviews: ReviewApi?,
  action: (EdictNextRepositoryState, EdictManagementService) -> T,
): T = EdictNextRepositoryState.open(paths.root).use { state ->
  val layout = EdictLayout(paths.root.parent, paths.root)
  action(state, EdictManagementService(state, layout, ciReviews = reviews).withTools(layout))
}

private fun EdictManagementService.callOrFail(name: String, arguments: JsonObject) {
  val response = call(name, arguments)
  check(response.flag("isError") != true) { "$name failed: $response" }
}

/** A worker of the only task of a new plan for [skill]. */
private fun EdictNextRepositoryState.worker(skill: String): Delegation {
  val plan = createPlan("Run $skill", listOf(EdictNextRepositoryState.Step(skill, "Run $skill")))
  return launch(plan.token, plan.plan.tasks.single().id, skill)
}
