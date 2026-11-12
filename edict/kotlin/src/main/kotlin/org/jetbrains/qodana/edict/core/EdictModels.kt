@file:OptIn(kotlinx.serialization.ExperimentalSerializationApi::class)

package org.jetbrains.qodana.edict.core

import kotlinx.serialization.EncodeDefault
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import org.jetbrains.qodana.edict.ci.ReviewReference
import org.jetbrains.qodana.edict.ci.ReviewRepository
import kotlinx.serialization.json.Json
import org.jetbrains.qodana.edict.common.sha256
import java.security.MessageDigest

fun stableSignalId(key: String): String = "s-${sha256(key).take(10)}"

internal val EdictJson: Json = Json {
  ignoreUnknownKeys = true
  prettyPrint = true
}

@Serializable
internal enum class EdictLanguage(val fileExtension: String) {
  Kotlin("kt"),
  Java("java"),

  ;

  companion object {
    fun fromPathOrNull(path: String): EdictLanguage? = when (path.substringAfterLast('.', missingDelimiterValue = "").lowercase()) {
      Kotlin.fileExtension -> Kotlin
      Java.fileExtension -> Java
      else -> null
    }
  }
}

@Serializable
enum class EdictSignalLabel {
  POSITIVE,
  NEGATIVE,
}

@Serializable
enum class EdictSignalStrength {
  STRONG,
  @Suppress("unused")
  WEAK,
}

@Serializable
sealed interface EdictSignalSource {
  @Serializable
  @SerialName("FromPR")
  data class FromPR(
    val prNumber: Int,
    val title: String,
    val discussionMessages: List<String>,
    val diffPositiveToNegative: String,
    val url: String,
  ) : EdictSignalSource

  @Serializable
  @SerialName("FromCommit")
  // TODO: edict-retrospective-signal-analysis tells agents to add the parent revision,
  //  commit message, and diff here; publishing rejects those unknown keys. Align the skills or this source.
  data class FromCommit(
    val commitRevision: String,
    val url: String? = null,
  ) : EdictSignalSource

  /** Feedback supplied with the project, in the checked-in format; Edict's own feedback is [GitHubEvaluatedFinding]. */
  @Deprecated("Edict records the answers to its own review comments as GitHubEvaluatedFinding")
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
    val message: String? = null,
    val url: String? = null,
  ) : EdictSignalSource

  /**
   * A finding of a not yet promoted inspection that users rated on a GitHub pull request: the thumbs on its review comment
   * give the Signal's label, and [replies] are the comment's replies as their authors wrote them.
   */
  @Serializable
  @SerialName("GitHubEvaluatedFinding")
  data class GitHubEvaluatedFinding(
    val inspectionName: String,
    val inspectionDescription: String,
    /** The problem the inspection reported, as its message says. */
    val problemMessage: String,
    /** The web page of the review comment. */
    val url: String,
    val replies: List<EdictReply>,
    /** The SHA-256 hex of the inspection's bytes that reported the finding: each version of an inspection is its own. */
    val inspectionDigest: String,
  ) : EdictSignalSource

  @Serializable
  @SerialName("Generated")
  data class Generated(val resultMessage: String? = null) : EdictSignalSource
}

/** A reply to a review comment: what the user with the login [author] wrote. */
@Serializable
data class EdictReply(val author: String, val body: String)

@Serializable
data class EdictLineRange(
  val start: Int,
  val end: Int,
) {
  fun intersects(other: EdictLineRange): Boolean = start <= other.end && other.start <= end
}

@Serializable
data class EdictFileRevision(
  val path: String,
  val revision: String,
  val expectedRanges: List<EdictLineRange>? = null,
)

@Serializable
data class EdictSignalProvenance(
  val workItemId: String,
  val analysisBatchId: String? = null,
)

@Serializable
data class EdictSignal(
  val id: String,
  val idempotencyKey: String = "",
  val fileRevision: EdictFileRevision,
  val source: EdictSignalSource,
  val label: EdictSignalLabel,
  val description: String,
  val strength: EdictSignalStrength = EdictSignalStrength.STRONG,
  val syntheticExampleId: String? = null,
  val provenance: EdictSignalProvenance = EdictSignalProvenance(""),
) {
  val deduplicationKey: String
    get() = idempotencyKey.ifBlank { (source as? EdictSignalSource.SubmittedFeedback)?.suggestionId.orEmpty() }

  internal val isJvmLanguage: Boolean get() = EdictLanguage.fromPathOrNull(fileRevision.path) != null

  /** Whether this is a user's answer with [label] in a review comment to a finding of the inspection bytes of [inspectionDigest]. */
  internal fun isAnswer(label: EdictSignalLabel, inspectionDigest: String): Boolean =
    this.label == label && (source as? EdictSignalSource.GitHubEvaluatedFinding)?.inspectionDigest == inspectionDigest

  internal val language: EdictLanguage
    get() = requireNotNull(EdictLanguage.fromPathOrNull(fileRevision.path)) {
      "Unsupported Signal source language: ${fileRevision.path}"
    }
}

@Serializable
internal data class EdictCodeExampleMetadata(
  val id: String,
  val fileName: String,
  val label: EdictSignalLabel,
  val expectedRanges: List<EdictLineRange>? = null,
  val contextPath: String? = null,
)

@Serializable
internal enum class EdictClusterStatus {
  Pending,
  /** Pipeline processing cannot continue because infrastructure or the cluster input/state is broken. */
  Invalid,
  Generated,
  /** The cluster Signals are semantically incompatible and do not express one coherent code-quality rule. */
  Discontinued,
}

@Serializable
internal enum class PromotionPrState {
  ON_REVIEW,
  ACCEPTED,
  DECLINED,
  CLOSED,
}

@Serializable
internal data class InspectionPromotion(
  val id: String,
  val inspectionDigest: String,
  val targetBranch: String,
  val targetPath: String,
  val pullRequest: ReviewReference,
  val state: PromotionPrState,
  val createdAt: String,
  val updatedAt: String,
)

/** A source-fix PR requests feedback on weak evidence */
@Serializable
internal data class WeakSignalPromotion(
  val pullRequest: ReviewReference,
  val state: PromotionPrState = PromotionPrState.ON_REVIEW,
)

@Serializable
internal data class WeakSignalPromotionIndex(
  @EncodeDefault
  val activePromotions: Map<String, List<WeakSignalPromotion>> = emptyMap(),
)

@Serializable
internal data class EdictClusterManifest(
  val id: String,
  val language: EdictLanguage,
  val status: EdictClusterStatus,
  val predecessorId: String? = null,
  val promotions: List<InspectionPromotion> = emptyList(),
  /** Signals covered by the last successful generation or predecessor reuse; replacements are removed. */
  val processedSignalIds: Set<String> = emptySet(),
)

// --- Neighbour retrieval ------------------------------------------------------------------------
// Retrieval hands out locality without deciding cluster membership.

@Serializable
internal data class EdictNeighbour(
  val signalId: String,
  val distance: Double,
)

@Serializable
internal data class EdictSignalNeighbours(
  val signalId: String,
  val closest: List<EdictNeighbour> = emptyList(),
)

@Serializable
internal data class EdictNeighboursResponse(
  val neighbours: List<EdictSignalNeighbours> = emptyList(),
)

@Serializable
internal data class EdictSignalPreparationResponse(
  val signalId: String? = null,
  val signalPath: String? = null,
  val signal: EdictSignal? = null,
  val pairedSignal: EdictSignal? = null,
  val candidates: List<EdictSignalCandidate> = emptyList(),
  val summary: String,
  @EncodeDefault(EncodeDefault.Mode.ALWAYS)
  val nextAction: EdictNextAction =
    if (signalId == null) EdictNextAction.STOP_DISTRIBUTION else EdictNextAction.PROCESS_SIGNAL,
)

@Serializable
internal data class EdictSignalValidationResponse(
  val signalId: String,
  val summary: String,
  val added: Boolean,
  val next: EdictSignalPreparationResponse? = null,
)

@Serializable
internal sealed class EdictDistributionContextResponse {
  @Serializable
  @SerialName("signal")
  data class SignalContext(
    val signal: EdictSignal,
  ) : EdictDistributionContextResponse()

  @Serializable
  @SerialName("cluster")
  data class Cluster(
    val clusterId: String,
    val signals: List<EdictSignal>,
  ) : EdictDistributionContextResponse()
}

// --- Distribution -------------------------------------------------------------------------------

/** One useful comparison for a Signal: a current cluster, or another inbox Signal. */
@Serializable
internal sealed interface EdictSignalCandidate {
  val nearestDistance: Double

  @Serializable
  @SerialName("cluster")
  data class Cluster(
    val clusterId: String,
    override val nearestDistance: Double,
    val memberCount: Int,
    /** The members of one of the nearest small clusters, which then counts as loaded context for the current Signal. */
    val signals: List<EdictSignal>? = null,
  ) : EdictSignalCandidate

  @Serializable
  @SerialName("signal")
  data class SignalCandidate(
    val signalId: String,
    val signalPath: String,
    override val nearestDistance: Double,
    val label: EdictSignalLabel,
    val path: String,
    val description: String,
  ) : EdictSignalCandidate
}

@Serializable
internal data class EdictValidationIssue(
  val path: String,
  val message: String,
)

@Serializable
internal data class EdictCalculateNeighboursResponse(
  val summary: String,
)

@Serializable
internal data class EdictGenerationTarget(
  val clusterId: String,
  val clusterDirectory: String,
)

@Serializable
internal data class EdictGenerationClustersResponse(
  val clusters: List<EdictGenerationTarget>,
  @EncodeDefault(EncodeDefault.Mode.ALWAYS)
  val maxConcurrentClusterTasks: Int = EDICT_MAX_CONCURRENT_CLUSTER_TASKS,
  val summary: String,
)

@Serializable
internal data class EdictCodeExampleValidationResponse(
  val success: Boolean,
  val summary: String,
  val issues: List<String> = emptyList(),
  @EncodeDefault(EncodeDefault.Mode.ALWAYS)
  val nextAction: EdictNextAction =
    if (success) EdictNextAction.CONTINUE_CLUSTER_GENERATION else EdictNextAction.REPAIR_CODE_EXAMPLE,
)

@Serializable
internal data class EdictMutationResponse(
  val success: Boolean,
  val summary: String,
)

/** The [directory] that holds the cluster's new Signals and its code examples without their labels, for a blind mapping. */
@Serializable
internal data class EdictSignalMappingResponse(
  val directory: String,
  /** The new Signals, each in `signals/<id>.json`. */
  val signalIds: List<String>,
  /** The code examples, each as its source file in `examples/<id>/`. */
  val exampleIds: List<String>,
)

/** What a blind mapper sees of a Signal: where its case is, without its label, description or source. */
@Serializable
internal data class EdictUnlabelledSignal(val id: String, val fileRevision: EdictFileRevision)

@Serializable
internal enum class EdictInspectionAction {
  CONFLICT,
  SKIP,
  GENERATE,
}

@Serializable
internal data class EdictInspectionActionResponse(
  val action: EdictInspectionAction,
  val summary: String,
  val conflictingSignalIds: List<String> = emptyList(),
)

@Serializable
internal data class EdictInspectionFailure(
  val exampleId: String,
  val message: String,
)

@Serializable
internal data class EdictInspectionValidationResponse(
  val compilationSuccess: Boolean,
  /** Compilation, metadata and example execution are valid; evidence accuracy is advisory. */
  val overallSuccess: Boolean,
  val summary: String,
  val failures: List<EdictInspectionFailure> = emptyList(),
  val achievedAccuracyPercent: Int? = null,
  val reportedNegativeExampleIds: List<String> = emptyList(),
  val uncoveredPositiveExampleIds: List<String> = emptyList(),
  @EncodeDefault(EncodeDefault.Mode.ALWAYS)
  val nextAction: EdictNextAction = if (overallSuccess) EdictNextAction.REVIEW_INSPECTION else EdictNextAction.REPAIR_INSPECTION,
)

@Serializable
internal data class EdictInspectionResultsResponse(
  val weakSignalReviewConfigPath: String,
  /** At 0 the analyzed candidate is final: it can still be marked Generated, but not analyzed again once changed. */
  val remainingProjectAnalyses: Int,
  /** True when the project findings equal those of this cluster's previous analysis, which were already reviewed. */
  @EncodeDefault(EncodeDefault.Mode.ALWAYS)
  val findingsUnchanged: Boolean = false,
)

/**
 * `clusters/<id>/evaluation.json`: the final score of one inspection on one example set, written only by
 * `edict_record_evaluation`. Every example weighs the project findings the weak-signal review assigned to it, at
 * least 1: tp sums the weights of satisfied positive examples, fn of unsatisfied positive ones, and fp of negative
 * examples the inspection reports, each with the [directFindings] of its kind. Finalising Generated requires one
 * matching the published inspection and examples.
 */
@Serializable
internal data class EdictEvaluation(
  val inspectionHash: String,
  val exampleSetDigest: String,
  val action: EdictInspectionAction,
  val tp: Int,
  val fp: Int,
  val fn: Int,
  val precision: Double,
  val recall: Double,
  val examples: Map<String, EdictExampleEvaluation>,
  /** The published inspection's project analysis and its weak-signal review; null when this run analyzed no such bytes. */
  val projectReview: EdictProjectReview?,
  val inspectionName: String = "",
  val inspectionDescription: String = "",
  val directFindings: EdictDirectFindings = EdictDirectFindings(tp = 0, fp = 0),
)

/**
 * The findings an evaluation counts directly rather than through an example's weight, [tp] true and [fp] false positives:
 * the inspection does not decide the example of each as the finding was classified, or users confirmed one in a review.
 */
@Serializable
internal data class EdictDirectFindings(val tp: Int, val fp: Int)

@Serializable
internal data class EdictExampleEvaluation(
  val label: EdictSignalLabel,
  val strong: Boolean,
  val satisfied: Boolean,
  val projectFindings: Int,
  val weight: Int,
)

@Serializable
internal data class EdictProjectReview(
  val projectRevision: String,
  val totalFindings: Int,
  val reviewedFindings: Int,
  val uncertain: Int,
  val unassigned: Int,
  val unreproduced: Int,
  val inspectionDescription: String = "",
  /** Exact locations classified TP for these inspection bytes, retained across server restarts. */
  val truePositiveFindings: List<EdictProjectFinding> = emptyList(),
  val reportUrl: String? = null,
)

@Serializable
internal enum class EdictFindingClassification { TP, FP, UNCERTAIN }

@Serializable
internal data class EdictReviewedFinding(
  val path: String,
  val line: Int,
  val classification: EdictFindingClassification,
  val exampleId: String? = null,
)

@Serializable
internal data class EdictRecordEvaluationResponse(
  val success: Boolean,
  val summary: String,
  val evaluation: EdictEvaluation? = null,
)

@Serializable
internal data class EdictFinaliseClusterResponse(
  val success: Boolean,
  val summary: String,
  val issues: List<EdictValidationIssue> = emptyList(),
)

@Serializable
internal data class EdictProjectFinding(
  val fileRevision: EdictFileRevision,
  val message: String? = null,
  val contextSnippet: String? = null,
  val contextStartLine: Int? = null,
)

@Serializable
internal data class EdictInspectionFindings(
  val clusterId: String,
  val candidateDigest: String,
  val projectRevision: String,
  val inspectionDescription: String,
  val language: EdictLanguage,
  val findings: List<EdictProjectFinding>,
)

@Serializable
internal data class EdictWeakSignalReviewConfig(
  val clusterDirectory: String,
  val candidateInspection: String,
  val candidateDigest: String,
  val findingsPath: String,
  val inspectedProject: String,
  val privateScratchDirectory: String,
  val outputPath: String,
  val exampleStatus: Map<String, EdictReviewExampleStatus>,
  val unreproducedFindings: Map<String, EdictUnreproducedFindings>,
)

@Serializable
internal data class EdictUnreproducedFindings(
  val stillReported: List<String>,
  val noLongerReported: Int,
)

@Serializable
internal data class EdictReviewExampleStatus(
  val label: EdictSignalLabel,
  val strong: Boolean,
  val reported: Boolean,
)

@Serializable
internal enum class EdictNextAction {
  PROCESS_SIGNAL,
  /** The current distribution task assigned its share of the batch; a new task continues it. */
  HAND_OFF,
  STOP_DISTRIBUTION,
  REPAIR_CODE_EXAMPLE,
  CONTINUE_CLUSTER_GENERATION,
  REPAIR_INSPECTION,
  REVIEW_INSPECTION,
}

/** Configuration an agent needs for this run; the root prompt carries none of it. */
@Serializable
internal data class EdictRunContext(
  val projectDirectory: String,
  val stateDirectory: String,
  val scratchDirectory: String,
  val reviewRepository: ReviewRepository? = null,
  val defaultSignalCount: Int,
)

/** Resets the keys only extraction uses to their defaults, which encoding leaves out. */
internal fun EdictSignal.asEvidence(): EdictSignal =
  copy(idempotencyKey = "", provenance = EdictSignalProvenance(""))

internal fun sha256Hex(text: String): String = sha256Hex(text.toByteArray(Charsets.UTF_8))

internal fun sha256Hex(bytes: ByteArray): String = MessageDigest.getInstance("SHA-256")
  .digest(bytes)
  .joinToString("") { byte -> "%02x".format(byte) }

internal const val EMBEDDING_MODEL: String = "thenlper/gte-large"
internal const val EMBEDDING_MODEL_REVISION: String = "4bef63f39fcc5e2d6b0aae83089f307af4970164"
// Cluster workers spawn overseers and per-Signal reducers. Keep enough native-agent
// capacity for those descendants instead of exhausting the session at the first wave.
internal const val EDICT_MAX_CONCURRENT_CLUSTER_TASKS: Int = 6
internal const val EDICT_INSPECTION_SUFFIX: String = ".inspection.kts"
internal const val EDICT_CANDIDATE_SUFFIX: String = ".candidate.kts"

/** Cluster ids and rule ids share one shape. */
internal val EDICT_KEBAB_CASE: Regex = Regex("[a-z][a-z0-9]*(?:-[a-z0-9]+)*")

internal const val EDICT_NEIGHBOUR_COUNT: Int = 10
