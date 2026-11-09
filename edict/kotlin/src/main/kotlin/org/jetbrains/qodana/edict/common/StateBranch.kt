// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.common

import java.nio.file.Path
import kotlin.io.path.appendText
import kotlin.io.path.exists
import kotlin.io.path.isDirectory
import kotlin.io.path.listDirectoryEntries
import kotlin.io.path.readText

/**
 * The Edict state on its own [branch] of the project's repository, checked out as a worktree at the layout's state
 * directory while the project's checkout stays on its branch. The branch shares no history with the project's branches,
 * so nothing merges or rebases it into them; a CI run that starts from a fresh clone finds the state on origin.
 */
internal class StateBranch(layout: EdictLayout, private val branch: String) {
  private val project = layout.root
  private val stateDirectory = layout.stateDirectory
  private val checkedOut: Boolean get() = stateDirectory.resolve(".git").exists()

  /**
   * Checks [branch] out at the state directory, unless the directory already holds a state; fails outside a git
   * repository, where Edict cannot work.
   */
  fun checkOut(): StateCheckout {
    if (checkedOut) return head().let { if (it == branch) StateCheckout.Existing else StateCheckout.OtherBranch(it) }
    check(isGitWorkTree(project)) { "$project is not a git repository; Edict keeps its state on a branch of the project's" }
    if (stateDirectory.isDirectory() && stateDirectory.listDirectoryEntries().isNotEmpty()) return StateCheckout.PlainDirectory
    git(project, "check-ref-format", "--branch", branch)
    excludeFromProject()
    val checkout = when {
      git(project, "branch", "--list", branch).isNotBlank() -> StateCheckout.LocalBranch
      fetchFromOrigin(project) -> {
        // Not tracking: a single-branch clone, as CI checks a project out, has no fetch refspec for the branch.
        git(project, "branch", "--no-track", branch, "origin/$branch")
        StateCheckout.FromOrigin
      }
      else -> {
        val emptyTree = git(project, "mktree").trim()
        val root = git(project, *EDICT_IDENTITY, "commit-tree", emptyTree, "-m", "Start the Edict state").trim()
        git(project, "branch", branch, root)
        StateCheckout.NewBranch
      }
    }
    // A state directory deleted by hand leaves its worktree registered, which would refuse the new one.
    git(project, "worktree", "prune")
    git(project, "worktree", "add", stateDirectory.toString(), branch)
    return checkout
  }

  /** The checked-out state, which can be pushed; fails while the state is not checked out on [branch]. */
  fun worktree(): Worktree {
    check(checkedOut) {
      "$stateDirectory is not a worktree of $branch; `qodana edict install` checks the branch out into an empty state " +
        "directory of a git project"
    }
    val head = head()
    check(head == branch) { "$stateDirectory is on $head, not on edict.stateBranch $branch" }
    return Worktree()
  }

  fun describe(checkout: StateCheckout): String = when (checkout) {
    StateCheckout.Existing -> "Kept the existing checkout in $stateDirectory"
    is StateCheckout.OtherBranch ->
      "Kept the existing checkout of ${checkout.head} in $stateDirectory, which is not edict.stateBranch $branch; " +
        "`qodana edict state push` refuses it until they agree"
    StateCheckout.LocalBranch -> "Checked out $branch at $stateDirectory"
    StateCheckout.FromOrigin -> "Checked out $branch from origin at $stateDirectory"
    StateCheckout.NewBranch -> "Started $branch at $stateDirectory; `qodana edict state push` publishes it"
    StateCheckout.PlainDirectory ->
      "Keeping the Edict state in the plain directory $stateDirectory, which already holds files outside a worktree"
  }

  inner class Worktree internal constructor() {
    /**
     * Commits every change with [message], rebases the commit onto the branch that another run may have pushed to
     * origin meanwhile, and pushes the branch to origin, setting it as upstream. Rebases again when another run pushes
     * between the rebase and the push, at most [MAX_PUSH_ATTEMPTS] times.
     */
    fun push(message: String) {
      commit(message)
      var base = originRevision()
      repeat(MAX_PUSH_ATTEMPTS) {
        if (base != null) rebaseOnto("origin/$branch")
        try {
          git(stateDirectory, "push", "--quiet", "--no-verify", "--set-upstream", "origin", "HEAD:refs/heads/$branch")
          return
        }
        catch (e: IllegalStateException) {
          base = originMovedFrom(base, e) ?: throw e
        }
      }
      error("Other runs kept pushing $branch to origin; $MAX_PUSH_ATTEMPTS pushes of the Edict state were refused")
    }

    /**
     * Moves the checkout to the branch as origin has it now, dropping its local commits, and returns that commit's full SHA;
     * fails on uncommitted changes, which it would lose, and when origin has no such branch.
     */
    fun resetToOrigin(): String {
      check(git(stateDirectory, "status", "--porcelain").isBlank()) { "$stateDirectory has uncommitted changes" }
      check(fetchFromOrigin(stateDirectory)) { "origin has no $branch" }
      git(stateDirectory, "reset", "--quiet", "--hard", "origin/$branch")
      return revision("HEAD")
    }

    /**
     * Commits every change with [message] onto [base], the commit [resetToOrigin] returned, and pushes it to origin. False when origin
     * got other commits since, which the push refuses to overwrite.
     */
    fun publish(message: String, base: String): Boolean {
      commit(message)
      try {
        git(stateDirectory, "push", "--quiet", "--no-verify", "origin", "HEAD:refs/heads/$branch")
        return true
      }
      catch (e: IllegalStateException) {
        if (originMovedFrom(base, e) != null) return false
        throw e
      }
    }

    private fun rebaseOnto(upstream: String) {
      try {
        git(stateDirectory, *EDICT_IDENTITY, "rebase", "--quiet", "--no-verify", upstream)
      }
      catch (e: IllegalStateException) {
        // A server must not find the state in the middle of a rebase.
        runCatching { git(stateDirectory, "rebase", "--abort") }.exceptionOrNull()?.let(e::addSuppressed)
        throw e
      }
    }

    /**
     * The commit origin's branch moved to from [base] after a push [failed]; null when it did not move. Git exits alike for a
     * push refused as not fast-forward and for one that failed otherwise, and only after the first did origin move. A failed
     * fetch adds to [failed].
     */
    private fun originMovedFrom(base: String?, failed: IllegalStateException): String? = try {
      originRevision()?.takeIf { it != base }
    }
    catch (fetch: IllegalStateException) {
      failed.addSuppressed(fetch)
      null
    }

    /** The commit origin's branch is on, fetched now; null when origin has no such branch. */
    private fun originRevision(): String? = if (fetchFromOrigin(stateDirectory)) revision("origin/$branch") else null

    /** Commits every change with [message], if there is any; Edict's own commits, which the project's hooks are not for. */
    private fun commit(message: String) {
      git(stateDirectory, "add", "--all")
      if (git(stateDirectory, "status", "--porcelain").isNotBlank()) {
        git(stateDirectory, *EDICT_IDENTITY, "commit", "--quiet", "--no-verify", "-m", message)
      }
    }

    private fun revision(name: String): String = git(stateDirectory, "rev-parse", "--verify", name).trim()
  }

  /** Fetches [branch] from origin into `origin/<branch>`; false when there is no origin or it has no such branch. */
  private fun fetchFromOrigin(directory: Path): Boolean {
    if ("origin" !in git(directory, "remote").lines()) return false
    if (git(directory, "ls-remote", "origin", "refs/heads/$branch").isBlank()) return false
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
    val content = if (exclude.exists()) exclude.readText() else ""
    if (pattern in content.lines()) return
    exclude.parent.toFile().mkdirs()
    exclude.appendText(ignoreFileAddition(content, listOf(pattern)))
  }

  /** The branch the state directory is on; `HEAD` when it is on none. */
  private fun head(): String = git(stateDirectory, "rev-parse", "--abbrev-ref", "HEAD").trim()
}

/** What [StateBranch.checkOut] did with the state directory. */
internal sealed interface StateCheckout {
  /** The state directory already is a git checkout of the branch, which is kept as it is. */
  data object Existing : StateCheckout

  /** The state directory already is a git checkout, of [head] instead of the branch; it is kept, but cannot be pushed. */
  data class OtherBranch(val head: String) : StateCheckout

  /** Checked out the branch that the repository already had. */
  data object LocalBranch : StateCheckout

  /** Checked out the branch from origin; the first push sets it as upstream. */
  data object FromOrigin : StateCheckout

  /** Started the branch with an empty commit; the first `qodana edict state push` publishes it. */
  data object NewBranch : StateCheckout

  /** The state directory already holds files outside a worktree, such as a prepared state; it stays a plain directory. */
  data object PlainDirectory : StateCheckout
}

internal const val DEFAULT_EDICT_STATE_BRANCH = "edict/state"

/** Pushes of the state that [StateBranch.Worktree.push] tries while other runs keep pushing the branch meanwhile. */
private const val MAX_PUSH_ATTEMPTS = 5

/** Edict commits as itself, so it doesn't fail where no git identity is configured, as on a fresh CI runner. */
private val EDICT_IDENTITY = arrayOf("-c", "user.name=Qodana Edict", "-c", "user.email=qodana-support@jetbrains.com")

private fun git(directory: Path, vararg arguments: String): String = runProcess(directory, listOf("git", *arguments))

/** Whether [directory] is inside a git working tree; git exits with an error outside one. */
private fun isGitWorkTree(directory: Path): Boolean =
  ProcessBuilder("git", "rev-parse", "--is-inside-work-tree")
    .directory(directory.toFile())
    .redirectOutput(ProcessBuilder.Redirect.DISCARD)
    .redirectError(ProcessBuilder.Redirect.DISCARD)
    .start()
    .waitFor() == 0
