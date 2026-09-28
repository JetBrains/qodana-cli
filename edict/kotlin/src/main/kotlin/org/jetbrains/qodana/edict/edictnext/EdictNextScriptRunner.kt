package org.jetbrains.qodana.edict.edictnext

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.withContext
import org.jetbrains.qodana.edict.edictnext.EdictScriptRunner.EdictNextResourceInstaller.readEdictNextResource
import java.nio.file.Path
import kotlin.io.path.copyTo
import kotlin.io.path.createDirectories
import kotlin.io.path.createTempDirectory
import kotlin.io.path.isDirectory
import kotlin.io.path.listDirectoryEntries
import kotlin.io.path.readText
import kotlin.io.path.writeBytes
import kotlin.io.resolve
import kotlin.io.writeBytes

/**
 * Owns the retrieval script end to end: its own temporary directory, the venv, the embedding cache, and the
 * process runs. The agent supplies no path, never sees the script, and gets only a decoded response.
 */
internal class EdictScriptRunner(private val workspace: EdictNextWorkspace) {
  private var home: Path? = null
  private var prepared: PreparedScript? = null

  /** Installs the retrieval script and its dependencies in a fresh directory. */
  suspend fun prepareEnvironment() {
    val directory = withContext(Dispatchers.IO) { createTempDirectory("edict-next-script") }
    home = directory
    val entry = writeResource(SCRIPT_RESOURCE, directory.resolve(SCRIPT_FILE_NAME))
    val requirements = writeResource(REQUIREMENTS_RESOURCE, directory.resolve(REQUIREMENTS_FILE_NAME))
    val python = createVenv(directory, requirements)
    prepared = PreparedScript(entry, python, directory.resolve("embeddings"))
  }

  /** Builds the retrieval index once and returns the nearest neighbours for every selected inbox Signal. */
  suspend fun prepareCorpus(
    corpus: List<EdictNextSignal>,
    signalIds: List<String>,
  ): Map<String, EdictNextSignalNeighbours> {
    val script = requirePrepared()
    val request = EdictNextCorpusIndexRequest(
      model = EDICT_NEXT_MODEL,
      modelRevision = EDICT_NEXT_MODEL_REVISION,
      corpus = corpus,
      signalIds = signalIds,
      neighbourCount = EDICT_NEXT_NEIGHBOUR_COUNT,
    )
    val responsePath = workspace.script.responsePath
    workspace.script.writeRequest(EdictNextJson.encodeToString(EdictNextCorpusIndexRequest.serializer(), request))
    execute(
      listOf(
        script.python.toString(),
        script.entry.toString(),
        "--request", workspace.script.requestPath.toString(),
        "--output", responsePath.toString(),
        "--cache", script.embeddingCache.toString(),
      ),
      logName = "prepare-corpus",
    )
    return EdictNextJson.decodeFromString(EdictNextNeighboursResponse.serializer(), responsePath.readText())
      .neighbours.associateBy(EdictNextSignalNeighbours::signalId)
  }

  /** Seeds the script's cache from the repository, so a run re-embeds only what the corpus gained. */
  suspend fun importEmbeddingCache(source: Path) {
    copyEmbeddings(source, requirePrepared().embeddingCache)
  }

  /** Writes the cache back into the working copy, so the next run inherits it through the commit. */
  suspend fun exportEmbeddingCache(target: Path) {
    copyEmbeddings(requirePrepared().embeddingCache, target)
  }

  /** Deletes the run's directory: the wheels and the model stay in the shared pip and Hugging Face caches. */
  fun deleteEnvironment() {
    prepared = null
    val directory = home
    home = null
    if (directory != null) directory.toFile().deleteRecursively()
  }

  private fun requirePrepared(): PreparedScript =
    checkNotNull(prepared) { "The retrieval environment was not prepared before the run" }

  private suspend fun copyEmbeddings(source: Path, target: Path) {
    withContext(Dispatchers.IO) {
      if (!source.isDirectory()) {
        return@withContext
      }
      target.createDirectories()
      source.listDirectoryEntries("*$EMBEDDING_SUFFIX").forEach { entry ->
        entry.copyTo(target.resolve(entry.fileName), overwrite = true)
      }
    }
  }

  private suspend fun writeResource(resource: String, target: Path): Path =
    withContext(Dispatchers.IO) { target.also { it.writeBytes(readEdictNextResource(resource)) } }

  /** One venv per run, in the script's own directory: the agent can reach neither it nor the requirements. */
  private suspend fun createVenv(home: Path, requirements: Path): Path {
    val venv = home.resolve("venv")
    val python = venv.resolve("bin").resolve("python")
    execute(listOf(basePython(), "-m", "venv", venv.toString()), logName = "venv")
    execute(
      listOf(python.toString(), "-m", "pip", "install", "--quiet", "--disable-pip-version-check", "-r", requirements.toString()),
      logName = "pip-install",
    )
    return python
  }

  /**
   * Output goes to [logName] rather than a pipe: a blocking pipe read cannot be cancelled, so it would outlive
   * every timeout around it and prevent the kill that is the only thing able to end it.
   */
  private suspend fun execute(command: List<String>, logName: String) {
    val logPath = workspace.script.logPath(logName)
    logPath.parent.createDirectories()
    // NonCancellable: a cancellation landing on the start must not lose the handle the kill below needs.
    val process = withContext(NonCancellable + Dispatchers.IO) { start(command, logPath) }
    try {
      val exitCode = withContext(Dispatchers.IO) { process.waitFor() }
      System.err.println("Edict Next script: ${command.first()} exit=$exitCode, log=$logPath")
      check(exitCode == 0) {
        "Command failed with exit code $exitCode: ${command.joinToString(" ")}\n${logPath.readText().takeLast(OUTPUT_TAIL)}"
      }
    }
    finally {
      killTree(process)
    }
  }

  @Suppress("SSBasedInspection") // ProcessBuilder redirection has no nio overload.
  private fun start(command: List<String>, logPath: Path): Process =
    ProcessBuilder(command).redirectErrorStream(true).redirectOutput(logPath.toFile()).start()

  /** Deepest first and unconditional, so a parent that already exited cannot leave a pip child writing to the venv. */
  @Suppress("UseProcessDescendantsWithCare") // A local venv build: the tree is pip and python, both ours.
  private fun killTree(process: Process) {
    process.descendants().toList().asReversed().forEach { descendant -> descendant.destroyForcibly() }
    process.destroyForcibly()
  }

  private fun basePython(): String = System.getProperty("qodana.edictnext.python") ?: "python3"

  private data class PreparedScript(val entry: Path, val python: Path, val embeddingCache: Path)

  private companion object {
    const val SCRIPT_RESOURCE: String = "/edict-next/cluster.py"
    const val REQUIREMENTS_RESOURCE: String = "/edict-next/requirements.txt"
    const val SCRIPT_FILE_NAME: String = "cluster.py"
    const val REQUIREMENTS_FILE_NAME: String = "requirements.txt"
    const val EMBEDDING_SUFFIX: String = ".npy"
    const val OUTPUT_TAIL: Int = 4_000
  }


  internal object EdictNextResourceInstaller {
    fun manifestEntries(): List<String> = readEdictNextResource(MANIFEST_RESOURCE).decodeToString().lineSequence()
      .map(String::trim)
      .filter { it.isNotEmpty() && !it.startsWith('#') }
      .toList()

    fun install(workspace: EdictNextWorkspace) {
      val agentDirectory = workspace.agentsDirectory.also { it.createDirectories() }
      manifestEntries().forEach { resourcePath ->
        val parts = resourcePath.split('/')
        val destination = agentDirectory.resolve("skills").resolve(parts[3]).resolve(parts.drop(4).joinToString("/")).normalize()
        destination.parent.createDirectories()
        destination.writeBytes(readEdictNextResource("/$resourcePath"))
      }
    }


    internal fun readEdictNextResource(path: String): ByteArray =
      EdictNextResourceInstaller::class.java.getResourceAsStream(path)?.use { it.readAllBytes() }
        ?: throw IllegalStateException("Bundled Edict Next resource is missing: $path")

    private const val MANIFEST_RESOURCE: String = "/edict-next-resources.txt"
  }

}
