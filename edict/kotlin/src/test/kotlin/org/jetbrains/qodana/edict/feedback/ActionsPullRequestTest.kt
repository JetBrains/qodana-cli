// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

import org.jetbrains.qodana.edict.feedback.support.PULL_REQUEST
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import org.junitpioneer.jupiter.ClearEnvironmentVariable
import org.junitpioneer.jupiter.SetEnvironmentVariable
import java.nio.file.Path
import kotlin.io.path.writeText
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

class ActionsPullRequestTest {
  @TempDir
  lateinit var directory: Path

  @Test
  fun `a pull request between branches of the repository is its own`() {
    assertEquals(ActionsPullRequest.Own(PULL_REQUEST), read(pullRequestEvent(headRepository = """{"full_name": "owner/project"}""")))
  }

  @Test
  fun `a pull request from another repository comes from a fork`() {
    val event = pullRequestEvent(headRepository = """{"full_name": "contributor/project"}""")

    assertEquals(ActionsPullRequest.FromFork(PULL_REQUEST), read(event))
  }

  @Test
  fun `a pull request whose fork was deleted comes from a fork`() {
    assertEquals(ActionsPullRequest.FromFork(PULL_REQUEST), read(pullRequestEvent(headRepository = "null")))
  }

  @Test
  fun `a push is for no pull request`() {
    assertEquals(ActionsPullRequest.None, read("""{"ref": "refs/heads/main", "after": "d0d0cafed0d0cafed0d0cafed0d0cafed0d0cafe"}"""))
  }

  @Test
  @SetEnvironmentVariable(key = "GITHUB_EVENT_PATH", value = "/home/runner/work/_temp/_github_workflow/event.json")
  fun `the event file is the one GitHub Actions names`() {
    assertEquals(Path.of("/home/runner/work/_temp/_github_workflow/event.json"), actionsEventPath())
  }

  @Test
  @ClearEnvironmentVariable(key = "GITHUB_EVENT_PATH")
  fun `outside GitHub Actions there is no event file`() {
    val failure = assertFailsWith<IllegalStateException> { actionsEventPath() }

    assertEquals("GITHUB_EVENT_PATH is not set; Edict feedback runs in GitHub Actions", failure.message)
  }

  private fun read(event: String): ActionsPullRequest =
    actionsPullRequest(directory.resolve("event.json").apply { writeText(event) })
}

/** The fields of a `pull_request` event of [PULL_REQUEST] that the run reads, with [headRepository] as the head's `repo`. */
private fun pullRequestEvent(headRepository: String): String = """
  {
    "action": "synchronize",
    "number": 7,
    "pull_request": {
      "number": 7,
      "html_url": "https://github.com/owner/project/pull/7",
      "head": {"ref": "feature", "sha": "d0d0cafed0d0cafed0d0cafed0d0cafed0d0cafe", "repo": $headRepository},
      "base": {"ref": "main", "repo": {"name": "project", "full_name": "owner/project", "owner": {"login": "owner"}}}
    }
  }
""".trimIndent()
