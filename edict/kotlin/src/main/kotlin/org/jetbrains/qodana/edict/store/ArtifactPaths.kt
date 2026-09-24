// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.store

import org.jetbrains.qodana.edict.signals.validSourcePath

internal val identifier = Regex("[a-z0-9][a-z0-9_-]*")

internal fun covered(scope: List<String>, name: String): Boolean = scope.any { name == it || name.startsWith("$it/") }
internal fun validArtifactPath(name: String) {
    require(
        validSourcePath(name) && name.split('/')
            .none { it.endsWith('.') || it.endsWith(' ') }) { "Invalid relative artifact path '$name'" }
}

internal fun operation(name: String): String {
    if (runCatching { validArtifactPath(name) }.isFailure) return ""
    val p = name.split('/')
    if (p.size == 2 && p[0] == "inbox" && p[1].endsWith(".json") && identifier.matches(p[1].removeSuffix(".json"))) return "inbox.write"
    if (p.size == 2 && p[0] == "inspections" && listOf(
            ".candidate.kts",
            ".inspection.kts"
        ).any { p[1].endsWith(it) && identifier.matches(p[1].removeSuffix(it)) }
    ) return "inspection.write"
    if (p.size >= 3 && p[0] == "clusters" && identifier.matches(p[1])) {
        if (p.size == 3 && p[2] in listOf("description.json", "history.md")) return "cluster.write"
        if (p.size == 4 && p[2] == "signals" && p[3].endsWith(".json") && identifier.matches(p[3].removeSuffix(".json"))) return "cluster.signal.write"
        if (p.size >= 5 && p[2] == "synthetic-examples" && identifier.matches(p[3]) &&
            (p.size == 5 && p[4] == "metadata.json" || p.size >= 6 && p[4] == "project")
        ) return "example.write"
    }
    return ""
}

internal fun readable(name: String): Boolean = operation(name).isNotEmpty() ||
        runCatching { validArtifactPath(name) }.isSuccess && name.startsWith("plans/") && name.count { it == '/' } == 1 && name.endsWith(
    ".json"
)
