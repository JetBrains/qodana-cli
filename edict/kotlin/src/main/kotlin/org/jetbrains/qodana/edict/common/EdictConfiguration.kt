// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.common

import org.yaml.snakeyaml.LoaderOptions
import org.yaml.snakeyaml.Yaml
import org.yaml.snakeyaml.constructor.Constructor
import org.yaml.snakeyaml.error.YAMLException
import org.yaml.snakeyaml.introspector.MissingProperty
import org.yaml.snakeyaml.introspector.Property
import org.yaml.snakeyaml.introspector.PropertyUtils
import java.net.URI
import java.nio.file.Files
import java.nio.file.InvalidPathException
import java.nio.file.Path
import kotlin.io.path.isRegularFile

internal data class EdictConfiguration(
  val mcpPort: Int = DEFAULT_EDICT_MCP_PORT,
  val statePath: String = DEFAULT_EDICT_STATE_PATH,
  val ci: EdictCIConfiguration? = null,
  val promotion: PromotionConfiguration? = null,
) {
  init {
    require(mcpPort in 1..65535) { "edict.mcpPort must be between 1 and 65535: $mcpPort" }
    require(statePath.isNotBlank()) { "edict.statePath must not be blank" }
    try {
      Path.of(statePath)
    }
    catch (e: InvalidPathException) {
      throw IllegalArgumentException("edict.statePath is invalid: $statePath", e)
    }
  }
}

internal const val DEFAULT_EDICT_MCP_PORT = 27182
internal const val DEFAULT_EDICT_STATE_PATH = ".edict"

internal enum class EdictCIProvider { GITHUB, SPACE }

internal data class EdictCIConfiguration(val url: String) {
  private val path = webUrl(url, "CI repository").pathSegments()

  init {
    require(path.size == 2 || path.size == 4 && path[0] == "p" && path[2] == "repositories") {
      "CI repository URL must end with /<owner>/<repository> for GitHub or /p/<project-key>/repositories/<repository> for Space"
    }
  }

  val provider = if (path.size == 2) EdictCIProvider.GITHUB else EdictCIProvider.SPACE
  val owner = path[if (provider == EdictCIProvider.GITHUB) 0 else 1].validRepositoryName("CI owner or project key")
  val repository = path.last().removeSuffix(".git").validRepositoryName("CI repository")
}

internal data class PromotionConfiguration(
  val reviewer: String,
  val targetBranch: String? = null,
  val inspectionsDirectory: String = "inspections",
) {
  init {
    require(reviewer.isNotBlank() && reviewer.none { it in "/\\\u0000\r\n" }) { "Promotion reviewer is invalid" }
    targetBranch?.let {
      require(
        it.isNotBlank() && !it.startsWith('-') && !it.endsWith('.') &&
          ".." !in it && it.none { character -> character in " ~^:?*[\\\u0000\r\n" },
      ) { "Promotion target branch is invalid" }
    }
    val path = Path.of(inspectionsDirectory)
    require(inspectionsDirectory.isNotBlank() && !path.isAbsolute && path.none { it.toString() == ".." }) {
      "Promotion inspections directory must stay inside the target repository"
    }
  }
}

internal object EdictYamlConfiguration {
  fun load(path: Path?): EdictConfiguration {
    if (path == null) return EdictConfiguration()
    require(path.isRegularFile()) { "Qodana configuration does not exist or is not a file: $path" }
    require(Files.size(path) <= MAX_YAML_BYTES) { "Qodana configuration exceeds 2 MiB: $path" }

    val options = LoaderOptions().apply {
      isAllowDuplicateKeys = false
      maxAliasesForCollections = 50
      nestingDepthLimit = 50
      codePointLimit = MAX_YAML_BYTES
    }
    val constructor = Constructor(QodanaYaml::class.java, options).apply { propertyUtils = QodanaPropertyUtils() }
    val yaml = try {
      Files.newBufferedReader(path).use { reader -> Yaml(constructor).load<QodanaYaml?>(reader) }
    }
    catch (e: YAMLException) {
      throw IllegalArgumentException("Cannot parse Qodana configuration $path: ${e.message}", e)
    } ?: return EdictConfiguration()

    return yaml.edict?.toConfiguration() ?: EdictConfiguration()
  }

  private const val MAX_YAML_BYTES = 2 * 1024 * 1024
}

internal class QodanaYaml {
  var edict: EdictYaml? = null
}

internal class EdictYaml {
  var mcpPort: Int? = null
  var statePath: String? = null
  var ci: EdictCIYaml? = null
  var promotion: PromotionYaml? = null

  fun toConfiguration() = EdictConfiguration(
    mcpPort = mcpPort ?: DEFAULT_EDICT_MCP_PORT,
    statePath = statePath?.required("edict.statePath") ?: DEFAULT_EDICT_STATE_PATH,
    ci = ci?.toConfiguration(),
    promotion = promotion?.toConfiguration(),
  )
}

internal class EdictCIYaml {
  var url: String? = null

  fun toConfiguration() = EdictCIConfiguration(url.required("edict.ci.url"))
}

internal class PromotionYaml {
  var reviewer: String? = null
  var targetBranch: String? = null
  var inspectionsDirectory: String? = null

  fun toConfiguration() = PromotionConfiguration(
    reviewer = reviewer.required("edict.promotion.reviewer"),
    targetBranch = targetBranch?.required("edict.promotion.targetBranch"),
    inspectionsDirectory = inspectionsDirectory?.required("edict.promotion.inspectionsDirectory") ?: "inspections",
  )
}

private class QodanaPropertyUtils : PropertyUtils() {
  override fun getProperty(type: Class<out Any>, name: String): Property =
    if (type == QodanaYaml::class.java) runCatching { super.getProperty(type, name) }.getOrElse { MissingProperty(name) }
    else super.getProperty(type, name)
}

private fun String?.required(path: String): String =
  this?.trim()?.takeIf(String::isNotEmpty) ?: error("$path is required")

private fun webUrl(value: String, label: String): URI {
  val uri = try {
    URI(value)
  }
  catch (e: Exception) {
    throw IllegalArgumentException("$label URL is invalid", e)
  }
  require(
    uri.scheme in setOf("http", "https") && uri.host != null && uri.userInfo == null &&
      uri.query == null && uri.fragment == null,
  ) { "$label must be an HTTP(S) URL without credentials, query, or fragment" }
  return uri
}

private fun URI.pathSegments(): List<String> = path.trim('/').split('/').filter(String::isNotEmpty)

private fun String.validRepositoryName(label: String): String = also {
  require(isNotBlank() && this !in setOf(".", "..") && none { character -> character in "/\\\u0000\r\n" }) {
    "$label is invalid"
  }
}
