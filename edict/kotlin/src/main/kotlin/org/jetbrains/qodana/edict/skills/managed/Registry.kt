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
                "edict-distribution",
                "edict-cluster-generation",
                "edict-batch-signal-analysis",
                "edict-pr-signal-analysis",
                "edict-git-history-signal-enrichment",
                "edict-retrospective-signal-analysis",
                "edict-promote",
                "edict-promote-weak-signal",
                "edict-check-weak-signal-promotion",
                "edict-check-promotion",
                "edict-feedback-signal-analysis",
            ),
        ),
        Policy("edict-distribution"),
        Policy(
            "edict-cluster-generation",
            listOf("edict-code-example-overseer", "edict-generation-round"),
        ),
        Policy(
            "edict-generation-round",
            listOf(
                "edict-inspection-shallow-review",
                "edict-weak-signal-review",
                "edict-inspection-code-review",
            ),
        ),
        Policy("edict-code-example-overseer", delegates = listOf("edict-code-example", "edict-signal-mapping")),
        Policy("edict-code-example"),
        Policy("edict-signal-mapping"),
        Policy("edict-inspection-shallow-review"),
        Policy("edict-inspection-code-review"),
        Policy("edict-weak-signal-review"),
        Policy(
            "edict-batch-signal-analysis",
            delegates = listOf("edict-signal-analysis"),
        ),
        Policy(
            "edict-pr-signal-analysis",
            delegates = listOf("edict-signal-analysis"),
        ),
        Policy("edict-git-history-signal-enrichment"),
        Policy(
            "edict-retrospective-signal-analysis",
            delegates = listOf("edict-signal-analysis"),
        ),
        Policy("edict-signal-analysis"),
        Policy("edict-feedback-signal-analysis"),
        Policy("edict-promote"),
        Policy("edict-promote-weak-signal"),
        Policy("edict-check-weak-signal-promotion"),
        Policy("edict-check-promotion", delegates = listOf("edict-promotion-decision")),
        Policy("edict-promotion-decision"),
    )

    private val byName = policies.associateBy(Policy::name)

    operator fun get(name: String): Policy = byName[name] ?: error("Unknown managed skill: $name")

    fun installedName(name: String): String = get(name).name

    fun skillPath(name: String): String = "${installedName(name)}/SKILL.md"
}
