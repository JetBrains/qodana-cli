// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.common

import org.jetbrains.qodana.edict.ci.CiProviderId
import org.jetbrains.qodana.edict.ci.ReviewRepository
import java.net.URI
import java.net.URISyntaxException
import org.yaml.snakeyaml.LoaderOptions
import org.yaml.snakeyaml.Yaml
import org.yaml.snakeyaml.constructor.Constructor
import org.yaml.snakeyaml.error.YAMLException
import org.yaml.snakeyaml.introspector.MissingProperty
import org.yaml.snakeyaml.introspector.Property
import org.yaml.snakeyaml.introspector.PropertyUtils
import java.nio.file.Files
import java.nio.file.InvalidPathException
import java.nio.file.Path
import kotlin.io.path.isRegularFile

// TODO: I believe promotion at least should be non-null, as ci may be missing if user supplied different signal source, but promotion cannot be missing
// TODO: (Keep this one) - currently we have the whole pipeline too coupled, as if we require each part for edict pipeline to function. We need to split it somehow
internal data class EdictConfiguration(
  val mcpPort: Int = DEFAULT_EDICT_MCP_PORT,
  val statePath: String = DEFAULT_EDICT_STATE_PATH,
  /** The branch of the project's repository that `qodana edict install` checks out at [statePath]. */
  val stateBranch: String = DEFAULT_EDICT_STATE_BRANCH,
  val ci: ReviewRepository? = null,
  val promotion: PromotionConfiguration? = null,
  val extraction: ExtractionConfiguration = ExtractionConfiguration(),
  val distribution: DistributionConfiguration = DistributionConfiguration(),
  val generation: GenerationConfiguration = GenerationConfiguration(),
  val calculatePrice: Boolean = false,
  val projectAnalysisTimeoutMinutes: Long = DEFAULT_PROJECT_ANALYSIS_TIMEOUT_MINUTES,
  /** Review accounts that post as users but are bots, such as a build server's; providers mark their apps themselves. */
  val bots: Set<String> = emptySet(),
) {
  /** Where promotion PRs are opened: `edict.promotion.url`, else the `edict.ci.url` repository reviews are read from. */
  val promotionRepository: ReviewRepository? = promotion?.repository ?: ci

  init {
    require(promotion == null || promotionRepository != null) { "edict.promotion requires edict.promotion.url or edict.ci.url" }
    require(mcpPort in 1..65535) { "edict.mcpPort must be between 1 and 65535: $mcpPort" }
    require(projectAnalysisTimeoutMinutes >= 1) {
      "edict.projectAnalysisTimeoutMinutes must be at least 1: $projectAnalysisTimeoutMinutes"
    }
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
internal const val DEFAULT_PROJECT_ANALYSIS_TIMEOUT_MINUTES = 30L

/** [defaultSignalCount] is the daily extraction target when the request does not specify one. */
internal data class ExtractionConfiguration(
  val defaultSignalCount: Int = DEFAULT_SIGNAL_COUNT,
) {
  init {
    require(defaultSignalCount >= 1) {
      "edict.extraction.defaultSignalCount must be at least 1: $defaultSignalCount"
    }
  }
}

internal const val DEFAULT_SIGNAL_COUNT = 100

/** [defaultSignalCount] caps the distribution batch; configured only through Qodana YAML. */
internal data class DistributionConfiguration(
  val defaultSignalCount: Int = DEFAULT_DISTRIBUTION_SIGNAL_COUNT,
) {
  init {
    require(defaultSignalCount >= 1) {
      "edict.distribution.defaultSignalCount must be at least 1: $defaultSignalCount"
    }
  }
}

internal const val DEFAULT_DISTRIBUTION_SIGNAL_COUNT = 300

/**
 * [maxProjectAnalyses] caps the expensive project analysis, and the reviews that follow it, per cluster in one run.
 * [defaultGenerationCount] is the number of generation targets used when the generation request does not specify one.
 */
internal data class GenerationConfiguration(
  val maxProjectAnalyses: Int = DEFAULT_MAX_PROJECT_ANALYSES,
  val defaultGenerationCount: Int = DEFAULT_GENERATION_COUNT,
) {
  init {
    require(maxProjectAnalyses >= 1) { "edict.generation.maxProjectAnalyses must be at least 1: $maxProjectAnalyses" }
    require(defaultGenerationCount >= 1) {
      "edict.generation.defaultGenerationCount must be at least 1: $defaultGenerationCount"
    }
  }
}

internal const val DEFAULT_MAX_PROJECT_ANALYSES = 3
internal const val DEFAULT_GENERATION_COUNT = 5

internal data class PromotionConfiguration(
  val reviewer: String,
  val targetBranch: String? = null,
  val inspectionsDirectory: String = "inspections",
  val targetPrecision: Double = 0.7,
  val targetRecall: Double = 0.6,
  val targetF1: Double = 0.0,
  /** Where promotion PRs are opened when it isn't the `edict.ci.url` repository. */
  val repository: ReviewRepository? = null,
) {
  init {
    mapOf("targetPrecision" to targetPrecision, "targetRecall" to targetRecall, "targetF1" to targetF1).forEach { (name, value) ->
      require(value.isFinite() && value in 0.0..1.0) { "edict.promotion.$name must be between 0 and 1: $value" }
    }
    require(reviewer.isNotBlank() && reviewer.none { it in "/\\\u0000\r\n" }) { "Promotion reviewer is invalid" }
    targetBranch?.let {
      require(
        it.isNotBlank() && !it.startsWith('-') && !it.endsWith('.') &&
          ".." !in it && it.none { character -> character in " ~^:?*[\\\u0000\r\n" },
      ) { "Promotion target branch is invalid" }
      // Branches are hashed into promotion ids and looked up by name, so one branch must have one spelling.
      require(!it.startsWith("refs/")) { "Name the promotion target branch without refs/heads/" }
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

    val options = LoaderOptions().apply { isAllowDuplicateKeys = false }
    val constructor = Constructor(QodanaYaml::class.java, options).apply { propertyUtils = QodanaPropertyUtils() }
    val yaml = try {
      Files.newBufferedReader(path).use { reader -> Yaml(constructor).load<QodanaYaml?>(reader) }
    } catch (e: YAMLException) {
      throw IllegalArgumentException("Cannot parse Qodana configuration $path: ${e.message}", e)
    }

    return yaml?.edict?.toConfiguration() ?: EdictConfiguration()
  }

  private const val MAX_YAML_BYTES = 2 * 1024 * 1024
}

internal class QodanaYaml {
  var edict: EdictYaml? = null
}

internal class EdictYaml {
  var mcpPort: Int? = null
  var statePath: String? = null
  var stateBranch: String? = null
  var ci: EdictCIYaml? = null
  var promotion: PromotionYaml? = null
  var extraction: ExtractionYaml? = null
  var distribution: DistributionYaml? = null
  var generation: GenerationYaml? = null
  var calculatePrice: Boolean? = null
  var projectAnalysisTimeoutMinutes: Long? = null
  var bots: List<String>? = null

  fun toConfiguration() = EdictConfiguration(
    mcpPort = mcpPort ?: DEFAULT_EDICT_MCP_PORT,
    statePath = statePath?.checkNotEmpty("edict.statePath") ?: DEFAULT_EDICT_STATE_PATH,
    stateBranch = stateBranch?.checkNotEmpty("edict.stateBranch") ?: DEFAULT_EDICT_STATE_BRANCH,
    ci = ci?.toConfiguration(),
    promotion = promotion?.toConfiguration(),
    extraction = extraction?.toConfiguration() ?: ExtractionConfiguration(),
    distribution = distribution?.toConfiguration() ?: DistributionConfiguration(),
    generation = generation?.toConfiguration() ?: GenerationConfiguration(),
    calculatePrice = calculatePrice ?: false,
    projectAnalysisTimeoutMinutes = projectAnalysisTimeoutMinutes ?: DEFAULT_PROJECT_ANALYSIS_TIMEOUT_MINUTES,
    bots = bots.orEmpty().mapIndexed { i, bot -> bot.checkNotEmpty("edict.bots[$i]") }.toSet(),
  )
}

internal class ExtractionYaml {
  var defaultSignalCount: Int? = null

  fun toConfiguration() = ExtractionConfiguration(
    defaultSignalCount = defaultSignalCount ?: DEFAULT_SIGNAL_COUNT,
  )
}

internal class DistributionYaml {
  var defaultSignalCount: Int? = null

  fun toConfiguration() = DistributionConfiguration(
    defaultSignalCount = defaultSignalCount ?: DEFAULT_DISTRIBUTION_SIGNAL_COUNT,
  )
}

internal class GenerationYaml {
  var maxProjectAnalyses: Int? = null
  var defaultGenerationCount: Int? = null

  fun toConfiguration() = GenerationConfiguration(
    maxProjectAnalyses = maxProjectAnalyses ?: DEFAULT_MAX_PROJECT_ANALYSES,
    defaultGenerationCount = defaultGenerationCount ?: DEFAULT_GENERATION_COUNT,
  )
}

internal class EdictCIYaml {
  var url: String? = null

  fun toConfiguration() = reviewRepository(url.checkNotEmpty("edict.ci.url"), "edict.ci.url")
}

internal class PromotionYaml {
  var reviewer: String? = null
  var targetBranch: String? = null
  var inspectionsDirectory: String? = null
  var targetPrecision: Double? = null
  var targetRecall: Double? = null
  var targetF1: Double? = null
  var url: String? = null

  fun toConfiguration() = PromotionConfiguration(
    reviewer = reviewer.checkNotEmpty("edict.promotion.reviewer"),
    targetBranch = targetBranch?.checkNotEmpty("edict.promotion.targetBranch"),
    inspectionsDirectory = inspectionsDirectory?.checkNotEmpty("edict.promotion.inspectionsDirectory") ?: "inspections",
    targetPrecision = targetPrecision ?: 0.7,
    targetRecall = targetRecall ?: 0.6,
    targetF1 = targetF1 ?: 0.0,
    repository = url?.checkNotEmpty("edict.promotion.url")?.let { reviewRepository(it, "edict.promotion.url") },
  )
}

private class QodanaPropertyUtils : PropertyUtils() {
  override fun getProperty(type: Class<out Any>, name: String): Property =
    if (type == QodanaYaml::class.java) runCatching { super.getProperty(type, name) }.getOrElse { MissingProperty(name) }
    else super.getProperty(type, name)
}

private fun String?.checkNotEmpty(name: String): String =
  this?.trim()?.takeIf(String::isNotEmpty) ?: error("$name is required to be defined as non-empty string")

/**
 * The repository a web URL names, by its path: `https://<host>/<owner>/<repo>` on GitHub and
 * `https://<host>/p/<project-key>/repositories/<repo>` on Space.
 */
internal fun reviewRepository(url: String, path: String): ReviewRepository {
  val uri = try {
    URI(url)
  }
  catch (e: URISyntaxException) {
    throw IllegalArgumentException("$path is not a URL: $url", e)
  }
  require(uri.scheme == "https" && uri.host != null && uri.port == -1 && uri.userInfo == null && uri.query == null && uri.fragment == null) {
    "$path must be an https URL without a port, credentials, query or fragment: $url"
  }
  val host = uri.host.lowercase()
  val segments = uri.path.split('/').filter(String::isNotEmpty)
  return when {
    segments.size == 2 -> ReviewRepository(CiProviderId.GITHUB, host, segments[0], segments[1].removeSuffix(".git"))
    segments.size == 4 && segments[0] == "p" && segments[2] == "repositories" ->
      ReviewRepository(CiProviderId.SPACE, host, segments[1], segments[3].removeSuffix(".git"))
    else -> throw IllegalArgumentException(
      "$path must end with /<owner>/<repository> for GitHub or /p/<project-key>/repositories/<repository> for Space: $url",
    )
  }
}
