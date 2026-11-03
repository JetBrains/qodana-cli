// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.ci.github

import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonObject
import org.jetbrains.qodana.edict.ci.support.ADDED_BY_917
import org.jetbrains.qodana.edict.ci.support.BEFORE_6007
import org.jetbrains.qodana.edict.ci.support.BEFORE_917
import org.jetbrains.qodana.edict.ci.support.BINARY_FILE_PATH
import org.jetbrains.qodana.edict.ci.support.DIRECTORY_PATH
import org.jetbrains.qodana.edict.ci.support.LARGE_FILE_PATH
import org.jetbrains.qodana.edict.ci.support.MERGE_6007
import org.jetbrains.qodana.edict.ci.support.MERGE_917
import org.jetbrains.qodana.edict.ci.support.MISSING_REVISION
import org.jetbrains.qodana.edict.ci.support.PATH_6007
import org.jetbrains.qodana.edict.ci.support.RELEASE_HEAD
import org.jetbrains.qodana.edict.ci.support.kotlinRecording
import org.jetbrains.qodana.edict.ci.support.kotlinScripted
import org.jetbrains.qodana.edict.ci.support.with
import org.jetbrains.qodana.edict.ci.support.withBody
import org.jetbrains.qodana.edict.common.number
import java.nio.file.NoSuchFileException
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

/** Reading files and branches of JetBrains/kotlin, replaying what GitHub answered. */
internal class GitHubFileTest {
  @Test
  fun `a file is read as it is at a revision`() {
    // PATH_6007 as PR 6007 merged it.
    val recording = kotlinRecording("file-at-revision")

    assertEquals(recording.result<String>(), recording.replay { file(MERGE_6007, PATH_6007) })
  }

  @Test
  fun `a file missing at a revision is reported missing by its path`() {
    // A path missing at MERGE_6007: GitHub answers 404 "Not Found".
    val recording = kotlinRecording("file-missing-path")

    val error = assertFailsWith<NoSuchFileException> {
      recording.replay { file(MERGE_6007, "core/descriptors/Missing.kt") }
    }
    assertEquals("core/descriptors/Missing.kt", error.message)
  }

  @Test
  fun `a file at a revision GitHub doesn't have fails`() {
    // PATH_6007 at a revision GitHub doesn't have: a 404 that names the ref.
    val recording = kotlinRecording("file-unknown-revision")

    val error = assertFailsWith<IllegalArgumentException> { recording.replay { file(MISSING_REVISION, PATH_6007) } }
    assertEquals("GitHub file read returned HTTP 404: No commit found for the ref $MISSING_REVISION", error.message)
  }

  @Test
  fun `a directory is not read as a file`() {
    // DIRECTORY_PATH, which GitHub lists instead of returning a file.
    val recording = kotlinRecording("file-directory")

    val error = assertFailsWith<IllegalStateException> { recording.replay { file(RELEASE_HEAD, DIRECTORY_PATH) } }
    assertEquals("GitHub file read did not return an object", error.message)
  }

  @Test
  fun `a file GitHub serves without content is rejected`() {
    // LARGE_FILE_PATH, which GitHub returns without content.
    val recording = kotlinRecording("file-too-large")

    val error = assertFailsWith<IllegalArgumentException> { recording.replay { file(RELEASE_HEAD, LARGE_FILE_PATH) } }
    assertEquals("GitHub did not return complete base64 file content", error.message)
  }

  @Test
  fun `a binary file is rejected`() {
    // BINARY_FILE_PATH, a PNG icon.
    val recording = kotlinRecording("file-binary")

    val error = assertFailsWith<IllegalArgumentException> { recording.replay { file(RELEASE_HEAD, BINARY_FILE_PATH) } }
    assertEquals("Provider file content is invalid, binary or truncated", error.message)
  }

  @Test
  fun `a file shorter than GitHub's size of it is rejected`() {
    // file-at-revision, but GitHub's size of the file is one byte more than its content.
    val recording = kotlinRecording("file-at-revision").answering("GET", "/contents/$PATH_6007") { read ->
      listOf(read.withBody(read.response.body.with("size" to JsonPrimitive(read.response.body.jsonObject.number("size") + 1))))
    }

    val error = assertFailsWith<IllegalArgumentException> { recording.replay { file(MERGE_6007, PATH_6007) } }
    assertEquals("Provider file content is invalid, binary or truncated", error.message)
  }

  @Test
  fun `a branch name is rejected as a revision before any request`() {
    val error = assertFailsWith<IllegalArgumentException> { kotlinScripted().replay { file("master", PATH_6007) } }

    assertEquals("Source reads require exact revision and repository-relative path", error.message)
  }

  @Test
  fun `a path out of the repository is rejected before any request`() {
    val error = assertFailsWith<IllegalArgumentException> {
      kotlinScripted().replay { file(MERGE_6007, "../$PATH_6007") }
    }

    assertEquals("Source reads require exact revision and repository-relative path", error.message)
  }

  @Test
  fun `a changed file is diffed between the revisions`() {
    // PATH_6007 before and after PR 6007's change.
    val recording = kotlinRecording("diff-of-merged-6007")

    assertEquals(recording.result<String>(), recording.replay { diff(BEFORE_6007, MERGE_6007, PATH_6007, PATH_6007) })
  }

  @Test
  fun `an added file is diffed from nothing`() {
    // ADDED_BY_917, which the merge of PR 917 added, so it is missing before.
    val recording = kotlinRecording("diff-of-added-file-917")

    assertEquals(
      recording.result<String>(),
      recording.replay { diff(BEFORE_917, MERGE_917, ADDED_BY_917, ADDED_BY_917) },
    )
  }

  @Test
  fun `without a configured branch reviews target the default branch`() {
    // JetBrains/kotlin, whose default branch is master.
    val recording = kotlinRecording("default-branch")

    assertEquals("master", recording.replay { resolveTargetBranch(null) })
  }

  @Test
  fun `a configured branch is the target without asking GitHub`() {
    assertEquals("1.3.70", kotlinScripted().replay { resolveTargetBranch("1.3.70") })
  }
}
