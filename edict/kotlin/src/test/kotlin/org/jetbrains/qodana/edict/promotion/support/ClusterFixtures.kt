// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.promotion.support

import kotlinx.serialization.encodeToString
import org.jetbrains.qodana.edict.edictnext.EdictNextClusterDirectory
import org.jetbrains.qodana.edict.edictnext.EdictNextClusterManifest
import org.jetbrains.qodana.edict.edictnext.EdictNextClusterStatus
import org.jetbrains.qodana.edict.edictnext.EdictNextCodeExampleMetadata
import org.jetbrains.qodana.edict.edictnext.EdictNextEvaluation
import org.jetbrains.qodana.edict.edictnext.EdictNextExampleDirectory
import org.jetbrains.qodana.edict.edictnext.EdictNextFileRevision
import org.jetbrains.qodana.edict.edictnext.EdictNextJson
import org.jetbrains.qodana.edict.edictnext.EdictNextLanguage
import org.jetbrains.qodana.edict.edictnext.EdictNextLineRange
import org.jetbrains.qodana.edict.edictnext.EdictNextSignal
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalLabel
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalSource
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalStrength
import org.jetbrains.qodana.edict.edictnext.EdictNextStoredCluster
import org.jetbrains.qodana.edict.edictnext.EdictRepository
import org.jetbrains.qodana.edict.edictnext.EdictRepositoryDirectory
import org.jetbrains.qodana.edict.edictnext.InspectionPromotion
import java.nio.file.Path
import kotlin.io.path.createDirectories
import kotlin.io.path.writeBytes
import kotlin.io.path.writeText

/** A cluster as a test stores it. Its processed Signals are all of [signals], as generation leaves them. */
internal data class ClusterFixture(
  val signals: List<EdictNextSignal>,
  val id: String = "rule",
  val status: EdictNextClusterStatus = EdictNextClusterStatus.Generated,
  val promotions: List<InspectionPromotion> = emptyList(),
  /** The generated inspection's text; null when it is missing. */
  val inspection: String? = INSPECTION,
  /** Each example's source is [EXAMPLE_SOURCE]. */
  val examples: List<EdictNextCodeExampleMetadata> = emptyList(),
  val evaluation: EdictNextEvaluation? = null,
)

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
internal fun EdictRepository.store(cluster: ClusterFixture): EdictNextStoredCluster {
  val directory = EdictNextClusterDirectory(paths.clustersDirectory.resolve(cluster.id))
  directory.signalsDirectory.createDirectories()
  directory.manifestPath.writeText(EdictNextJson.encodeToString(EdictNextClusterManifest(
    cluster.id,
    EdictNextLanguage.Kotlin,
    cluster.status,
    promotions = cluster.promotions,
    processedSignalIds = cluster.signals.mapTo(LinkedHashSet(), EdictNextSignal::id),
  )))
  directory.historyPath.writeText(HISTORY)
  cluster.signals.forEach { directory.signalPath(it.id).writeText(EdictNextJson.encodeToString(it)) }
  cluster.examples.forEach { storeExample(directory, it) }
  cluster.evaluation?.let { directory.evaluationPath.writeText(EdictNextJson.encodeToString(it)) }
  paths.inspectionsDirectory.createDirectories()
  cluster.inspection?.let { paths.inspectionPath(cluster.id).writeText(it) }
  return loadCluster(cluster.id)
}

/** Replaces the generated inspection of [clusterId] with [content], as a later generation or a broken write would. */
internal fun EdictRepository.replaceInspection(clusterId: String, content: ByteArray) {
  paths.inspectionPath(clusterId).writeBytes(content)
}

/** Stores a cluster [id] whose manifest cannot be read. */
internal fun EdictRepository.storeUnreadableCluster(id: String) {
  paths.clustersDirectory.resolve(id).createDirectories().resolve("cluster.json").writeText("not JSON")
}

private fun storeExample(cluster: EdictNextClusterDirectory, metadata: EdictNextCodeExampleMetadata) {
  val example = EdictNextExampleDirectory(cluster.examplesDirectory.resolve(metadata.id))
  example.projectDirectory.createDirectories()
  example.metadataPath.writeText(EdictNextJson.encodeToString(metadata))
  example.sourcePath(metadata.fileName).writeText(EXAMPLE_SOURCE)
}

/** The id of the fixture Signal numbered [index]; two digits, so ids sort as their numbers do. */
internal fun signalId(index: Int): String = "signal-%02d".format(index)

/** A strong positive Signal numbered [index] in `Example<file>.kt`, from a PR whose discussion it links to. */
internal fun referencedSignal(index: Int, file: Int = index): EdictNextSignal = EdictNextSignal(
  id = signalId(index),
  fileRevision = EdictNextFileRevision("Example$file.kt", SIGNAL_REVISION),
  source = EdictNextSignalSource.FromPR(
    index + 1, "Correction $index", listOf("Fix the problem"), "",
    "https://github.com/source/project/pull/${index + 1}#discussion_r${index + 1}",
  ),
  label = EdictNextSignalLabel.POSITIVE,
  description = "positive $index",
)

/** A generated cluster [id] with one referenced strong positive Signal in each of [files] source files. */
internal fun referencedCluster(
  files: Int = MIN_FILES,
  promotions: List<InspectionPromotion> = emptyList(),
  id: String = "rule",
): ClusterFixture = ClusterFixture((0 until files).map(::referencedSignal), id, promotions = promotions)

/** [referencedCluster] whose last Signal comes from [source]. */
internal fun clusterWithSource(source: EdictNextSignalSource): ClusterFixture =
  referencedCluster().let { cluster -> cluster.copy(signals = cluster.signals.dropLast(1) + cluster.signals.last().copy(source = source)) }

/** A generated cluster with strong positive Signals in six source files, none of which links to a PR or commit diff. */
internal fun unreferencedCluster(): ClusterFixture =
  ClusterFixture(List(6) { referencedSignal(it).copy(source = EdictNextSignalSource.FromCommit(COMMIT_REVISION)) })

/** Eight Signals of one source file, each at another revision, alternately positive and negative. */
internal fun oneFileAtEightRevisions(): ClusterFixture = ClusterFixture(List(8) { index ->
  referencedSignal(index, file = 0).let {
    it.copy(
      fileRevision = it.fileRevision.copy(revision = "$index".repeat(40)),
      label = if (index % 2 == 0) EdictNextSignalLabel.POSITIVE else EdictNextSignalLabel.NEGATIVE,
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
    referencedSignal(11, file = 4).copy(label = EdictNextSignalLabel.NEGATIVE),
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
  commitSignal(4, "SpinningWorker.kt", EdictNextSignalLabel.POSITIVE, "Avoid spinning until completion"),
  commitSignal(5, "Worker.kt", EdictNextSignalLabel.NEGATIVE, "Use the completion latch"),
  referencedSignal(6).copy(source = EdictNextSignalSource.Generated(), description = "Unreferenced generated evidence"),
  weakSignal(7, file = 7).copy(
    source = EdictNextSignalSource.FromPR(99, "Weak evidence", emptyList(), "", "https://github.com/source/project/pull/99"),
  ),
))

/** [referencedCluster] whose first Signal and second PR title use Markdown syntax. */
internal fun markdownCluster(): ClusterFixture = referencedCluster().let { cluster ->
  val second = cluster.signals[1]
  cluster.copy(signals = listOf(
    cluster.signals[0].copy(description = "Do not use [unsafe](https://wrong.test) <markup> or `ticks`\nacross lines"),
    second.copy(source = (second.source as EdictNextSignalSource.FromPR).copy(title = "Fix *bold* _under_ #1 & co\\")),
  ) + cluster.signals.drop(2))
}

/**
 * Ten Signals in five source files, two per file; the first file's first Signal is unreferenced and the second
 * file's first is negative, so their second Signals represent them.
 */
internal fun sharedFileCluster(): ClusterFixture = ClusterFixture(List(10) { referencedSignal(it, file = it % MIN_FILES) }.map {
  when (it.id) {
    signalId(0) -> it.copy(source = EdictNextSignalSource.Generated())
    signalId(1) -> it.copy(label = EdictNextSignalLabel.NEGATIVE)
    else -> it
  }
})

/** [referencedCluster] whose first Signal names one line and whose second names a range and a line. */
internal fun rangedCluster(): ClusterFixture = referencedCluster().let { cluster ->
  cluster.copy(signals = listOf(
    cluster.signals[0].withRanges(EdictNextLineRange(3, 3)),
    cluster.signals[1].withRanges(EdictNextLineRange(7, 9), EdictNextLineRange(12, 12)),
  ) + cluster.signals.drop(2))
}

/** Clusters inspection promotion skips: too few strong positive source files, or not generated. */
internal enum class IneligibleCluster(val cluster: ClusterFixture) {
  FOUR_SOURCE_FILES(referencedCluster(files = 4)),
  PENDING(referencedCluster().copy(status = EdictNextClusterStatus.Pending)),
}

/** Signal sources that don't link a PR or commit diff a promotion PR can cite. */
internal enum class UnreferencedSource(val source: EdictNextSignalSource) {
  COMMIT_WITHOUT_URL(EdictNextSignalSource.FromCommit(COMMIT_REVISION)),
  SUBMITTED_FEEDBACK(EdictNextSignalSource.SubmittedFeedback(url = "https://example.com/feedback")),
  EMPTY_URL(prSource("")),
  SCRIPT(prSource("javascript:alert(1)")),
  NO_HOST(prSource("https:///missing-host")),
  ANGLE_BRACKETS(prSource("https://host/<path>")),
  USER_INFO(prSource("https://user:password@host/path")),
}

/** Strong positive source files an inspection PR needs. */
private const val MIN_FILES = 5

private fun prSource(url: String) = EdictNextSignalSource.FromPR(1, "Correction", listOf("Fix the problem"), "", url)

private fun weakSignal(index: Int, file: Int): EdictNextSignal =
  referencedSignal(index, file).copy(description = "weak $index", strength = EdictNextSignalStrength.WEAK)

private fun commitSignal(index: Int, path: String, label: EdictNextSignalLabel, description: String) = EdictNextSignal(
  id = signalId(index),
  fileRevision = EdictNextFileRevision(path, COMMIT_REVISION, listOf(EdictNextLineRange(12, 15))),
  source = EdictNextSignalSource.FromCommit(COMMIT_REVISION, "https://github.com/source/project/commit/$COMMIT_REVISION"),
  label = label,
  description = description,
)

private fun EdictNextSignal.withRanges(vararg ranges: EdictNextLineRange) =
  copy(fileRevision = fileRevision.copy(expectedRanges = ranges.toList()))
