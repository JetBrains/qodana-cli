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

internal fun f1(precision: Double, recall: Double): Double =
    if (precision + recall > 0) 2 * precision * recall / (precision + recall) else 0.0

internal fun calculateMetrics(ruleId: String, gold: List<Finding>, findings: List<Finding>): InspectionMetrics {
    val baseline = gold.filter { it.ruleId == ruleId }
    val actual = findings.filter { it.ruleId == ruleId }
    val tp = actual.filter { candidate -> baseline.any { matchExact(it, candidate) || matchLenient(it, candidate) } }
    val fp = actual.size - tp.size
    val fn = baseline.count { expected -> tp.none { matchExact(expected, it) } }
    val recall = if (baseline.isEmpty()) 0.0 else tp.size.toDouble() / baseline.size
    val precision = if (actual.isEmpty()) 0.0 else tp.size.toDouble() / actual.size
    return InspectionMetrics(ruleId, tp.size, fp, fn, recall, precision, f1(precision, recall))
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
)
