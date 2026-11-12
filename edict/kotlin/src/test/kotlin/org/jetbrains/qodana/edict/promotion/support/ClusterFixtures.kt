// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.promotion.support

import kotlinx.serialization.encodeToString
import org.jetbrains.qodana.edict.core.EdictClusterDirectory
import org.jetbrains.qodana.edict.core.EdictClusterManifest
import org.jetbrains.qodana.edict.core.EdictClusterStatus
import org.jetbrains.qodana.edict.core.EdictCodeExampleMetadata
import org.jetbrains.qodana.edict.core.EdictEvaluation
import org.jetbrains.qodana.edict.core.EdictExampleDirectory
import org.jetbrains.qodana.edict.core.EdictExampleEvaluation
import org.jetbrains.qodana.edict.core.EdictFileRevision
import org.jetbrains.qodana.edict.core.EdictInspectionAction
import org.jetbrains.qodana.edict.core.EdictJson
import org.jetbrains.qodana.edict.core.EdictLanguage
import org.jetbrains.qodana.edict.core.EdictLineRange
import org.jetbrains.qodana.edict.core.EdictProjectReview
import org.jetbrains.qodana.edict.core.EdictRepository
import org.jetbrains.qodana.edict.core.EdictRepositoryDirectory
import org.jetbrains.qodana.edict.core.EdictSignal
import org.jetbrains.qodana.edict.core.EdictSignalLabel
import org.jetbrains.qodana.edict.core.EdictSignalSource
import org.jetbrains.qodana.edict.core.EdictSignalStrength
import org.jetbrains.qodana.edict.core.EdictStoredCluster
import org.jetbrains.qodana.edict.core.InspectionPromotion
import org.jetbrains.qodana.edict.core.exampleSetDigest
import org.jetbrains.qodana.edict.core.sha256Hex
import java.nio.file.Path
import kotlin.io.path.createDirectories
import kotlin.io.path.writeBytes
import kotlin.io.path.writeText

/** A cluster as a test stores it. Its processed Signals are all of [signals], as generation leaves them. */
internal data class ClusterFixture(
  val signals: List<EdictSignal>,
  val id: String = "rule",
  val status: EdictClusterStatus = EdictClusterStatus.Generated,
  val promotions: List<InspectionPromotion> = emptyList(),
  /** The generated inspection's text; null when it is missing. */
  val inspection: String? = INSPECTION,
  /** Each example's source is [EXAMPLE_SOURCE]. */
  val examples: List<EdictCodeExampleMetadata> = emptyList(),
  /** Null when the inspection was never evaluated. */
  val evaluation: EvaluationFixture? = EvaluationFixture(),
  /** Ids of the Signals that no generation took into account yet; every other Signal was. */
  val unprocessed: Set<String> = emptySet(),
)

/**
 * An evaluation of the stored cluster that satisfies the example of each of its Signals but the [missed] ones. It is
 * current, of the stored inspection and examples, unless [inspectionDigest] or [exampleSetDigest] name others.
 */
internal data class EvaluationFixture(
  val precision: Double = 1.0,
  val recall: Double = 1.0,
  /** Ids of the Signals whose examples the inspection misses. */
  val missed: Set<String> = emptySet(),
  val projectReview: EdictProjectReview? = null,
  val inspectionDigest: String? = null,
  val exampleSetDigest: String? = null,
)

/** The name every fixture evaluation gives the inspection. */
internal const val INSPECTION_NAME = "Worker thread blocking"

/** The description every fixture evaluation gives the inspection. */
internal const val INSPECTION_SUMMARY = "Reports blocking calls on the worker thread."

/** The history every stored cluster starts with. */
internal const val HISTORY = "Created.\n"

/** The source of every stored code example. */
internal const val EXAMPLE_SOURCE = "fun example() {}\n"

/** The full revision of the source files the fixture Signals observed. */
internal const val SIGNAL_REVISION = "c0ffee00c0ffee00c0ffee00c0ffee00c0ffee00"

/** The full revision of the commit the commit-sourced fixture Signals come from. */
internal const val COMMIT_REVISION = "d0d0cafed0d0cafed0d0cafed0d0cafed0d0cafe"

/** An empty repository state in [directory]. */
internal fun promotionRepository(directory: Path): EdictRepository =
  EdictRepository(EdictRepositoryDirectory(directory.resolve("state").createDirectories()))

/** Writes [cluster] and returns it as the repository loads it. */
internal fun EdictRepository.store(cluster: ClusterFixture): EdictStoredCluster {
  val directory = EdictClusterDirectory(paths.clustersDirectory.resolve(cluster.id))
  directory.signalsDirectory.createDirectories()
  directory.manifestPath.writeText(EdictJson.encodeToString(EdictClusterManifest(
    cluster.id,
    EdictLanguage.Kotlin,
    cluster.status,
    promotions = cluster.promotions,
    processedSignalIds = cluster.signals.map(EdictSignal::id).filterTo(LinkedHashSet()) { it !in cluster.unprocessed },
  )))
  directory.historyPath.writeText(HISTORY)
  cluster.signals.forEach { directory.signalPath(it.id).writeText(EdictJson.encodeToString(it)) }
  cluster.examples.forEach { storeExample(directory, it) }
  paths.inspectionsDirectory.createDirectories()
  cluster.inspection?.let { paths.inspectionPath(cluster.id).writeText(it) }
  cluster.evaluation?.let { directory.evaluationPath.writeText(EdictJson.encodeToString(it.of(loadCluster(cluster.id), cluster.inspection ?: INSPECTION))) }
  return loadCluster(cluster.id)
}

private fun EvaluationFixture.of(cluster: EdictStoredCluster, inspection: String): EdictEvaluation = EdictEvaluation(
  inspectionHash = inspectionDigest ?: sha256Hex(inspection),
  exampleSetDigest = exampleSetDigest ?: exampleSetDigest(cluster),
  action = EdictInspectionAction.GENERATE,
  tp = cluster.signals.size - missed.size,
  fp = 0,
  fn = missed.size,
  precision = precision,
  recall = recall,
  examples = cluster.signals.filter { it.syntheticExampleId != null }.associate {
    it.syntheticExampleId!! to EdictExampleEvaluation(it.label, it.strength == EdictSignalStrength.STRONG, it.id !in missed, 0, 1)
  },
  projectReview = projectReview,
  inspectionName = INSPECTION_NAME,
  inspectionDescription = INSPECTION_SUMMARY,
)

/** Replaces the generated inspection of [clusterId] with [content], as a later generation or a broken write would. */
internal fun EdictRepository.replaceInspection(clusterId: String, content: ByteArray) {
  paths.inspectionPath(clusterId).writeBytes(content)
}

/** Stores a cluster [id] whose manifest cannot be read. */
internal fun EdictRepository.storeUnreadableCluster(id: String) {
  paths.clustersDirectory.resolve(id).createDirectories().resolve("cluster.json").writeText("not JSON")
}

private fun storeExample(cluster: EdictClusterDirectory, metadata: EdictCodeExampleMetadata) {
  val example = EdictExampleDirectory(cluster.examplesDirectory.resolve(metadata.id))
  example.projectDirectory.createDirectories()
  example.metadataPath.writeText(EdictJson.encodeToString(metadata))
  example.sourcePath(metadata.fileName).writeText(EXAMPLE_SOURCE)
}

/** The id of the fixture Signal numbered [index]; two digits, so ids sort as their numbers do. */
internal fun signalId(index: Int): String = "signal-%02d".format(index)

/**
 * A strong positive Signal numbered [index] on lines 3 to 5 of `Example<file>.kt`, from a PR whose discussion it links
 * to; its example is `example-<index>`.
 */
internal fun referencedSignal(index: Int, file: Int = index): EdictSignal = EdictSignal(
  id = signalId(index),
  fileRevision = EdictFileRevision("Example$file.kt", SIGNAL_REVISION, listOf(EdictLineRange(3, 5))),
  source = EdictSignalSource.FromPR(
    index + 1, "Correction $index", listOf("Fix the problem"), "",
    "https://github.com/source/project/pull/${index + 1}#discussion_r${index + 1}",
  ),
  label = EdictSignalLabel.POSITIVE,
  description = "positive $index",
  syntheticExampleId = "example-$index",
)

/** A generated cluster [id] with one referenced strong positive Signal in each of [files] source files. */
internal fun referencedCluster(
  files: Int = MIN_FILES,
  promotions: List<InspectionPromotion> = emptyList(),
  id: String = "rule",
): ClusterFixture = ClusterFixture((0 until files).map(::referencedSignal), id, promotions = promotions)

/** The cluster with every Signal from [source]. */
internal fun ClusterFixture.withSources(source: EdictSignalSource): ClusterFixture = copy(signals = signals.map { it.copy(source = source) })

/** [referencedCluster] whose last Signal comes from [source]. */
internal fun clusterWithSource(source: EdictSignalSource): ClusterFixture =
  referencedCluster().let { cluster -> cluster.copy(signals = cluster.signals.dropLast(1) + cluster.signals.last().copy(source = source)) }

/** A generated cluster with referenced strong positive Signals in six source files, whose examples the inspection misses. */
internal fun missedCluster(): ClusterFixture = ClusterFixture(List(6, ::referencedSignal)).let {
  it.copy(evaluation = EvaluationFixture(missed = it.signals.mapTo(HashSet(), EdictSignal::id)))
}

/** Eight Signals of one source file, each at another revision, alternately positive and negative. */
internal fun oneFileAtEightRevisions(): ClusterFixture = ClusterFixture(List(8) { index ->
  referencedSignal(index, file = 0).let {
    it.copy(
      fileRevision = it.fileRevision.copy(revision = "$index".repeat(40)),
      label = if (index % 2 == 0) EdictSignalLabel.POSITIVE else EdictSignalLabel.NEGATIVE,
    )
  }
})

/** Five Signals of `Worker.kt` files in five directories. */
internal fun sameNameInFiveDirectories(): ClusterFixture = ClusterFixture(List(MIN_FILES) { index ->
  referencedSignal(index).let { it.copy(fileRevision = it.fileRevision.copy(path = "src/package$index/Worker.kt")) }
})

/** Ten strong positive Signals spread over four source files, and a weak and a strong negative one in a fifth. */
internal fun fourFilesWithWeakAndNegativeSignals(): ClusterFixture = ClusterFixture(
  List(10) { referencedSignal(it, file = it % 4) } + weakSignal(10, file = 4) +
    referencedSignal(11, file = 4).copy(label = EdictSignalLabel.NEGATIVE),
)

/**
 * A cluster whose referenced strong positive Signals are `signal-00` to `signal-04`; `signal-04` comes from a commit
 * and names its lines. A negative, an unreferenced and a weak Signal accompany them.
 */
internal fun describedCluster(): ClusterFixture = ClusterFixture(listOf(
  referencedSignal(0).copy(description = "Avoid blocking the worker thread"),
  referencedSignal(1),
  referencedSignal(2),
  referencedSignal(3),
  commitSignal(4, "SpinningWorker.kt", EdictSignalLabel.POSITIVE, "Avoid spinning until completion"),
  commitSignal(5, "Worker.kt", EdictSignalLabel.NEGATIVE, "Use the completion latch"),
  referencedSignal(6).copy(source = EdictSignalSource.Generated(), description = "Unreferenced generated evidence"),
  weakSignal(7, file = 7).copy(
    source = EdictSignalSource.FromPR(99, "Weak evidence", emptyList(), "", "https://github.com/source/project/pull/99"),
  ),
))

/** [referencedCluster] whose first Signal's description uses Markdown syntax over two lines. */
internal fun markdownCluster(): ClusterFixture = referencedCluster().let { cluster ->
  cluster.copy(signals = listOf(
    cluster.signals[0].copy(description = "Do not use [unsafe](https://wrong.test) <markup> or `ticks` & *bold* _under_ #1\\\nacross lines"),
  ) + cluster.signals.drop(1))
}

/**
 * Ten Signals in five source files, two per file; the first file's first Signal is unreferenced and the second
 * file's first is negative, so their second Signals represent them.
 */
internal fun sharedFileCluster(): ClusterFixture = ClusterFixture(List(10) { referencedSignal(it, file = it % MIN_FILES) }.map {
  when (it.id) {
    signalId(0) -> it.copy(source = EdictSignalSource.Generated())
    signalId(1) -> it.copy(label = EdictSignalLabel.NEGATIVE)
    else -> it
  }
})

/** Clusters inspection promotion does not select: too few strong positive source files, or not generated. */
internal enum class IneligibleCluster(val cluster: ClusterFixture) {
  FOUR_SOURCE_FILES(referencedCluster(files = 4)),
  PENDING(referencedCluster().copy(status = EdictClusterStatus.Pending)),
}

/** Signal sources that don't link a PR or commit diff a promotion PR can cite. */
internal enum class UnreferencedSource(val source: EdictSignalSource) {
  SUBMITTED_FEEDBACK(EdictSignalSource.SubmittedFeedback(url = "https://example.com/feedback")),
  EMPTY_URL(prSource("")),
  SCRIPT(prSource("javascript:alert(1)")),
  NO_HOST(prSource("https:///missing-host")),
  ANGLE_BRACKETS(prSource("https://host/<path>")),
  USER_INFO(prSource("https://user:password@host/path")),
}

/** Strong positive source files an inspection PR needs. */
private const val MIN_FILES = 5

private fun prSource(url: String) = EdictSignalSource.FromPR(1, "Correction", listOf("Fix the problem"), "", url)

private fun weakSignal(index: Int, file: Int): EdictSignal =
  referencedSignal(index, file).copy(description = "weak $index", strength = EdictSignalStrength.WEAK)

private fun commitSignal(index: Int, path: String, label: EdictSignalLabel, description: String) = EdictSignal(
  id = signalId(index),
  fileRevision = EdictFileRevision(path, COMMIT_REVISION, listOf(EdictLineRange(12, 15))),
  source = EdictSignalSource.FromCommit(COMMIT_REVISION, "https://github.com/source/project/commit/$COMMIT_REVISION"),
  label = label,
  description = description,
  syntheticExampleId = "example-$index",
)
