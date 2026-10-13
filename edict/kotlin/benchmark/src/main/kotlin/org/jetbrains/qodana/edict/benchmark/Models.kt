// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.benchmark

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonObject

data class BenchmarkInputs(
    val revision: String,
    val rules: List<String>,
    /** The gold manifest's rule for each Signal id; null maps Signals by their `benchmark/<Rule>/` source reference. */
    val ruleBySignal: Map<String, String>? = null,
)

data class Finding(
    val ruleId: String,
    val path: String?,
    val startLine: Long? = null,
    val charOffset: Long? = null,
    val charLength: Long? = null,
    /** Gold classification: POSITIVE, NEGATIVE or unlabeled (null); generated findings have none. */
    val label: String? = null,
)

@Serializable
data class InspectionMetrics(
    val ruleId: String,
    val truePositives: Int,
    val falsePositives: Int,
    val falseNegatives: Int,
    /** False positives matching a NEGATIVE gold result. */
    val matchedNegative: Int,
    /** False positives matching only an unlabeled gold result. */
    val matchedUnlabeled: Int,
    val recall: Double,
    val precision: Double,
    val f1Score: Double,
)

@Serializable
data class AggregateMetrics(
    val totalTP: Int,
    val totalFP: Int,
    val totalFN: Int,
    val totalMatchedNegative: Int,
    val totalMatchedUnlabeled: Int,
    val avgRecall: Double,
    val avgPrecision: Double,
    val avgF1Score: Double,
    val medianRecall: Double,
    val medianPrecision: Double,
    val medianF1Score: Double,
)

@Serializable
data class BenchmarkReport(
    val inspectionMetrics: List<InspectionMetrics>,
    val aggregate: AggregateMetrics,
    val totalInspectionsProcessed: Int,
    val successful: Int,
    val sourceRevision: String,
    val generationOutcomes: Map<String, String>,
    val clustersByRule: Map<String, List<String>> = emptyMap(),
    val clusterOutcomes: Map<String, String> = emptyMap(),
    val generationPriceReport: JsonObject? = null,
)
