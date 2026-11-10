// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

import io.github.oshai.kotlinlogging.KotlinLogging
import org.jetbrains.qodana.edict.common.EdictLayout
import org.jetbrains.qodana.edict.common.StateBranch
import org.jetbrains.qodana.edict.core.EdictRepository
import org.jetbrains.qodana.edict.core.EdictRepositoryDirectory
import java.nio.file.Path
import kotlin.io.path.copyTo
import kotlin.io.path.createDirectories
import kotlin.io.path.exists
import kotlin.io.path.listDirectoryEntries
import kotlin.io.path.name

private val logger = KotlinLogging.logger {}

/**
 * Checks [stateBranch] out at the state directory of [layout] and copies the inspections that [selectFeedbackInspections] picks for
 * the workflow's [event] into [inspectionsDirectory], which must be outside the project and missing or empty. Leaves it so when the
 * run gets no feedback: it is for no pull request, for one from a fork, or nothing is selected.
 */
internal fun prepareFeedback(
    layout: EdictLayout,
    stateBranch: String,
    event: ActionsPullRequest,
    inspectionsDirectory: Path
) {
    // The scan's git operations, such as committing quick fixes, could touch it inside the project.
    require(!layout.root.resolve(inspectionsDirectory).normalize().startsWith(layout.root)) {
        "The directory for the feedback inspections must be outside the project: $inspectionsDirectory"
    }
    // Whatever it holds would run for feedback too.
    require(!inspectionsDirectory.exists() || inspectionsDirectory.listDirectoryEntries().isEmpty()) {
        "The directory for the feedback inspections is not empty: $inspectionsDirectory"
    }
    val pullRequest = event.feedbackPullRequest() ?: return
    val branch = StateBranch(layout, stateBranch)
    if (!branch.onOrigin()) {
        logger.warn { "$FEEDBACK_UNAVAILABLE: origin has no $stateBranch, which holds the Edict state; the Edict routine publishes it" }
        return
    }
    // Only the current state is read, and `post` commits onto the state origin has then.
    val checkout = branch.checkOut(shallow = true)
    logger.info { branch.describe(checkout) }
    val paths = EdictRepositoryDirectory(layout.stateDirectory)
    val inspections = selectFeedbackInspections(EdictRepository(paths), FeedbackRecords(paths), pullRequest).inspections
    if (inspections.isEmpty()) {
        logger.info { "No Edict inspection runs for feedback on ${pullRequest.url}" }
        return
    }
    inspectionsDirectory.createDirectories()
    inspections.forEach { it.path.copyTo(inspectionsDirectory.resolve(it.path.name)) }
    logger.info { "Edict feedback runs ${inspections.joinToString { it.clusterId }} on ${pullRequest.url}" }
}
