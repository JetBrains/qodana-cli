// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.common

import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import org.junitpioneer.jupiter.SetEnvironmentVariable
import java.nio.file.Path
import kotlin.io.path.createDirectories
import kotlin.io.path.exists
import kotlin.io.path.readText
import kotlin.io.path.writeText
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse

class StateBranchTest {
  @TempDir
  lateinit var directory: Path

  @Test
  fun `starts the branch without history when origin has none, out of the project's status`() {
    val project = cloneOrigin("project")

    val checkout = state(project).checkOut()

    assertEquals(StateCheckout.NewBranch, checkout)
    assertEquals("Start the Edict state\n", git(project, "log", "--format=%s", BRANCH))
    assertEquals(BRANCH, git(project.resolve(".edict"), "symbolic-ref", "--short", "HEAD").trim())
    assertEquals("", git(project, "status", "--porcelain"))
  }

  @Test
  @SetEnvironmentVariable(key = "GIT_CONFIG_GLOBAL", value = "/dev/null")
  fun `starts the branch where no git identity is configured, as on a fresh CI runner`() {
    val project = cloneOrigin("project")
    git(project, "config", "--unset", "user.name")
    git(project, "config", "--unset", "user.email")
    // Git would otherwise guess an identity from the system.
    git(project, "config", "user.useConfigOnly", "true")

    val checkout = state(project).checkOut()

    assertEquals(StateCheckout.NewBranch, checkout)
    assertEquals("Qodana Edict <edict@qodana.invalid>\n", git(project, "log", "--format=%an <%ae>", BRANCH))
  }

  @Test
  fun `checks out the branch that origin has and tracks it`() {
    publishState("first", "clusters/c.json" to "{}")
    val project = cloneOrigin("second")

    val checkout = state(project).checkOut()

    assertEquals(StateCheckout.FromOrigin, checkout)
    assertEquals("{}", project.resolve(".edict/clusters/c.json").readText())
    assertEquals("origin/$BRANCH", git(project.resolve(".edict"), "rev-parse", "--abbrev-ref", "@{upstream}").trim())
  }

  @Test
  fun `checks out the local branch of a removed worktree`() {
    val project = cloneOrigin("project")
    state(project).checkOut()
    git(project, "worktree", "remove", ".edict")

    assertEquals(StateCheckout.LocalBranch, state(project).checkOut())
  }

  @Test
  fun `keeps an existing checkout`() {
    val project = cloneOrigin("project")
    state(project).checkOut()

    assertEquals(StateCheckout.Existing, state(project).checkOut())
  }

  @Test
  fun `keeps the state a plain directory outside a git repository`() {
    val project = directory.resolve("plain").createDirectories().toRealPath()

    val checkout = state(project).checkOut()

    assertEquals(StateCheckout.PlainDirectory("$project is not a git repository"), checkout)
  }

  @Test
  fun `keeps a state directory that already holds files, such as state committed on the project's branch`() {
    val project = cloneOrigin("project")
    project.resolve(".edict").createDirectories().resolve("plan.json").writeText("{}")

    val checkout = state(project).checkOut()

    assertEquals(StateCheckout.PlainDirectory("${project.resolve(".edict")} already holds files outside a worktree"), checkout)
  }

  @Test
  fun `push commits the state and publishes the branch`() {
    publishState("project", "clusters/c.json" to "{}")

    assertEquals("Run 1\nStart the Edict state\n", git(origin(), "log", "--format=%s", BRANCH))
    assertEquals("{}", git(origin(), "show", "$BRANCH:clusters/c.json"))
  }

  @Test
  fun `push rebases onto the state that another run pushed first`() {
    publishState("first", "first.json" to "1")
    val second = cloneOrigin("second")
    state(second).checkOut()
    publishState("third", "third.json" to "3")
    second.resolve(".edict/second.json").writeText("2")

    state(second).worktree().push("Run 2")

    assertEquals("Run 2\nRun 1\nRun 1\nStart the Edict state\n", git(origin(), "log", "--format=%s", BRANCH))
    assertEquals("first.json\nsecond.json\nthird.json\n", git(origin(), "ls-tree", "--name-only", BRANCH))
  }

  @Test
  fun `a push that conflicts with the state another run pushed fails and leaves the worktree on its branch`() {
    publishState("first", "plan.json" to "1")
    val second = cloneOrigin("second")
    state(second).checkOut()
    publishState("third", "plan.json" to "3")
    second.resolve(".edict/plan.json").writeText("2")

    assertFailsWith<IllegalStateException> { state(second).worktree().push("Run 2") }

    assertEquals(BRANCH, git(second.resolve(".edict"), "symbolic-ref", "--short", "HEAD").trim())
    val rebase = Path.of(git(second.resolve(".edict"), "rev-parse", "--path-format=absolute", "--git-path", "rebase-merge").trim())
    assertFalse(rebase.exists(), "the rebase was left in progress")
  }

  @Test
  fun `the state of a plain directory cannot be pushed`() {
    val project = cloneOrigin("project")
    project.resolve(".edict").createDirectories()

    val failure = assertFailsWith<IllegalStateException> { state(project).worktree() }

    assertEquals(
      "${project.resolve(".edict")} is not a worktree of $BRANCH; `qodana edict install` checks the branch out there",
      failure.message,
    )
  }

  private fun state(project: Path) = StateBranch(EdictLayout(project, project.resolve(".edict")), BRANCH)

  private fun origin(): Path = directory.resolve("origin.git")

  /** A clone of a bare origin with one commit on master, as a CI run checks the project out; origin is created once. */
  private fun cloneOrigin(name: String): Path {
    if (!origin().toFile().exists()) {
      git(directory, "init", "--quiet", "--bare", "--initial-branch=master", origin().toString())
      val seed = clone("seed")
      seed.resolve("Main.kt").writeText("fun main() {}\n")
      git(seed, "add", ".")
      git(seed, "commit", "--quiet", "-m", "Add main")
      git(seed, "push", "--quiet", "origin", "master")
    }
    return clone(name)
  }

  private fun clone(name: String): Path {
    git(directory, "clone", "--quiet", origin().toString(), name)
    val clone = directory.resolve(name).toRealPath()
    git(clone, "config", "user.name", "Edict Test")
    git(clone, "config", "user.email", "edict-test@example.invalid")
    git(clone, "config", "commit.gpgsign", "false")
    return clone
  }

  /** A run in a fresh clone: checks the state out, writes [file] into it and pushes it as "Run 1". */
  private fun publishState(name: String, file: Pair<String, String>) {
    val project = cloneOrigin(name)
    state(project).checkOut()
    project.resolve(".edict").resolve(file.first).apply { parent.createDirectories() }.writeText(file.second)
    state(project).worktree().push("Run 1")
  }
}

private const val BRANCH = "edict/state"

private fun git(directory: Path, vararg arguments: String): String = runProcess(directory, listOf("git", *arguments))
