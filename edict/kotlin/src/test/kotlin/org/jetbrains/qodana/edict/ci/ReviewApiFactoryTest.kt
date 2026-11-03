// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci

import org.jetbrains.qodana.edict.ci.github.gitHubApiUrl
import org.jetbrains.qodana.edict.ci.github.gitHubReviewApi
import org.jetbrains.qodana.edict.ci.github.gitHubToken
import org.jetbrains.qodana.edict.ci.space.spaceReviewApi
import org.jetbrains.qodana.edict.ci.support.KOTLIN
import org.jetbrains.qodana.edict.ci.support.ULTIMATE
import org.junitpioneer.jupiter.ClearEnvironmentVariable
import org.junitpioneer.jupiter.SetEnvironmentVariable
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

/** The clients' API URLs and tokens; [ReviewApiFactoryIntegrationTest] reads the real repositories through them. */
@ClearEnvironmentVariable(key = "GITHUB_TOKEN")
@ClearEnvironmentVariable(key = "GH_TOKEN")
@ClearEnvironmentVariable(key = "SPACE_TOKEN")
internal class ReviewApiFactoryTest {
  @Test
  fun `github_com, a GitHub Enterprise Cloud host and any other host each have their API`() {
    val hosts = listOf("github.com", "jetbrains.ghe.com", "ghe.com", "git.jetbrains.com")

    assertEquals(
      listOf("https://api.github.com", "https://api.jetbrains.ghe.com", "https://ghe.com/api/v3", "https://git.jetbrains.com/api/v3"),
      hosts.map(::gitHubApiUrl),
    )
  }

  @Test
  @SetEnvironmentVariable(key = "GITHUB_TOKEN", value = "github-token")
  fun `a GitHub repository named under www_github_com is refused`() {
    val repository = ReviewRepository(CiProviderId.GITHUB, "www.github.com", "JetBrains", "kotlin")

    val error = assertFailsWith<IllegalArgumentException> { gitHubReviewApi(repository, emptySet()) }
    assertEquals("Name the repository as https://github.com/<owner>/<repository>, without www", error.message)
  }

  @Test
  @SetEnvironmentVariable(key = "GITHUB_TOKEN", value = "github-token")
  @SetEnvironmentVariable(key = "GH_TOKEN", value = "gh-token")
  fun `GITHUB_TOKEN is preferred to GH_TOKEN`() {
    assertEquals("github-token", gitHubToken())
  }

  @Test
  @SetEnvironmentVariable(key = "GITHUB_TOKEN", value = " ")
  @SetEnvironmentVariable(key = "GH_TOKEN", value = "gh-token")
  fun `GH_TOKEN is taken when GITHUB_TOKEN is blank`() {
    assertEquals("gh-token", gitHubToken())
  }

  @Test
  fun `a GitHub client without a token is refused`() {
    val error = assertFailsWith<IllegalArgumentException> { gitHubReviewApi(KOTLIN, emptySet()) }

    assertEquals("github:github.com/JetBrains/kotlin requires GITHUB_TOKEN or GH_TOKEN in the server environment", error.message)
  }

  @Test
  fun `a Space client without a token is refused`() {
    val error = assertFailsWith<IllegalArgumentException> { spaceReviewApi(ULTIMATE, emptySet()) }

    assertEquals("space:jetbrains.team/ij/ultimate requires SPACE_TOKEN in the server environment", error.message)
  }
}
