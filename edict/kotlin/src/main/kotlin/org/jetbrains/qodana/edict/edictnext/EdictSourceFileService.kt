// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.edictnext

import org.jetbrains.qodana.edict.ci.ReviewExtractionApi
import org.jetbrains.qodana.edict.ci.ReviewRepository
import org.jetbrains.qodana.edict.common.GitRepository
import org.jetbrains.qodana.edict.signals.validRevision
import org.jetbrains.qodana.edict.signals.validSourcePath
import java.nio.file.Path

/**
 * Reads repository files at an exact commit: from local Git when it has the commit, otherwise from the [remote] review
 * repository, so commits missing from a shallow or squash-merged checkout stay readable.
 */
internal class EdictSourceFileService(
  projectRoot: Path,
  private val provider: ReviewExtractionApi,
  private val remote: ReviewRepository?,
) {
  // Opened on first read, so a project without Git still serves every other tool.
  private val local by lazy { GitRepository(projectRoot) }

  /**
   * Returns the file with 1-based line numbers; with an [anchor], only its lines and [radius] lines around them. A path
   * missing at a local commit fails without asking the remote, which has the same commit.
   */
  fun fileAtRef(path: String, ref: String, anchor: EdictNextLineRange? = null, radius: Int = DEFAULT_RADIUS): String {
    require(validRevision(ref) && validSourcePath(path)) { "Requires a full commit SHA and a repository-relative path" }
    require(radius >= 0) { "Radius must not be negative" }
    anchor?.let { require(it.start >= 1 && it.end >= it.start) { "Invalid anchor lines ${it.start}-${it.end}" } }
    val content = if (hasLocalCommit(ref)) local.fileAt(ref, path)
    else provider.file(
      checkNotNull(remote) { "Commit $ref is not in the local repository, and edict.ci.url names no remote one" },
      ref,
      path,
    )
    return numberedLines(content, anchor, radius)
  }

  private fun hasLocalCommit(ref: String): Boolean =
    runCatching { local.git("cat-file", "-e", "$ref^{commit}") }.isSuccess

  companion object {
    const val DEFAULT_RADIUS = 100
  }
}

private fun numberedLines(content: String, anchor: EdictNextLineRange?, radius: Int): String {
  val lines = content.lines().let { if (it.size > 1 && it.last().isEmpty()) it.dropLast(1) else it }
  val first = anchor?.let { (it.start - 1 - radius).coerceAtLeast(0) } ?: 0
  val last = anchor?.let { (it.end + radius).coerceAtMost(lines.size) } ?: lines.size
  require(first < last) { "Anchor line ${anchor?.start} is beyond the file's ${lines.size} lines" }
  return (first until last).joinToString("\n") { "%5d| %s".format(it + 1, lines[it]) }
}
