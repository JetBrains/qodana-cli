// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback.support

import org.jetbrains.qodana.edict.ci.CiProviderId
import org.jetbrains.qodana.edict.ci.ReviewReference
import org.jetbrains.qodana.edict.ci.ReviewRepository
import org.jetbrains.qodana.edict.edictnext.EdictNextJson
import org.jetbrains.qodana.edict.edictnext.EdictNextLineRange
import org.jetbrains.qodana.edict.edictnext.EdictRepositoryDirectory
import org.jetbrains.qodana.edict.feedback.FeedbackRecord
import java.nio.file.Path
import kotlin.io.path.createDirectories
import kotlin.io.path.writeText

/** The repository the fixture pull requests belong to. */
internal val PROJECT = ReviewRepository(CiProviderId.GITHUB, "github.com", "owner", "project")

/** Pull request [number] of [PROJECT]. */
internal fun pullRequest(number: Int): ReviewReference = PROJECT.reference("$number", "https://github.com/owner/project/pull/$number")

/** The pull request the fixture workflows run for. */
internal val PULL_REQUEST = pullRequest(7)

/** Record `f-<index>` of a finding of [clusterId] on [pullRequest]. */
internal fun feedbackRecord(index: Int, clusterId: String = "rule", pullRequest: ReviewReference = PULL_REQUEST): FeedbackRecord =
  FeedbackRecord(
    id = "f-%02d".format(index),
    clusterId = clusterId,
    pullRequest = pullRequest,
    revision = "c0ffee00c0ffee00c0ffee00c0ffee00c0ffee00",
    path = "src/Main.kt",
    lines = EdictNextLineRange(3, 3),
    inspectionDescription = "Reports blocking calls on the worker thread.",
    message = "Blocking call on the worker thread",
    createdAt = "2026-11-05T10:00:00Z",
  )

/** Stores [records] as awaiting an answer. */
internal fun EdictRepositoryDirectory.storeActive(vararg records: FeedbackRecord) {
  records.forEach { store(activeFeedbackDirectory, it) }
}

/** Stores [records] as having left the active ones in October 2026. */
internal fun EdictRepositoryDirectory.storeHistory(vararg records: FeedbackRecord) {
  records.forEach { store(feedbackHistoryDirectory.resolve("2026-10"), it) }
}

private fun store(directory: Path, record: FeedbackRecord) {
  directory.createDirectories().resolve("${record.id}.json").writeText(EdictNextJson.encodeToString(FeedbackRecord.serializer(), record))
}
