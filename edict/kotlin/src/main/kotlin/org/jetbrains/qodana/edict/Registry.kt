// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict

import kotlinx.serialization.Serializable

@Serializable data class Policy(
    val name: String, val operations: List<String> = emptyList(),
    val writes: List<String> = emptyList(), val delegates: List<String> = emptyList(),
)

object Registry {
    private val generation = listOf("cluster.write", "cluster.signal.write", "example.write", "inspection.write")
    private val all = listOf("inbox.write", "inbox.delete") + generation
    val policies: List<Policy> = listOf(
        Policy("edict_manager", all, delegates = listOf("edict-run", "edict-batch-signal-analysis", "edict-pr-signal-analysis", "edict-distribution", "edict-generation")),
        Policy("edict-run", generation + "inbox.delete", delegates = listOf("edict-prepare", "edict-distribution", "edict-generation")),
        Policy("edict-prepare"),
        Policy("edict-distribution", listOf("inbox.delete", "cluster.write", "cluster.signal.write"), listOf("inbox.delete", "cluster.write", "cluster.signal.write")),
        Policy("edict-generation", generation, delegates = listOf("edict-cluster-generation")),
        Policy("edict-cluster-generation", generation, listOf("cluster.write", "inspection.write"), listOf("edict-code-example", "edict-inspection-code-review", "edict-weak-signal-review", "edict-inspection-value-review")),
        Policy("edict-code-example", listOf("example.write", "cluster.signal.write"), listOf("example.write", "cluster.signal.write")),
        Policy("edict-inspection-code-review"),
        Policy("edict-inspection-value-review"),
        Policy("edict-weak-signal-review", listOf("example.write", "cluster.signal.write"), delegates = listOf("edict-code-example")),
        Policy("edict-batch-signal-analysis", listOf("inbox.write"), listOf("inbox.write"), listOf("edict-signal-analysis")),
        Policy("edict-pr-signal-analysis", listOf("inbox.write"), listOf("inbox.write"), listOf("edict-signal-analysis")),
        Policy("edict-signal-analysis"),
    )
    private val byName = policies.associateBy(Policy::name)
    operator fun get(name: String): Policy = byName[name] ?: error("Unknown managed skill: $name")
    fun installedName(name: String): String = if (name == "edict_manager") name else "managed-$name"
    fun skillPath(name: String): String = "${installedName(name)}/SKILL.md"
}
