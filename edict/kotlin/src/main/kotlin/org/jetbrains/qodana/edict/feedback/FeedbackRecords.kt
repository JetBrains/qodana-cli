// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

import kotlinx.serialization.Serializable
import org.jetbrains.qodana.edict.ci.ReviewReference
import org.jetbrains.qodana.edict.edictnext.EdictNextJson
import org.jetbrains.qodana.edict.edictnext.EdictNextLineRange
import org.jetbrains.qodana.edict.edictnext.EdictRepositoryDirectory
import java.nio.file.Path
import kotlin.io.path.isDirectory
import kotlin.io.path.listDirectoryEntries
import kotlin.io.path.nameWithoutExtension
import kotlin.io.path.readText

/**
 * A finding of a not yet promoted inspection, reserved for a review comment on [pullRequest]; the comment's answer becomes a
 * Signal of the cluster [clusterId].
 */
@Serializable
internal data class FeedbackRecord(
    /** Names the record's file; the review comment carries it. */
    val id: String,
    val clusterId: String,
    val pullRequest: ReviewReference,
    /** The full SHA of the commit the inspection reported the finding on. */
    val revision: String,
    /** The repository-relative path of the file the finding is in. */
    val path: String,
    val lines: EdictNextLineRange,
    val inspectionDescription: String,
    /** The problem the inspection reported, as its message says. */
    val message: String,
    /** When the finding was reserved, as an ISO-8601 instant. */
    val createdAt: String,
)

/** The feedback records of the state in [paths]. */
internal class FeedbackRecords(private val paths: EdictRepositoryDirectory) {
    /** The records that await an answer. */
    fun active(): List<FeedbackRecord> = load(paths.activeFeedbackDirectory)

    /** The records that left [active], of every month. */
    fun history(): List<FeedbackRecord> = directories(paths.feedbackHistoryDirectory).flatMap(::load)
}

private fun directories(directory: Path): List<Path> =
    if (directory.isDirectory()) directory.listDirectoryEntries().filter(Path::isDirectory).sorted() else emptyList()

private fun load(directory: Path): List<FeedbackRecord> =
    if (!directory.isDirectory()) emptyList()
    else directory.listDirectoryEntries("*.json").sorted().map { file ->
        EdictNextJson.decodeFromString(FeedbackRecord.serializer(), file.readText()).also {
            require(it.id == file.nameWithoutExtension) { "Feedback record $file holds record ${it.id}" }
        }
    }
