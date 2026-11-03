// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.common

import java.nio.file.Path
import kotlin.io.path.appendText
import kotlin.io.path.exists
import kotlin.io.path.isDirectory
import kotlin.io.path.listDirectoryEntries
import kotlin.io.path.readLines

/**
 * The Edict state on its own [branch] of the project's repository, checked out as a worktree at the layout's state
 * directory while the project's checkout stays on its branch. The branch shares no history with the project's branches,
 * so nothing merges or rebases it into them; a CI run that starts from a fresh clone finds the state on origin.
 */
internal class StateBranch(layout: EdictLayout, private val branch: String) {
  private val project = layout.root
  private val stateDirectory = layout.stateDirectory
  private val checkedOut: Boolean get() = stateDirectory.resolve(".git").exists()

  /** Checks [branch] out at the state directory, unless the state has to stay a plain directory. */
  fun checkOut(): StateCheckout {
    if (checkedOut) return StateCheckout.Existing
    if (!isGitWorkTree(project)) return StateCheckout.PlainDirectory("$project is not a git repository")
    if (stateDirectory.isDirectory() && stateDirectory.listDirectoryEntries().isNotEmpty()) {
      return StateCheckout.PlainDirectory("$stateDirectory already holds files outside a worktree")
    }
    git(project, "check-ref-format", "--branch", branch)
    excludeFromProject()
    val checkout = when {
      git(project, "branch", "--list", branch).isNotBlank() -> StateCheckout.LocalBranch
      fetchFromOrigin(project) -> {
        git(project, "branch", "--track", branch, "origin/$branch")
        StateCheckout.FromOrigin
      }
      else -> {
        val emptyTree = git(project, "mktree").trim()
        // Edict's own commit: install must not fail where no git identity is configured, as on a fresh CI runner.
        val root = git(
          project, "-c", "user.name=Qodana Edict", "-c", "user.email=edict@qodana.invalid",
          "commit-tree", emptyTree, "-m", "Start the Edict state",
        ).trim()
        git(project, "branch", branch, root)
        StateCheckout.NewBranch
      }
    }
    git(project, "worktree", "add", stateDirectory.toString(), branch)
    return checkout
  }

  /** The checked-out state, which can be pushed; fails while the state is not checked out on [branch]. */
  fun worktree(): Worktree {
    check(checkedOut) { "$stateDirectory is not a worktree of $branch; `qodana edict install` checks the branch out there" }
    val head = git(stateDirectory, "symbolic-ref", "--short", "HEAD").trim()
    check(head == branch) { "$stateDirectory is on $head, not on edict.stateBranch $branch" }
    return Worktree()
  }

  fun describe(checkout: StateCheckout): String = when (checkout) {
    StateCheckout.Existing -> "Kept the existing checkout in $stateDirectory"
    StateCheckout.LocalBranch -> "Checked out $branch at $stateDirectory"
    StateCheckout.FromOrigin -> "Checked out $branch from origin at $stateDirectory"
    StateCheckout.NewBranch -> "Started $branch at $stateDirectory; `qodana edict state push` publishes it"
    is StateCheckout.PlainDirectory -> "Keeping the Edict state in the plain directory $stateDirectory: ${checkout.reason}"
  }

  inner class Worktree internal constructor() {
    /**
     * Commits every change with [message], rebases the commit onto the branch that another run may have pushed to
     * origin meanwhile, and pushes the branch to origin, setting it as upstream.
     */
    fun push(message: String) {
      git(stateDirectory, "add", "--all")
      if (git(stateDirectory, "status", "--porcelain").isNotBlank()) git(stateDirectory, "commit", "--quiet", "-m", message)
      if (fetchFromOrigin(stateDirectory)) {
        try {
          git(stateDirectory, "rebase", "--quiet", "origin/$branch")
        }
        catch (e: IllegalStateException) {
          // A server must not find the state in the middle of a rebase.
          git(stateDirectory, "rebase", "--abort")
          throw e
        }
      }
      git(stateDirectory, "push", "--quiet", "--set-upstream", "origin", "HEAD:refs/heads/$branch")
    }
  }

  /** Fetches [branch] from origin into `origin/<branch>`; false when there is no origin or it has no such branch. */
  private fun fetchFromOrigin(directory: Path): Boolean {
    if ("origin" !in git(directory, "remote").lines()) return false
    if (git(directory, "ls-remote", "--heads", "origin", branch).isBlank()) return false
    git(directory, "fetch", "--quiet", "--no-tags", "origin", "+refs/heads/$branch:refs/remotes/origin/$branch")
    return true
  }

  /** Keeps the state out of the project's own status, without changing a tracked file such as `.gitignore`. */
  private fun excludeFromProject() {
    // Git prints the real path, as EdictLayout resolves the project's.
    val top = Path.of(git(project, "rev-parse", "--show-toplevel").trim())
    val state = stateDirectory.toAbsolutePath().normalize()
    if (!state.startsWith(top)) return
    val pattern = "/${top.relativize(state).joinToString("/")}/"
    val exclude = Path.of(git(project, "rev-parse", "--path-format=absolute", "--git-path", "info/exclude").trim())
    if (exclude.exists() && pattern in exclude.readLines()) return
    exclude.parent.toFile().mkdirs()
    exclude.appendText("$pattern\n")
  }
}

/** What [StateBranch.checkOut] did with the state directory. */
internal sealed interface StateCheckout {
  /** The state directory already is a git checkout, which is kept as it is. */
  data object Existing : StateCheckout

  /** Checked out the branch that the repository already had. */
  data object LocalBranch : StateCheckout

  /** Checked out the branch from origin, tracking it. */
  data object FromOrigin : StateCheckout

  /** Started the branch with an empty commit; the first `qodana edict state push` publishes it. */
  data object NewBranch : StateCheckout

  /** The state stays a plain directory, for [reason]. */
  data class PlainDirectory(val reason: String) : StateCheckout
}

internal const val DEFAULT_EDICT_STATE_BRANCH = "edict/state"

private fun git(directory: Path, vararg arguments: String): String = runProcess(directory, listOf("git", *arguments))

/** Whether [directory] is inside a git working tree; git exits with an error outside one. */
private fun isGitWorkTree(directory: Path): Boolean =
  ProcessBuilder("git", "rev-parse", "--is-inside-work-tree")
    .directory(directory.toFile())
    .redirectOutput(ProcessBuilder.Redirect.DISCARD)
    .redirectError(ProcessBuilder.Redirect.DISCARD)
    .start()
    .waitFor() == 0
