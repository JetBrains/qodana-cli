package org.jetbrains.qodana.edict.edictnext

import org.jetbrains.qodana.edict.ci.CiProviderId
import org.jetbrains.qodana.edict.ci.ReviewExtractionApi
import org.jetbrains.qodana.edict.ci.ReviewFetchResult
import org.jetbrains.qodana.edict.ci.ReviewRepository
import org.jetbrains.qodana.edict.ci.ReviewSelection
import org.jetbrains.qodana.edict.support.fixturePath
import org.jetbrains.qodana.edict.support.gitFixture
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import kotlin.test.Test
import kotlin.test.assertContains
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

class EdictSourceFileServiceTest {
  @TempDir
  lateinit var directory: Path

  private val remoteReads = mutableListOf<String>()

  private val provider = object : ReviewExtractionApi {
    override fun fetch(selection: ReviewSelection): ReviewFetchResult = error("not used")

    override fun file(repository: ReviewRepository, revision: String, path: String): String {
      remoteReads += "$revision:$path"
      return "class Remote {}\n"
    }

    override fun diff(repository: ReviewRepository, before: String, after: String, beforePath: String, afterPath: String): String =
      error("not used")
  }

  private val remote = ReviewRepository(CiProviderId.GITHUB, "owner", "repo")

  @Test
  fun `a local commit is read from Git with line numbers`() {
    val head = gitFixture(directory).resolve("HEAD")
    val files = EdictSourceFileService(directory, provider, remote)

    assertEquals(
      listOf(
        "    1| class Equality {",
        "    2|   boolean same(String a, String b) {",
        "    3|     return java.util.Objects.equals(a, b);",
        "    4|   }",
        "    5| }",
      ).joinToString("\n"),
      files.fileAtRef(fixturePath, head),
    )
    // A path missing at a local commit is missing remotely too.
    assertFailsWith<IllegalStateException> { files.fileAtRef("module/src/Missing.java", head) }
    assertEquals(emptyList(), remoteReads)
  }

  @Test
  fun `an anchor returns only its lines and the radius around them`() {
    val head = gitFixture(directory).resolve("HEAD")
    val files = EdictSourceFileService(directory, provider, null)

    assertEquals(
      listOf(
        "    2|   boolean same(String a, String b) {",
        "    3|     return java.util.Objects.equals(a, b);",
        "    4|   }",
      ).joinToString("\n"),
      files.fileAtRef(fixturePath, head, EdictNextLineRange(3, 3), radius = 1),
    )
    assertEquals("    5| }", files.fileAtRef(fixturePath, head, EdictNextLineRange(5, 5), radius = 0))
    val beyond = assertFailsWith<IllegalArgumentException> { files.fileAtRef(fixturePath, head, EdictNextLineRange(9, 9), radius = 0) }
    assertContains(beyond.message.orEmpty(), "beyond the file's 5 lines")
  }

  @Test
  fun `a commit missing locally is read from the configured review repository`() {
    gitFixture(directory)
    val missing = "a".repeat(40)

    assertEquals("    1| class Remote {}", EdictSourceFileService(directory, provider, remote).fileAtRef(fixturePath, missing))
    assertEquals(listOf("$missing:$fixturePath"), remoteReads)
    val unconfigured = assertFailsWith<IllegalStateException> {
      EdictSourceFileService(directory, provider, null).fileAtRef(fixturePath, missing)
    }
    assertContains(unconfigured.message.orEmpty(), "edict.ci.url")
  }
}
