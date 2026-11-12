package org.jetbrains.qodana.edict.core

import java.nio.file.Path
import java.util.UUID
import kotlin.random.Random
import kotlin.io.path.createDirectories
import kotlin.io.path.writeText

internal object EdictReviewArtifacts {
  fun create(
    findings: EdictInspectionFindings,
    clusterDirectory: Path,
    candidateInspection: Path,
    inspectedProject: Path,
    privateScratchDirectory: Path,
    remainingProjectAnalyses: Int,
    reviewedLocations: Set<Pair<String, Int>> = emptySet(),
    exampleStatus: Map<String, EdictReviewExampleStatus> = emptyMap(),
    unreproducedFindings: Map<String, EdictUnreproducedFindings> = emptyMap(),
  ): EdictInspectionResultsResponse {
    privateScratchDirectory.createDirectories()
    val attemptDirectory = privateScratchDirectory.resolve("attempt-${UUID.randomUUID()}").also { it.createDirectories() }
    val weakFindingsPath = attemptDirectory.resolve("weak-signal-findings.json")
    val weakReviewOutputPath = attemptDirectory.resolve("weak-signal-review").resolve("summary.md")
    weakReviewOutputPath.parent.createDirectories()

    val sample = sample(findings, reviewedLocations)
    weakFindingsPath.writeText(EdictJson.encodeToString(EdictInspectionFindings.serializer(), findings.copy(findings = sample)))

    val clusterPath = clusterDirectory.absoluteString()
    val candidatePath = candidateInspection.absoluteString()
    val projectPath = inspectedProject.absoluteString()
    val weakConfigPath = attemptDirectory.resolve("weak-signal-review.json")
    weakConfigPath.writeText(
      EdictJson.encodeToString(
        EdictWeakSignalReviewConfig.serializer(),
        EdictWeakSignalReviewConfig(
          clusterDirectory = clusterPath,
          candidateInspection = candidatePath,
          candidateDigest = findings.candidateDigest,
          findingsPath = weakFindingsPath.absoluteString(),
          inspectedProject = projectPath,
          privateScratchDirectory = attemptDirectory.absoluteString(),
          outputPath = weakReviewOutputPath.absoluteString(),
          exampleStatus = exampleStatus,
          unreproducedFindings = unreproducedFindings,
        ),
      ),
    )
    return EdictInspectionResultsResponse(
      weakSignalReviewConfigPath = weakConfigPath.absoluteString(),
      remainingProjectAnalyses = remainingProjectAnalyses,
      weakReviewNeeded = sample.isNotEmpty() || unreproducedFindings.isNotEmpty(),
    )
  }

  /** Up to [SAMPLE_SIZE] findings not reviewed in this run yet, so none is reviewed twice. */
  private fun sample(findings: EdictInspectionFindings, reviewedLocations: Set<Pair<String, Int>>): List<EdictProjectFinding> =
    findings.findings.filter { it.location() !in reviewedLocations }
      .shuffled(Random(findings.candidateDigest.hashCode()))
      .take(SAMPLE_SIZE)
      .sortedWith(compareBy({ it.fileRevision.path }, { it.location().second }))

  private fun Path.absoluteString(): String = toAbsolutePath().normalize().toString()

  private const val SAMPLE_SIZE: Int = 20
}
