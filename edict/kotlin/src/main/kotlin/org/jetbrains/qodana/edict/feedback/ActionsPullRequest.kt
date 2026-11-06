// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject
import org.jetbrains.qodana.edict.ci.CiProviderId
import org.jetbrains.qodana.edict.ci.ReviewReference
import org.jetbrains.qodana.edict.common.number
import org.jetbrains.qodana.edict.common.obj
import org.jetbrains.qodana.edict.common.text
import org.jetbrains.qodana.edict.common.wireJson
import java.nio.file.Path
import kotlin.io.path.readText

/** The pull request a GitHub Actions workflow runs for. */
internal sealed interface ActionsPullRequest {
    /** The workflow runs for something else, such as a push. */
    data object None : ActionsPullRequest

    /** The pull request comes from another repository, a fork or a deleted one, whose code gets no feedback. */
    data class FromFork(val reference: ReviewReference) : ActionsPullRequest

    /** A pull request between branches of the repository itself. */
    data class Own(val reference: ReviewReference) : ActionsPullRequest
}

/** The file holding the event of the workflow run, which GitHub Actions names in `GITHUB_EVENT_PATH` for every run. */
internal fun actionsEventPath(): Path = System.getenv("GITHUB_EVENT_PATH")?.takeIf(String::isNotBlank)?.let(Path::of)
    ?: error("GITHUB_EVENT_PATH is not set; Edict feedback runs in GitHub Actions")

/** The pull request of the workflow run whose event, a webhook payload, is in the file [eventPath]. */
internal fun actionsPullRequest(eventPath: Path): ActionsPullRequest {
    val event = wireJson.parseToJsonElement(eventPath.readText()).jsonObject
    val pullRequest = event["pull_request"] as? JsonObject ?: return ActionsPullRequest.None
    val base = pullRequest.obj("base").obj("repo")
    val owner = base.obj("owner").text("login")
    val number = pullRequest.number("number").toString()
    val reference = ReviewReference(CiProviderId.GITHUB, owner, base.text("name"), number, pullRequest.text("html_url"))
    // A deleted fork leaves no head repository.
    val headRepository = pullRequest.obj("head")["repo"] as? JsonObject
    return if (headRepository?.text("full_name") == base.text("full_name")) ActionsPullRequest.Own(reference)
    else ActionsPullRequest.FromFork(reference)
}
