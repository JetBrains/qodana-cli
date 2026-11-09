// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.common

import org.jetbrains.qodana.edict.common.support.GitOrigin
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

  private val origin by lazy { GitOrigin(directory) }

  @Test
  fun `starts the branch without history when origin has none, out of the project's status`() {
    val project = origin.clone("project")

    val checkout = state(project).checkOut()

    assertEquals(StateCheckout.NewBranch, checkout)
    assertEquals("Start the Edict state\n", git(project, "log", "--format=%s", BRANCH))
    assertEquals(BRANCH, git(project.resolve(".edict"), "symbolic-ref", "--short", "HEAD").trim())
    assertEquals("", git(project, "status", "--porcelain"))
  }

  @Test
  @SetEnvironmentVariable(key = "GIT_CONFIG_GLOBAL", value = "/dev/null")
  fun `starts the branch where no git identity is configured, as on a fresh CI runner`() {
    val project = origin.clone("project")
    git(project, "config", "--unset", "user.name")
    git(project, "config", "--unset", "user.email")
    // Git would otherwise guess an identity from the system.
    git(project, "config", "user.useConfigOnly", "true")

    val checkout = state(project).checkOut()

    assertEquals(StateCheckout.NewBranch, checkout)
    assertEquals("Qodana Edict <qodana-support@jetbrains.com>\n", git(project, "log", "--format=%an <%ae>", BRANCH))
  }

  @Test
  fun `checks out the branch that origin has`() {
    origin.publishState("first", mapOf("clusters/c.json" to "{}"))
    val project = origin.clone("second")

    val checkout = state(project).checkOut()

    assertEquals(StateCheckout.FromOrigin, checkout)
    assertEquals("{}", project.resolve(".edict/clusters/c.json").readText())
  }

  @Test
  fun `checks out the branch that origin has into a single-branch clone, as CI checks a project out`() {
    origin.publishState("first", mapOf("clusters/c.json" to "{}"))
    val project = origin.clone("second", "--single-branch")

    val checkout = state(project).checkOut()

    assertEquals(StateCheckout.FromOrigin, checkout)
    assertEquals("{}", project.resolve(".edict/clusters/c.json").readText())
  }

  @Test
  fun `a branch of origin whose name only ends with the state branch's is another branch`() {
    val other = origin.clone("other")
    git(other, "push", "--quiet", "origin", "HEAD:refs/heads/x/$BRANCH")
    val project = origin.clone("project")

    assertEquals(StateCheckout.NewBranch, state(project).checkOut())
  }

  @Test
  fun `checks out the local branch of a removed worktree`() {
    val project = origin.clone("project")
    state(project).checkOut()
    git(project, "worktree", "remove", ".edict")

    assertEquals(StateCheckout.LocalBranch, state(project).checkOut())
  }

  @Test
  fun `checks out the local branch again after the state directory was deleted by hand`() {
    val project = origin.clone("project")
    state(project).checkOut()
    project.resolve(".edict").toFile().deleteRecursively()

    assertEquals(StateCheckout.LocalBranch, state(project).checkOut())
    assertEquals(BRANCH, git(project.resolve(".edict"), "symbolic-ref", "--short", "HEAD").trim())
  }

  @Test
  fun `keeps an existing checkout`() {
    val project = origin.clone("project")
    state(project).checkOut()

    assertEquals(StateCheckout.Existing, state(project).checkOut())
  }

  @Test
  fun `keeps an existing checkout of another branch, and says it is not the state branch`() {
    val project = origin.clone("project")
    state(project).checkOut()
    git(project.resolve(".edict"), "switch", "--quiet", "-c", "other")

    assertEquals(StateCheckout.OtherBranch("other"), state(project).checkOut())
  }

  @Test
  fun `the state's exclusion starts a line of its own when the exclude file doesn't end its last line`() {
    val project = origin.clone("project")
    val exclude = project.resolve(".git/info/exclude").apply { writeText("*.log") }

    state(project).checkOut()

    assertEquals("*.log\n/.edict/\n", exclude.readText())
  }

  @Test
  fun `refuses a project outside a git repository`() {
    val project = directory.resolve("plain").createDirectories().toRealPath()

    val error = assertFailsWith<IllegalStateException> { state(project).checkOut() }

    assertEquals("$project is not a git repository; Edict keeps its state on a branch of the project's", error.message)
  }

  @Test
  fun `keeps a state directory that already holds files, such as state committed on the project's branch`() {
    val project = origin.clone("project")
    project.resolve(".edict").createDirectories().resolve("plan.json").writeText("{}")

    val checkout = state(project).checkOut()

    assertEquals(StateCheckout.PlainDirectory, checkout)
  }

  @Test
  fun `push commits the state and publishes the branch`() {
    origin.publishState("project", mapOf("clusters/c.json" to "{}"))

    assertEquals("Run 1\nStart the Edict state\n", git(origin.path, "log", "--format=%s", BRANCH))
    assertEquals("{}", git(origin.path, "show", "$BRANCH:clusters/c.json"))
  }

  @Test
  fun `push rebases onto the state that another run pushed first`() {
    origin.publishState("first", mapOf("first.json" to "1"))
    val second = origin.clone("second")
    state(second).checkOut()
    origin.publishState("third", mapOf("third.json" to "3"))
    second.resolve(".edict/second.json").writeText("2")

    state(second).worktree().push("Run 2")

    assertEquals("Run 2\nRun 1\nRun 1\nStart the Edict state\n", git(origin.path, "log", "--format=%s", BRANCH))
    assertEquals("first.json\nsecond.json\nthird.json\n", git(origin.path, "ls-tree", "--name-only", BRANCH))
  }

  @Test
  @SetEnvironmentVariable(key = "GIT_CONFIG_GLOBAL", value = "/dev/null")
  fun `push commits and rebases where no git identity is configured`() {
    origin.publishState("first", mapOf("first.json" to "1"))
    val second = origin.clone("second")
    state(second).checkOut()
    origin.publishState("third", mapOf("third.json" to "3"))
    git(second, "config", "--unset", "user.name")
    git(second, "config", "--unset", "user.email")
    // Git would otherwise guess an identity from the system.
    git(second, "config", "user.useConfigOnly", "true")
    second.resolve(".edict/second.json").writeText("2")

    state(second).worktree().push("Run 2")

    assertEquals("Run 2 by Qodana Edict\nRun 1 by Qodana Edict\n", git(origin.path, "log", "-2", "--format=%s by %an", BRANCH))
  }

  @Test
  fun `push runs none of the project's hooks`() {
    origin.publishState("first", mapOf("first.json" to "1"))
    val second = origin.clone("second")
    state(second).checkOut()
    origin.publishState("third", mapOf("third.json" to "3"))
    listOf("pre-commit", "commit-msg", "pre-rebase", "pre-push").forEach { failingHook(second, it) }
    second.resolve(".edict/second.json").writeText("2")

    state(second).worktree().push("Run 2")

    assertEquals("first.json\nsecond.json\nthird.json\n", git(origin.path, "ls-tree", "--name-only", BRANCH))
  }

  @Test
  fun `a push that conflicts with the state another run pushed fails and leaves the worktree on its branch`() {
    origin.publishState("first", mapOf("plan.json" to "1"))
    val second = origin.clone("second")
    state(second).checkOut()
    origin.publishState("third", mapOf("plan.json" to "3"))
    second.resolve(".edict/plan.json").writeText("2")

    assertFailsWith<IllegalStateException> { state(second).worktree().push("Run 2") }

    assertEquals(BRANCH, git(second.resolve(".edict"), "symbolic-ref", "--short", "HEAD").trim())
    val rebase = Path.of(git(second.resolve(".edict"), "rev-parse", "--path-format=absolute", "--git-path", "rebase-merge").trim())
    assertFalse(rebase.exists(), "the rebase was left in progress")
  }

  @Test
  fun `the state of a plain directory cannot be pushed`() {
    val project = origin.clone("project")
    project.resolve(".edict").createDirectories()

    val failure = assertFailsWith<IllegalStateException> { state(project).worktree() }

    assertEquals(
      "${project.resolve(".edict")} is not a worktree of $BRANCH; `qodana edict install` checks the branch out into an empty " +
        "state directory of a git project",
      failure.message,
    )
  }

  @Test
  fun `reset takes the branch as origin has it, without the commits of the checkout`() {
    origin.publishState("first", mapOf("first.json" to "1"))
    val second = origin.clone("second")
    state(second).checkOut()
    second.resolve(".edict/local.json").writeText("2")
    git(second.resolve(".edict"), "add", "--all")
    git(second.resolve(".edict"), "commit", "--quiet", "-m", "Local")
    origin.publishState("third", mapOf("third.json" to "3"))

    val base = state(second).worktree().resetToOrigin()

    assertEquals(git(origin.path, "rev-parse", BRANCH).trim(), base)
    assertEquals("$base\n", git(second.resolve(".edict"), "rev-parse", "HEAD"))
  }

  @Test
  fun `reset refuses a checkout with uncommitted changes, which it would lose`() {
    origin.publishState("first", mapOf("first.json" to "1"))
    val second = origin.clone("second")
    state(second).checkOut()
    second.resolve(".edict/local.json").writeText("2")

    val failure = assertFailsWith<IllegalStateException> { state(second).worktree().resetToOrigin() }

    assertEquals("${second.resolve(".edict")} has uncommitted changes", failure.message)
  }

  @Test
  fun `publish pushes the changes onto the branch as reset took it`() {
    origin.publishState("first", mapOf("first.json" to "1"))
    val second = origin.clone("second")
    state(second).checkOut()
    val worktree = state(second).worktree()
    val base = worktree.resetToOrigin()
    second.resolve(".edict/second.json").writeText("2")

    val published = worktree.publish("Run 2", base)

    assertEquals(true, published)
    assertEquals("Run 2\nRun 1\n", git(origin.path, "log", "-2", "--format=%s", BRANCH))
  }

  @Test
  fun `publish reports a push that lost to another run's, leaving origin as that run made it`() {
    origin.publishState("first", mapOf("first.json" to "1"))
    val second = origin.clone("second")
    state(second).checkOut()
    val worktree = state(second).worktree()
    val base = worktree.resetToOrigin()
    origin.raceNextStatePush("other", mapOf("other.json" to "o"))
    second.resolve(".edict/second.json").writeText("2")

    val published = worktree.publish("Run 2", base)

    assertEquals(false, published)
    assertEquals("first.json\nother.json\n", git(origin.path, "ls-tree", "--name-only", BRANCH))
  }

  @Test
  fun `publish fails when origin refuses the push for another reason`() {
    origin.publishState("first", mapOf("first.json" to "1"))
    val second = origin.clone("second")
    state(second).checkOut()
    val worktree = state(second).worktree()
    val base = worktree.resetToOrigin()
    failingHook(origin.path, "pre-receive")
    second.resolve(".edict/second.json").writeText("2")

    assertFailsWith<IllegalStateException> { worktree.publish("Run 2", base) }
  }

  private fun state(project: Path) = StateBranch(EdictLayout(project, project.resolve(".edict")), BRANCH)

  /** A [name] hook of the repository [project] that refuses whatever it is run for. */
  private fun failingHook(project: Path, name: String) {
    val hooks = Path.of(git(project, "rev-parse", "--path-format=absolute", "--git-path", "hooks").trim()).createDirectories()
    hooks.resolve(name).apply { writeText("#!/bin/sh\nexit 1\n") }.toFile().setExecutable(true)
  }
}

// The branch GitOrigin publishes the state on.
private const val BRANCH = DEFAULT_EDICT_STATE_BRANCH

private fun git(directory: Path, vararg arguments: String): String = runProcess(directory, listOf("git", *arguments))
