// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

import com.jetbrains.qodana.sarif.SarifUtil
import com.jetbrains.qodana.sarif.model.OriginalUriBaseIds
import com.jetbrains.qodana.sarif.model.Result
import org.jetbrains.qodana.edict.core.EdictLineRange
import org.jetbrains.qodana.edict.signals.validSourcePath
import java.nio.file.Path

/** The prefix Qodana gives the ids of the inspections it runs for feedback, so that they never clash with the profile's. */
private const val INTERNAL_INSPECTION_ID_PREFIX = "qodana.internal."

/** The run property of a SARIF report that holds the results of those inspections, apart from the report's own. */
private const val INTERNAL_RESULTS_PROPERTY = "qodana.internal.results"

/** A problem that an inspection run for feedback reported. */
internal data class InternalFinding(
    val clusterId: String,
    /** The repository-relative path of the file the problem is in. */
    val path: String,
    val lines: EdictLineRange,
    /** The problem, as the inspection's message says it. */
    val message: String,
)

/** What a SARIF report holds for feedback: the [findings] of the inspections run for it, on the commit [revision]. */
internal data class FeedbackReport(
    /** The commit the report names as analysed, as written; blank when it names none. */
    val revision: String,
    val findings: List<InternalFinding>,
)

/**
 * The SARIF report at [sarif], with the problems that the inspections run for feedback reported on lines of files: a problem
 * of a whole file or project has no line to comment on, and one in a file whose path no Signal can hold is left out. Read as the IDE's tests read the property, without the report's own
 * results, which may be many.
 */
internal fun feedbackReport(sarif: Path): FeedbackReport {
    val runs = SarifUtil.readReport(sarif, false).runs.orEmpty()
    val findings = runs.flatMap { run ->
        val results = run.properties?.get(INTERNAL_RESULTS_PROPERTY)?.let(SarifUtil::readResultsFromObject).orEmpty()
        results.mapNotNull { internalFinding(it, run.originalUriBaseIds ?: OriginalUriBaseIds()) }
    }
    val revision = runs.firstOrNull()?.versionControlProvenance?.firstOrNull()?.revisionId.orEmpty()
    return FeedbackReport(revision, findings)
}

private fun internalFinding(result: Result, baseIds: OriginalUriBaseIds): InternalFinding? {
    val location = result.locations?.firstOrNull()?.physicalLocation ?: return null
    val artifact = location.artifactLocation ?: return null
    val region = location.region ?: return null
    val uri = artifact.uri?.takeIf(String::isNotEmpty) ?: return null
    val start = region.startLine?.takeIf { it > 0 } ?: return null
    val end = (region.endLine ?: start).coerceAtLeast(start)
    val path = (baseUri(baseIds, artifact.uriBaseId, emptySet()) + uri).takeIf(::validSourcePath) ?: return null
    return InternalFinding(
        result.ruleId.orEmpty().removePrefix(INTERNAL_INSPECTION_ID_PREFIX),
        path,
        EdictLineRange(start, end),
        result.message?.text.orEmpty(),
    )
}

/**
 * The directory [baseId] names, following the base it is relative to, as the Qodana action resolves annotation paths: the
 * project directory relative to the repository root, which has no URI. An unknown or cyclic base names none.
 */
private fun baseUri(baseIds: OriginalUriBaseIds, baseId: String?, visited: Set<String>): String {
    val base = baseIds[baseId ?: return ""] ?: return ""
    if (baseId in visited) return ""
    val parent = base.uriBaseId
    val uri = (if (parent.isNullOrEmpty()) "" else baseUri(baseIds, parent, visited + baseId)) + base.uri.orEmpty()
    return if (uri.isEmpty() || uri.endsWith('/')) uri else "$uri/"
}
