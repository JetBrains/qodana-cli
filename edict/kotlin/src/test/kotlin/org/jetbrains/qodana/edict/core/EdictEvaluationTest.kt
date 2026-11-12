// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.core

import kotlin.test.Test
import kotlin.test.assertEquals

class EdictEvaluationTest {
  @Test
  fun `writes the scores it computes from the examples`() {
    assertEquals(EVALUATION_JSON, EdictJson.encodeToString(EdictEvaluation.serializer(), EVALUATION))
  }

  @Test
  fun `computes the scores again on reading a file of an earlier version`() {
    // Scores of an earlier formula, which neither the totals nor the example's weight follow from, and fields it wrote.
    val stale = EVALUATION_JSON
      .replace("\"tp\": 3", "\"tp\": 99")
      .replace("\"weight\": 3", "\"weight\": 7, \"strong\": true")
      .replace("\"projectReview\": null", "\"projectReview\": null, \"action\": \"GENERATE\", \"directFindings\": {\"tp\": 1, \"fp\": 0}")

    val read = EdictJson.decodeFromString(EdictEvaluation.serializer(), stale)

    assertEquals(EVALUATION_JSON, EdictJson.encodeToString(EdictEvaluation.serializer(), read))
  }

  @Test
  fun `scores 0 when no finding counts`() {
    val evaluation = EVALUATION.copy(examples = mapOf("allowed" to EdictExampleEvaluation(EdictSignalLabel.NEGATIVE, true, 0)))

    assertEquals(
      listOf(0.0, 0.0, 0.0, 0.0, 0.0, 0.0),
      with(evaluation) { listOf(tp.toDouble(), fp.toDouble(), fn.toDouble(), precision, recall, f1) },
    )
  }
}

/** Three findings on a reported positive, against a reported negative and a missed positive of weight 1 each. */
private val EVALUATION = EdictEvaluation(
  inspectionHash = "a".repeat(64),
  exampleSetDigest = "b".repeat(64),
  examples = mapOf(
    "found" to EdictExampleEvaluation(EdictSignalLabel.POSITIVE, satisfied = true, projectFindings = 3),
    "missed" to EdictExampleEvaluation(EdictSignalLabel.POSITIVE, satisfied = false, projectFindings = 0),
    "reported" to EdictExampleEvaluation(EdictSignalLabel.NEGATIVE, satisfied = false, projectFindings = 0),
  ),
  projectReview = null,
  inspectionName = "Sample rule",
  inspectionDescription = "Finds samples",
)

private val EVALUATION_JSON = """
  {
      "inspectionHash": "${"a".repeat(64)}",
      "exampleSetDigest": "${"b".repeat(64)}",
      "examples": {
          "found": {
              "label": "POSITIVE",
              "satisfied": true,
              "projectFindings": 3,
              "weight": 3
          },
          "missed": {
              "label": "POSITIVE",
              "satisfied": false,
              "projectFindings": 0,
              "weight": 1
          },
          "reported": {
              "label": "NEGATIVE",
              "satisfied": false,
              "projectFindings": 0,
              "weight": 1
          }
      },
      "projectReview": null,
      "inspectionName": "Sample rule",
      "inspectionDescription": "Finds samples",
      "tp": 3,
      "fp": 1,
      "fn": 1,
      "precision": 0.75,
      "recall": 0.75,
      "f1": 0.75
  }
""".trimIndent()
