// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.common.support

import org.jetbrains.qodana.edict.support.resource
import org.yaml.snakeyaml.DumperOptions
import org.yaml.snakeyaml.Yaml
import java.nio.file.Path
import kotlin.io.path.readText
import kotlin.io.path.writeText

/** The Qodana YAML file `config/<name>.yaml` of the test resources, for documents whose exact text is the case. */
internal fun qodanaYaml(name: String): Path = configResource("$name.yaml")

/** A `qodana.yaml` in [directory] holding [document]. */
internal fun qodanaYaml(directory: Path, document: String): Path =
  directory.resolve("qodana.yaml").also { it.writeText(document) }

/**
 * A `qodana.yaml` in [directory] whose `edict` section sets [values]: each maps a dotted path below `edict`, such as
 * `ci.url`, to its value. A later value of the same path replaces an earlier one; `null` writes the key without a value.
 */
internal fun edictYaml(directory: Path, vararg values: Pair<String, Any?>): Path =
  qodanaYaml(directory, YAML.dump(mapOf("edict" to section(values.toList()))))

/** [edictYaml] of a valid promotion by `reviewer` in the CI repository [KOTLIN_URL], with [values] set over it. */
internal fun promotionYaml(directory: Path, vararg values: Pair<String, Any?>): Path =
  edictYaml(directory, "ci.url" to KOTLIN_URL, "promotion.reviewer" to "reviewer", *values)

/** The URL of the `KOTLIN` repository. */
internal const val KOTLIN_URL = "https://github.com/JetBrains/kotlin"

/**
 * The message loading [qodanaYaml] of [name] fails with, from `config/<name>.message.txt`: the file's path stands for
 * `<path>`, and `<hash>` for the identity hash of each object the YAML parser names (see [withoutIdentityHashes]).
 */
internal fun parseFailure(name: String): String =
  configResource("$name.message.txt").readText().replace("<path>", qodanaYaml(name).toString())

/** [message] with the identity hash of each object it names, which differs per run, replaced by `<hash>`. */
internal fun withoutIdentityHashes(message: String?): String? = message?.replace(IDENTITY_HASH, "@<hash>")

/** A Qodana YAML file in [directory] of exactly [bytes] bytes: an empty Edict section, padded by a comment. */
internal fun paddedQodanaYaml(directory: Path, bytes: Int): Path {
  val section = "edict: {}\n#"
  return qodanaYaml(directory, section + "x".repeat(bytes - section.length - 1) + "\n")
}

/** The largest Qodana YAML file Edict reads: 2 MiB. */
internal const val MAX_QODANA_YAML_BYTES = 2 * 1024 * 1024

/** The nested mapping of dotted [values], each a path relative to this section paired with its value. */
private fun section(values: List<Pair<String, Any?>>): Map<String, Any?> =
  values.groupBy({ it.first.substringBefore('.') }) { it.first.substringAfter('.', "") to it.second }
    .mapValues { (_, children) -> if (children.any { it.first.isEmpty() }) children.last().second else section(children) }

private val YAML = Yaml(DumperOptions().apply { defaultFlowStyle = DumperOptions.FlowStyle.BLOCK })

private fun configResource(name: String): Path = Path.of(resource("config/$name").toURI())

private val IDENTITY_HASH = Regex("@[0-9a-f]+\\b")
