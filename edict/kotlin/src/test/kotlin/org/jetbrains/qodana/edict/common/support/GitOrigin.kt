// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.common.support

import org.jetbrains.qodana.edict.common.DEFAULT_EDICT_STATE_BRANCH
import org.jetbrains.qodana.edict.common.EdictLayout
import org.jetbrains.qodana.edict.common.StateBranch
import org.jetbrains.qodana.edict.common.runProcess
import java.nio.file.Path
import kotlin.io.path.createDirectories
import kotlin.io.path.exists
import kotlin.io.path.writeText

/**
 * A bare origin repository in [directory], created by the first clone with one commit on master, and its clones, as CI runs
 * check a project out.
 */
internal class GitOrigin(private val directory: Path) {
  val path: Path = directory.resolve("origin.git")

  /** A clone [name] of origin, with [options] for `git clone`, committing as `Edict Test`. */
  fun clone(name: String, vararg options: String): Path {
    if (!path.exists()) seed()
    return plainClone(name, *options)
  }

  /**
   * A run in a fresh clone [name]: checks the state out at `.edict`, writes [files], contents by their path in the state,
   * and pushes the state as "Run 1".
   */
  fun publishState(name: String, files: Map<String, String>) {
    val project = clone(name)
    val state = StateBranch(EdictLayout(project, project.resolve(".edict")), DEFAULT_EDICT_STATE_BRANCH)
    state.checkOut(shallow = false)
    files.forEach { (file, content) -> project.resolve(".edict").resolve(file).apply { parent.createDirectories() }.writeText(content) }
    state.worktree().push("Run 1")
  }

  /**
   * Makes the next push to the state lose a race to a run in a fresh clone [name] that writes [files]: just before origin
   * takes the push, that run's commit on the state as it is now lands, and origin refuses the push. Later pushes go through.
   */
  fun raceNextStatePush(name: String, files: Map<String, String>) {
    val project = clone(name)
    StateBranch(EdictLayout(project, project.resolve(".edict")), DEFAULT_EDICT_STATE_BRANCH).checkOut(shallow = false)
    val state = project.resolve(".edict")
    files.forEach { (file, content) -> state.resolve(file).apply { parent.createDirectories() }.writeText(content) }
    git(state, "add", "--all")
    git(state, "commit", "--quiet", "-m", "Run of $name")
    git(state, "push", "--quiet", "origin", "HEAD:$RACE_REF")
    // Receiving keeps new objects in quarantine, out of reach of ref updates, until the hook accepts them.
    val hook = """
      |#!/bin/sh
      |[ -e raced ] && exit 0
      |touch raced
      |env -u GIT_QUARANTINE_PATH -u GIT_OBJECT_DIRECTORY -u GIT_ALTERNATE_OBJECT_DIRECTORIES \
      |  git update-ref refs/heads/$DEFAULT_EDICT_STATE_BRANCH $RACE_REF
      |exit 1
      |""".trimMargin()
    path.resolve("hooks").createDirectories().resolve("pre-receive").apply { writeText(hook) }.toFile().setExecutable(true)
  }

  private fun seed() {
    git(directory, "init", "--quiet", "--bare", "--initial-branch=master", path.toString())
    val seed = plainClone("seed")
    seed.resolve("Main.kt").writeText("fun main() {}\n")
    git(seed, "add", ".")
    git(seed, "commit", "--quiet", "-m", "Add main")
    git(seed, "push", "--quiet", "origin", "master")
  }

  private fun plainClone(name: String, vararg options: String): Path {
    git(directory, "clone", "--quiet", *options, path.toString(), name)
    val clone = directory.resolve(name).toRealPath()
    git(clone, "config", "user.name", "Edict Test")
    git(clone, "config", "user.email", "edict-test@example.invalid")
    git(clone, "config", "commit.gpgsign", "false")
    return clone
  }
}

/** Where [GitOrigin.raceNextStatePush] keeps the commit of the run that wins the race. */
private const val RACE_REF = "refs/race/state"

private fun git(directory: Path, vararg arguments: String): String = runProcess(directory, listOf("git", *arguments))
