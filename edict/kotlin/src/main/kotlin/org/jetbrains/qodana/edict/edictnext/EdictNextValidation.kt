package org.jetbrains.qodana.edict.edictnext

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
): EdictNextCodeExampleValidationResponse {
  val directory = EdictNextExampleDirectory(
    repository.paths.clustersDirectory.resolve(clusterId).resolve("synthetic-examples").resolve(exampleId),
  )
  if (!directory.metadataPath.isRegularFile()) {
    return EdictNextCodeExampleValidationResponse(
      false,
      "Code example has no metadata.json",
      listOf(directory.metadataPath.toString())
    )
  }
  val metadata = try {
    EdictNextJson.decodeFromString(EdictNextCodeExampleMetadata.serializer(), directory.metadataPath.readText())
  }
  catch (e: Exception) {
    return EdictNextCodeExampleValidationResponse(false, "Invalid code example metadata", listOf(e.message.orEmpty()))
  }
  val sourcePath = directory.sourcePath(metadata.fileName)
  if (!sourcePath.isRegularFile()) {
    return EdictNextCodeExampleValidationResponse(
      false,
      "Code example source file is missing",
      listOf(sourcePath.toString())
    )
  }
  return validateCodeExample(directory, metadata, sourcePath, sourcePath.readText())
}

internal suspend fun validateRepositoryState(
  repository: EdictRepository,
  state: EdictNextRepositoryState,
): List<EdictNextValidationIssue> {
  val issues = mutableListOf<EdictNextValidationIssue>()
  val allSignals = state.clusters.flatMap(EdictNextStoredCluster::signals) + state.inboxSignals
  allSignals.groupingBy(EdictNextSignal::id).eachCount().filterValues { it != 1 }.forEach { (signalId, count) ->
    issues += EdictNextValidationIssue("signals/$signalId", "Signal occurs $count times")
  }
  allSignals.forEach { signal ->
    val path = "signals/${signal.id}.json"
    if (signal.id.isBlank()) issues += EdictNextValidationIssue(path, "Signal id must not be blank")
    if (signal.description.isBlank()) issues += EdictNextValidationIssue(path, "Signal description must not be blank")
    if (signal.fileRevision.path.isBlank()) issues += EdictNextValidationIssue(
      path,
      "Signal source path must not be blank"
    )
    if (signal.fileRevision.revision.isBlank()) issues += EdictNextValidationIssue(
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
      issues += EdictNextValidationIssue(cluster.directory.manifestPath.toString(), "Cluster contains duplicate promotion ids")
    }
    cluster.manifest.promotions.forEach { promotion ->
      try {
        validatePromotion(promotion)
      }
      catch (e: IllegalArgumentException) {
        issues += EdictNextValidationIssue(cluster.directory.manifestPath.toString(), e.message.orEmpty())
      }
    }
    val finalPath = state.relativePath(repository.paths.inspectionPath(cluster.id))
    val candidatePath = state.relativePath(cluster.candidateInspectionPath)
    when (cluster.manifest.status) {
      EdictNextClusterStatus.Generated -> {
        if (cluster.manifest.predecessorId != null) {
          issues += EdictNextValidationIssue(
            cluster.directory.manifestPath.toString(),
            "Generated cluster must not have a predecessor"
          )
        }
        if (finalPath !in state.filesByRelativePath) {
          issues += EdictNextValidationIssue(
            repository.paths.inspectionPath(cluster.id).toString(),
            "Generated cluster has no inspection"
          )
        }
        if (candidatePath in state.filesByRelativePath) {
          issues += EdictNextValidationIssue(
            cluster.candidateInspectionPath.toString(),
            "Generated cluster must not have a candidate"
          )
        }
        allowedInspectionPaths += finalPath
      }
      EdictNextClusterStatus.Pending, EdictNextClusterStatus.Invalid -> {
        allowedInspectionPaths += candidatePath
        cluster.manifest.predecessorId?.let { predecessorId ->
          val predecessorPath = state.relativePath(repository.paths.inspectionPath(predecessorId))
          allowedInspectionPaths += predecessorPath
          if (predecessorPath !in state.filesByRelativePath) {
            issues += EdictNextValidationIssue(
              repository.paths.inspectionPath(predecessorId).toString(),
              "Pending or Invalid cluster predecessor inspection is missing",
            )
          }
        }
      }
      EdictNextClusterStatus.Discontinued -> {
        if (cluster.manifest.predecessorId != null) {
          issues += EdictNextValidationIssue(
            cluster.directory.manifestPath.toString(),
            "Discontinued cluster must not have a predecessor"
          )
        }
        if (finalPath in state.filesByRelativePath) {
          issues += EdictNextValidationIssue(
            repository.paths.inspectionPath(cluster.id).toString(),
            "Discontinued cluster must not have an inspection"
          )
        }
        if (candidatePath in state.filesByRelativePath) {
          issues += EdictNextValidationIssue(
            cluster.candidateInspectionPath.toString(),
            "Discontinued cluster must not have a candidate"
          )
        }
      }
    }
  }

  state.filesByRelativePath.keys.filter { it.startsWith("inspections/") && it !in allowedInspectionPaths }.forEach { path ->
    issues += EdictNextValidationIssue(path, "Inspection file does not belong to the current cluster state")
  }
  return issues
}

internal fun validatePromotion(promotion: InspectionPromotion) {
  require(promotion.id.matches(Regex("p-[0-9a-f]{24}"))) { "Invalid promotion id '${promotion.id}'" }
  require(promotion.inspectionDigest.matches(Regex("[0-9a-f]{64}"))) { "Invalid promotion inspection digest" }
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

internal suspend fun validateCluster(
  cluster: EdictNextStoredCluster,
  history: String,
): List<EdictNextValidationIssue> {
  val issues = mutableListOf<EdictNextValidationIssue>()
  if (!EDICT_NEXT_KEBAB_CASE.matches(cluster.id)) {
    issues += EdictNextValidationIssue(cluster.directory.root.toString(), "Invalid cluster id '${cluster.id}'")
  }
  if (cluster.signals.isEmpty()) {
    issues += EdictNextValidationIssue(cluster.directory.root.toString(), "Cluster must contain at least one Signal")
  }
  if (cluster.signals.any { !it.isJvmLanguage }) {
    issues += EdictNextValidationIssue(
      cluster.directory.root.toString(),
      "Clusters may contain only Java or Kotlin Signals"
    )
  }
  else if (cluster.signals.any { it.language != cluster.manifest.language }) {
    issues += EdictNextValidationIssue(
      cluster.directory.root.toString(),
      "Cluster language does not match every Signal"
    )
  }
  issues += collectClusterExampleIssues(
    cluster,
    requireEverySignal = cluster.manifest.status != EdictNextClusterStatus.Pending &&
                         cluster.manifest.status != EdictNextClusterStatus.Invalid,
  )

  if (cluster.manifest.status != EdictNextClusterStatus.Pending && history.isBlank()) {
    issues += EdictNextValidationIssue(
      cluster.historyPath.toString(),
      "Non-Pending cluster must have non-empty history"
    )
  }
  return issues
}

internal suspend fun validateClusterExamples(
  cluster: EdictNextStoredCluster,
): List<EdictNextValidationIssue> = buildList {
  addAll(collectClusterExampleIssues(cluster, requireEverySignal = true))
  cluster.examples.forEach { example ->
    val expectedRanges = example.metadata.expectedRanges
    when (example.metadata.label) {
      EdictNextSignalLabel.POSITIVE -> if (expectedRanges?.size != 1) {
        add(
          EdictNextValidationIssue(
            example.directory.root.toString(),
            "A positive example must declare exactly one expected range"
          )
        )
      }
      EdictNextSignalLabel.NEGATIVE -> if (!expectedRanges.isNullOrEmpty()) {
        add(
          EdictNextValidationIssue(
            example.directory.root.toString(),
            "A negative example must not declare expected ranges"
          )
        )
      }
    }
  }
}

private suspend fun collectClusterExampleIssues(
  cluster: EdictNextStoredCluster,
  requireEverySignal: Boolean,
): List<EdictNextValidationIssue> = buildList {
  if (cluster.examples.any { it.sourcePath.extension != cluster.manifest.language.fileExtension }) {
    add(
      EdictNextValidationIssue(
        cluster.directory.root.toString(),
        "A code example source language does not match the cluster language"
      )
    )
  }
  cluster.examples.forEach { example ->
    validateCodeExample(example.directory, example.metadata, example.sourcePath, example.code).issues.forEach { issue ->
      add(EdictNextValidationIssue(example.directory.root.toString(), issue))
    }
  }
  cluster.signals.forEach { signal ->
    val exampleId = signal.syntheticExampleId
    val example = cluster.examples.singleOrNull { it.metadata.id == exampleId }
    if (exampleId != null && example == null) {
      add(
        EdictNextValidationIssue(
          cluster.directory.root.toString(),
          "Signal '${signal.id}' references missing example '$exampleId'"
        )
      )
    }
    if (example != null && example.metadata.label != signal.label) {
      add(
        EdictNextValidationIssue(
          cluster.directory.root.toString(),
          "Signal '${signal.id}' and example '$exampleId' have different labels"
        )
      )
    }
    if (exampleId == null && requireEverySignal) {
      add(EdictNextValidationIssue(cluster.directory.root.toString(), "Signal '${signal.id}' has no code example"))
    }
  }
}

internal suspend fun validateGeneratedTransition(
  repository: EdictRepository,
  cluster: EdictNextStoredCluster,
  history: String,
): List<EdictNextValidationIssue> {
  val issues = validateCluster(cluster, history).toMutableList()
  if (cluster.manifest.status != EdictNextClusterStatus.Pending) {
    issues += EdictNextValidationIssue(cluster.directory.manifestPath.toString(), "Cluster must be Pending")
  }
  cluster.signals.filter { it.syntheticExampleId == null }.forEach { signal ->
    issues += EdictNextValidationIssue(cluster.directory.root.toString(), "Signal '${signal.id}' has no code example")
  }
  if (history.isBlank()) {
    issues += EdictNextValidationIssue(cluster.historyPath.toString(), "Generated cluster must have non-empty history")
  }
  cluster.manifest.predecessorId?.let { predecessorId ->
    val predecessor = repository.paths.inspectionPath(predecessorId)
    if (!predecessor.isRegularFile()) {
      issues += EdictNextValidationIssue(predecessor.toString(), "Predecessor inspection is missing")
    }
  }
  val finalInspection = repository.paths.inspectionPath(cluster.id)
  if (finalInspection.exists() && cluster.manifest.predecessorId != cluster.id) {
    issues += EdictNextValidationIssue(finalInspection.toString(), "Unexpected existing final inspection")
  }
  return issues
}

internal fun validateDistributionChange(
  before: EdictNextRepositoryState,
  after: EdictNextRepositoryState,
  movedSignalIds: Set<String>,
): List<EdictNextValidationIssue> {
  val issues = mutableListOf<EdictNextValidationIssue>()
  val recipients = mutableMapOf<String, MutableSet<String>>()
  movedSignalIds.forEach { signalId ->
    val original = before.inboxSignalsById[signalId]
    if (original == null) {
      issues += EdictNextValidationIssue("inbox/$signalId.json", "Signal was not in the inbox before distribution")
      return@forEach
    }
    val owners = after.clusters.filter { signalId in it.signalIds }
    if (owners.size != 1 || signalId in after.inboxSignalIds) {
      issues += EdictNextValidationIssue("signals/$signalId", "Distributed Signal must belong to exactly one cluster")
      return@forEach
    }
    if (owners.single().signals.single { it.id == signalId } != original) {
      issues += EdictNextValidationIssue("signals/$signalId", "Distribution changed the Signal contents")
    }
    recipients.getOrPut(owners.single().id) { linkedSetOf() } += signalId
  }

  val allowedPaths = movedSignalIds.mapTo(mutableSetOf()) { "inbox/$it.json" }
  recipients.forEach { (clusterId, addedSignalIds) ->
    val current = after.clustersById.getValue(clusterId)
    val original = before.clustersById[clusterId]
    val expectedSignalIds = original?.signalIds.orEmpty() + addedSignalIds
    if (current.signalIds != expectedSignalIds) {
      issues += EdictNextValidationIssue(
        current.directory.root.toString(),
        "Distribution changed cluster membership beyond the moved Signals"
      )
    }
    allowedPaths += "clusters/$clusterId/cluster.json"
    if (original == null) allowedPaths += "clusters/$clusterId/history.md"
    addedSignalIds.forEach { allowedPaths += "clusters/$clusterId/signals/$it.json" }
  }

  changedPaths(before, after).filterNot(allowedPaths::contains).forEach { path ->
    issues += EdictNextValidationIssue(path, "Distribution changed repository state outside the moved Signals")
  }
  return issues
}

internal fun validateGenerationChange(
  before: EdictNextRepositoryState,
  after: EdictNextRepositoryState,
  generationSignalIds: Set<String>,
): List<EdictNextValidationIssue> {
  val issues = mutableListOf<EdictNextValidationIssue>()
  val targets = before.clusters.filter { it.signalIds.any(generationSignalIds::contains) }
  val currentTargets = targets.mapNotNull { target ->
    val matches = after.clusters.filter { it.signalIds == target.signalIds }
    if (matches.size != 1) {
      issues += EdictNextValidationIssue(
        target.directory.root.toString(),
        "Frozen generation target must remain one cluster with the same Signals",
      )
      null
    }
    else {
      val current = matches.single()
      if ((current.manifest.status == EdictNextClusterStatus.Pending ||
           current.manifest.status == EdictNextClusterStatus.Invalid) &&
          current.manifest.predecessorId != target.manifest.predecessorId) {
        issues += EdictNextValidationIssue(
          current.directory.manifestPath.toString(),
          "Pending or Invalid generation target must retain its frozen predecessor",
        )
      }
      target to current
    }
  }
  val allowedClusterIds = (targets.map(EdictNextStoredCluster::id) + currentTargets.map { it.second.id }).toSet()
  val allowedInspectionIds = allowedClusterIds + targets.mapNotNull { it.manifest.predecessorId }
  changedPaths(before, after).filterNot { path ->
    allowedClusterIds.any { path.startsWith("clusters/$it/") } ||
    allowedInspectionIds.any {
      path == "inspections/$it${EDICT_NEXT_INSPECTION_SUFFIX}" || path == "inspections/$it${EDICT_NEXT_CANDIDATE_SUFFIX}"
    }
  }.forEach { path ->
    issues += EdictNextValidationIssue(path, "Generation changed repository state outside the frozen targets")
  }
  return issues
}


private fun changedPaths(before: EdictNextRepositoryState, after: EdictNextRepositoryState): Set<String> =
  (before.filesByRelativePath.keys + after.filesByRelativePath.keys).filterTo(sortedSetOf()) { path ->
    val previous = before.filesByRelativePath[path]
    val current = after.filesByRelativePath[path]
    previous == null || current == null || !previous.contentEquals(current)
  }

private suspend fun validateCodeExample(
  directory: EdictNextExampleDirectory,
  metadata: EdictNextCodeExampleMetadata,
  sourcePath: Path,
  sourceCode: String,
): EdictNextCodeExampleValidationResponse {
  val issues = mutableListOf<String>()
  if (metadata.id != directory.root.fileName.toString()) {
    issues += "Code example directory '${directory.root.fileName}' does not match metadata id '${metadata.id}'"
  }
  if (sourcePath.normalize() != directory.sourcePath(metadata.fileName)) {
    issues += "Code example source is outside its test project"
  }
  val lineCount = sourceCode.lineSequence().count().coerceAtLeast(1)
  metadata.expectedRanges.orEmpty().forEach { range ->
    if (range.start < 1 || range.end < range.start || range.end > lineCount) {
      issues += "Invalid target range ${range.start}-${range.end}; file has $lineCount lines"
    }
  }
  if (metadata.label == EdictNextSignalLabel.POSITIVE && metadata.expectedRanges.isNullOrEmpty()) {
    issues += "A positive example must declare expectedRanges"
  }
  return EdictNextCodeExampleValidationResponse(
    success = issues.isEmpty(),
    summary = if (issues.isEmpty()) "Code example and all target ranges are valid" else "Code example is structurally invalid",
    issues = issues,
  )
}
