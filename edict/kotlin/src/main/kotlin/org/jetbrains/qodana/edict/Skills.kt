// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict

import java.nio.file.Files
import java.nio.file.Path

object Skills {
    private val resources: List<String> by lazy { resource("/skills/index.txt").lines().filter(String::isNotBlank) }
    val names: List<String> by lazy {
        resources.map { it.substringBefore('/') }.distinct().sorted().also { bundled ->
            check(bundled == Registry.policies.map { Registry.installedName(it.name) }.sorted()) {
                "Bundled managed skills do not match the registry"
            }
        }
    }

    fun install(destination: Path, name: String? = null): List<String> {
        val selected = name?.let { require(it in names) { "Unknown bundled managed skill: $it" }; listOf(it) } ?: names
        resources.filter { it.substringBefore('/') in selected }.forEach { entry ->
            val target = destination.resolve(entry)
            Files.createDirectories(target.parent)
            Files.writeString(target, resource("/skills/$entry"))
        }
        return selected
    }

    fun read(name: String): String {
        require(name in names) { "Unknown bundled managed skill: $name" }
        return resource("/skills/$name/SKILL.md")
    }

    private fun resource(name: String): String = checkNotNull(javaClass.getResourceAsStream(name)) { "Missing resource $name" }
        .bufferedReader().use { it.readText() }
}
