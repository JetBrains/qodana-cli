package org.jetbrains.qodana.edict.edictnext

import com.intellij.ml.llm.qodana.agents.edictnext.EDICT_NEXT_CANDIDATE_SUFFIX
import com.intellij.ml.llm.qodana.agents.edictnext.EDICT_NEXT_INSPECTION_SUFFIX
import java.nio.file.Path
import kotlin.io.path.createDirectories
import kotlin.io.path.isDirectory
import kotlin.io.path.writeText

internal data class EdictRepositoryDirectory(val root: Path) {
  val clustersDirectory: Path = root.resolve("clusters")
  val inboxDirectory: Path = root.resolve("inbox")
  val inspectionsDirectory: Path = root.resolve("inspections")

  /** The durable embedding cache: a run imports it before distribution and exports it back before commit. */
  val embeddingsDirectory: Path = root.resolve("embeddings")

  init {
    require(root.isDirectory()) { "Edict repository does not exist or is not a directory: $root" }
  }

  fun inspectionPath(ruleId: String): Path = inspectionsDirectory.resolve(ruleId + EDICT_NEXT_INSPECTION_SUFFIX)

  fun candidateInspectionPath(clusterId: String): Path = inspectionsDirectory.resolve(clusterId + EDICT_NEXT_CANDIDATE_SUFFIX)
}

internal data class EdictNextWorkspace(val root: Path) {
  val agentsDirectory: Path = root.resolve(".agents")
  val codexLogDirectory: Path = root.resolve("codex")
  val worktree: Path = root.resolve("worktree")
  val script: EdictNextScriptDirectory = EdictNextScriptDirectory(root.resolve("script"))

  fun analysisDirectory(clusterId: String, attemptId: String): Path =
    root.resolve("analysis").resolve(clusterId).resolve(attemptId)

  companion object {
    fun forRun(baseLogDirectory: Path, runId: String): EdictNextWorkspace =
      EdictNextWorkspace(baseLogDirectory.resolve("edict-next").resolve(runId))
  }
}

/** The retrieval script's requests, responses, and logs, kept in the run's log directory as its audit trail. */
internal data class EdictNextScriptDirectory(val root: Path) {
  val requestPath: Path = root.resolve("neighbours.request.json")

  val responsePath: Path = root.resolve("neighbours.response.json")

  fun logPath(name: String): Path = root.resolve("$name.log")

  fun writeRequest(content: String) {
    root.createDirectories()
    requestPath.writeText(content)
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

internal data class EdictNextAnalysisOutput(val root: Path) {
  fun batchDirectory(batchId: Int): Path = root.resolve("batched-analysis").resolve("batch-$batchId")
}
