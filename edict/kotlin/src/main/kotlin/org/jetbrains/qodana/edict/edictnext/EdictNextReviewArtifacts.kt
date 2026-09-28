package org.jetbrains.qodana.edict.edictnext

import java.nio.file.Path
import java.util.UUID
import kotlin.io.path.createDirectories
import kotlin.io.path.writeText

internal object EdictNextReviewArtifacts {
  fun create(
    findings: EdictNextInspectionFindings,
    clusterDirectory: Path,
    candidateInspection: Path,
    inspectedProject: Path,
    privateScratchDirectory: Path,
  ): EdictNextInspectionResultsResponse {
    privateScratchDirectory.createDirectories()
    val attemptDirectory = privateScratchDirectory.resolve("attempt-${UUID.randomUUID()}").also { it.createDirectories() }
    val weakFindingsPath = attemptDirectory.resolve("weak-signal-findings.json")
    val weakReviewOutputPath = attemptDirectory.resolve("weak-signal-review").resolve("summary.md")
    weakReviewOutputPath.parent.createDirectories()

    weakFindingsPath.writeText(
      EdictNextJson.encodeToString(
        EdictNextInspectionFindings.serializer(),
        findings.copy(findings = findings.findings.take(SAMPLE_SIZE)),
      ),
    )

    val clusterPath = clusterDirectory.absoluteString()
    val candidatePath = candidateInspection.absoluteString()
    val projectPath = inspectedProject.absoluteString()
    val weakConfigPath = attemptDirectory.resolve("weak-signal-review.json")
    weakConfigPath.writeText(
      EdictNextJson.encodeToString(
        EdictNextWeakSignalReviewConfig.serializer(),
        EdictNextWeakSignalReviewConfig(
          clusterDirectory = clusterPath,
          candidateInspection = candidatePath,
          candidateDigest = findings.candidateDigest,
          findingsPath = weakFindingsPath.absoluteString(),
          inspectedProject = projectPath,
          privateScratchDirectory = attemptDirectory.absoluteString(),
          outputPath = weakReviewOutputPath.absoluteString(),
        ),
      ),
    )
    return EdictNextInspectionResultsResponse(
      weakSignalReviewConfigPath = weakConfigPath.absoluteString(),
    )
  }

  private fun Path.absoluteString(): String = toAbsolutePath().normalize().toString()

  private const val SAMPLE_SIZE: Int = 20
}
