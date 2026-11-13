// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

import io.github.oshai.kotlinlogging.KotlinLogging
import kotlinx.serialization.Serializable
import kotlinx.serialization.builtins.ListSerializer
import org.jetbrains.qodana.edict.core.EdictClusterStatus
import org.jetbrains.qodana.edict.core.EdictJson
import org.jetbrains.qodana.edict.core.EdictRepository
import org.jetbrains.qodana.edict.core.EdictRepositoryDirectory
import org.jetbrains.qodana.edict.core.EdictSignalLabel
import org.jetbrains.qodana.edict.core.EdictStoredCluster
import org.jetbrains.qodana.edict.core.EvaluatedInspection
import org.jetbrains.qodana.edict.core.StoredInspection
import org.jetbrains.qodana.edict.core.storedInspection
import org.jetbrains.qodana.edict.signals.validDigest
import org.jetbrains.qodana.edict.signals.validSourcePath
import kotlin.io.path.createDirectories
import kotlin.io.path.isRegularFile
import kotlin.io.path.readText
import kotlin.io.path.writeText

private val logger = KotlinLogging.logger {}

/** The 👎 answers to one inspection's findings that no generation took into account yet, at most, for it to get more. */
internal const val MAX_UNPROCESSED_FALSE_POSITIVES = 5

/** An inspection whose findings may become review comments: the generated inspection of cluster [clusterId]. */
@Serializable
internal data class FeedbackInspection(
    val clusterId: String,
    /** The SHA-256 hex of the inspection's bytes. */
    val digest: String,
    /** The path of the inspection's file, relative to the state directory. */
    val path: String,
    val inspectionName: String,
    val inspectionDescription: String,
    /** The precision the current evaluation of the inspection's bytes gives it. */
    val precision: Double,
) {
    init {
        require(validDigest(digest)) { "Feedback inspection $clusterId has an invalid digest: $digest" }
        require(validSourcePath(path)) { "Feedback inspection $clusterId has an invalid path: $path" }
    }
}

/** The inspections of the state in [paths] whose findings may become review comments, the most precise first. */
internal class FeedbackInspections(private val paths: EdictRepositoryDirectory) {
    /** The stored inspections; none before the state was first pushed with them. */
    fun load(): List<FeedbackInspection> {
        val file = paths.feedbackInspectionsPath
        return if (file.isRegularFile()) EdictJson.decodeFromString(SERIALIZER, file.readText()) else emptyList()
    }

    /** Stores [inspections], replacing those stored before. */
    fun save(inspections: List<FeedbackInspection>) {
        paths.feedbackInspectionsPath.parent.createDirectories()
        paths.feedbackInspectionsPath.writeText(EdictJson.encodeToString(SERIALIZER, inspections))
    }
}

/**
 * The generated inspections of [repository], in any cluster state, that have an evaluation of their
 * bytes on their cluster's current examples with at least [targetPrecision], that no promotion proposed, and whose findings
 * got at most [MAX_UNPROCESSED_FALSE_POSITIVES] 👎 answers that no generation took into account yet.
 *
 * The state push saves the day's work whatever this chooses, so broken state is logged, not thrown: a cluster that does not
 * load, or whose inspection or evaluation does not, is left out; an inbox that does not load leaves out every inspection,
 * because the 👎 answers waiting there are unknown.
 */
internal fun chooseFeedbackInspections(repository: EdictRepository, targetPrecision: Double): List<FeedbackInspection> {
    val inbox = runCatching { repository.loadInboxSignals() }.getOrElse { failure ->
        logger.error(failure) { "Edict feedback offers no inspections: the inbox does not load" }
        return emptyList()
    }
    val clusters = repository.loadEachCluster().mapNotNull { (id, cluster) -> cluster.orLeftOut(id, "it does not load") }
    val unprocessed = inbox + clusters.flatMap(EdictStoredCluster::unprocessedSignals)
    return clusters
        .asSequence()
        .mapNotNull { cluster ->
            runCatching { repository.getInspectionForFeedbackIfSuitable(cluster) }
                .orLeftOut(cluster.id, "its inspection or evaluation does not load")
        }
        .filter { it.evaluation.precision >= targetPrecision }
        .filter { inspection ->
            // Do not show inspection that has too many FPs reported
            unprocessed.count { it.isAnswer(EdictSignalLabel.NEGATIVE, inspection.digest) } <= MAX_UNPROCESSED_FALSE_POSITIVES
        }
        .map { inspection ->
            FeedbackInspection(
                clusterId = inspection.clusterId,
                digest = inspection.digest,
                path = repository.paths.root.relativize(inspection.path).joinToString("/"),
                inspectionName = inspection.evaluation.inspectionName.ifBlank { inspection.clusterId },
                inspectionDescription = inspection.evaluation.inspectionDescription,
                precision = inspection.evaluation.precision,
            )
        }
        .toList()
}

private val SERIALIZER = ListSerializer(FeedbackInspection.serializer())

/** The value, or null after logging why cluster [clusterId] is left out of feedback. */
private fun <T> Result<T>.orLeftOut(clusterId: String, reason: String): T? = getOrElse { failure ->
    logger.error(failure) { "Edict feedback leaves out $clusterId: $reason" }
    null
}

/**
 * The cluster's generated inspection, unless no evaluation of its current bytes and examples ranks it or a promotion proposed
 * these bytes. A cluster without its inspection only loses its feedback, not that of the others.
 */
private fun EdictRepository.getInspectionForFeedbackIfSuitable(cluster: EdictStoredCluster): EvaluatedInspection? =
    when (val inspection = storedInspection(cluster, loadEvaluation(cluster))) {
        StoredInspection.Missing -> {
            // Only a Generated cluster must have one: the others may never have been generated, or were discontinued.
            if (cluster.manifest.status == EdictClusterStatus.Generated) {
                logger.warn { "Edict feedback leaves out ${cluster.id}: its generated inspection is missing" }
            }
            null
        }
        StoredInspection.NotEvaluated -> null
        is EvaluatedInspection -> inspection.takeUnless { cluster.isPromoted(it.digest) }
    }

/**
 * Whether a promotion proposed exactly the inspection bytes of [digest], whatever its state: it awaits its review, was accepted,
 * or was closed and awaits its decision. Only deciding a closed one regenerates the inspection, so the promotions of earlier
 * bytes are decided ones, and the new bytes may get feedback.
 */
private fun EdictStoredCluster.isPromoted(digest: String): Boolean = manifest.promotions.any { it.inspectionDigest == digest }
