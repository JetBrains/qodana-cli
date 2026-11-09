// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback.support

/** The message every fixture result reports. */
internal const val FINDING_MESSAGE = "Blocking call on the worker thread"

/**
 * The SARIF report of a scan of [revision] of the project in [projectDirectory] of the repository, as Qodana's base ids name
 * it, whose inspections run for feedback reported [results], as [internalResult] makes them.
 */
internal fun sarifReport(vararg results: String, projectDirectory: String = "", revision: String = HEAD_REVISION): String = """
  {
    "runs": [{
      "versionControlProvenance": [{"repositoryUri": "https://github.com/owner/project.git", "revisionId": "$revision", "branch": "feature"}],
      "originalUriBaseIds": {
        "PROJECTROOT": {"description": {"text": "The root directory of the repository"}},
        "SRCROOT": {"uri": "$projectDirectory", "uriBaseId": "PROJECTROOT"}
      },
      "results": [${regularResult()}],
      "properties": {"qodana.internal.results": [${results.joinToString(",")}]}
    }]
  }
""".trimIndent()

/**
 * A result of the inspection of cluster [clusterId] at [uri], relative to the analysed project, on lines [startLine] to
 * [endLine]; without a [startLine], the problem is of the whole file.
 */
internal fun internalResult(clusterId: String, uri: String, startLine: Int?, endLine: Int? = null): String {
    val lines = listOfNotNull(startLine?.let { "\"startLine\": $it" }, endLine?.let { "\"endLine\": $it" })
    val region = if (lines.isEmpty()) "" else """, "region": {${lines.joinToString()}}"""
    return """
      {
        "ruleId": "qodana.internal.$clusterId",
        "level": "warning",
        "message": {"text": "$FINDING_MESSAGE"},
        "locations": [{"physicalLocation": {"artifactLocation": {"uri": "$uri", "uriBaseId": "SRCROOT"}$region}}]
      }
    """.trimIndent()
}

/** A result of the profile's own inspections, which feedback leaves alone. */
private fun regularResult(): String = """
  {
    "ruleId": "UNUSED_IMPORT",
    "message": {"text": "Unused import directive"},
    "locations": [{"physicalLocation": {"artifactLocation": {"uri": "src/Main.kt", "uriBaseId": "SRCROOT"}, "region": {"startLine": 1}}}]
  }
""".trimIndent()
