package org.jetbrains.qodana.edict.common

import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import kotlin.io.path.writeText
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

class GitExecutionBranchTest {
  @TempDir lateinit var directory: Path

  @Test
  fun `uses checked out branch and rejects detached HEAD`() {
    runProcess(directory, listOf("git", "init", "--initial-branch=feature/example"))
    directory.resolve("source.txt").writeText("source\n")
    val repository = GitRepository(directory)
    repository.git("add", "source.txt")
    repository.git("-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "Initial")
    assertEquals("feature/example", repository.currentBranch())
    repository.git("checkout", "--detach", "HEAD")
    val failure = assertFailsWith<IllegalArgumentException> { repository.currentBranch() }
    assertEquals("The repository has no checked-out branch", failure.message)
  }
}
