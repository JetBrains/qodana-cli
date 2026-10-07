package org.jetbrains.qodana.edict.edictnext

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.SerializationStrategy
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonObject
import org.jetbrains.qodana.edict.common.GitRepository
import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.StandardCopyOption
import java.nio.file.StandardOpenOption
import kotlin.io.path.createDirectories
import kotlin.io.path.deleteIfExists
import kotlin.io.path.exists
import kotlin.io.path.isDirectory
import kotlin.io.path.isRegularFile
import kotlin.io.path.listDirectoryEntries
import kotlin.io.path.name
import kotlin.io.path.readBytes
import kotlin.io.path.readText
import kotlin.io.path.writeBytes
import kotlin.io.path.writeText

internal class EdictRepository(val paths: EdictRepositoryDirectory) {
  suspend fun loadState(): EdictNextRepositoryState = EdictNextRepositoryState(
    root = paths.root,
    clusters = loadClusters(),
    inboxSignals = loadInboxSignals(),
    filesByRelativePath = loadRepositoryFiles(),
  )

  fun loadClusters(): List<EdictNextStoredCluster> =
    paths.clustersDirectory
      .takeIf { path -> path.isDirectory() }?.listDirectoryEntries().orEmpty()
      .filter(Path::isDirectory)
      .sortedBy(Path::name)
      .map { clusterPath -> EdictNextClusterDirectory(clusterPath) }
      .map { clusterDir -> loadCluster(clusterDir) }

  fun loadCluster(clusterId: String): EdictNextStoredCluster =
    loadCluster(EdictNextClusterDirectory(paths.clustersDirectory.resolve(clusterId)))

  private fun loadCluster(clusterDirectory: EdictNextClusterDirectory): EdictNextStoredCluster {
    require(clusterDirectory.root.parent == paths.clustersDirectory) {
      "Cluster is outside the repository cluster directory: ${clusterDirectory.root}"
    }
    require(clusterDirectory.manifestPath.isRegularFile()) { "Cluster ${clusterDirectory.root.name} has no cluster.json" }
    val manifest = EdictNextJson.decodeFromString<EdictNextClusterManifest>(clusterDirectory.manifestPath.readText())
    require(manifest.id == clusterDirectory.root.name) {
      "Cluster directory '${clusterDirectory.root.name}' does not match manifest id '${manifest.id}'"
    }
    require(manifest.promotions.distinctBy(InspectionPromotion::id).size == manifest.promotions.size) {
      "Cluster '${manifest.id}' contains duplicate promotion ids"
    }
    manifest.promotions.forEach(::validatePromotion)
    return EdictNextStoredCluster(
      directory = clusterDirectory,
      manifest = manifest,
      signals = loadSignals(clusterDirectory.signalsDirectory),
      examples = loadExamples(clusterDirectory),
      candidateInspectionPath = paths.candidateInspectionPath(manifest.id),
    )
  }

  fun loadInboxSignals(): List<EdictNextSignal> = loadSignals(paths.inboxDirectory)

  fun clusterSignalIds(clusterId: String): Set<String> {
    require(EDICT_NEXT_KEBAB_CASE.matches(clusterId)) { "Invalid cluster id '$clusterId'" }
    val clusterDirectory = EdictNextClusterDirectory(paths.clustersDirectory.resolve(clusterId))
    require(clusterDirectory.manifestPath.isRegularFile()) { "Cluster '$clusterId' does not exist" }
    val manifest = EdictNextJson.decodeFromString<EdictNextClusterManifest>(clusterDirectory.manifestPath.readText())
    require(manifest.id == clusterId) { "Cluster manifest id '${manifest.id}' does not match '$clusterId'" }
    return loadSignals(clusterDirectory.signalsDirectory).mapTo(linkedSetOf(), EdictNextSignal::id)
  }

  fun saveCodeExample(
    clusterId: String,
    exampleId: String,
    metadata: EdictNextCodeExampleMetadata,
    sourceCode: String,
  ) {
    require(EDICT_NEXT_KEBAB_CASE.matches(clusterId)) { "Invalid cluster id '$clusterId'" }
    require(EDICT_NEXT_KEBAB_CASE.matches(exampleId)) { "Invalid example id '$exampleId'" }
    require(metadata.id == exampleId) { "Example metadata id '${metadata.id}' does not match '$exampleId'" }
    val clusterDirectory = EdictNextClusterDirectory(paths.clustersDirectory.resolve(clusterId))
    require(clusterDirectory.manifestPath.isRegularFile()) { "Cluster '$clusterId' does not exist" }
    val manifest = EdictNextJson.decodeFromString<EdictNextClusterManifest>(clusterDirectory.manifestPath.readText())
    require(manifest.status == EdictNextClusterStatus.Pending) { "Cluster '$clusterId' is not Pending" }
    val fileName = Path.of(metadata.fileName)
    require(!fileName.isAbsolute && fileName.nameCount == 1 && metadata.fileName == fileName.fileName.toString()) {
      "Example source file must be a single relative file name"
    }
    require(fileName.toString().substringAfterLast('.', "") == manifest.language.fileExtension) {
      "Example source language does not match cluster '$clusterId'"
    }
    clusterDirectory.examplesDirectory.createDirectories()
    val temporaryRoot = Files.createTempDirectory(clusterDirectory.root, ".edict-example-")
    try {
      val temporary = EdictNextExampleDirectory(temporaryRoot)
      temporary.projectDirectory.createDirectories()
      temporary.sourcePath(metadata.fileName).writeText(sourceCode)
      write(temporary.metadataPath, metadata, EdictNextCodeExampleMetadata.serializer())
      val target = clusterDirectory.examplesDirectory.resolve(exampleId)
      if (target.exists()) target.toFile().deleteRecursively()
      Files.move(temporaryRoot, target, StandardCopyOption.ATOMIC_MOVE)
    }
    finally {
      temporaryRoot.toFile().deleteRecursively()
    }
  }

  fun assignCodeExample(clusterId: String, signalId: String, exampleId: String) {
    val cluster = loadCluster(clusterId)
    val signal = cluster.signals.singleOrNull { it.id == signalId }
                 ?: error("Signal '$signalId' does not belong to cluster '$clusterId'")
    val example = cluster.examples.singleOrNull { it.metadata.id == exampleId }
                  ?: error("Code example '$exampleId' does not belong to cluster '$clusterId'")
    require(signal.label == example.metadata.label) {
      "Signal '$signalId' and example '$exampleId' have different labels"
    }
    val signalPath = cluster.directory.signalPath(signalId)
    val document = EdictNextJson.parseToJsonElement(signalPath.readText()).jsonObject
    signalPath.writeText(
      EdictNextJson.encodeToString(
        JsonObject.serializer(),
        JsonObject(document + ("syntheticExampleId" to JsonPrimitive(exampleId))),
      ),
    )
  }

  fun deleteCodeExample(clusterId: String, exampleId: String) {
    require(EDICT_NEXT_KEBAB_CASE.matches(clusterId)) { "Invalid cluster id '$clusterId'" }
    require(EDICT_NEXT_KEBAB_CASE.matches(exampleId)) { "Invalid example id '$exampleId'" }
    val clusterDirectory = EdictNextClusterDirectory(paths.clustersDirectory.resolve(clusterId))
    loadSignals(clusterDirectory.signalsDirectory).forEach { signal ->
      require(signal.syntheticExampleId != exampleId) {
        "Code example '$exampleId' is still assigned to Signal '${signal.id}'"
      }
    }
    val exampleDirectory = clusterDirectory.examplesDirectory.resolve(exampleId)
    require(exampleDirectory.isDirectory()) { "Code example '$exampleId' does not exist" }
    exampleDirectory.toFile().deleteRecursively()
  }

  fun saveCandidateInspection(clusterId: String, code: String) {
    require(loadCluster(clusterId).manifest.status == EdictNextClusterStatus.Pending) {
      "Cluster '$clusterId' is not Pending"
    }
    paths.candidateInspectionPath(clusterId).also { path ->
      path.parent.createDirectories()
      path.writeText(code)
    }
  }

  fun appendHistory(clusterId: String, entry: String) {
    require(entry.isNotBlank()) { "History entry must not be blank" }
    val cluster = loadCluster(clusterId)
    require(cluster.manifest.status == EdictNextClusterStatus.Pending) { "Cluster '$clusterId' is not Pending" }
    Files.writeString(
      cluster.historyPath,
      entry.trimEnd() + "\n",
      StandardOpenOption.CREATE,
      StandardOpenOption.APPEND,
    )
  }

  fun markGenerated(cluster: EdictNextStoredCluster, selectedInspection: Path) {
    require(selectedInspection.isRegularFile()) { "Selected inspection does not exist: $selectedInspection" }
    val finalInspection = paths.inspectionPath(cluster.id)
    finalInspection.parent.createDirectories()
    val previousFinalCode = if (selectedInspection != finalInspection && finalInspection.isRegularFile()) {
      finalInspection.readBytes()
    }
    else {
      null
    }
    if (selectedInspection != finalInspection) {
      Files.copy(selectedInspection, finalInspection, StandardCopyOption.REPLACE_EXISTING)
    }
    try {
      write(
        cluster.directory.manifestPath,
        cluster.manifest.copy(status = EdictNextClusterStatus.Generated, predecessorId = null),
        EdictNextClusterManifest.serializer(),
      )
    }
    catch (e: Exception) {
      if (selectedInspection != finalInspection) {
        try {
          if (previousFinalCode == null) {
            finalInspection.deleteIfExists()
          }
          else {
            finalInspection.writeBytes(previousFinalCode)
          }
        }
        catch (rollbackError: Exception) {
          e.addSuppressed(rollbackError)
        }
      }
      throw e
    }
    if (cluster.candidateInspectionPath != finalInspection) {
      cluster.candidateInspectionPath.deleteIfExists()
    }
    cluster.manifest.predecessorId?.let(paths::inspectionPath)?.takeIf { it != finalInspection }?.deleteIfExists()
  }

  @Synchronized
  fun appendPromotion(
    clusterId: String,
    expectedInspectionDigest: String,
    promotion: InspectionPromotion,
  ): InspectionPromotion {
    val cluster = loadCluster(clusterId)
    require(cluster.manifest.status == EdictNextClusterStatus.Generated) { "Cluster '$clusterId' is not Generated" }
    val inspection = paths.inspectionPath(clusterId)
    require(inspection.isRegularFile()) { "Generated inspection for '$clusterId' does not exist" }
    require(sha256Hex(inspection.readBytes()) == expectedInspectionDigest) {
      "Generated inspection for '$clusterId' changed while promotion was being created"
    }
    validatePromotion(promotion)
    cluster.manifest.promotions.firstOrNull { it.id == promotion.id }?.let { existing ->
      require(existing == promotion) { "Promotion id '${promotion.id}' already identifies a different promotion" }
      return existing
    }
    require(cluster.manifest.promotions.none {
      it.inspectionDigest == promotion.inspectionDigest &&
        it.targetBranch == promotion.targetBranch &&
        it.targetPath == promotion.targetPath &&
        it.pullRequest.provider == promotion.pullRequest.provider &&
        it.pullRequest.owner == promotion.pullRequest.owner &&
        it.pullRequest.repository == promotion.pullRequest.repository
    }) { "This inspection version has already been promoted to the configured target" }
    writeAtomically(
      cluster.directory.manifestPath,
      cluster.manifest.copy(promotions = cluster.manifest.promotions + promotion),
      EdictNextClusterManifest.serializer(),
    )
    return promotion
  }

  @Synchronized
  fun applyResolvedPromotion(
    clusterId: String,
    promotionId: String,
    resolvedState: PromotionPrState,
    updatedAt: String,
    expectedInspectionDigest: String? = null,
    targetStatus: EdictNextClusterStatus? = null,
    rationale: String? = null,
  ): InspectionPromotion {
    val cluster = loadCluster(clusterId)
    val index = cluster.manifest.promotions.indexOfFirst { it.id == promotionId }
    require(index >= 0) { "Unknown promotion '$promotionId' in cluster '$clusterId'" }
    val current = cluster.manifest.promotions[index]
    val updated = current.copy(state = resolvedState, updatedAt = updatedAt)
    val inspection = paths.inspectionPath(clusterId)
    val updatedManifest: EdictNextClusterManifest
    val historyEntry: String

    when (resolvedState) {
      PromotionPrState.ACCEPTED -> {
        require(current.state == PromotionPrState.ON_REVIEW) {
          "Illegal promotion transition ${current.state} -> $resolvedState"
        }
        require(expectedInspectionDigest == null && targetStatus == null && rationale == null) {
          "Accepted promotions cannot change the cluster"
        }
        updatedManifest = cluster.manifest.copy(
          promotions = cluster.manifest.promotions.toMutableList().also { it[index] = updated },
        )
        historyEntry = "Promotion `$promotionId` accepted after merged PR ${current.pullRequest.url}.\n"
      }
      PromotionPrState.CLOSED -> {
        val resolvedTargetStatus = requireNotNull(targetStatus) { "Promotion decision target status is required" }
        val resolvedRationale = requireNotNull(rationale) { "Promotion decision rationale is required" }
        require(resolvedTargetStatus in setOf(EdictNextClusterStatus.Pending, EdictNextClusterStatus.Discontinued)) {
          "Promotion decisions may move a cluster only to Pending or Discontinued"
        }
        require(resolvedRationale.isNotBlank()) { "Promotion decision rationale is required" }
        require(cluster.manifest.status == EdictNextClusterStatus.Generated) { "Cluster '$clusterId' is not Generated" }
        require(current.state in setOf(PromotionPrState.ON_REVIEW, PromotionPrState.CLOSED)) {
          "Promotion '$promotionId' is not awaiting a closed-review decision"
        }
        require(current.inspectionDigest == expectedInspectionDigest) {
          "Promotion does not identify the expected inspection version"
        }
        require(inspection.isRegularFile()) { "Generated inspection for '$clusterId' does not exist" }
        require(sha256Hex(inspection.readBytes()) == expectedInspectionDigest) {
          "Promotion is stale: the generated inspection has changed"
        }
        updatedManifest = cluster.manifest.copy(
          status = resolvedTargetStatus,
          predecessorId = cluster.id.takeIf { resolvedTargetStatus == EdictNextClusterStatus.Pending },
          promotions = cluster.manifest.promotions.toMutableList().also { it[index] = updated },
        )
        historyEntry = "Promotion `$promotionId` moved cluster to $resolvedTargetStatus: ${resolvedRationale.trim()}\n"
      }
      PromotionPrState.ON_REVIEW, PromotionPrState.DECLINED -> {
        error("Promotion resolution cannot set state to $resolvedState")
      }
    }
    validatePromotion(updated)
    val history = cluster.historyPath
    val originalHistory = history.takeIf(Path::isRegularFile)?.readBytes()
    val backup = if (resolvedState == PromotionPrState.CLOSED && targetStatus == EdictNextClusterStatus.Discontinued) {
      Files.createTempFile(inspection.parent, ".edict-discontinued-", ".inspection.kts")
    }
    else null
    try {
      backup?.let { Files.move(inspection, it, StandardCopyOption.REPLACE_EXISTING) }
      try {
        writeAtomically(
          cluster.directory.manifestPath,
          updatedManifest,
          EdictNextClusterManifest.serializer(),
        )
        Files.writeString(
          history,
          historyEntry,
          StandardOpenOption.CREATE,
          StandardOpenOption.APPEND,
        )
      }
      catch (e: Exception) {
        writeAtomically(cluster.directory.manifestPath, cluster.manifest, EdictNextClusterManifest.serializer())
        if (originalHistory == null) history.deleteIfExists() else history.writeBytes(originalHistory)
        backup?.takeIf(Path::exists)?.let { Files.move(it, inspection, StandardCopyOption.REPLACE_EXISTING) }
        throw e
      }
      backup?.deleteIfExists()
    }
    finally {
      if (backup?.exists() == true && !inspection.exists()) {
        Files.move(backup, inspection, StandardCopyOption.REPLACE_EXISTING)
      }
      else {
        backup?.deleteIfExists()
      }
    }
    return updated
  }

  /**
   * Moves a Pending cluster to [newId] with its Signals, examples, and history, and stores [candidateCode] as the candidate
   * under the new id when one is given. The predecessor stays under its own id.
   */
  fun renameCluster(cluster: EdictNextStoredCluster, newId: String, candidateCode: String?) {
    require(cluster.manifest.status == EdictNextClusterStatus.Pending) { "Cluster '${cluster.id}' is not Pending" }
    require(EDICT_NEXT_KEBAB_CASE.matches(newId)) { "Invalid cluster id '$newId'" }
    val target = EdictNextClusterDirectory(paths.clustersDirectory.resolve(newId))
    require(!target.root.exists()) { "Cluster '$newId' already exists" }
    require(!paths.inspectionPath(newId).exists() && !paths.candidateInspectionPath(newId).exists()) {
      "An inspection with id '$newId' already exists"
    }
    Files.move(cluster.directory.root, target.root, StandardCopyOption.ATOMIC_MOVE)
    write(target.manifestPath, cluster.manifest.copy(id = newId), EdictNextClusterManifest.serializer())
    if (candidateCode != null) {
      paths.candidateInspectionPath(newId).writeText(candidateCode)
      cluster.candidateInspectionPath.deleteIfExists()
    }
  }

  /** A Discontinued cluster keeps no inspection: its candidate and predecessor go. */
  fun markDiscontinued(cluster: EdictNextStoredCluster) {
    write(
      cluster.directory.manifestPath,
      cluster.manifest.copy(status = EdictNextClusterStatus.Discontinued, predecessorId = null),
      EdictNextClusterManifest.serializer(),
    )
    cluster.candidateInspectionPath.deleteIfExists()
    cluster.manifest.predecessorId?.let(paths::inspectionPath)?.deleteIfExists()
  }

  /** An Invalid cluster keeps its candidate and predecessor, so a later run can continue from them. */
  fun markInvalid(cluster: EdictNextStoredCluster) {
    write(
      cluster.directory.manifestPath,
      cluster.manifest.copy(status = EdictNextClusterStatus.Invalid),
      EdictNextClusterManifest.serializer(),
    )
  }

  fun addSignalToCluster(signalId: String, clusterId: String) {
    val signal = loadInboxSignals().singleOrNull { it.id == signalId } ?: error("Inbox Signal '$signalId' does not exist")
    val clusterDirectory = EdictNextClusterDirectory(paths.clustersDirectory.resolve(clusterId))

    if (!clusterDirectory.root.exists()) {
      createCluster(clusterDirectory, signal)
      return
    }

    val cluster = loadCluster(clusterId)
    require(signal.language == cluster.manifest.language) { "Signal language does not match cluster '${cluster.id}'" }

    cluster.directory.signalsDirectory.createDirectories()
    Files.move(paths.inboxDirectory.resolve("$signalId.json"), cluster.directory.signalPath(signalId))

    val predecessorId = when (cluster.manifest.status) {
      EdictNextClusterStatus.Generated -> cluster.id
      EdictNextClusterStatus.Pending, EdictNextClusterStatus.Invalid -> cluster.manifest.predecessorId
      EdictNextClusterStatus.Discontinued -> null
    }
    write(
      cluster.directory.manifestPath,
      cluster.manifest.copy(status = EdictNextClusterStatus.Pending, predecessorId = predecessorId),
      EdictNextClusterManifest.serializer(),
    )
  }

  private fun createCluster(cluster: EdictNextClusterDirectory, signal: EdictNextSignal) {
    cluster.root.createDirectories()
    cluster.signalsDirectory.createDirectories()
    Files.move(paths.inboxDirectory.resolve("${signal.id}.json"), cluster.signalPath(signal.id))
    write(
      cluster.manifestPath,
      EdictNextClusterManifest(cluster.root.name, signal.language, EdictNextClusterStatus.Pending),
      EdictNextClusterManifest.serializer()
    )
    cluster.historyPath.writeText("Created for Signal `${signal.id}`.\n")
  }

  private fun loadExamples(clusterDirectory: EdictNextClusterDirectory): List<EdictNextStoredExample> {
    if (!clusterDirectory.examplesDirectory.isDirectory()) return emptyList()
    val issues = mutableListOf<String>()
    val examples = clusterDirectory.examplesDirectory.listDirectoryEntries().filter(Path::isDirectory).sortedBy(Path::name).mapNotNull { path ->
      val directory = EdictNextExampleDirectory(path)
      try {
        require(directory.metadataPath.isRegularFile()) { "Code example ${directory.root.name} has no metadata.json" }
        val metadata = EdictNextJson.decodeFromString<EdictNextCodeExampleMetadata>(directory.metadataPath.readText())
        require(metadata.id == directory.root.name) {
          "Code example directory '${directory.root.name}' does not match metadata id '${metadata.id}'"
        }
        val sourcePath = directory.sourcePath(metadata.fileName)
        require(sourcePath.isRegularFile()) { "Code example '${metadata.id}' has no source file ${metadata.fileName}" }
        EdictNextStoredExample(directory, metadata, sourcePath, sourcePath.readText())
      }
      catch (e: Exception) {
        issues += "Cannot read code example '${directory.root}': ${e.message ?: e}"
        null
      }
    }
    if (issues.isNotEmpty()) throw EdictNextCodeExampleReadException(issues)
    return examples
  }

  private fun loadSignals(directory: Path): List<EdictNextSignal> {
    if (!directory.isDirectory()) return emptyList()
    return directory.listDirectoryEntries("*.json")
      .filter(Path::isRegularFile).sortedBy(Path::name)
      .map { path ->
        EdictNextJson.decodeFromString<EdictNextSignal>(path.readText()).also { signal ->
          require(path.fileName.toString() == "${signal.id}.json") {
            "Signal file '${path.fileName}' does not match Signal id '${signal.id}'"
          }
        }
    }
  }

  private fun <T> write(path: Path, value: T, serializer: SerializationStrategy<T>) {
    path.parent.createDirectories()
    path.writeText(EdictNextJson.encodeToString(serializer, value))
  }

  private fun <T> writeAtomically(path: Path, value: T, serializer: SerializationStrategy<T>) {
    path.parent.createDirectories()
    val temporary = Files.createTempFile(path.parent, ".${path.fileName}.", ".tmp")
    try {
      temporary.writeText(EdictNextJson.encodeToString(serializer, value))
      try {
        Files.move(temporary, path, StandardCopyOption.ATOMIC_MOVE, StandardCopyOption.REPLACE_EXISTING)
      }
      catch (_: java.nio.file.AtomicMoveNotSupportedException) {
        Files.move(temporary, path, StandardCopyOption.REPLACE_EXISTING)
      }
    }
    finally {
      temporary.deleteIfExists()
    }
  }

  private suspend fun loadRepositoryFiles(): Map<String, ByteArray> {
    val gitIgnoredFiles = loadGitIgnoredFiles()
    return withContext(Dispatchers.IO) {
      Files.walk(paths.root).use { files ->
        files.filter(Files::isRegularFile)
          .filter { path ->
            val relative = paths.root.relativize(path)
            !relative.startsWith(GIT_PATH) &&
              !relative.startsWith(EMBEDDINGS_PATH) &&
              !relative.startsWith(PLANS_PATH) &&
              relative.fileName.toString() !in MANAGEMENT_FILES &&
              relative !in gitIgnoredFiles
          }
          .sorted()
          .toList()
          .associate { path -> paths.root.relativize(path).toString() to path.readBytes() }
      }
    }
  }

  private suspend fun loadGitIgnoredFiles(): Set<Path> {
    return withContext(Dispatchers.IO) {
      val repository = GitRepository(paths.root)
      val edictRoot = paths.root.toAbsolutePath().normalize()
      repository.git("ls-files", "-z", "--ignored", "--others", "--exclude-standard")
        .splitToSequence('\u0000')
        .filter(String::isNotEmpty)
        .map { relative -> repository.root.resolve(relative).normalize() }
        .filter { path -> path.startsWith(edictRoot) }
        .map { path -> edictRoot.relativize(path) }
        .toSet()
    }
  }

  private companion object {
    val GIT_PATH: Path = Path.of(".git")
    val EMBEDDINGS_PATH: Path = Path.of("embeddings")
    val PLANS_PATH: Path = Path.of("plans")
    val MANAGEMENT_FILES: Set<String> = setOf(".edict-mcp.lock", ".edict-mcp-current")
  }
}

internal class EdictNextCodeExampleReadException(val issues: List<String>) : IllegalArgumentException()

internal data class EdictNextStoredCluster(
  val directory: EdictNextClusterDirectory,
  val manifest: EdictNextClusterManifest,
  val signals: List<EdictNextSignal>,
  val examples: List<EdictNextStoredExample>,
  val candidateInspectionPath: Path,
) {
  val id: String get() = manifest.id
  val signalIds: Set<String> get() = signals.mapTo(LinkedHashSet(), EdictNextSignal::id)
  val historyPath: Path get() = directory.historyPath
}

internal data class EdictNextStoredExample(
  val directory: EdictNextExampleDirectory,
  val metadata: EdictNextCodeExampleMetadata,
  val sourcePath: Path,
  val code: String,
)
