package org.jetbrains.qodana.edict.edictnext

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.SerializationStrategy
import org.jetbrains.qodana.edict.git.GitRepository
import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.StandardCopyOption
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
    return EdictNextStoredCluster(
      directory = clusterDirectory,
      manifest = manifest,
      signals = loadSignals(clusterDirectory.signalsDirectory),
      examples = loadExamples(clusterDirectory),
      candidateInspectionPath = paths.candidateInspectionPath(manifest.id),
    )
  }

  fun loadInboxSignals(): List<EdictNextSignal> = loadSignals(paths.inboxDirectory)

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

  private suspend fun loadRepositoryFiles(): Map<String, ByteArray> {
    val gitIgnoredFiles = loadGitIgnoredFiles()
    return withContext(Dispatchers.IO) {
      Files.walk(paths.root).use { files ->
        files.filter(Files::isRegularFile)
          .filter { path ->
            val relative = paths.root.relativize(path)
            !relative.startsWith(GIT_PATH) && !relative.startsWith(EMBEDDINGS_PATH) && relative !in gitIgnoredFiles
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

internal data class EdictNextRepositoryState(
  val root: Path,
  val clusters: List<EdictNextStoredCluster>,
  val inboxSignals: List<EdictNextSignal>,
  val filesByRelativePath: Map<String, ByteArray>,
) {
  val clustersById: Map<String, EdictNextStoredCluster> = clusters.associateBy(EdictNextStoredCluster::id)
  val inboxSignalsById: Map<String, EdictNextSignal> = inboxSignals.associateBy(EdictNextSignal::id)
  val inboxSignalIds: Set<String> = inboxSignals.mapTo(LinkedHashSet(), EdictNextSignal::id)
  val jvmInboxSignalIds: Set<String> = inboxSignals.filter(EdictNextSignal::isJvmLanguage)
    .mapTo(LinkedHashSet(), EdictNextSignal::id)
  val signalsById: Map<String, EdictNextSignal> = (clusters.flatMap(EdictNextStoredCluster::signals) + inboxSignals)
    .associateBy(EdictNextSignal::id)

  fun relativePath(path: Path): String = root.relativize(path).toString()

  fun inspectionCode(clusterId: String): String? = filesByRelativePath[
    "inspections/$clusterId$EDICT_NEXT_INSPECTION_SUFFIX"
  ]?.decodeToString()
}
