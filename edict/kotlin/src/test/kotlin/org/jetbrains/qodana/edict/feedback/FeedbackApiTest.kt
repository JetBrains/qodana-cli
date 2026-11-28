// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import org.junitpioneer.jupiter.ClearEnvironmentVariable
import org.junitpioneer.jupiter.SetEnvironmentVariable
import java.nio.file.Path
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertIs

class FeedbackApiTest {
  @TempDir
  lateinit var project: Path

  @Test
  @SetEnvironmentVariable(key = "GITHUB_ACTIONS", value = "true")
  fun `without edict ci a GitHub Actions run gives its feedback`() {
    assertIs<GitHubFeedbackApi>(ciRunFeedbackApi(null, project))
  }

  @Test
  @ClearEnvironmentVariable(key = "GITHUB_ACTIONS")
  fun `without edict ci a run in no known CI fails`() {
    val failure = assertFailsWith<IllegalStateException> { ciRunFeedbackApi(null, project) }

    assertEquals(
      "edict.ci is not set in qodana.yaml, and the environment shows no CI run that Edict feedback knows",
      failure.message,
    )
  }
}
