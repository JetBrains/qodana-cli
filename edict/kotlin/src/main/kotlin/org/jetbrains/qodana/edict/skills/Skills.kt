// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.skills

import org.jetbrains.qodana.edict.skills.managed.Registry
import java.nio.file.Files
import java.nio.file.Path

object Skills {
    private val resources: List<String> by lazy { resource("/skills/index.txt").lines().filter(String::isNotBlank) }
    private val resourceRoots: Map<String, String> by lazy {
        resources.filter { it.endsWith("/SKILL.md") }.associate { entry ->
            val name = resource("/skills/$entry").lineSequence()
                .firstOrNull { it.startsWith("name: ") }
                ?.removePrefix("name: ")
                ?: error("Bundled skill has no name: $entry")
            name to entry.substringBeforeLast('/')
        }
    }
    val names: List<String> by lazy {
        Registry.policies.map { Registry.installedName(it.name) }.sorted().also { managed ->
            check(resourceRoots.keys == managed.toSet()) {
                "Bundled skills do not match the managed registry"
            }
        }
    }

    fun install(destination: Path, name: String? = null): List<String> {
        val selected = name?.let { require(it in names) { "Unknown bundled managed skill: $it" }; listOf(it) } ?: names
        return install(destination, selected)
    }

    private fun install(destination: Path, selected: List<String>): List<String> {
        selected.forEach { name ->
            val root = resourceRoots.getValue(name)
            resources.filter { it == "$root/SKILL.md" || it.startsWith("$root/") }.forEach { entry ->
                val target = destination.resolve(name).resolve(entry.removePrefix("$root/"))
                Files.createDirectories(target.parent)
                Files.writeString(target, resource("/skills/$entry"))
            }
        }
        return selected
    }

    fun read(name: String): String {
        require(name in names) { "Unknown bundled skill: $name" }
        return resource("/skills/${resourceRoots.getValue(name)}/SKILL.md")
    }

    private fun resource(name: String): String = checkNotNull(javaClass.getResourceAsStream(name)) { "Missing resource $name" }
        .bufferedReader().use { it.readText() }

}
