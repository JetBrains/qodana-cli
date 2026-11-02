// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.edictnext.support

import org.jetbrains.qodana.edict.ci.PullRequest
import org.jetbrains.qodana.edict.ci.ReviewMessage
import org.jetbrains.qodana.edict.ci.ReviewThread
import org.jetbrains.qodana.edict.edictnext.EdictNextFileRevision
import org.jetbrains.qodana.edict.edictnext.EdictNextLineRange
import org.jetbrains.qodana.edict.edictnext.EdictNextSignal
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalLabel
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalProvenance
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalSource
import org.jetbrains.qodana.edict.edictnext.stableSignalId
import org.jetbrains.qodana.edict.extraction.reviews.PrAnalysisDateRange
import org.jetbrains.qodana.edict.extraction.reviews.PrItem
import org.jetbrains.qodana.edict.extraction.reviews.PrSignalDiff
import org.jetbrains.qodana.edict.extraction.reviews.PrSignalDraft
import java.time.LocalDate
import java.time.ZoneOffset

/** The UTC date the PR analysis runs on; it selects only the complete dates before it. */
internal val TODAY: LocalDate = LocalDate.of(2026, 10, 9)
internal val YESTERDAY: LocalDate = TODAY.minusDays(1)

/** The base revision of every fixture pull request. */
internal const val BASE = "f550b76e8c9d0e703fdc0ec9c2b0a23a4076f082"

/** The head revision of every fixture pull request. */
internal const val HEAD = "9b3ddcfd3b482ebe96600d8d37a5bd802a3e7051"

/** The revision between [BASE] and [HEAD] that fixture threads are commented at. */
internal const val COMMENTED = "144c0844248b8fff0d46b00156f6bcf79b18400a"

/** The repository-relative path of the file [CORRECTION]'s thread comments on. */
internal const val CORRECTED_PATH = "src/Example.kt"

/** Files of the provider at both revisions of the correction, none of them [CORRECTED_PATH]. */
internal val OTHER_FILES_ONLY = mapOf((COMMENTED to "README.md") to "Readme\n", (HEAD to "README.md") to "Readme\n")

/** The one message of every fixture thread. */
internal const val REVIEWER_MESSAGE = "Call good() here"

/**
 * A pull request of JetBrains/kotlin with [threads], merged on [mergedOn] at noon UTC plus [number] seconds, so a
 * higher number merged later that day.
 */
internal fun pullRequest(
  number: Int,
  vararg threads: ReviewThread,
  base: String = BASE,
  head: String = HEAD,
  mergedOn: LocalDate = YESTERDAY,
): PullRequest = PullRequest(
  number = number,
  url = "https://github.com/JetBrains/kotlin/pull/$number",
  title = "KT-$number Call good() instead of bad()",
  body = "Replaces the bad() call.",
  baseRevision = base,
  headRevision = head,
  closeTimestamp = mergedOn.atTime(12, 0).plusSeconds(number.toLong()).toInstant(ZoneOffset.UTC).toEpochMilli(),
  threads = threads.toList(),
)

/** A thread of pull request [pullRequest] on lines [start] to [end] of [path] at [revision], with [REVIEWER_MESSAGE]. */
internal fun thread(
  id: String,
  path: String = CORRECTED_PATH,
  revision: String = COMMENTED,
  start: Int? = 2,
  end: Int? = 2,
  pullRequest: Int = 7,
): ReviewThread = ReviewThread(
  threadId = id,
  reviewDiscussionUrl = "https://github.com/JetBrains/kotlin/pull/$pullRequest#discussion_r$id",
  filePath = path,
  originalCommitSha = revision,
  anchorLine = start,
  anchorEndLine = end,
  messages = listOf(ReviewMessage("reviewer", REVIEWER_MESSAGE, "2026-10-08T10:00:00Z")),
)

/** [count] pull requests merged on [date], numbered from [first]; PR `n` has the one thread `n` on `src/Example<n>.kt`. */
internal fun pullRequestsMergedOn(date: LocalDate, count: Int, first: Int = 1): List<PullRequest> =
  (first until first + count).map { number ->
    pullRequest(number, thread("$number", "src/Example$number.kt", pullRequest = number), mergedOn = date)
  }

/** PR 7, merged yesterday, with the one thread `r1` on [line] of [CORRECTED_PATH]. */
internal fun correctionAnchoredAt(line: Int): PullRequest = pullRequest(7, thread("r1", start = line, end = line))

/** The pull request most tests analyze: PR 7, merged yesterday, with the one thread `r1` on line 2 of [CORRECTED_PATH]. */
internal val CORRECTION: PullRequest = correctionAnchoredAt(2)

/** The range of the one UTC [date]. */
internal fun day(date: LocalDate): PrAnalysisDateRange = PrAnalysisDateRange(date.toString(), date.toString())

/** The files of [CORRECTION], by revision and path: `bad()` on line 2 at its base and comment revisions, `good()` at its head. */
internal fun correctionFiles(): Map<Pair<String, String>, String> = mapOf(
  (BASE to CORRECTED_PATH) to "fun a() {\n  bad()\n}\n",
  (COMMENTED to CORRECTED_PATH) to "fun a() {\n  bad()\n}\n",
  (HEAD to CORRECTED_PATH) to "fun a() {\n  good()\n}\n",
)

/**
 * Files of [CORRECTION] whose diff is over 4000 characters: line 100 changes from `bad()` to `good()`, and lines 200 to
 * 250, each over 80 characters, are all rewritten.
 */
internal fun largeDiffFiles(): Map<Pair<String, String>, String> = mapOf(
  (BASE to CORRECTED_PATH) to largeSource("bad()", "removed", 'y'),
  (COMMENTED to CORRECTED_PATH) to largeSource("bad()", "removed", 'y'),
  (HEAD to CORRECTED_PATH) to largeSource("good()", "added", 'z'),
)

/** Files of [CORRECTION] whose diff from the comment revision to the head is over 600 lines: all its 700 lines change. */
internal fun longDiffFiles(): Map<Pair<String, String>, String> = mapOf(
  (COMMENTED to CORRECTED_PATH) to (1..700).joinToString("") { "old $it\n" },
  (HEAD to CORRECTED_PATH) to (1..700).joinToString("") { "new $it\n" },
)

/** The evidence diff of [longDiffFiles]: its first 600 lines, the headers and 597 removed lines, then a pointer to the rest. */
internal val LONG_DIFF_FIRST_600_LINES: String =
  (listOf("--- a/$CORRECTED_PATH", "+++ b/$CORRECTED_PATH", "@@ -1,700 +1,700 @@") + (1..597).map { "-old $it" } +
    "… read the full diff for the rest").joinToString("\n")

/**
 * A worker's draft of [label] on [lines] of the item's file [at] a revision, diffed from [beforePath] at [before] to the
 * item's file at the head. By default a positive draft is at [before] and a negative one at the head, and the file kept
 * its path.
 */
internal fun draft(
  item: PrItem,
  label: EdictNextSignalLabel,
  lines: EdictNextLineRange = EdictNextLineRange(2, 2),
  before: String = item.pr.baseRevision,
  at: String = if (label == EdictNextSignalLabel.POSITIVE) before else item.pr.headRevision,
  beforePath: String = item.thread.filePath,
): PrSignalDraft = PrSignalDraft(
  workItemId = item.workItemId,
  fileRevision = EdictNextFileRevision(item.thread.filePath, at, listOf(lines)),
  label = label,
  description = DRAFT_DESCRIPTION,
  diff = PrSignalDiff(before, item.pr.headRevision, beforePath, item.thread.filePath),
)

/** The description of every fixture draft and Signal. */
internal const val DRAFT_DESCRIPTION = "Calls bad() where good() is required"

/**
 * A complete FromPR Signal of [label] on line 2 of the item's file in batch [batchId], under the idempotency [key]: as a
 * worker supplies it, and as the analysis completes a [draft]. It repeats the item's PR [title], discussion [messages]
 * and URL, with [diff]; a positive Signal is at the base by default, a negative one at the head.
 */
internal fun prSignal(
  item: PrItem,
  batchId: String,
  label: EdictNextSignalLabel,
  key: String = "${item.workItemId}:$label",
  diff: String = "--- a/${item.thread.filePath}\n+++ b/${item.thread.filePath}\n@@ -2 +2 @@\n-  bad()\n+  good()\n",
  at: String = if (label == EdictNextSignalLabel.POSITIVE) item.pr.baseRevision else item.pr.headRevision,
  title: String = item.pr.title,
  messages: List<String> = listOf(REVIEWER_MESSAGE),
  description: String = DRAFT_DESCRIPTION,
  workItemId: String = item.workItemId,
): EdictNextSignal = EdictNextSignal(
  id = stableSignalId(key),
  idempotencyKey = key,
  fileRevision = EdictNextFileRevision(item.thread.filePath, at, listOf(EdictNextLineRange(2, 2))),
  source = EdictNextSignalSource.FromPR(
    prNumber = item.pr.number,
    title = title,
    discussionMessages = messages,
    diffPositiveToNegative = diff,
    url = item.thread.reviewDiscussionUrl,
  ),
  label = label,
  description = description,
  provenance = EdictNextSignalProvenance(workItemId, batchId),
)

/** A 250-line source with [fix] on line 100 and long lines starting with [replaced] on lines 200 to 250. */
private fun largeSource(fix: String, replaced: String, fill: Char): String = (1..250).joinToString("") { line ->
  when (line) {
    100 -> fix
    in 200..250 -> "$replaced $line ${"$fill".repeat(80)}"
    else -> "context $line ${"x".repeat(40)}"
  } + "\n"
}
