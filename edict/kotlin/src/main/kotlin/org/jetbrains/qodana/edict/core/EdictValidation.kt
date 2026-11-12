package org.jetbrains.qodana.edict.core

import org.jetbrains.qodana.edict.signals.validDigest
import java.nio.file.Path
import kotlin.collections.plusAssign
import kotlin.io.path.exists
import kotlin.io.path.extension
import kotlin.io.path.isRegularFile
import kotlin.io.path.readText

internal suspend fun validateCodeExample(
  repository: EdictRepository,
  clusterId: String,
  exampleId: String,
): EdictCodeExampleValidationResponse {
  val directory = EdictExampleDirectory(
    repository.paths.clustersDirectory.resolve(clusterId).resolve("synthetic-examples").resolve(exampleId),
  )
  if (!directory.metadataPath.isRegularFile()) {
    return EdictCodeExampleValidationResponse(
      false,
      "Code example has no metadata.json",
      listOf(directory.metadataPath.toString())
    )
  }
  val metadata = try {
    EdictJson.decodeFromString(EdictCodeExampleMetadata.serializer(), directory.metadataPath.readText())
  }
  catch (e: Exception) {
    return EdictCodeExampleValidationResponse(false, "Invalid code example metadata", listOf(e.message.orEmpty()))
  }
  val sourcePath = directory.sourcePath(metadata.fileName)
  if (!sourcePath.isRegularFile()) {
    return EdictCodeExampleValidationResponse(
      false,
      "Code example source file is missing",
      listOf(sourcePath.toString())
    )
  }
  return validateCodeExample(directory, metadata, sourcePath, sourcePath.readText())
}

internal suspend fun validateRepositoryState(
  repository: EdictRepository,
  state: EdictRepositoryState,
): List<EdictValidationIssue> {
  val issues = mutableListOf<EdictValidationIssue>()
  try {
    val active = if (repository.paths.activeWeakSignalPromotionsPath.isRegularFile()) repository.loadActiveWeakSignalPromotions() else emptyMap()
    active.keys.forEach { clusterId ->
      if (state.clusters.none { it.id == clusterId }) {
        issues += EdictValidationIssue(repository.paths.activeWeakSignalPromotionsPath.toString(), "Unknown source cluster '$clusterId'")
      }
    }
  } catch (e: IllegalArgumentException) {
    issues += EdictValidationIssue(repository.paths.activeWeakSignalPromotionsPath.toString(), e.message.orEmpty())
  }
  val allSignals = state.clusters.flatMap(EdictStoredCluster::signals) + state.inboxSignals
  allSignals.groupingBy(EdictSignal::id).eachCount().filterValues { it != 1 }.forEach { (signalId, count) ->
    issues += EdictValidationIssue("signals/$signalId", "Signal occurs $count times")
  }
  allSignals.forEach { signal ->
    val path = "signals/${signal.id}.json"
    if (signal.id.isBlank()) issues += EdictValidationIssue(path, "Signal id must not be blank")
    if (signal.description.isBlank()) issues += EdictValidationIssue(path, "Signal description must not be blank")
    if (signal.fileRevision.path.isBlank()) issues += EdictValidationIssue(
      path,
      "Signal source path must not be blank"
    )
    if (signal.fileRevision.revision.isBlank()) issues += EdictValidationIssue(
      path,
      "Signal source revision must not be blank"
    )
  }

  val allowedInspectionPaths = mutableSetOf<String>()
  state.clusters.forEach { cluster ->
    val history = state.filesByRelativePath[state.relativePath(cluster.historyPath)]?.decodeToString().orEmpty()
    issues += validateCluster(cluster, history)
    val promotionIds = cluster.manifest.promotions.map(InspectionPromotion::id)
    if (promotionIds.distinct().size != promotionIds.size) {
      issues += EdictValidationIssue(cluster.directory.manifestPath.toString(), "Cluster contains duplicate promotion ids")
    }
    cluster.manifest.promotions.forEach { promotion ->
      try {
        validatePromotion(promotion)
      }
      catch (e: IllegalArgumentException) {
        issues += EdictValidationIssue(cluster.directory.manifestPath.toString(), e.message.orEmpty())
      }
    }
    val finalPath = state.relativePath(repository.paths.inspectionPath(cluster.id))
    val candidatePath = state.relativePath(cluster.candidateInspectionPath)
    when (cluster.manifest.status) {
      EdictClusterStatus.Generated -> {
        if (cluster.manifest.predecessorId != null) {
          issues += EdictValidationIssue(
            cluster.directory.manifestPath.toString(),
            "Generated cluster must not have a predecessor"
          )
        }
        if (finalPath !in state.filesByRelativePath) {
          issues += EdictValidationIssue(
            repository.paths.inspectionPath(cluster.id).toString(),
            "Generated cluster has no inspection"
          )
        }
        if (candidatePath in state.filesByRelativePath) {
          issues += EdictValidationIssue(
            cluster.candidateInspectionPath.toString(),
            "Generated cluster must not have a candidate"
          )
        }
        allowedInspectionPaths += finalPath
      }
      EdictClusterStatus.Pending, EdictClusterStatus.Invalid -> {
        allowedInspectionPaths += candidatePath
        cluster.manifest.predecessorId?.let { predecessorId ->
          val predecessorPath = state.relativePath(repository.paths.inspectionPath(predecessorId))
          allowedInspectionPaths += predecessorPath
          if (predecessorPath !in state.filesByRelativePath) {
            issues += EdictValidationIssue(
              repository.paths.inspectionPath(predecessorId).toString(),
              "Pending or Invalid cluster predecessor inspection is missing",
            )
          }
        }
      }
      EdictClusterStatus.Discontinued -> {
        if (cluster.manifest.predecessorId != null) {
          issues += EdictValidationIssue(
            cluster.directory.manifestPath.toString(),
            "Discontinued cluster must not have a predecessor"
          )
        }
        if (finalPath in state.filesByRelativePath) {
          issues += EdictValidationIssue(
            repository.paths.inspectionPath(cluster.id).toString(),
            "Discontinued cluster must not have an inspection"
          )
        }
        if (candidatePath in state.filesByRelativePath) {
          issues += EdictValidationIssue(
            cluster.candidateInspectionPath.toString(),
            "Discontinued cluster must not have a candidate"
          )
        }
      }
    }
  }

  state.filesByRelativePath.keys.filter { it.startsWith("inspections/") && it !in allowedInspectionPaths }.forEach { path ->
    issues += EdictValidationIssue(path, "Inspection file does not belong to the current cluster state")
  }
  return issues
}

internal fun validatePromotion(promotion: InspectionPromotion) {
  require(promotion.id.matches(Regex("p-[0-9a-f]{24}"))) { "Invalid promotion id '${promotion.id}'" }
  require(validDigest(promotion.inspectionDigest)) { "Invalid promotion inspection digest" }
  require(promotion.targetBranch.isNotBlank()) { "Promotion target branch is required" }
  val targetPath = Path.of(promotion.targetPath)
  require(promotion.targetPath.isNotBlank() && !targetPath.isAbsolute && targetPath.none { it.toString() == ".." }) {
    "Promotion target path must stay inside the target repository"
  }
  require(promotion.pullRequest.owner.isNotBlank() && promotion.pullRequest.repository.isNotBlank()) {
    "Promotion repository identity is required"
  }
  require(promotion.pullRequest.id.isNotBlank() && promotion.pullRequest.url.isNotBlank()) { "Promotion PR identity is required" }
  require(java.time.Instant.parse(promotion.createdAt) <= java.time.Instant.parse(promotion.updatedAt)) {
    "Promotion update timestamp precedes creation"
  }
}

internal fun validateWeakSignalPromotion(promotion: WeakSignalPromotion) {
  require(promotion.pullRequest.owner.isNotBlank() && promotion.pullRequest.repository.isNotBlank() &&
    promotion.pullRequest.id.isNotBlank() && promotion.pullRequest.url.isNotBlank()) { "Weak-signal promotion PR identity is required" }
}

internal suspend fun validateCluster(
  cluster: EdictStoredCluster,
  history: String,
): List<EdictValidationIssue> {
  val issues = mutableListOf<EdictValidationIssue>()
  if (!EDICT_KEBAB_CASE.matches(cluster.id)) {
    issues += EdictValidationIssue(cluster.directory.root.toString(), "Invalid cluster id '${cluster.id}'")
  }
  if (cluster.signals.isEmpty()) {
    issues += EdictValidationIssue(cluster.directory.root.toString(), "Cluster must contain at least one Signal")
  }
  if (cluster.signals.any { !it.isJvmLanguage }) {
    issues += EdictValidationIssue(
      cluster.directory.root.toString(),
      "Clusters may contain only Java or Kotlin Signals"
    )
  }
  else if (cluster.signals.any { it.language != cluster.manifest.language }) {
    issues += EdictValidationIssue(
      cluster.directory.root.toString(),
      "Cluster language does not match every Signal"
    )
  }
  issues += collectClusterExampleIssues(
    cluster,
    requireEverySignal = cluster.manifest.status != EdictClusterStatus.Pending &&
                         cluster.manifest.status != EdictClusterStatus.Invalid,
  )

  if (cluster.manifest.status != EdictClusterStatus.Pending && history.isBlank()) {
    issues += EdictValidationIssue(
      cluster.historyPath.toString(),
      "Non-Pending cluster must have non-empty history"
    )
  }
  return issues
}

internal suspend fun validateClusterExamples(
  cluster: EdictStoredCluster,
): List<EdictValidationIssue> = buildList {
  addAll(collectClusterExampleIssues(cluster, requireEverySignal = true))
  cluster.examples.forEach { example ->
    val expectedRanges = example.metadata.expectedRanges
    when (example.metadata.label) {
      EdictSignalLabel.POSITIVE -> if (expectedRanges?.size != 1) {
        add(
          EdictValidationIssue(
            example.directory.root.toString(),
            "A positive example must declare exactly one expected range"
          )
        )
      }
      EdictSignalLabel.NEGATIVE -> if (!expectedRanges.isNullOrEmpty()) {
        add(
          EdictValidationIssue(
            example.directory.root.toString(),
            "A negative example must not declare expected ranges"
          )
        )
      }
    }
  }
}

private suspend fun collectClusterExampleIssues(
  cluster: EdictStoredCluster,
  requireEverySignal: Boolean,
): List<EdictValidationIssue> = buildList {
  if (cluster.examples.any { it.sourcePath.extension != cluster.manifest.language.fileExtension }) {
    add(
      EdictValidationIssue(
        cluster.directory.root.toString(),
        "A code example source language does not match the cluster language"
      )
    )
  }
  cluster.examples.forEach { example ->
    validateCodeExample(example.directory, example.metadata, example.sourcePath, example.code).issues.forEach { issue ->
      add(EdictValidationIssue(example.directory.root.toString(), issue))
    }
  }
  cluster.signals.forEach { signal ->
    val exampleId = signal.syntheticExampleId
    val example = cluster.examples.singleOrNull { it.metadata.id == exampleId }
    if (exampleId != null && example == null) {
      add(
        EdictValidationIssue(
          cluster.directory.root.toString(),
          "Signal '${signal.id}' references missing example '$exampleId'"
        )
      )
    }
    if (example != null && example.metadata.label != signal.label) {
      add(
        EdictValidationIssue(
          cluster.directory.root.toString(),
          "Signal '${signal.id}' and example '$exampleId' have different labels"
        )
      )
    }
    if (exampleId == null && requireEverySignal) {
      add(EdictValidationIssue(cluster.directory.root.toString(), "Signal '${signal.id}' has no code example"))
    }
  }
}

internal suspend fun validateGeneratedTransition(
  repository: EdictRepository,
  cluster: EdictStoredCluster,
  history: String,
): List<EdictValidationIssue> {
  val issues = validateCluster(cluster, history).toMutableList()
  if (cluster.manifest.status != EdictClusterStatus.Pending) {
    issues += EdictValidationIssue(cluster.directory.manifestPath.toString(), "Cluster must be Pending")
  }
  cluster.signals.filter { it.syntheticExampleId == null }.forEach { signal ->
    issues += EdictValidationIssue(cluster.directory.root.toString(), "Signal '${signal.id}' has no code example")
  }
  if (history.isBlank()) {
    issues += EdictValidationIssue(cluster.historyPath.toString(), "Generated cluster must have non-empty history")
  }
  cluster.manifest.predecessorId?.let { predecessorId ->
    val predecessor = repository.paths.inspectionPath(predecessorId)
    if (!predecessor.isRegularFile()) {
      issues += EdictValidationIssue(predecessor.toString(), "Predecessor inspection is missing")
    }
  }
  val finalInspection = repository.paths.inspectionPath(cluster.id)
  if (finalInspection.exists() && cluster.manifest.predecessorId != cluster.id) {
    issues += EdictValidationIssue(finalInspection.toString(), "Unexpected existing final inspection")
  }
  return issues
}

internal suspend fun validateCodeExample(
  directory: EdictExampleDirectory,
  metadata: EdictCodeExampleMetadata,
  sourcePath: Path,
  sourceCode: String,
): EdictCodeExampleValidationResponse {
  val issues = mutableListOf<String>()
  if (metadata.id != directory.root.fileName.toString()) {
    issues += "Code example directory '${directory.root.fileName}' does not match metadata id '${metadata.id}'"
  }
  if (sourcePath.normalize() != directory.sourcePath(metadata.fileName)) {
    issues += "Code example source is outside its test project"
  }
  // TODO: report PSI parse errors, as the in-IDE Edict did with PsiErrorElement; needs a PSI file parsing tool in the
  //  IntelliJ MCP that returns error descriptions and lines (generate_psi_tree returns only an outline).
  val lineCount = sourceCode.lineSequence().count().coerceAtLeast(1)
  metadata.expectedRanges.orEmpty().forEach { range ->
    if (range.start < 1 || range.end < range.start || range.end > lineCount) {
      issues += "Invalid target range ${range.start}-${range.end}; file has $lineCount lines"
    }
  }
  if (metadata.label == EdictSignalLabel.POSITIVE && metadata.expectedRanges.isNullOrEmpty()) {
    issues += "A positive example must declare expectedRanges"
  }
  return EdictCodeExampleValidationResponse(
    success = issues.isEmpty(),
    summary = if (issues.isEmpty()) "Code example and all target ranges are valid" else "Code example is structurally invalid",
    issues = issues,
  )
}
