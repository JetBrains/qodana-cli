// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.common

import org.jetbrains.qodana.edict.ci.CiProviderId
import org.jetbrains.qodana.edict.ci.ReviewRepository
import org.jetbrains.qodana.edict.ci.support.KOTLIN
import org.jetbrains.qodana.edict.common.support.KOTLIN_URL
import org.jetbrains.qodana.edict.common.support.MAX_QODANA_YAML_BYTES
import org.jetbrains.qodana.edict.common.support.edictYaml
import org.jetbrains.qodana.edict.common.support.paddedQodanaYaml
import org.jetbrains.qodana.edict.common.support.parseFailure
import org.jetbrains.qodana.edict.common.support.promotionYaml
import org.jetbrains.qodana.edict.common.support.qodanaYaml
import org.jetbrains.qodana.edict.common.support.withoutIdentityHashes
import org.junit.jupiter.api.io.TempDir
import org.junit.jupiter.params.ParameterizedTest
import org.junit.jupiter.params.provider.CsvSource
import org.junit.jupiter.params.provider.ValueSource
import java.nio.file.Path
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

internal class EdictYamlConfigurationTest {
  @TempDir
  lateinit var directory: Path

  @Test
  fun `no Qodana configuration gives the defaults`() {
    assertEquals(EdictConfiguration(), EdictYamlConfiguration.load(null))
  }

  @ParameterizedTest
  @ValueSource(strings = ["", "version: '1.0'", "edict: {}", "edict: {extraction: {}, distribution: {}, generation: {}}"])
  fun `a configuration that sets no Edict value gives the defaults, whatever else it configures`(document: String) {
    assertEquals(EdictConfiguration(), EdictYamlConfiguration.load(qodanaYaml(directory, document)))
  }

  @Test
  fun `every Edict value is read from its section, ignoring the rest of the Qodana configuration`() {
    assertEquals(
      EdictConfiguration(
        mcpPort = 27200,
        statePath = "../edict-state",
        ci = KOTLIN,
        promotion = PromotionConfiguration("reviewer-login", "release/2.3", "quality/inspections", KOTLIN_EDICT),
        extraction = ExtractionConfiguration(250),
        distribution = DistributionConfiguration(750),
        generation = GenerationConfiguration(maxProjectAnalyses = 5, defaultGenerationCount = 10),
        calculatePrice = true,
        projectAnalysisTimeoutMinutes = 75,
        bots = setOf("KotlinBuild"),
      ),
      EdictYamlConfiguration.load(qodanaYaml("full")),
    )
  }

  @Test
  fun `the lowest value of every number is accepted`() {
    val path = edictYaml(
      directory,
      "mcpPort" to 1,
      "projectAnalysisTimeoutMinutes" to 1,
      "extraction.defaultSignalCount" to 1,
      "distribution.defaultSignalCount" to 1,
      "generation.maxProjectAnalyses" to 1,
      "generation.defaultGenerationCount" to 1,
    )

    assertEquals(
      EdictConfiguration(
        mcpPort = 1,
        extraction = ExtractionConfiguration(1),
        distribution = DistributionConfiguration(1),
        generation = GenerationConfiguration(maxProjectAnalyses = 1, defaultGenerationCount = 1),
        projectAnalysisTimeoutMinutes = 1,
      ),
      EdictYamlConfiguration.load(path),
    )
  }

  @Test
  fun `the highest port is accepted`() {
    assertEquals(EdictConfiguration(mcpPort = 65535), EdictYamlConfiguration.load(edictYaml(directory, "mcpPort" to 65535)))
  }

  @Test
  fun `a GitHub URL with a trailing slash names the same repository`() {
    assertEquals(EdictConfiguration(ci = KOTLIN), EdictYamlConfiguration.load(edictYaml(directory, "ci.url" to "$KOTLIN_URL/")))
  }

  @Test
  fun `a GitHub clone URL names its repository without git`() {
    assertEquals(EdictConfiguration(ci = KOTLIN), EdictYamlConfiguration.load(edictYaml(directory, "ci.url" to "$KOTLIN_URL.git")))
  }

  @Test
  fun `a URL names its host in lowercase`() {
    assertEquals(
      EdictConfiguration(ci = KOTLIN),
      EdictYamlConfiguration.load(edictYaml(directory, "ci.url" to "https://GitHub.com/JetBrains/kotlin")),
    )
  }

  @Test
  fun `a Space repository URL names its project and repository`() {
    assertEquals(EdictConfiguration(ci = ULTIMATE), EdictYamlConfiguration.load(edictYaml(directory, "ci.url" to ULTIMATE_URL)))
  }

  @Test
  fun `a Space clone URL names its repository without git`() {
    assertEquals(EdictConfiguration(ci = ULTIMATE), EdictYamlConfiguration.load(edictYaml(directory, "ci.url" to "$ULTIMATE_URL.git")))
  }

  @Test
  fun `promotion opens reviews in its own repository when it names one`() {
    val configuration = EdictYamlConfiguration.load(promotionYaml(directory, "promotion.url" to KOTLIN_EDICT_URL))

    assertEquals(EdictConfiguration(ci = KOTLIN, promotion = PromotionConfiguration("reviewer", repository = KOTLIN_EDICT)), configuration)
    assertEquals(KOTLIN_EDICT, configuration.promotionRepository)
  }

  @Test
  fun `promotion without its own repository opens reviews in the CI repository`() {
    val configuration = EdictYamlConfiguration.load(promotionYaml(directory))

    assertEquals(EdictConfiguration(ci = KOTLIN, promotion = PromotionConfiguration("reviewer")), configuration)
    assertEquals(KOTLIN, configuration.promotionRepository)
  }

  @Test
  fun `promotion without any repository is refused`() {
    val path = edictYaml(directory, "promotion.reviewer" to "reviewer")

    val error = assertFailsWith<IllegalArgumentException> { EdictYamlConfiguration.load(path) }
    assertEquals("edict.promotion requires edict.promotion.url or edict.ci.url", error.message)
  }

  @ParameterizedTest
  @ValueSource(
    strings = [
      "http://github.com/JetBrains/kotlin", "https://github.com:8443/JetBrains/kotlin", "https://user@github.com/JetBrains/kotlin",
      "https://github.com/JetBrains/kotlin?tab=readme", "https://github.com/JetBrains/kotlin#readme",
    ],
  )
  fun `a CI URL that is not a plain https URL is refused`(url: String) {
    val error = assertFailsWith<IllegalArgumentException> { EdictYamlConfiguration.load(edictYaml(directory, "ci.url" to url)) }

    assertEquals("edict.ci.url must be an https URL without a port, credentials, query or fragment: $url", error.message)
  }

  @Test
  fun `a promotion URL that is not a plain https URL is refused under its own name`() {
    val path = promotionYaml(directory, "promotion.url" to "http://github.com/qodana/kotlin-edict")

    val error = assertFailsWith<IllegalArgumentException> { EdictYamlConfiguration.load(path) }
    assertEquals(
      "edict.promotion.url must be an https URL without a port, credentials, query or fragment: http://github.com/qodana/kotlin-edict",
      error.message,
    )
  }

  @Test
  fun `a CI URL that does not parse is refused`() {
    val path = edictYaml(directory, "ci.url" to "https://github.com/JetBrains/kot lin")

    val error = assertFailsWith<IllegalArgumentException> { EdictYamlConfiguration.load(path) }
    assertEquals("edict.ci.url is not a URL: https://github.com/JetBrains/kot lin", error.message)
  }

  @ParameterizedTest
  @ValueSource(strings = ["https://github.com/JetBrains", "https://jetbrains.team/p/ij/repos/ultimate"])
  fun `a CI URL whose path names no repository is refused`(url: String) {
    val error = assertFailsWith<IllegalArgumentException> { EdictYamlConfiguration.load(edictYaml(directory, "ci.url" to url)) }

    assertEquals(
      "edict.ci.url must end with /<owner>/<repository> for GitHub or /p/<project-key>/repositories/<repository> for Space: $url",
      error.message,
    )
  }

  @ParameterizedTest
  @CsvSource(
    "ci.url,",
    "statePath, '   '",
    "promotion.reviewer, ' '",
    "promotion.targetBranch, ''",
    "promotion.inspectionsDirectory, ' '",
    "promotion.url, ' '",
  )
  fun `a missing or blank value of a configured section is required`(key: String, value: String?) {
    val error = assertFailsWith<IllegalStateException> { EdictYamlConfiguration.load(promotionYaml(directory, key to value)) }

    assertEquals("edict.$key is required", error.message)
  }

  @ParameterizedTest
  @CsvSource(
    "mcpPort, 0, edict.mcpPort must be between 1 and 65535: 0",
    "mcpPort, 65536, edict.mcpPort must be between 1 and 65535: 65536",
    "projectAnalysisTimeoutMinutes, 0, edict.projectAnalysisTimeoutMinutes must be at least 1: 0",
    "extraction.defaultSignalCount, 0, edict.extraction.defaultSignalCount must be at least 1: 0",
    "distribution.defaultSignalCount, 0, edict.distribution.defaultSignalCount must be at least 1: 0",
    "generation.maxProjectAnalyses, 0, edict.generation.maxProjectAnalyses must be at least 1: 0",
    "generation.defaultGenerationCount, 0, edict.generation.defaultGenerationCount must be at least 1: 0",
  )
  fun `a number out of its range is refused with the value`(key: String, value: Int, message: String) {
    val error = assertFailsWith<IllegalArgumentException> { EdictYamlConfiguration.load(edictYaml(directory, key to value)) }

    assertEquals(message, error.message)
  }

  @Test
  fun `a state path the file system cannot name is refused`() {
    val path = edictYaml(directory, "statePath" to "state\u0000dir")

    val error = assertFailsWith<IllegalArgumentException> { EdictYamlConfiguration.load(path) }
    assertEquals("edict.statePath is invalid: state\u0000dir", error.message)
  }

  @Test
  fun `a reviewer with a slash is refused`() {
    val path = promotionYaml(directory, "promotion.reviewer" to "team/reviewer")

    val error = assertFailsWith<IllegalArgumentException> { EdictYamlConfiguration.load(path) }
    assertEquals("Promotion reviewer is invalid", error.message)
  }

  @Test
  fun `a target branch Git would not accept is refused`() {
    val path = promotionYaml(directory, "promotion.targetBranch" to "feature branch")

    val error = assertFailsWith<IllegalArgumentException> { EdictYamlConfiguration.load(path) }
    assertEquals("Promotion target branch is invalid", error.message)
  }

  @Test
  fun `a target branch named by its full ref is refused`() {
    val path = promotionYaml(directory, "promotion.targetBranch" to "refs/heads/main")

    val error = assertFailsWith<IllegalArgumentException> { EdictYamlConfiguration.load(path) }
    assertEquals("Name the promotion target branch without refs/heads/", error.message)
  }

  @Test
  fun `an inspections directory outside the target repository is refused`() {
    val path = promotionYaml(directory, "promotion.inspectionsDirectory" to "../outside")

    val error = assertFailsWith<IllegalArgumentException> { EdictYamlConfiguration.load(path) }
    assertEquals("Promotion inspections directory must stay inside the target repository", error.message)
  }

  @Test
  fun `malformed YAML is refused with the parser's explanation`() {
    val error = assertFailsWith<IllegalArgumentException> { EdictYamlConfiguration.load(qodanaYaml("malformed")) }

    assertEquals(parseFailure("malformed"), error.message)
  }

  @Test
  fun `a duplicate key is refused`() {
    val error = assertFailsWith<IllegalArgumentException> { EdictYamlConfiguration.load(qodanaYaml("duplicate-key")) }

    assertEquals(parseFailure("duplicate-key"), error.message)
  }

  @Test
  fun `an unknown key in the Edict section is refused`() {
    val error = assertFailsWith<IllegalArgumentException> { EdictYamlConfiguration.load(qodanaYaml("unknown-edict-key")) }

    assertEquals(parseFailure("unknown-edict-key"), withoutIdentityHashes(error.message))
  }

  @Test
  fun `a configuration of exactly 2 MiB is read`() {
    assertEquals(EdictConfiguration(), EdictYamlConfiguration.load(paddedQodanaYaml(directory, MAX_QODANA_YAML_BYTES)))
  }

  @Test
  fun `a configuration over 2 MiB is refused`() {
    val path = paddedQodanaYaml(directory, MAX_QODANA_YAML_BYTES + 1)

    val error = assertFailsWith<IllegalArgumentException> { EdictYamlConfiguration.load(path) }
    assertEquals("Qodana configuration exceeds 2 MiB: $path", error.message)
  }

  @Test
  fun `a configuration path that does not exist is refused`() {
    val path = directory.resolve("qodana.yaml")

    val error = assertFailsWith<IllegalArgumentException> { EdictYamlConfiguration.load(path) }
    assertEquals("Qodana configuration does not exist or is not a file: $path", error.message)
  }
}

private const val KOTLIN_EDICT_URL = "https://github.com/qodana/kotlin-edict"
private const val ULTIMATE_URL = "https://jetbrains.team/p/ij/repositories/ultimate"
private val KOTLIN_EDICT = ReviewRepository(CiProviderId.GITHUB, "github.com", "qodana", "kotlin-edict")
private val ULTIMATE = ReviewRepository(CiProviderId.SPACE, "jetbrains.team", "ij", "ultimate")
