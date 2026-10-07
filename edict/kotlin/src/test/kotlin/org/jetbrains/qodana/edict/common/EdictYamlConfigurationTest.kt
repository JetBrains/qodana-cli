// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.common

import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import kotlin.io.path.writeText
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
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
        ci:
          url: https://github.com/JetBrains/qodana-cli
        promotion:
          reviewer: reviewer-login
          targetBranch: main
          inspectionsDirectory: quality/inspections
      """,
    ))

    assertEquals(EdictCIConfiguration("https://github.com/JetBrains/qodana-cli"), configuration.ci)
    assertEquals(PromotionConfiguration("reviewer-login", "main", "quality/inspections"), configuration.promotion)
    assertEquals(EdictCIProvider.GITHUB, configuration.ci?.provider)
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

    assertEquals(EdictCIProvider.SPACE, configuration.ci?.provider)
    assertEquals("QD", configuration.ci?.owner)
    assertEquals("qodana-cli", configuration.ci?.repository)
  }

  @Test
  fun `missing Edict configuration remains optional and promotion directory defaults`() {
    assertEquals(EdictConfiguration(), EdictYamlConfiguration.load(null))
    val withoutEdict = EdictYamlConfiguration.load(yaml("version: \"1.0\""))
    assertNull(withoutEdict.ci)
    assertNull(withoutEdict.promotion)

    val promotion = EdictYamlConfiguration.load(yaml(
      """
      edict:
        promotion:
          reviewer: reviewer
      """,
    )).promotion
    assertEquals("inspections", promotion?.inspectionsDirectory)
    assertNull(promotion?.targetBranch)
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
