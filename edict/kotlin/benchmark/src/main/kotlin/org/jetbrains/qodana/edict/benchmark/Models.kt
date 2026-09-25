// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.benchmark

import kotlinx.serialization.Serializable

@Serializable
data class LineRange(val start: Int, val end: Int)

@Serializable
data class FileRevision(
    val path: String,
    val revision: String = "",
    val expectedProblemRanges: List<LineRange>? = null,
)

@Serializable
data class Specification(
    val ruleId: String,
    val description: String,
    val language: String,
    val positiveExamples: List<FileRevision> = emptyList(),
    val negativeExamples: List<FileRevision> = emptyList(),
    val optionalPositiveExamples: List<FileRevision> = emptyList(),
    val optionalNegativeExamples: List<FileRevision> = emptyList(),
    val optionalPrecisionThreshold: Double = 0.8,
    val optionalRecallThreshold: Double = 0.7,
    val optionalF1Threshold: Double = 0.1,
)

@Serializable
data class BenchmarkInputs(
    val revision: String,
    val clusterToRule: Map<String, String>,
    val specifications: List<Specification>,
)

data class Finding(
    val ruleId: String,
    val path: String?,
    val startLine: Long? = null,
    val charOffset: Long? = null,
    val charLength: Long? = null,
)

// Report field names and calculations follow generation-benchmark/next/GenerationBenchmarkScript2.kt.
@Serializable
data class SpecSatisfactionResult(
    val requiredPositivePassed: Int,
    val requiredPositiveTotal: Int,
    val requiredNegativePassed: Int,
    val requiredNegativeTotal: Int,
    val optionalPositivePassed: Int,
    val optionalPositiveTotal: Int,
    val optionalNegativePassed: Int,
    val optionalNegativeTotal: Int,
    val optionalPrecision: Double,
    val optionalRecall: Double,
    val optionalF1: Double,
    val satisfiesSpec: Boolean,
)

@Serializable
data class InspectionMetrics(
    val ruleId: String,
    val truePositives: Int,
    val falsePositives: Int,
    val falseNegatives: Int,
    val recall: Double,
    val precision: Double,
    val f1Score: Double,
    val satisfiesSpec: Boolean,
    val specSatisfaction: SpecSatisfactionResult,
)

@Serializable
data class AggregateMetrics(
    val totalTP: Int,
    val totalFP: Int,
    val totalFN: Int,
    val avgRecall: Double,
    val avgPrecision: Double,
    val avgF1Score: Double,
    val medianRecall: Double,
    val medianPrecision: Double,
    val medianF1Score: Double,
    val specSatisfiedCount: Int,
    val specSatisfiedRate: Double,
)

@Serializable
enum class ExampleClassification { TP, FP, TN, FN }

@Serializable
data class ClassifiedFileRevision(
    val path: String,
    val revision: String,
    val expectedProblemRanges: List<LineRange>?,
    val classification: ExampleClassification,
)

@Serializable
data class ClassifiedSpecification(
    val ruleId: String,
    val description: String,
    val language: String,
    val positiveExamples: List<ClassifiedFileRevision>,
    val negativeExamples: List<ClassifiedFileRevision>,
    val optionalPositiveExamples: List<ClassifiedFileRevision>,
    val optionalNegativeExamples: List<ClassifiedFileRevision>,
) {
    val allExamples get() = positiveExamples + negativeExamples + optionalPositiveExamples + optionalNegativeExamples
    val tp get() = allExamples.count { it.classification == ExampleClassification.TP }
    val fp get() = allExamples.count { it.classification == ExampleClassification.FP }
    val tn get() = allExamples.count { it.classification == ExampleClassification.TN }
    val fn get() = allExamples.count { it.classification == ExampleClassification.FN }
    val precision get() = if (tp + fp > 0) tp.toDouble() / (tp + fp) else 1.0
    val recall get() = if (tp + fn > 0) tp.toDouble() / (tp + fn) else 1.0
}

@Serializable
data class SpecGoldAggregateMetrics(
    val avgPrecision: Double,
    val avgRecall: Double,
    val totalTP: Int,
    val totalFP: Int,
    val totalTN: Int,
    val totalFN: Int,
)

@Serializable
data class BenchmarkReport(
    val inspectionMetrics: List<InspectionMetrics>,
    val aggregate: AggregateMetrics,
    val totalInspectionsProcessed: Int,
    val successful: Int,
    val specGoldMetrics: SpecGoldAggregateMetrics,
    val sourceRevision: String,
    val generationOutcomes: Map<String, String>,
)
