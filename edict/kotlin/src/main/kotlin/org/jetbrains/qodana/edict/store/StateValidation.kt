// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.store

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.jsonObject
import org.jetbrains.qodana.edict.common.json
import org.jetbrains.qodana.edict.common.sha256
import org.jetbrains.qodana.edict.model.Signal
import org.jetbrains.qodana.edict.model.SignalLabel
import org.jetbrains.qodana.edict.signals.SignalValidation

@Serializable
data class StateValidationIssue(val path: String, val message: String)

@Serializable
data class StateValidationResponse(
    val success: Boolean,
    val summary: String,
    val issues: List<StateValidationIssue> = emptyList()
)

private data class ClusterDescription(
    val id: String,
    val language: String,
    val status: String,
    val predecessorId: String?
)

private data class ExampleMetadata(
    val id: String,
    val fileName: String,
    val label: SignalLabel,
    val ranges: List<IntRange>
)

data class DistributionValidationSnapshot(
    val receipt: ValidationReceipt,
    val stateHashes: Map<String, String>
)

fun prepareDistributionValidation(store: Store, paths: List<String>): DistributionValidationSnapshot {
    val validated = validateInboxChanges(store, paths)
    val hashes = store.list().filterNot { it.startsWith("plans/") }.associateWith { store.read(it).hash }
    val receipt = validated.copy(
        receiptId = "distribution-${sha256(
            validated.receiptId + "|" + hashes.entries.joinToString("|") { "${it.key}:${it.value}" }
        ).take(24)}"
    )
    return DistributionValidationSnapshot(receipt, hashes)
}

fun validateDistributionState(store: Store, snapshot: DistributionValidationSnapshot): StateValidationResponse {
    val receipt = snapshot.receipt
    val issues = repositoryIssues(store).toMutableList()
    val expectedReceiptId = "distribution-${sha256(
        validationReceiptId(receipt.validatedFiles) + "|" +
                snapshot.stateHashes.entries.joinToString("|") { "${it.key}:${it.value}" }
    ).take(24)}"
    if (receipt.receiptId != expectedReceiptId) {
        issues += StateValidationIssue("inbox", "Distribution receipt does not match its frozen state")
    }
    if (receipt.validatedFiles.map { it.relativePath }.distinct().size != receipt.validatedFiles.size) {
        issues += StateValidationIssue("inbox", "Distribution receipt must contain distinct selected inbox files")
    }
    val statePaths = store.list()
    val recipientClusterIds = mutableSetOf<String>()
    receipt.validatedFiles.forEach { selected ->
        val expectedPath = "inbox/${selected.signalId}.json"
        if (selected.relativePath != expectedPath) {
            issues += StateValidationIssue(selected.relativePath, "Receipt path must be $expectedPath")
            return@forEach
        }
        val destinations = statePaths.filter {
            it.startsWith("clusters/") && it.endsWith("/signals/${selected.signalId}.json")
        }
        if (selected.relativePath in statePaths) {
            issues += StateValidationIssue(selected.relativePath, "Selected Signal remains in the inbox")
        }
        if (destinations.size != 1) {
            issues += StateValidationIssue(
                "signals/${selected.signalId}",
                "Distributed Signal must belong to exactly one cluster; found ${destinations.size}"
            )
        } else {
            recipientClusterIds += destinations.single().split('/')[1]
            val destination = store.read(destinations.single())
            if (destination.hash != selected.sha256) {
                issues += StateValidationIssue(destination.path, "Distribution changed the Signal contents")
            }
            val signal = parseSignal(store, destination.path, issues)
            if (signal != null && signal.idempotencyKey.ifBlank { signal.source.suggestionId.orEmpty() } != selected.idempotencyKey) {
                issues += StateValidationIssue(destination.path, "Distribution changed the Signal idempotency key")
            }
        }
    }
    val currentHashes = statePaths.filterNot { it.startsWith("plans/") }.associateWith { store.read(it).hash }
    val changedPaths = (snapshot.stateHashes.keys + currentHashes.keys).filterTo(sortedSetOf()) {
        snapshot.stateHashes[it] != currentHashes[it]
    }
    val selectedIds = receipt.validatedFiles.mapTo(hashSetOf()) { it.signalId }
    changedPaths.filterNot { path ->
        path in receipt.validatedFiles.map { it.relativePath } ||
                recipientClusterIds.any { clusterId ->
                    path == "clusters/$clusterId/description.json" ||
                            path == "clusters/$clusterId/history.md" ||
                            selectedIds.any { signalId -> path == "clusters/$clusterId/signals/$signalId.json" }
                }
    }.forEach { path ->
        issues += StateValidationIssue(path, "Distribution changed repository state outside the moved Signals")
    }
    return response("Distribution changes are valid", "distribution", issues)
}

fun validateCodeExampleState(store: Store, clusterId: String, exampleId: String): StateValidationResponse {
    val issues = mutableListOf<StateValidationIssue>()
    validateExample(store, clusterId, exampleId, clusterLanguage(store, clusterId, issues), issues)
    return response("Code example structure is valid", "code example", issues)
}

fun validateClusterExamplesState(store: Store, clusterId: String): StateValidationResponse {
    val issues = mutableListOf<StateValidationIssue>()
    validateClusterExamples(store, clusterId, requireEverySignal = true, issues)
    return response("Every Signal has a structurally valid focused code example", "cluster code example", issues)
}

fun validateGenerationState(store: Store, clusterIds: List<String>): StateValidationResponse {
    val issues = repositoryIssues(store).toMutableList()
    if (clusterIds.distinct().size != clusterIds.size) {
        issues += StateValidationIssue("clusters", "Generation target IDs must be distinct")
    }
    val descriptions = store.list("clusters").filter { it.endsWith("/description.json") }
        .associateBy { it.split('/')[1] }
    clusterIds.forEach { clusterId ->
        if (!identifier.matches(clusterId)) {
            issues += StateValidationIssue("clusters/$clusterId", "Invalid generation target ID")
        } else if (clusterId !in descriptions) {
            issues += StateValidationIssue("clusters/$clusterId", "Frozen generation target is missing")
        }
    }
    return response("Generation state is valid; inspection execution evidence remains external", "generation", issues)
}

private fun repositoryIssues(store: Store): List<StateValidationIssue> = buildList {
    val paths = store.list()
    val signalsById = mutableMapOf<String, MutableList<String>>()
    paths.filter { it.startsWith("inbox/") && it.endsWith(".json") || it.contains("/signals/") && it.endsWith(".json") }
        .forEach { path ->
            parseSignal(store, path, this)?.let { signal -> signalsById.getOrPut(signal.id) { mutableListOf() } += path }
        }
    signalsById.filterValues { it.size != 1 }.forEach { (id, locations) ->
        add(StateValidationIssue("signals/$id", "Signal occurs ${locations.size} times: ${locations.joinToString()}"))
    }

    val descriptionPaths = paths.filter { it.startsWith("clusters/") && it.endsWith("/description.json") }
    val descriptions = descriptionPaths.mapNotNull { path -> parseDescription(store, path, this) }
    val clusterIds = descriptions.map { it.id }.toSet()
    descriptions.forEach { description ->
        val root = "clusters/${description.id}"
        val members = paths.filter { it.startsWith("$root/signals/") && it.endsWith(".json") }
        if (members.isEmpty()) add(StateValidationIssue(root, "Cluster must contain at least one Signal"))
        members.forEach { path ->
            parseSignal(store, path, this)?.let { signal ->
                val expectedExtension = if (description.language == "Java") ".java" else ".kt"
                if (!signal.fileRevision.path.endsWith(expectedExtension, ignoreCase = true)) {
                    add(StateValidationIssue(path, "Signal language does not match ${description.language} cluster"))
                }
            }
        }
        validateClusterExamples(
            store,
            description.id,
            requireEverySignal = description.status in setOf("Generated", "Discontinued"),
            this
        )
        val history = "$root/history.md"
        if (description.status != "Pending" && (history !in paths || store.read(history).content.isBlank())) {
            add(StateValidationIssue(history, "Non-Pending cluster must have non-empty history"))
        }
        val candidate = "inspections/${description.id}.candidate.kts"
        val accepted = "inspections/${description.id}.inspection.kts"
        when (description.status) {
            "Generated" -> {
                if (accepted !in paths || store.read(accepted).content.isBlank()) {
                    add(StateValidationIssue(accepted, "Generated cluster has no inspection"))
                }
                if (candidate in paths) add(StateValidationIssue(candidate, "Generated cluster must not have a candidate"))
                if (description.predecessorId != null) {
                    add(StateValidationIssue("$root/description.json", "Generated cluster must not have a predecessorId"))
                }
            }
            "Discontinued" -> {
                if (accepted in paths) add(StateValidationIssue(accepted, "Discontinued cluster must not have an inspection"))
                if (candidate in paths) add(StateValidationIssue(candidate, "Discontinued cluster must not have a candidate"))
                if (description.predecessorId != null) {
                    add(StateValidationIssue("$root/description.json", "Discontinued cluster must not have a predecessorId"))
                }
            }
            "Pending", "Invalid" -> Unit
        }
    }
    paths.filter { it.startsWith("inspections/") }.forEach { path ->
        val fileName = path.substringAfterLast('/')
        val clusterId = fileName.removeSuffix(".candidate.kts").removeSuffix(".inspection.kts")
        if (clusterId !in clusterIds) add(StateValidationIssue(path, "Inspection file does not belong to a cluster"))
    }
}

private fun parseDescription(
    store: Store,
    path: String,
    issues: MutableList<StateValidationIssue>
): ClusterDescription? {
    val directoryId = path.split('/')[1]
    val value = parseObject(store, path, issues) ?: return null
    val id = value.string("id")
    val description = value.string("description")
    val language = value.string("language")
    val status = value.string("status")
    if (!identifier.matches(directoryId)) issues += StateValidationIssue(path, "Invalid cluster directory ID")
    if (id != directoryId) issues += StateValidationIssue(path, "Cluster description ID must match its directory")
    if (description.isBlank()) issues += StateValidationIssue(path, "Cluster description must not be blank")
    if (language !in setOf("Java", "Kotlin")) issues += StateValidationIssue(path, "Cluster language must be Java or Kotlin")
    if (status !in setOf("Pending", "Generated", "Discontinued", "Invalid")) {
        issues += StateValidationIssue(path, "Unsupported cluster status '$status'")
    }
    val knownProblems = value["knownProblems"]
    if (knownProblems !is JsonArray) {
        issues += StateValidationIssue(path, "knownProblems must be an array")
    } else knownProblems.forEachIndexed { index, item ->
        val problem = item as? JsonObject
        val valid = problem != null && problem.string("severity") == "MAJOR" &&
                problem.string("review") in setOf("code", "weak-signal", "value") &&
                listOf("category", "description", "evidence").all { problem.string(it).isNotBlank() } &&
                (problem["suggestion"] == null || problem["suggestion"] is JsonPrimitive)
        if (!valid) issues += StateValidationIssue(path, "Invalid knownProblems[$index]")
    }
    val predecessor = value["predecessorId"]?.let { (it as? JsonPrimitive)?.contentOrNull }
    if (predecessor != null && !identifier.matches(predecessor)) {
        issues += StateValidationIssue(path, "Invalid predecessorId")
    }
    return ClusterDescription(id, language, status, predecessor)
}

private fun validateClusterExamples(
    store: Store,
    clusterId: String,
    requireEverySignal: Boolean,
    issues: MutableList<StateValidationIssue>
) {
    val root = "clusters/$clusterId"
    val language = clusterLanguage(store, clusterId, issues)
    val paths = store.list(root)
    val exampleIds = paths.asSequence().filter { it.startsWith("$root/synthetic-examples/") }
        .mapNotNull { it.split('/').getOrNull(3) }.toSortedSet()
    val metadata = exampleIds.mapNotNull { validateExample(store, clusterId, it, language, issues) }
        .associateBy { it.id }
    paths.filter { it.startsWith("$root/signals/") && it.endsWith(".json") }.forEach { path ->
        val signal = parseSignal(store, path, issues) ?: return@forEach
        val exampleId = signal.syntheticExampleId
        if (exampleId == null) {
            if (requireEverySignal) issues += StateValidationIssue(path, "Signal has no code example")
        } else {
            val example = metadata[exampleId]
            if (example == null) issues += StateValidationIssue(path, "Signal references missing example '$exampleId'")
            else if (example.label != signal.label) issues += StateValidationIssue(path, "Signal and example '$exampleId' have different labels")
        }
    }
}

private fun validateExample(
    store: Store,
    clusterId: String,
    exampleId: String,
    language: String?,
    issues: MutableList<StateValidationIssue>
): ExampleMetadata? {
    val root = "clusters/$clusterId/synthetic-examples/$exampleId"
    if (!identifier.matches(clusterId) || !identifier.matches(exampleId)) {
        issues += StateValidationIssue(root, "Invalid cluster or example ID")
        return null
    }
    val metadataPath = "$root/metadata.json"
    val paths = store.list(root)
    if (metadataPath !in paths) {
        issues += StateValidationIssue(metadataPath, "Code example has no metadata.json")
        return null
    }
    val value = parseObject(store, metadataPath, issues) ?: return null
    val id = value.string("id")
    val fileName = value.string("fileName")
    val label = runCatching { SignalLabel.valueOf(value.string("label")) }.getOrNull()
    if (id != exampleId) issues += StateValidationIssue(metadataPath, "Metadata ID must match its directory")
    if (fileName.isBlank() || '/' in fileName || '\\' in fileName) {
        issues += StateValidationIssue(metadataPath, "fileName must name one source file")
    }
    val expectedExtension = when (language) { "Java" -> ".java"; "Kotlin" -> ".kt"; else -> null }
    if (expectedExtension != null && !fileName.endsWith(expectedExtension, ignoreCase = true)) {
        issues += StateValidationIssue(metadataPath, "Code example language does not match the cluster")
    }
    if (label == null) issues += StateValidationIssue(metadataPath, "label must be POSITIVE or NEGATIVE")
    val rawRanges = value["expectedRanges"] as? JsonArray
    if (rawRanges == null) issues += StateValidationIssue(metadataPath, "expectedRanges must be an array")
    val ranges = rawRanges.orEmpty().mapIndexedNotNull { index, item ->
        val range = item as? JsonObject
        val start = (range?.get("start") as? JsonPrimitive)?.takeUnless { it.isString }?.intOrNull
        val end = (range?.get("end") as? JsonPrimitive)?.takeUnless { it.isString }?.intOrNull
        if (start == null || end == null || start < 1 || end < start) {
            issues += StateValidationIssue(metadataPath, "Invalid expectedRanges[$index]")
            null
        } else start..end
    }
    if (label == SignalLabel.POSITIVE && ranges.size != 1) {
        issues += StateValidationIssue(metadataPath, "A positive example must declare exactly one expected range")
    }
    if (label == SignalLabel.NEGATIVE && ranges.isNotEmpty()) {
        issues += StateValidationIssue(metadataPath, "A negative example must not declare expected ranges")
    }
    val sourcePath = "$root/project/$fileName"
    val sourceFiles = paths.filter { it.startsWith("$root/project/") }
    if (sourcePath !in sourceFiles) {
        issues += StateValidationIssue(sourcePath, "Code example source file is missing")
    } else {
        if (sourceFiles != listOf(sourcePath)) issues += StateValidationIssue(root, "Code example must contain one source file")
        val source = store.read(sourcePath).content
        if (source.isBlank()) issues += StateValidationIssue(sourcePath, "Code example source must not be blank")
        val lineCount = source.lines().size
        ranges.filter { it.last > lineCount }.forEach { range ->
            issues += StateValidationIssue(metadataPath, "Expected range ${range.first}-${range.last} exceeds $lineCount source lines")
        }
    }
    return if (label == null) null else ExampleMetadata(id, fileName, label, ranges)
}

private fun clusterLanguage(store: Store, clusterId: String, issues: MutableList<StateValidationIssue>): String? {
    val path = "clusters/$clusterId/description.json"
    return parseDescription(store, path, issues)?.language
}

private fun parseSignal(store: Store, path: String, issues: MutableList<StateValidationIssue>): Signal? = try {
    SignalValidation.validate(path, store.read(path).content)
} catch (e: Exception) {
    issues += StateValidationIssue(path, e.message.orEmpty())
    null
}

private fun parseObject(store: Store, path: String, issues: MutableList<StateValidationIssue>): JsonObject? = try {
    json.parseToJsonElement(store.read(path).content).jsonObject
} catch (e: Exception) {
    issues += StateValidationIssue(path, "Invalid JSON: ${e.message.orEmpty()}")
    null
}

private fun JsonObject.string(name: String): String = (get(name) as? JsonPrimitive)?.takeIf { it.isString }?.content.orEmpty()

private fun response(ok: String, subject: String, issues: List<StateValidationIssue>) = StateValidationResponse(
    success = issues.isEmpty(),
    summary = if (issues.isEmpty()) ok else "Found ${issues.size} $subject issue(s)",
    issues = issues
)
