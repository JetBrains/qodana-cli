// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.skills.managed

import kotlinx.serialization.Serializable

@Serializable
data class Policy(
    val name: String,
    val delegates: List<String> = emptyList(),
)

object Registry {
    val policies: List<Policy> = listOf(
        Policy(
            "edict_manager",
            delegates = listOf(
                "edict-next-run",
                "edict-next-distribution",
                "edict-next-generation",
                "edict-batch-signal-analysis",
                "edict-pr-signal-analysis",
                "edict-git-history-signal-analysis",
                "edict-retrospective-signal-analysis",
                "edict-promote",
                "ecict-check-promotion",
            ),
        ),
        Policy(
            "edict-next-run",
            delegates = listOf("edict-next-distribution", "edict-next-generation"),
        ),
        Policy("edict-next-distribution"),
        Policy("edict-next-generation", delegates = listOf("edict-next-cluster-generation")),
        Policy(
            "edict-next-cluster-generation",
            listOf(
                "edict-next-code-example-overseer",
                "edict-next-inspection-code-review",
                "edict-next-weak-signal-review",
            ),
        ),
        Policy("edict-next-code-example-overseer", delegates = listOf("edict-next-code-example")),
        Policy("edict-next-code-example"),
        Policy("edict-next-inspection-code-review"),
        Policy("edict-next-weak-signal-review", delegates = listOf("edict-next-code-example")),
        Policy(
            "edict-batch-signal-analysis",
            delegates = listOf("edict-signal-analysis"),
        ),
        Policy(
            "edict-pr-signal-analysis",
            delegates = listOf("edict-signal-analysis"),
        ),
        Policy(
            "edict-git-history-signal-analysis",
            delegates = listOf("edict-signal-analysis"),
        ),
        Policy(
            "edict-retrospective-signal-analysis",
            delegates = listOf("edict-signal-analysis"),
        ),
        Policy("edict-signal-analysis"),
        Policy("edict-promote"),
        Policy("ecict-check-promotion", delegates = listOf("edict-promotion-decision")),
        Policy("edict-promotion-decision"),
    )

    private val byName = policies.associateBy(Policy::name)

    operator fun get(name: String): Policy = byName[name] ?: error("Unknown managed skill: $name")

    fun installedName(name: String): String = get(name).name

    fun skillPath(name: String): String = "${installedName(name)}/SKILL.md"
}
