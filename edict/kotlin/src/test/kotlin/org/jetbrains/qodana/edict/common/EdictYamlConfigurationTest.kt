// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.common

import org.jetbrains.qodana.edict.ci.CiProviderId
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import kotlin.io.path.writeText
import kotlin.test.Test
import kotlin.test.assertContains
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class EdictYamlConfigurationTest {
  @TempDir
  lateinit var directory: Path

  @Test
  fun `loads Edict sections while ignoring unrelated Qodana configuration`() {
    val configuration = EdictYamlConfiguration.load(yaml(
      """
      version: "1.0"
      profile:
        name: qodana.recommended
      include:
        - name: SomeInspection
      edict:
        statePath: ../edict-state
        calculatePrice: true
        ci:
          url: https://github.com/JetBrains/qodana-cli
        promotion:
          reviewer: reviewer-login
          targetBranch: main
          inspectionsDirectory: quality/inspections
      """,
    ))

    assertEquals("../edict-state", configuration.statePath)
    assertTrue(configuration.calculatePrice)
    assertEquals(EdictCIConfiguration("https://github.com/JetBrains/qodana-cli"), configuration.ci)
    assertEquals(PromotionConfiguration("reviewer-login", "main", "quality/inspections"), configuration.promotion)
    assertEquals(CiProviderId.GITHUB, configuration.ci?.provider)
    assertEquals("JetBrains", configuration.ci?.owner)
    assertEquals("qodana-cli", configuration.ci?.repository)
  }

  @Test
  fun `derives Space project and repository from one URL`() {
    val configuration = EdictYamlConfiguration.load(yaml(
      """
      edict:
        ci:
          url: https://jetbrains.team/p/QD/repositories/qodana-cli
      """,
    ))

    assertEquals(CiProviderId.SPACE, configuration.ci?.provider)
    assertEquals("QD", configuration.ci?.owner)
    assertEquals("qodana-cli", configuration.ci?.repository)
  }

  @Test
  fun `missing Edict configuration remains optional and promotion directory defaults`() {
    assertEquals(EdictConfiguration(), EdictYamlConfiguration.load(null))
    val withoutEdict = EdictYamlConfiguration.load(yaml("version: \"1.0\""))
    assertNull(withoutEdict.ci)
    assertNull(withoutEdict.promotion)
    assertFalse(withoutEdict.calculatePrice)

    val promotion = EdictYamlConfiguration.load(yaml(
      """
      edict:
        promotion:
          reviewer: reviewer
      """,
    )).promotion
    assertEquals("inspections", promotion?.inspectionsDirectory)
    assertNull(promotion?.targetBranch)

    val invalidStatePath = assertFailsWith<IllegalStateException> {
      EdictYamlConfiguration.load(yaml("edict:\n  statePath: '   '"))
    }
    assertTrue(invalidStatePath.message.orEmpty().contains("edict.statePath is required"))
  }

  @Test
  fun `generation limits have defaults and are configurable`() {
    val defaults = EdictYamlConfiguration.load(null).generation
    assertEquals(3, defaults.maxProjectAnalyses)
    assertEquals(5, defaults.defaultGenerationCount)
    val empty = EdictYamlConfiguration.load(yaml("edict:\n  generation: {}\n")).generation
    assertEquals(3, empty.maxProjectAnalyses)
    assertEquals(5, empty.defaultGenerationCount)
    val configured = EdictYamlConfiguration.load(yaml(
      "edict:\n  generation:\n    maxProjectAnalyses: 5\n    defaultGenerationCount: 10\n",
    )).generation
    assertEquals(5, configured.maxProjectAnalyses)
    assertEquals(10, configured.defaultGenerationCount)
    val invalid = assertFailsWith<IllegalArgumentException> {
      EdictYamlConfiguration.load(yaml("edict:\n  generation:\n    maxProjectAnalyses: 0\n"))
    }
    assertContains(invalid.message.orEmpty(), "edict.generation.maxProjectAnalyses must be at least 1")
    val invalidGenerationCount = assertFailsWith<IllegalArgumentException> {
      EdictYamlConfiguration.load(yaml("edict:\n  generation:\n    defaultGenerationCount: 0\n"))
    }
    assertContains(
      invalidGenerationCount.message.orEmpty(),
      "edict.generation.defaultGenerationCount must be at least 1",
    )
  }

  @Test
  fun `price reporting is controlled by a boolean switch`() {
    assertTrue(EdictYamlConfiguration.load(yaml("edict:\n  calculatePrice: true\n")).calculatePrice)
    assertFalse(EdictYamlConfiguration.load(yaml("edict:\n  calculatePrice: false\n")).calculatePrice)
  }

  @Test
  fun `rejects partial and unknown Edict sections clearly`() {
    val partialCI = assertFailsWith<IllegalStateException> {
      EdictYamlConfiguration.load(yaml("edict:\n  ci:\n    url:\n"))
    }
    assertTrue(partialCI.message.orEmpty().contains("edict.ci.url is required"))

    val partialPromotion = assertFailsWith<IllegalStateException> {
      EdictYamlConfiguration.load(yaml(
        """
        edict:
          promotion:
            targetBranch: main
            inspectionsDirectory: inspections
        """,
      ))
    }
    assertTrue(partialPromotion.message.orEmpty().contains("edict.promotion.reviewer is required"))

    val unknown = assertFailsWith<IllegalArgumentException> {
      EdictYamlConfiguration.load(yaml("edict:\n  ciRepository: qodana-cli\n"))
    }
    assertTrue(unknown.message.orEmpty().contains("ciRepository"))
  }

  @Test
  fun `rejects unsupported repository URL shapes`() {
    val invalid = assertFailsWith<IllegalArgumentException> {
      EdictYamlConfiguration.load(yaml(
        """
        edict:
          ci:
            url: https://example.com/not-a-supported/repository/shape
        """,
      ))
    }
    assertTrue(invalid.message.orEmpty().contains("CI repository URL"))
  }

  @Test
  fun `rejects malformed duplicate and unsafe YAML configuration`() {
    assertFailsWith<IllegalArgumentException> { EdictYamlConfiguration.load(yaml("edict: [")) }
    assertFailsWith<IllegalArgumentException> {
      EdictYamlConfiguration.load(yaml(
        """
        edict:
          ci:
            url: https://github.com/JetBrains/one
            url: https://github.com/JetBrains/two
        """,
      ))
    }
    val unsafe = assertFailsWith<IllegalArgumentException> {
      EdictYamlConfiguration.load(yaml(
        """
        edict:
          promotion:
            reviewer: reviewer
            targetBranch: main
            inspectionsDirectory: ../outside
        """,
      ))
    }
    assertTrue(unsafe.message.orEmpty().contains("must stay inside"))
  }

  private fun yaml(content: String): Path = directory.resolve("qodana-${System.nanoTime()}.yaml").also {
    it.writeText(content.trimIndent() + "\n")
  }
}
