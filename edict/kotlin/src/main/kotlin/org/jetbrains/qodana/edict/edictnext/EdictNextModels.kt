@file:OptIn(kotlinx.serialization.ExperimentalSerializationApi::class)

package com.intellij.ml.llm.qodana.agents.edictnext

import kotlinx.serialization.EncodeDefault
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import java.security.MessageDigest

internal val EdictNextJson: Json = Json {
  ignoreUnknownKeys = true
  prettyPrint = true
}

@Serializable
internal enum class EdictNextLanguage(val fileExtension: String) {
  Kotlin("kt"),
  Java("java"),

  ;

  companion object {
    fun fromPathOrNull(path: String): EdictNextLanguage? = when (path.substringAfterLast('.', missingDelimiterValue = "").lowercase()) {
      Kotlin.fileExtension -> Kotlin
      Java.fileExtension -> Java
      else -> null
    }
  }
}

@Serializable
internal enum class EdictNextSignalLabel {
  POSITIVE,
  NEGATIVE,
}

@Serializable
internal enum class EdictNextSignalStrength {
  STRONG,
  @Suppress("unused")
  WEAK,
}

@Serializable
internal sealed interface EdictNextSignalSource {
  @Serializable
  @SerialName("FromPR")
  data class FromPR(
    val prNumber: Int,
    val title: String,
    val discussionMessages: List<String>,
    val diffPositiveToNegative: String,
  ) : EdictNextSignalSource

  @Serializable
  @SerialName("FromCommit")
  data class FromCommit(
    val commitRevision: String,
    val message: String,
  ) : EdictNextSignalSource

  @Serializable
  @SerialName("SubmittedFeedback")
  @Suppress("unused")
  data class SubmittedFeedback(
    val inspectionName: String? = null,
    val inspectionDescription: String? = null,
    val problemMessage: String? = null,
    val codeSnippet: String? = null,
    val reason: String? = null,
    val suggestionId: String? = null,
  ) : EdictNextSignalSource

  @Serializable
  @SerialName("Generated")
  data class Generated(val resultMessage: String? = null) : EdictNextSignalSource
}

@Serializable
internal data class EdictNextLineRange(
  val start: Int,
  val end: Int,
) {
  fun intersects(other: EdictNextLineRange): Boolean = start <= other.end && other.start <= end
}

@Serializable
internal data class EdictNextFileRevision(
  val path: String,
  val revision: String,
  val expectedRanges: List<EdictNextLineRange>? = null,
)

@Serializable
internal data class EdictNextSignal(
  val id: String,
  val fileRevision: EdictNextFileRevision,
  val source: EdictNextSignalSource,
  val label: EdictNextSignalLabel,
  val description: String,
  val strength: EdictNextSignalStrength = EdictNextSignalStrength.STRONG,
  val syntheticExampleId: String? = null,
) {
  val isJvmLanguage: Boolean get() = EdictNextLanguage.fromPathOrNull(fileRevision.path) != null
  val language: EdictNextLanguage
    get() = requireNotNull(EdictNextLanguage.fromPathOrNull(fileRevision.path)) {
      "Unsupported Signal source language: ${fileRevision.path}"
    }
}

@Serializable
internal data class EdictNextCodeExampleMetadata(
  val id: String,
  val fileName: String,
  val label: EdictNextSignalLabel,
  val expectedRanges: List<EdictNextLineRange>? = null,
)

@Serializable
internal enum class EdictNextClusterStatus {
  Pending,
  /** Pipeline processing cannot continue because infrastructure or the cluster input/state is broken. */
  Invalid,
  Generated,
  /** The cluster Signals are semantically incompatible and do not express one coherent code-quality rule. */
  Discontinued,
}

@Serializable
internal data class EdictNextClusterManifest(
  val id: String,
  val language: EdictNextLanguage,
  val status: EdictNextClusterStatus,
  val predecessorId: String? = null,
)

// --- Bundled script contract -------------------------------------------------------------------
// A pure function of its request file. The script hands out locality and never applies a distance
// threshold; what belongs to which cluster is the agent's verdict.

@Serializable
internal data class EdictNextCorpusIndexRequest(
  val model: String,
  val modelRevision: String,
  val corpus: List<EdictNextSignal>,
  val signalIds: List<String>,
  val neighbourCount: Int,
)

@Serializable
internal data class EdictNextScriptNeighbour(
  val signalId: String,
  val distance: Double,
)

@Serializable
internal data class EdictNextSignalNeighbours(
  val signalId: String,
  val closest: List<EdictNextScriptNeighbour> = emptyList(),
)

@Serializable
internal data class EdictNextNeighboursResponse(
  val neighbours: List<EdictNextSignalNeighbours> = emptyList(),
)

@Serializable
internal data class EdictNextSignalPreparationResponse(
  val signalId: String? = null,
  val signalPath: String? = null,
  val signal: EdictNextSignal? = null,
  val candidates: List<EdictNextSignalCandidate> = emptyList(),
  val summary: String,
  @EncodeDefault(EncodeDefault.Mode.ALWAYS)
  val nextAction: EdictNextNextAction =
    if (signalId == null) EdictNextNextAction.STOP_DISTRIBUTION else EdictNextNextAction.PROCESS_SIGNAL,
)

@Serializable
internal data class EdictNextSignalValidationResponse(
  val signalId: String,
  val summary: String,
  val added: Boolean,
)

@Serializable
internal sealed class EdictNextDistributionContextResponse {
  @Serializable
  @SerialName("signal")
  data class Signal(
    val signal: EdictNextDistributionSignal,
  ) : EdictNextDistributionContextResponse()

  @Serializable
  @SerialName("cluster")
  data class Cluster(
    val clusterId: String,
    val signals: List<EdictNextDistributionSignal>,
  ) : EdictNextDistributionContextResponse()
}

@Serializable
internal data class EdictNextDistributionSignal(
  val id: String,
  val fileRevision: EdictNextFileRevision,
  val label: EdictNextSignalLabel,
  val description: String,
)

// --- Distribution -------------------------------------------------------------------------------

/** One useful comparison for a Signal: a current cluster, or another inbox Signal. */
@Serializable
internal sealed interface EdictNextSignalCandidate {
  val nearestDistance: Double

  @Serializable
  @SerialName("cluster")
  data class Cluster(
    val clusterId: String,
    override val nearestDistance: Double,
  ) : EdictNextSignalCandidate

  @Serializable
  @SerialName("signal")
  data class Signal(
    val signalId: String,
    val signalPath: String,
    override val nearestDistance: Double,
  ) : EdictNextSignalCandidate
}

@Serializable
internal data class EdictNextValidationIssue(
  val path: String,
  val message: String,
)

@Serializable
internal data class EdictNextValidationResponse(
  val success: Boolean,
  val summary: String,
  val issues: List<EdictNextValidationIssue> = emptyList(),
  @EncodeDefault(EncodeDefault.Mode.ALWAYS)
  val nextAction: EdictNextNextAction =
    if (success) EdictNextNextAction.PUBLISH else EdictNextNextAction.REPAIR_REPOSITORY,
)

@Serializable
internal data class EdictNextPreparePipelineResponse(
  val summary: String,
)

@Serializable
internal data class EdictNextGenerationTarget(
  val clusterId: String,
  val clusterDirectory: String,
)

@Serializable
internal data class EdictNextGenerationClustersResponse(
  val clusters: List<EdictNextGenerationTarget>,
  @EncodeDefault(EncodeDefault.Mode.ALWAYS)
  val maxConcurrentClusterTasks: Int = EDICT_NEXT_MAX_CONCURRENT_CLUSTER_TASKS,
  val summary: String,
)

@Serializable
internal data class EdictNextCodeExampleValidationResponse(
  val success: Boolean,
  val summary: String,
  val issues: List<String> = emptyList(),
  @EncodeDefault(EncodeDefault.Mode.ALWAYS)
  val nextAction: EdictNextNextAction =
    if (success) EdictNextNextAction.CONTINUE_CLUSTER_GENERATION else EdictNextNextAction.REPAIR_CODE_EXAMPLE,
)

@Serializable
internal enum class EdictNextInspectionAction {
  CONFLICT,
  SKIP,
  GENERATE,
}

@Serializable
internal data class EdictNextInspectionActionResponse(
  val action: EdictNextInspectionAction,
  val summary: String,
  val conflictingSignalIds: List<String> = emptyList(),
)

@Serializable
internal data class EdictNextInspectionFailure(
  val exampleId: String,
  val message: String,
)

@Serializable
internal data class EdictNextInspectionValidationResponse(
  val compilationSuccess: Boolean,
  val overallSuccess: Boolean,
  val summary: String,
  val failures: List<EdictNextInspectionFailure> = emptyList(),
  val achievedAccuracyPercent: Int? = null,
  val reportedNegativeExampleIds: List<String> = emptyList(),
  val uncoveredPositiveExampleIds: List<String> = emptyList(),
  @EncodeDefault(EncodeDefault.Mode.ALWAYS)
  val nextAction: EdictNextNextAction = if (overallSuccess) EdictNextNextAction.ANALYZE_PROJECT else EdictNextNextAction.REPAIR_INSPECTION,
)

@Serializable
internal data class EdictNextInspectionResultsResponse(
  val weakSignalReviewConfigPath: String,
)

@Serializable
internal data class EdictNextMarkGeneratedResponse(
  val success: Boolean,
  val summary: String,
  val issues: List<EdictNextValidationIssue> = emptyList(),
)

@Serializable
internal data class EdictNextProjectFinding(
  val fileRevision: EdictNextFileRevision,
  val message: String? = null,
  val contextSnippet: String? = null,
  val contextStartLine: Int? = null,
)

@Serializable
internal data class EdictNextInspectionFindings(
  val clusterId: String,
  val candidateDigest: String,
  val projectRevision: String,
  val inspectionDescription: String,
  val language: EdictNextLanguage,
  val findings: List<EdictNextProjectFinding>,
)

@Serializable
internal data class EdictNextWeakSignalReviewConfig(
  val clusterDirectory: String,
  val candidateInspection: String,
  val candidateDigest: String,
  val findingsPath: String,
  val inspectedProject: String,
  val privateScratchDirectory: String,
  val outputPath: String,
)

@Serializable
internal enum class EdictNextNextAction {
  PROCESS_SIGNAL,
  STOP_DISTRIBUTION,
  REPAIR_CODE_EXAMPLE,
  CONTINUE_CLUSTER_GENERATION,
  REPAIR_INSPECTION,
  ANALYZE_PROJECT,
  REPAIR_REPOSITORY,
  PUBLISH,
}

internal fun sha256Hex(text: String): String = sha256Hex(text.toByteArray(Charsets.UTF_8))

internal fun sha256Hex(bytes: ByteArray): String = MessageDigest.getInstance("SHA-256")
  .digest(bytes)
  .joinToString("") { byte -> "%02x".format(byte) }

internal const val EDICT_NEXT_MODEL: String = "thenlper/gte-large"
internal const val EDICT_NEXT_MODEL_REVISION: String = "4bef63f39fcc5e2d6b0aae83089f307af4970164"
internal const val EDICT_NEXT_MAX_CONCURRENT_CLUSTER_TASKS: Int = 20
internal const val EDICT_NEXT_INSPECTION_SUFFIX: String = ".inspection.kts"
internal const val EDICT_NEXT_CANDIDATE_SUFFIX: String = ".candidate.kts"

/** Cluster ids and rule ids share one shape. */
internal val EDICT_NEXT_KEBAB_CASE: Regex = Regex("[a-z][a-z0-9]*(?:-[a-z0-9]+)*")

internal const val EDICT_NEXT_MAX_INBOX_SIGNALS_PER_RUN: Int = 100
internal const val EDICT_NEXT_NEIGHBOUR_COUNT: Int = 10
