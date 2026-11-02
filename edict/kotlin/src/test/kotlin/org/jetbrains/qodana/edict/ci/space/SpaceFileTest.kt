// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci.space

import org.jetbrains.qodana.edict.ci.support.RELEASE_191
import org.jetbrains.qodana.edict.ci.support.ULTIMATE_MERGED_REVIEW_BASE
import org.jetbrains.qodana.edict.ci.support.ULTIMATE_STARLARK_FILE
import org.jetbrains.qodana.edict.ci.support.answer
import org.jetbrains.qodana.edict.ci.support.ultimateRecording
import org.jetbrains.qodana.edict.ci.support.ultimateScripted
import java.nio.file.NoSuchFileException
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

/** Reading files, diffs and the target branch of IJ/ultimate, replayed from the real repository. */
internal class SpaceFileTest {
  @Test
  fun `a file is read as it is at a revision`() {
    // ULTIMATE_STARLARK_FILE at ULTIMATE_MERGED_REVIEW_BASE.
    val recording = ultimateRecording("file-at-revision")

    assertEquals(
      recording.result<String>(),
      recording.replay { file(ULTIMATE_MERGED_REVIEW_BASE, ULTIMATE_STARLARK_FILE) },
    )
  }

  @Test
  fun `a path missing at a revision is a missing file`() {
    // A path missing at ULTIMATE_MERGED_REVIEW_BASE.
    val recording = ultimateRecording("file-missing-path")

    val error = assertFailsWith<NoSuchFileException> {
      recording.replay { file(ULTIMATE_MERGED_REVIEW_BASE, "plugins/bazel/Missing.kt") }
    }
    assertEquals("plugins/bazel/Missing.kt", error.message)
  }

  @Test
  fun `a read at a revision Space doesn't have fails rather than finding no file`() {
    // ULTIMATE_STARLARK_FILE at the all-zero revision, which Space doesn't have.
    val recording = ultimateRecording("file-unknown-revision")

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { file("0".repeat(40), ULTIMATE_STARLARK_FILE) }
    }
    assertEquals("Space API returned HTTP 404: Missing unknown 0000000000000000000000000000000000000000", error.message)
  }

  @Test
  fun `a file Space sends incomplete is refused`() {
    // ULTIMATE_STARLARK_FILE, with Space sending fewer bytes than it says the file has.
    val recording = ultimateRecording("file-at-revision")
      .failingAt("GET", "/content", answer(200, """{"partBase64":"cGFydA==","totalSize":5}"""))

    val error = assertFailsWith<IllegalArgumentException> {
      recording.replay { file(ULTIMATE_MERGED_REVIEW_BASE, ULTIMATE_STARLARK_FILE) }
    }
    assertEquals("Provider file content is invalid, binary or truncated", error.message)
  }

  @Test
  fun `a branch name instead of a revision is refused before any request`() {
    val error = assertFailsWith<IllegalArgumentException> {
      ultimateScripted().replay { file("master", ULTIMATE_STARLARK_FILE) }
    }
    assertEquals("Source reads require exact revision and repository-relative path", error.message)
  }

  @Test
  fun `an absolute path is refused before any request`() {
    val error = assertFailsWith<IllegalArgumentException> {
      ultimateScripted().replay { file(ULTIMATE_MERGED_REVIEW_BASE, "/$ULTIMATE_STARLARK_FILE") }
    }
    assertEquals("Source reads require exact revision and repository-relative path", error.message)
  }

  @Test
  fun `a configured target branch is taken as it is, without a request`() {
    assertEquals(RELEASE_191, ultimateScripted().replay { resolveTargetBranch(RELEASE_191) })
  }

  @Test
  fun `without a configured branch reviews target the default branch`() {
    // IJ/ultimate, whose default branch is master.
    val recording = ultimateRecording("default-branch")

    assertEquals("master", recording.replay { resolveTargetBranch(null) })
  }
}
