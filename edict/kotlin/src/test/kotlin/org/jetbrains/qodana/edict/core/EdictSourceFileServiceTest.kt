// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.core

import org.jetbrains.qodana.edict.core.support.MISSING_COMMIT
import org.jetbrains.qodana.edict.core.support.reviewsHaving
import org.jetbrains.qodana.edict.support.fixturePath
import org.jetbrains.qodana.edict.support.gitFixture
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

internal class EdictSourceFileServiceTest {
  @TempDir
  lateinit var directory: Path

  @Test
  fun `a commit of the local repository is read from Git with line numbers`() {
    val head = gitFixture(directory).resolve("HEAD")

    assertEquals(NUMBERED_EQUALITY, EdictSourceFileService(directory, null).fileAtRef(fixturePath, head))
  }

  @Test
  fun `a commit of the local repository is read without asking the review repository`() {
    val head = gitFixture(directory).resolve("HEAD")
    val remote = reviewsHaving(head, fixturePath, "class Remote {}\n")

    EdictSourceFileService(directory, remote).fileAtRef(fixturePath, head)
    assertEquals(emptyList(), remote.fileReads)
  }

  @Test
  fun `a path missing at a local commit fails without asking the review repository, which has the same commit`() {
    val head = gitFixture(directory).resolve("HEAD")
    val remote = reviewsHaving(head, "module/src/Missing.java", "class Missing {}\n")

    val error = assertFailsWith<IllegalStateException> { EdictSourceFileService(directory, remote).fileAtRef("module/src/Missing.java", head) }
    assertEquals("git exited 128: fatal: path 'module/src/Missing.java' does not exist in '$head'\n", error.message)
    assertEquals(emptyList(), remote.fileReads)
  }

  @Test
  fun `an anchor returns its lines and the radius around them`() {
    val head = gitFixture(directory).resolve("HEAD")

    assertEquals(
      "    2|   boolean same(String a, String b) {\n    3|     return java.util.Objects.equals(a, b);\n    4|   }",
      EdictSourceFileService(directory, null).fileAtRef(fixturePath, head, EdictLineRange(3, 3), radius = 1),
    )
  }

  @Test
  fun `a radius reaching past the last line stops at it`() {
    val head = gitFixture(directory).resolve("HEAD")

    assertEquals(
      "    4|   }\n    5| }",
      EdictSourceFileService(directory, null).fileAtRef(fixturePath, head, EdictLineRange(5, 5), radius = 1),
    )
  }

  @Test
  fun `a radius reaching before the first line stops at it`() {
    val head = gitFixture(directory).resolve("HEAD")

    assertEquals(
      "    1| class Equality {\n    2|   boolean same(String a, String b) {",
      EdictSourceFileService(directory, null).fileAtRef(fixturePath, head, EdictLineRange(1, 1), radius = 1),
    )
  }

  @Test
  fun `an anchor beyond the end of the file is refused`() {
    val head = gitFixture(directory).resolve("HEAD")

    val error = assertFailsWith<IllegalArgumentException> {
      EdictSourceFileService(directory, null).fileAtRef(fixturePath, head, EdictLineRange(6, 6), radius = 0)
    }
    assertEquals("Anchor line 6 is beyond the file's 5 lines", error.message)
  }

  @Test
  fun `a commit missing locally is read from the review repository`() {
    gitFixture(directory)
    val remote = reviewsHaving(MISSING_COMMIT, fixturePath, "class Remote {}\n")

    assertEquals("    1| class Remote {}", EdictSourceFileService(directory, remote).fileAtRef(fixturePath, MISSING_COMMIT))
    assertEquals(listOf(MISSING_COMMIT to fixturePath), remote.fileReads)
  }

  @Test
  fun `a project without Git reads every commit from the review repository`() {
    val remote = reviewsHaving(MISSING_COMMIT, fixturePath, "class Remote {}\n")

    assertEquals("    1| class Remote {}", EdictSourceFileService(directory, remote).fileAtRef(fixturePath, MISSING_COMMIT))
  }

  @Test
  fun `an empty file is one empty numbered line`() {
    val remote = reviewsHaving(MISSING_COMMIT, fixturePath, "")

    assertEquals("    1| ", EdictSourceFileService(directory, remote).fileAtRef(fixturePath, MISSING_COMMIT))
  }

  @Test
  fun `a commit missing locally without a review repository is refused`() {
    gitFixture(directory)

    val error = assertFailsWith<IllegalStateException> { EdictSourceFileService(directory, null).fileAtRef(fixturePath, MISSING_COMMIT) }
    assertEquals("Commit $MISSING_COMMIT is not in the local repository, and edict.ci.url names no remote one", error.message)
  }

  @Test
  fun `an abbreviated revision is refused`() {
    val error = assertFailsWith<IllegalArgumentException> { EdictSourceFileService(directory, null).fileAtRef(fixturePath, "aaaaaaa") }

    assertEquals("Requires a full commit SHA and a repository-relative path", error.message)
  }

  @Test
  fun `a path outside the repository is refused`() {
    val error = assertFailsWith<IllegalArgumentException> {
      EdictSourceFileService(directory, null).fileAtRef("../Equality.java", MISSING_COMMIT)
    }

    assertEquals("Requires a full commit SHA and a repository-relative path", error.message)
  }

  @Test
  fun `a negative radius is refused`() {
    val error = assertFailsWith<IllegalArgumentException> {
      EdictSourceFileService(directory, null).fileAtRef(fixturePath, MISSING_COMMIT, EdictLineRange(1, 1), radius = -1)
    }

    assertEquals("Radius must not be negative", error.message)
  }

  @Test
  fun `an anchor that ends before it starts is refused`() {
    val error = assertFailsWith<IllegalArgumentException> {
      EdictSourceFileService(directory, null).fileAtRef(fixturePath, MISSING_COMMIT, EdictLineRange(3, 2))
    }

    assertEquals("Invalid anchor lines 3-2", error.message)
  }
}

/** The Git fixture's file at its head, numbered. */
private val NUMBERED_EQUALITY = listOf(
  "    1| class Equality {",
  "    2|   boolean same(String a, String b) {",
  "    3|     return java.util.Objects.equals(a, b);",
  "    4|   }",
  "    5| }",
).joinToString("\n")
