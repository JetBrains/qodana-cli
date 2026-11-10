package org.jetbrains.qodana.edict.core

import java.nio.file.Path
import kotlin.io.path.isDirectory

internal data class EdictRepositoryDirectory(val root: Path) {
  val clustersDirectory: Path = root.resolve("clusters")
  val inboxDirectory: Path = root.resolve("inbox")
  val inspectionsDirectory: Path = root.resolve("inspections")
  val promotionsDirectory: Path = root.resolve("promotions")
  val activeWeakSignalPromotionsPath: Path = promotionsDirectory.resolve("active.json")

  /** Findings reserved for review comments that await an answer, one `<record id>.json` each. */
  val activeFeedbackDirectory: Path = root.resolve("feedback").resolve("active")

  /** Answered or abandoned feedback records, in a `<yyyy-MM>` directory of the month they were reserved in. */
  val feedbackHistoryDirectory: Path = root.resolve("feedback").resolve("history")

  fun weakSignalPromotionHistoryPath(clusterId: String): Path {
    require(EDICT_KEBAB_CASE.matches(clusterId)) { "Invalid cluster id '$clusterId'" }
    return promotionsDirectory.resolve("history").resolve("$clusterId.json")
  }

  val embeddingsDirectory: Path = root.resolve("embeddings")
  val gteEmbeddingCacheDirectory: Path = embeddingsDirectory.resolve("gte-large-$EMBEDDING_MODEL_REVISION")

  init {
    require(root.isDirectory()) { "Edict repository does not exist or is not a directory: $root" }
  }

  fun inspectionPath(ruleId: String): Path = inspectionsDirectory.resolve(ruleId + EDICT_INSPECTION_SUFFIX)

  fun candidateInspectionPath(clusterId: String): Path = inspectionsDirectory.resolve(clusterId + EDICT_CANDIDATE_SUFFIX)
}

internal data class EdictClusterDirectory(val root: Path) {
  val manifestPath: Path = root.resolve("cluster.json")
  val signalsDirectory: Path = root.resolve("signals")
  val examplesDirectory: Path = root.resolve("synthetic-examples")
  val historyPath: Path = root.resolve("history.md")
  val evaluationPath: Path = root.resolve("evaluation.json")

  fun signalPath(signalId: String): Path = signalsDirectory.resolve("$signalId.json")
}

internal data class EdictExampleDirectory(val root: Path) {
  val metadataPath: Path = root.resolve("metadata.json")
  val projectDirectory: Path = root.resolve("project").toAbsolutePath().normalize()

  fun sourcePath(fileName: String): Path = projectDirectory.resolve(fileName).normalize()
}

/** GTE model files shared by every run on this machine, next to the Edict tooling the Qodana CLI extracts. */
internal fun edictModelDirectory(): Path = userCacheDirectory()
  .resolve("JetBrains").resolve("Qodana").resolve("edict").resolve("models").resolve("gte-large-$EMBEDDING_MODEL_REVISION")

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
