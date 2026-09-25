// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.benchmark

import kotlin.math.abs

// Preserve the reference's inclusive region intersection and two-line tolerance,
// including its asymmetric FN calculation (exact matches only).
internal fun matchExact(a: Finding, b: Finding): Boolean =
    a.path != null && a.path == b.path && a.charOffset != null && a.charLength != null &&
        b.charOffset != null && b.charLength != null &&
        a.charOffset <= b.charOffset + b.charLength && b.charOffset <= a.charOffset + a.charLength

internal fun matchLenient(a: Finding, b: Finding): Boolean =
    a.path != null && a.path == b.path && a.startLine != null && b.startLine != null &&
        abs(a.startLine - b.startLine) <= 2

private fun positivePasses(example: FileRevision, findings: List<Finding>): Boolean {
    val fileFindings = findings.filter { it.path == example.path }
    val ranges = example.expectedProblemRanges
    return if (ranges.isNullOrEmpty()) fileFindings.isNotEmpty() else ranges.all { range ->
        fileFindings.any { it.startLine != null && it.startLine in range.start.toLong()..range.end.toLong() }
    }
}

private fun negativePasses(example: FileRevision, findings: List<Finding>): Boolean {
    val fileFindings = findings.filter { it.path == example.path }
    val ranges = example.expectedProblemRanges
    return if (ranges.isNullOrEmpty()) fileFindings.isEmpty() else ranges.none { range ->
        fileFindings.any { it.startLine != null && it.startLine in range.start.toLong()..range.end.toLong() }
    }
}

internal fun f1(precision: Double, recall: Double): Double =
    if (precision + recall > 0) 2 * precision * recall / (precision + recall) else 0.0

internal fun specSatisfaction(spec: Specification, findings: List<Finding>): SpecSatisfactionResult {
    val own = findings.filter { it.ruleId == spec.ruleId }
    val reqPos = spec.positiveExamples.count { positivePasses(it, own) }
    val reqNeg = spec.negativeExamples.count { negativePasses(it, own) }
    val optPos = spec.optionalPositiveExamples.count { positivePasses(it, own) }
    val optNeg = spec.optionalNegativeExamples.count { negativePasses(it, own) }
    val fn = spec.optionalPositiveExamples.size - optPos
    val fp = spec.optionalNegativeExamples.size - optNeg
    val precision = if (optPos + fp == 0) 1.0 else optPos.toDouble() / (optPos + fp)
    val recall = if (optPos + fn == 0) 1.0 else optPos.toDouble() / (optPos + fn)
    val optionalF1 = f1(precision, recall)
    return SpecSatisfactionResult(
        reqPos, spec.positiveExamples.size, reqNeg, spec.negativeExamples.size,
        optPos, spec.optionalPositiveExamples.size, optNeg, spec.optionalNegativeExamples.size,
        precision, recall, optionalF1,
        reqPos == spec.positiveExamples.size && reqNeg == spec.negativeExamples.size &&
            precision >= spec.optionalPrecisionThreshold && recall >= spec.optionalRecallThreshold &&
            optionalF1 >= spec.optionalF1Threshold,
    )
}

internal fun calculateMetrics(spec: Specification, gold: List<Finding>, findings: List<Finding>): InspectionMetrics {
    val baseline = gold.filter { it.ruleId == spec.ruleId }
    val actual = findings.filter { it.ruleId == spec.ruleId }
    val tp = actual.filter { candidate -> baseline.any { matchExact(it, candidate) || matchLenient(it, candidate) } }
    val fp = actual.size - tp.size
    val fn = baseline.count { expected -> tp.none { matchExact(expected, it) } }
    val recall = if (baseline.isEmpty()) 0.0 else tp.size.toDouble() / baseline.size
    val precision = if (actual.isEmpty()) 0.0 else tp.size.toDouble() / actual.size
    val satisfaction = specSatisfaction(spec, actual)
    return InspectionMetrics(spec.ruleId, tp.size, fp, fn, recall, precision, f1(precision, recall),
                             satisfaction.satisfiesSpec, satisfaction)
}

internal fun List<Double>.median(): Double {
    if (isEmpty()) return 0.0
    val sorted = sorted()
    val mid = size / 2
    return if (size % 2 == 0) (sorted[mid - 1] + sorted[mid]) / 2 else sorted[mid]
}

private fun List<Double>.averageOrZero() = if (isEmpty()) 0.0 else average()

internal fun aggregate(metrics: List<InspectionMetrics>): AggregateMetrics = AggregateMetrics(
    metrics.sumOf { it.truePositives }, metrics.sumOf { it.falsePositives }, metrics.sumOf { it.falseNegatives },
    metrics.map { it.recall }.averageOrZero(), metrics.map { it.precision }.averageOrZero(),
    metrics.map { it.f1Score }.averageOrZero(), metrics.map { it.recall }.median(),
    metrics.map { it.precision }.median(), metrics.map { it.f1Score }.median(),
    metrics.count { it.satisfiesSpec },
    if (metrics.isEmpty()) 0.0 else metrics.count { it.satisfiesSpec }.toDouble() / metrics.size,
)

internal fun compareSpecWithGold(spec: Specification, gold: List<Finding>): ClassifiedSpecification {
    val own = gold.filter { it.ruleId == spec.ruleId }
    fun classify(example: FileRevision, positive: Boolean): ClassifiedFileRevision {
        val classification = if (positive) {
            if (positivePasses(example, own)) ExampleClassification.TP else ExampleClassification.FP
        } else {
            if (negativePasses(example, own)) ExampleClassification.TN else ExampleClassification.FN
        }
        return ClassifiedFileRevision(example.path, example.revision, example.expectedProblemRanges, classification)
    }
    return ClassifiedSpecification(spec.ruleId, spec.description, spec.language,
        spec.positiveExamples.map { classify(it, true) }, spec.negativeExamples.map { classify(it, false) },
        spec.optionalPositiveExamples.map { classify(it, true) }, spec.optionalNegativeExamples.map { classify(it, false) })
}

internal fun specGoldAggregate(comparisons: List<ClassifiedSpecification>): SpecGoldAggregateMetrics =
    SpecGoldAggregateMetrics(comparisons.map { it.precision }.averageOrZero(),
        comparisons.map { it.recall }.averageOrZero(), comparisons.sumOf { it.tp }, comparisons.sumOf { it.fp },
        comparisons.sumOf { it.tn }, comparisons.sumOf { it.fn })
