package org.jetbrains.qodana.edict.common

import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import kotlin.io.path.writeText
import kotlin.test.Test
import kotlin.test.assertContains
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

class GitExecutionBranchTest {
  @TempDir lateinit var directory: Path

  @Test
  fun `uses checked out branch and rejects detached HEAD`() {
    runProcess(directory, listOf("git", "init", "--initial-branch=feature/source-fixes"))
    directory.resolve("source.txt").writeText("source\n")
    val repository = GitRepository(directory)
    repository.git("add", "source.txt")
    repository.git("-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "Initial")
    assertEquals("feature/source-fixes", repository.currentBranch())
    repository.git("checkout", "--detach", "HEAD")
    val failure = assertFailsWith<IllegalArgumentException> { repository.currentBranch() }
    assertContains(failure.message.orEmpty(), "check out the project branch")
  }
}
