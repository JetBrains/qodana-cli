// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci

import org.jetbrains.qodana.edict.ci.support.KOTLIN
import org.junit.jupiter.params.ParameterizedTest
import org.junit.jupiter.params.provider.ValueSource
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertTrue

internal class ReviewRepositoryTest {
  @Test
  fun `a repository is labelled by its provider, host, owner and repository`() {
    assertEquals("github:github.com/JetBrains/kotlin", KOTLIN.label())
  }

  @ParameterizedTest
  @ValueSource(strings = ["GitHub.com", "github.com:443"])
  fun `a host that is not a lowercase host name is rejected`(host: String) {
    val error = assertFailsWith<IllegalArgumentException> { ReviewRepository(CiProviderId.GITHUB, host, "JetBrains", "kotlin") }
    assertEquals("Invalid host: $host", error.message)
  }

  @ParameterizedTest
  @ValueSource(strings = ["..", "Jet/Brains"])
  fun `an owner that is not one path segment is rejected`(owner: String) {
    val error = assertFailsWith<IllegalArgumentException> { ReviewRepository(CiProviderId.GITHUB, "github.com", owner, "kotlin") }
    assertEquals("Invalid owner or repository", error.message)
  }

  @Test
  fun `a repository that is not one path segment is rejected`() {
    val error = assertFailsWith<IllegalArgumentException> {
      ReviewRepository(CiProviderId.GITHUB, "github.com", "JetBrains", "kotlin/compiler")
    }
    assertEquals("Invalid owner or repository", error.message)
  }

  @Test
  fun `a review of the repository refers to it`() {
    assertEquals(
      ReviewReference(CiProviderId.GITHUB, "JetBrains", "kotlin", "6007", "https://github.com/JetBrains/kotlin/pull/6007"),
      KOTLIN.reference("6007", "https://github.com/JetBrains/kotlin/pull/6007"),
    )
  }

  @Test
  fun `a reviewed repository has the lowercase host of the review URL`() {
    val reference = ReviewReference(CiProviderId.GITHUB, "JetBrains", "kotlin", "6007", "https://GitHub.com/JetBrains/kotlin/pull/6007")

    assertEquals(KOTLIN, reference.reviewRepository)
  }

  @Test
  fun `a Space reviewed repository is named by the reference, as Space review URLs omit it`() {
    val reference = ReviewReference(CiProviderId.SPACE, "ij", "ultimate", "1Iu3Gp1HKnF6", "https://jetbrains.team/p/ij/reviews/224837")

    assertEquals(ReviewRepository(CiProviderId.SPACE, "jetbrains.team", "ij", "ultimate"), reference.reviewRepository)
  }

  @Test
  fun `a review URL without a host is rejected`() {
    val error = assertFailsWith<IllegalArgumentException> {
      ReviewReference(CiProviderId.GITHUB, "JetBrains", "kotlin", "6007", "/JetBrains/kotlin/pull/6007")
    }
    assertEquals("Invalid host: ", error.message)
  }

  @Test
  fun `a repository owns its reviews`() {
    assertTrue(KOTLIN.owns(KOTLIN.reference("6007", "https://github.com/JetBrains/kotlin/pull/6007")))
  }

  @Test
  fun `a repository does not own a review on another host`() {
    val reference =
      ReviewReference(CiProviderId.GITHUB, "JetBrains", "kotlin", "6007", "https://ghe.example.com/JetBrains/kotlin/pull/6007")

    assertFalse(KOTLIN.owns(reference))
  }

  @Test
  fun `requiring ownership of another repository's review names both repositories`() {
    val reference =
      ReviewReference(CiProviderId.GITHUB, "JetBrains", "intellij-community", "1", "https://github.com/JetBrains/intellij-community/pull/1")

    val error = assertFailsWith<IllegalArgumentException> { KOTLIN.requireOwns(reference) }
    assertEquals(
      "Review https://github.com/JetBrains/intellij-community/pull/1 (github:github.com/JetBrains/intellij-community) " +
        "is not in the configured github:github.com/JetBrains/kotlin",
      error.message,
    )
  }
}
