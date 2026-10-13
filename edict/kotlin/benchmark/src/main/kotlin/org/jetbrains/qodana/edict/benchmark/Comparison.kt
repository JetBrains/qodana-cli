// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.benchmark

import kotlin.math.abs

// Preserve the reference's inclusive region intersection and two-line tolerance.
internal fun matchExact(a: Finding, b: Finding): Boolean =
    a.path != null && a.path == b.path && a.charOffset != null && a.charLength != null &&
        b.charOffset != null && b.charLength != null &&
        a.charOffset <= b.charOffset + b.charLength && b.charOffset <= a.charOffset + a.charLength

internal fun matchLenient(a: Finding, b: Finding): Boolean =
    a.path != null && a.path == b.path && a.startLine != null && b.startLine != null &&
        abs(a.startLine - b.startLine) <= 2

internal fun f1(precision: Double, recall: Double): Double =
    if (precision + recall > 0) 2 * precision * recall / (precision + recall) else 0.0

internal const val POSITIVE = "POSITIVE"
internal const val NEGATIVE = "NEGATIVE"

private fun matches(expected: Finding, actual: Finding) = matchExact(expected, actual) || matchLenient(expected, actual)

// A finding matching a POSITIVE result is a true positive and any other a false positive. A false positive matching a
// NEGATIVE result is a confirmed one; one matching only an unlabeled result may still be an unconfirmed positive.
internal fun calculateMetrics(ruleId: String, gold: List<Finding>, findings: List<Finding>): InspectionMetrics {
    val (positives, others) = gold.filter { it.ruleId == ruleId }.partition { it.label == POSITIVE }
    val (negatives, unlabeled) = others.partition { it.label == NEGATIVE }
    val actual = findings.filter { it.ruleId == ruleId }
    val (tp, fp) = actual.partition { candidate -> positives.any { matches(it, candidate) } }
    val (matchedNegative, rest) = fp.partition { candidate -> negatives.any { matches(it, candidate) } }
    val matchedUnlabeled = rest.count { candidate -> unlabeled.any { matches(it, candidate) } }
    val coveredGold = positives.count { expected -> actual.any { matches(expected, it) } }
    val recall = if (positives.isEmpty()) 0.0 else coveredGold.toDouble() / positives.size
    val precision = if (actual.isEmpty()) 0.0 else tp.size.toDouble() / actual.size
    return InspectionMetrics(ruleId, tp.size, fp.size, positives.size - coveredGold, matchedNegative.size, matchedUnlabeled,
        recall, precision, f1(precision, recall))
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
    metrics.sumOf { it.matchedNegative }, metrics.sumOf { it.matchedUnlabeled },
    metrics.map { it.recall }.averageOrZero(), metrics.map { it.precision }.averageOrZero(),
    metrics.map { it.f1Score }.averageOrZero(), metrics.map { it.recall }.median(),
    metrics.map { it.precision }.median(), metrics.map { it.f1Score }.median(),
)
