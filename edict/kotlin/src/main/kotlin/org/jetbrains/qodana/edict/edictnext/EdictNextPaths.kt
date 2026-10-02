package org.jetbrains.qodana.edict.edictnext

import java.nio.file.Path
import kotlin.io.path.isDirectory

internal data class EdictRepositoryDirectory(val root: Path) {
  val clustersDirectory: Path = root.resolve("clusters")
  val inboxDirectory: Path = root.resolve("inbox")
  val inspectionsDirectory: Path = root.resolve("inspections")

  val embeddingsDirectory: Path = root.resolve("embeddings")
  val gteEmbeddingCacheDirectory: Path = embeddingsDirectory.resolve("gte-large-$EDICT_NEXT_MODEL_REVISION")

  init {
    require(root.isDirectory()) { "Edict repository does not exist or is not a directory: $root" }
  }

  fun inspectionPath(ruleId: String): Path = inspectionsDirectory.resolve(ruleId + EDICT_NEXT_INSPECTION_SUFFIX)

  fun candidateInspectionPath(clusterId: String): Path = inspectionsDirectory.resolve(clusterId + EDICT_NEXT_CANDIDATE_SUFFIX)
}

internal data class EdictNextWorkspace(val root: Path) {
  val worktree: Path = root.resolve("worktree")
  val neighboursResponsePath: Path = root.resolve("neighbours.response.json")

  companion object {
    fun forRun(baseLogDirectory: Path, runId: String): EdictNextWorkspace =
      EdictNextWorkspace(baseLogDirectory.resolve("edict-next").resolve(runId))
  }
}

internal data class EdictNextClusterDirectory(val root: Path) {
  val manifestPath: Path = root.resolve("cluster.json")
  val signalsDirectory: Path = root.resolve("signals")
  val examplesDirectory: Path = root.resolve("synthetic-examples")
  val historyPath: Path = root.resolve("history.md")

  fun signalPath(signalId: String): Path = signalsDirectory.resolve("$signalId.json")
}

internal data class EdictNextExampleDirectory(val root: Path) {
  val metadataPath: Path = root.resolve("metadata.json")
  val projectDirectory: Path = root.resolve("project").toAbsolutePath().normalize()

  fun sourcePath(fileName: String): Path = projectDirectory.resolve(fileName).normalize()
}

/** GTE model files shared by every run on this machine, next to the Edict tooling the Qodana CLI extracts. */
internal fun edictNextModelDirectory(): Path = userCacheDirectory()
  .resolve("JetBrains").resolve("Qodana").resolve("edict").resolve("models").resolve("gte-large-$EDICT_NEXT_MODEL_REVISION")

/** Resolves the per-user cache root like Go's `os.UserCacheDir`. */
internal fun userCacheDirectory(): Path {
  val os = System.getProperty("os.name").lowercase()
  return when {
    os.startsWith("windows") -> Path.of(environmentVariable("LocalAppData") ?: error("%LocalAppData% is not defined"))
    os.startsWith("mac") -> homeDirectory().resolve("Library").resolve("Caches")
    else -> xdgCacheHome() ?: homeDirectory().resolve(".cache")
  }
}

private fun environmentVariable(name: String): String? = System.getenv(name)?.takeIf(String::isNotEmpty)

private fun homeDirectory(): Path = Path.of(environmentVariable("HOME") ?: error($$"$HOME is not defined"))

private fun xdgCacheHome(): Path? = environmentVariable("XDG_CACHE_HOME")?.let(Path::of)?.also { path ->
  require(path.isAbsolute) { $$"Path in $XDG_CACHE_HOME is relative: $$path" }
}
