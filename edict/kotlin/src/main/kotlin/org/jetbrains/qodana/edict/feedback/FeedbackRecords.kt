// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

import kotlinx.serialization.Serializable
import org.jetbrains.qodana.edict.ci.ReviewReference
import org.jetbrains.qodana.edict.edictnext.EdictNextJson
import org.jetbrains.qodana.edict.edictnext.EdictNextLineRange
import org.jetbrains.qodana.edict.edictnext.EdictRepositoryDirectory
import java.nio.file.Path
import java.time.Instant
import java.time.YearMonth
import java.time.ZoneOffset
import kotlin.io.path.createDirectories
import kotlin.io.path.deleteExisting
import kotlin.io.path.isDirectory
import kotlin.io.path.listDirectoryEntries
import kotlin.io.path.nameWithoutExtension
import kotlin.io.path.readText
import kotlin.io.path.writeText

/** A finding of a not yet promoted inspection, reserved for a review comment on [pullRequest]. */
@Serializable
internal data class FeedbackRecord(
    /** Names the record's file; the review comment carries it. */
    val id: String,
    /** The cluster whose inspection reported the finding. */
    val clusterId: String,
    val pullRequest: ReviewReference,
    /** The full SHA of the commit the inspection reported the finding on. */
    val revision: String,
    /** The repository-relative path of the file the finding is in. */
    val path: String,
    val lines: EdictNextLineRange,
    /** The line the review comment is on: the first of [lines] that the pull request adds. */
    val commentLine: Int,
    val inspectionName: String,
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

    /** Stores [record] as awaiting an answer. */
    fun addActive(record: FeedbackRecord) {
        paths.activeFeedbackDirectory.createDirectories().resolve(record.fileName).writeText(record.fileText())
    }

    /** Moves the active [record] to the history of the month it was reserved in. */
    fun moveToHistory(record: FeedbackRecord) {
        val month = YearMonth.from(Instant.parse(record.createdAt).atZone(ZoneOffset.UTC))
        paths.feedbackHistoryDirectory.resolve(month.toString()).createDirectories().resolve(record.fileName).writeText(record.fileText())
        paths.activeFeedbackDirectory.resolve(record.fileName).deleteExisting()
    }
}

/** The name of the file that stores the record, in the directory of its state. */
internal val FeedbackRecord.fileName: String get() = "$id.json"

/** The record as its file stores it. */
internal fun FeedbackRecord.fileText(): String = EdictNextJson.encodeToString(FeedbackRecord.serializer(), this)

private fun directories(directory: Path): List<Path> =
    if (directory.isDirectory()) directory.listDirectoryEntries().filter(Path::isDirectory).sorted() else emptyList()

private fun load(directory: Path): List<FeedbackRecord> =
    if (!directory.isDirectory()) emptyList()
    else directory.listDirectoryEntries("*.json").sorted().map { file ->
        EdictNextJson.decodeFromString(FeedbackRecord.serializer(), file.readText()).also {
            require(it.id == file.nameWithoutExtension) { "Feedback record $file holds record ${it.id}" }
        }
    }
