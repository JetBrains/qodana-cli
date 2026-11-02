// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci

import org.jetbrains.qodana.edict.ci.support.KOTLIN
import org.junit.jupiter.api.Tag
import org.junit.jupiter.api.condition.EnabledIfEnvironmentVariable
import kotlin.test.Test
import kotlin.test.assertEquals

/** The factories' clients read the real repositories with the tokens from the environment: only GETs are sent. */
@Tag("integration")
internal class ReviewApiFactoryIntegrationTest {
  @Test
  @EnabledIfEnvironmentVariable(named = "GITHUB_TOKEN", matches = ".+")
  fun `the GitHub client authenticates to api_github_com and reads the default branch`() {
    assertEquals("master", getReviewAPI(KOTLIN, emptySet()).resolveTargetBranch(null))
  }

  @Test
  @EnabledIfEnvironmentVariable(named = "SPACE_TOKEN", matches = ".+")
  fun `the Space client authenticates to the HTTP API of jetbrains_team and reads the default branch`() {
    val ultimate = ReviewRepository(CiProviderId.SPACE, "jetbrains.team", "ij", "ultimate")

    assertEquals("master", getReviewAPI(ultimate, emptySet()).resolveTargetBranch(null))
  }
}
