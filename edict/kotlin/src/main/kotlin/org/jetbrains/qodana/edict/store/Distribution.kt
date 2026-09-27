// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.store

import java.nio.file.Files
import java.nio.file.Path
import java.util.Comparator
import kotlinx.serialization.Serializable
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonPrimitive
import org.jetbrains.qodana.edict.common.json
import org.jetbrains.qodana.edict.common.runProcess
import org.jetbrains.qodana.edict.common.wireJson
import org.jetbrains.qodana.edict.model.FileRevision
import org.jetbrains.qodana.edict.model.Signal
import org.jetbrains.qodana.edict.model.SignalLabel
import org.jetbrains.qodana.edict.signals.SignalValidation

private const val EMBEDDING_MODEL = "thenlper/gte-large"
private const val EMBEDDING_MODEL_REVISION = "4bef63f39fcc5e2d6b0aae83089f307af4970164"
private const val NEIGHBOUR_COUNT = 10
private val clusterId = Regex("[a-z][a-z0-9]*(?:-[a-z0-9]+)*")

@Serializable
data class DistributionNeighbour(val signalId: String, val distance: Double)

@Serializable
data class SignalNeighbours(val signalId: String, val closest: List<DistributionNeighbour> = emptyList())

@Serializable
private data class NeighboursResponse(val neighbours: List<SignalNeighbours> = emptyList())

@Serializable
private data class EmbeddingRequest(
    val model: String,
    val modelRevision: String,
    val corpus: List<Signal>,
    val signalIds: List<String>,
    val neighbourCount: Int,
)

@Serializable
data class DistributionCandidate(
    val kind: String,
    val nearestDistance: Double,
    val clusterId: String? = null,
    val signalId: String? = null,
    val signalPath: String? = null,
)

@Serializable
data class NextSignalResponse(
    val signalId: String? = null,
    val signalPath: String? = null,
    val signal: Signal? = null,
    val candidates: List<DistributionCandidate> = emptyList(),
    val summary: String,
    val nextAction: String = if (signalId == null) "STOP_DISTRIBUTION" else "PROCESS_SIGNAL",
)

@Serializable
data class DistributionSignal(
    val id: String,
    val fileRevision: FileRevision,
    val label: SignalLabel,
    val description: String,
)

@Serializable
data class DistributionContextResponse(
    val kind: String,
    val signal: DistributionSignal? = null,
    val clusterId: String? = null,
    val signals: List<DistributionSignal> = emptyList(),
)

@Serializable
data class AddSignalResponse(val signalId: String, val summary: String, val added: Boolean)

internal fun interface NeighbourRetriever {
    fun prepare(corpus: List<Signal>, signalIds: List<String>, cacheDirectory: Path): Map<String, SignalNeighbours>
}

internal class PythonNeighbourRetriever(private val preparedPython: Path? = null) : NeighbourRetriever {
    override fun prepare(
        corpus: List<Signal>,
        signalIds: List<String>,
        cacheDirectory: Path,
    ): Map<String, SignalNeighbours> {
        if (signalIds.isEmpty()) return emptyMap()
        val home = Files.createTempDirectory("edict-embeddings-")
        try {
            val script = copyResource("/distribution/cluster.py", home.resolve("cluster.py"))
            val python = preparedPython?.toAbsolutePath()?.normalize() ?: preparePython(home)
            require(Files.isRegularFile(python) && Files.isExecutable(python)) {
                "Prepared embedding Python is not executable: $python"
            }
            Files.createDirectories(cacheDirectory)
            require(!Files.isSymbolicLink(cacheDirectory)) { "Embedding cache must not be a symlink" }
            val request = home.resolve("request.json")
            val output = home.resolve("response.json")
            Files.writeString(
                request,
                wireJson.encodeToString(
                    EmbeddingRequest(EMBEDDING_MODEL, EMBEDDING_MODEL_REVISION, corpus, signalIds, NEIGHBOUR_COUNT)
                )
            )
            runProcess(
                home,
                listOf(
                    python.toString(), script.toString(), "--request", request.toString(),
                    "--output", output.toString(), "--cache", cacheDirectory.toString()
                ),
                timeoutSeconds = 1_200,
            )
            return wireJson.decodeFromString<NeighboursResponse>(Files.readString(output)).neighbours
                .associateBy(SignalNeighbours::signalId)
        } finally {
            Files.walk(home).use { paths ->
                paths.sorted(Comparator.reverseOrder()).forEach(Files::deleteIfExists)
            }
        }
    }

    private fun copyResource(name: String, target: Path): Path {
        val source = checkNotNull(javaClass.getResourceAsStream(name)) { "Missing bundled resource $name" }
        source.use { Files.copy(it, target) }
        return target
    }

    private fun preparePython(home: Path): Path {
        val requirements = copyResource("/distribution/requirements.txt", home.resolve("requirements.txt"))
        val venv = home.resolve("venv")
        val python = if (System.getProperty("os.name").startsWith("Windows", ignoreCase = true)) {
            venv.resolve("Scripts/python.exe")
        } else {
            venv.resolve("bin/python")
        }
        val basePython = System.getProperty("qodana.edict.python") ?: "python3"
        runProcess(home, listOf(basePython, "-m", "venv", venv.toString()), timeoutSeconds = 300)
        runProcess(
            home,
            listOf(
                python.toString(), "-m", "pip", "install", "--quiet", "--disable-pip-version-check",
                "-r", requirements.toString()
            ),
            timeoutSeconds = 1_200,
        )
        return python
    }
}

/** Server-owned sequential distribution state, bound to one running managed distribution task. */
internal class DistributionService(
    private val store: Store,
    private val snapshots: Map<String, DistributionValidationSnapshot>,
    private val retriever: NeighbourRetriever = PythonNeighbourRetriever(),
) {
    private val sessions = mutableMapOf<String, Session>()

    @Synchronized
    fun nextSignal(token: String, receipt: ValidationReceipt): NextSignalResponse {
        val taskId = distributionTask(token)
        val session = sessions[taskId] ?: createSession(taskId, receipt).also { sessions[taskId] = it }
        require(session.snapshot.receipt == receipt) { "Distribution task is already bound to another receipt" }
        val selected = receipt.validatedFiles.asSequence()
            .sortedBy { it.signalId }
            .firstOrNull { it.relativePath in store.list("inbox") }
        if (selected == null) {
            session.clearCurrent()
            return NextSignalResponse(summary = "All selected Signals were distributed")
        }
        session.start(selected.signalId)
        val file = store.read(selected.relativePath)
        require(file.hash == selected.sha256) { "Selected Signal '${selected.signalId}' changed after preparation" }
        val signal = SignalValidation.validate(selected.relativePath, file.content)
        return NextSignalResponse(
            signalId = signal.id,
            signalPath = selected.relativePath,
            signal = signal,
            candidates = candidates(session.neighbours[signal.id], signal.id),
            summary = "Prepared Signal '${signal.id}' with its nearest candidates",
        )
    }

    @Synchronized
    fun context(token: String, kind: String, id: String): DistributionContextResponse {
        val session = session(token)
        session.requireCurrent()
        return when (kind) {
            "signal" -> DistributionContextResponse(
                kind = kind,
                signal = checkNotNull(session.originalSignals[id]) { "Signal '$id' does not exist in the prepared corpus" }
                    .toDistributionSignal(),
            )
            "cluster" -> {
                val root = "clusters/$id"
                require("$root/description.json" in store.list(root)) { "Cluster '$id' does not exist" }
                val signals = store.list(root).filter { it.startsWith("$root/signals/") && it.endsWith(".json") }
                    .map { path -> SignalValidation.validate(path, store.read(path).content).toDistributionSignal() }
                session.loadedClusterIds += id
                DistributionContextResponse(kind = kind, clusterId = id, signals = signals)
            }
            else -> error("Unsupported distribution context kind '$kind'; expected 'signal' or 'cluster'")
        }
    }

    @Synchronized
    fun addSignal(token: String, signalId: String, targetClusterId: String): AddSignalResponse {
        val session = session(token)
        require(signalId in session.selectedIds) { "Signal '$signalId' was not selected for this run" }
        session.requireCurrent(signalId)
        require(clusterId.matches(targetClusterId)) { "Cluster id must be lowercase kebab-case" }
        val selected = session.snapshot.receipt.validatedFiles.single { it.signalId == signalId }
        val inbox = store.read(selected.relativePath)
        require(inbox.hash == selected.sha256) { "Selected Signal '$signalId' changed after preparation" }
        val signal = SignalValidation.validate(selected.relativePath, inbox.content)
        val root = "clusters/$targetClusterId"
        val descriptionPath = "$root/description.json"
        val existingDescription = store.list(root).takeIf { descriptionPath in it }?.let { store.read(descriptionPath) }
        if (existingDescription != null) {
            require(targetClusterId in session.loadedClusterIds) {
                "Call edict_next_get_distribution_context with kind='cluster' and id='$targetClusterId' before assigning the current Signal"
            }
            val description = json.parseToJsonElement(existingDescription.content).let { it as? JsonObject }
                ?: error("Cluster '$targetClusterId' has an invalid description")
            require(description["language"]?.jsonPrimitive?.content == signal.language()) {
                "Signal language does not match cluster '$targetClusterId'"
            }
            val predecessor = when (description["status"]?.jsonPrimitive?.content) {
                "Generated" -> JsonPrimitive(targetClusterId)
                "Pending", "Invalid" -> description["predecessorId"]
                "Discontinued" -> null
                else -> error("Cluster '$targetClusterId' has an unsupported status")
            }
            val updated = description.toMutableMap().apply {
                put("status", JsonPrimitive("Pending"))
                if (predecessor == null) remove("predecessorId") else put("predecessorId", predecessor)
            }
            store.writeDistribution(token, descriptionPath, encode(JsonObject(updated)), existingDescription.hash)
        } else {
            require(store.list(root).isEmpty()) { "Cluster '$targetClusterId' is incomplete and has no description" }
            val description = JsonObject(
                linkedMapOf(
                    "id" to JsonPrimitive(targetClusterId),
                    "description" to JsonPrimitive(signal.description),
                    "language" to JsonPrimitive(signal.language()),
                    "status" to JsonPrimitive("Pending"),
                    "knownProblems" to JsonArray(emptyList()),
                )
            )
            store.writeDistribution(token, descriptionPath, encode(description), "")
        }

        val destinationPath = "$root/signals/$signalId.json"
        val destination = store.list(root).takeIf { destinationPath in it }?.let { store.read(destinationPath) }
        if (destination == null) {
            store.writeDistribution(token, destinationPath, inbox.content, "")
        } else {
            require(destination.hash == inbox.hash && destination.content == inbox.content) {
                "Cluster '$targetClusterId' already contains conflicting Signal '$signalId'"
            }
        }
        val historyPath = "$root/history.md"
        val history = store.list(root).takeIf { historyPath in it }?.let { store.read(historyPath) }
        val entry = if (history == null) "Created for Signal `$signalId`.\n" else "Assigned Signal `$signalId`.\n"
        if (history == null) store.writeDistribution(token, historyPath, entry, "")
        else if (!history.content.lines().any { "`$signalId`" in it }) {
            store.writeDistribution(token, historyPath, history.content + entry, history.hash)
        }
        val verified = store.read(destinationPath)
        require(verified.hash == inbox.hash && verified.content == inbox.content) {
            "Durable destination verification failed for Signal '$signalId'"
        }
        store.deleteDistribution(token, selected.relativePath, inbox.hash)
        session.complete(signalId)
        return AddSignalResponse(signalId, "Assigned Signal '$signalId' to '$targetClusterId'", true)
    }

    private fun createSession(taskId: String, receipt: ValidationReceipt): Session {
        val snapshot = checkNotNull(snapshots[receipt.receiptId]) {
            "Unknown distribution receipt; call edict_validate_inbox on this server before distribution"
        }
        require(snapshot.receipt == receipt) { "Distribution receipt contents changed" }
        val signalPaths = store.list().filter { it.startsWith("inbox/") || "/signals/" in it }
            .filter { it.endsWith(".json") }
        val signals = signalPaths.map { path -> SignalValidation.validate(path, store.read(path).content) }
        require(signals.map(Signal::id).distinct().size == signals.size) { "Prepared repository contains duplicate Signal IDs" }
        val selectedIds = receipt.validatedFiles.map(ValidatedInboxFile::signalId)
        val neighbours = retriever.prepare(signals, selectedIds, store.root.resolve("embeddings"))
        require(selectedIds.all { it in neighbours }) { "Embedding retrieval omitted a selected Signal" }
        return Session(taskId, snapshot, selectedIds.toSet(), signals.associateBy(Signal::id), neighbours)
    }

    private fun session(token: String): Session {
        val taskId = distributionTask(token)
        return checkNotNull(sessions[taskId]) { "Call edict_next_next_signal with the preparation receipt first" }
    }

    private fun distributionTask(token: String): String {
        val capability = store.authorize(token)
        require(capability.skill == "edict-distribution") { "Only a running edict-distribution task may distribute Signals" }
        return capability.taskId
    }

    private fun candidates(neighbours: SignalNeighbours?, currentSignalId: String): List<DistributionCandidate> {
        val closest = neighbours?.closest.orEmpty()
        val paths = store.list()
        val clusterBySignal = paths.asSequence().filter { "/signals/" in it && it.endsWith(".json") }
            .associate { path -> path.substringAfterLast('/').removeSuffix(".json") to path.split('/')[1] }
        val clusters = closest.mapNotNull { neighbour ->
            clusterBySignal[neighbour.signalId]?.let { it to neighbour.distance }
        }.groupBy({ it.first }, { it.second }).map { (id, distances) ->
            DistributionCandidate("cluster", distances.min(), clusterId = id)
        }
        val inbox = closest.filter { neighbour ->
            neighbour.signalId != currentSignalId && "inbox/${neighbour.signalId}.json" in paths
        }.map { neighbour ->
            DistributionCandidate(
                "signal", neighbour.distance, signalId = neighbour.signalId,
                signalPath = "inbox/${neighbour.signalId}.json"
            )
        }
        return (clusters + inbox).sortedBy(DistributionCandidate::nearestDistance)
    }

    private fun encode(value: JsonElement): String = json.encodeToString(JsonElement.serializer(), value) + "\n"

    private data class Session(
        val taskId: String,
        val snapshot: DistributionValidationSnapshot,
        val selectedIds: Set<String>,
        val originalSignals: Map<String, Signal>,
        val neighbours: Map<String, SignalNeighbours>,
        var currentSignalId: String? = null,
        val loadedClusterIds: MutableSet<String> = mutableSetOf(),
    ) {
        fun start(signalId: String) {
            if (currentSignalId == signalId) return
            currentSignalId = signalId
            loadedClusterIds.clear()
        }

        fun requireCurrent(signalId: String? = null) {
            val current = checkNotNull(currentSignalId) { "Call edict_next_next_signal first" }
            if (signalId != null) require(current == signalId) {
                "Signal '$signalId' is not the current prepared Signal '$current'"
            }
        }

        fun complete(signalId: String) {
            requireCurrent(signalId)
            clearCurrent()
        }

        fun clearCurrent() {
            currentSignalId = null
            loadedClusterIds.clear()
        }
    }
}

private fun Signal.toDistributionSignal() = DistributionSignal(id, fileRevision, label, description)

private fun Signal.language(): String = when (Path.of(fileRevision.path).fileName.toString().substringAfterLast('.', "").lowercase()) {
    "java" -> "Java"
    "kt", "kts" -> "Kotlin"
    else -> error("Signal '$id' has unsupported source language: ${fileRevision.path}")
}
