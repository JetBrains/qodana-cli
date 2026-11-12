// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.core

import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.encodeToJsonElement
import kotlinx.serialization.json.put
import org.jetbrains.qodana.edict.ci.ReviewSelection
import org.jetbrains.qodana.edict.ci.support.KOTLIN
import org.jetbrains.qodana.edict.common.wireJson
import org.jetbrains.qodana.edict.core.EdictSignalLabel.NEGATIVE
import org.jetbrains.qodana.edict.core.EdictSignalLabel.POSITIVE
import org.jetbrains.qodana.edict.core.support.BASE
import org.jetbrains.qodana.edict.core.support.COMMENTED
import org.jetbrains.qodana.edict.core.support.COMMENTED_SOURCE
import org.jetbrains.qodana.edict.core.support.CORRECTED_PATH
import org.jetbrains.qodana.edict.core.support.OTHER_FILES_ONLY
import org.jetbrains.qodana.edict.core.support.CORRECTION
import org.jetbrains.qodana.edict.core.support.HEAD
import org.jetbrains.qodana.edict.core.support.LONG_DIFF_FIRST_600_LINES
import org.jetbrains.qodana.edict.core.support.PR_ANALYSIS
import org.jetbrains.qodana.edict.core.support.PrAnalysisWorkspace
import org.jetbrains.qodana.edict.core.support.REVIEWER_MESSAGE
import org.jetbrains.qodana.edict.core.support.SIGNAL_ANALYSIS
import org.jetbrains.qodana.edict.core.support.TODAY
import org.jetbrains.qodana.edict.core.support.YESTERDAY
import org.jetbrains.qodana.edict.core.support.correctionAnchoredAt
import org.jetbrains.qodana.edict.core.support.correctionFiles
import org.jetbrains.qodana.edict.core.support.day
import org.jetbrains.qodana.edict.core.support.draft
import org.jetbrains.qodana.edict.core.support.largeDiffFiles
import org.jetbrains.qodana.edict.core.support.longDiffFiles
import org.jetbrains.qodana.edict.core.support.prSignal
import org.jetbrains.qodana.edict.core.support.pullRequest
import org.jetbrains.qodana.edict.core.support.pullRequestsMergedOn
import org.jetbrains.qodana.edict.core.support.thread
import org.jetbrains.qodana.edict.core.support.toolError
import org.jetbrains.qodana.edict.extraction.reviews.PrAnalysis
import org.jetbrains.qodana.edict.extraction.reviews.PrAnalysisDateRange
import org.jetbrains.qodana.edict.extraction.reviews.PrBatchSummary
import org.jetbrains.qodana.edict.extraction.reviews.PrItem
import org.jetbrains.qodana.edict.extraction.reviews.PrItemEvidence
import org.jetbrains.qodana.edict.extraction.reviews.PrPublicationReceipt
import org.jetbrains.qodana.edict.extraction.reviews.PrReceipt
import org.jetbrains.qodana.edict.extraction.reviews.PrSignalStoreFailure
import org.jetbrains.qodana.edict.extraction.reviews.RepositoryPrAnalysisCoverage
import org.jetbrains.qodana.edict.extraction.reviews.ReviewSelectionInput
import org.jetbrains.qodana.edict.signals.support.diff
import org.junit.jupiter.api.extension.RegisterExtension
import kotlin.test.Test
import kotlin.test.assertContains
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertNotEquals
import kotlin.test.assertNull

internal class ConfiguredPrAnalysisTest {
  @JvmField
  @RegisterExtension
  val pr = PrAnalysisWorkspace()

  @Test
  fun `a default batch fetches every pull request merged yesterday`() {
    pr.analysis.prepareBatch(pr.coordinator, ReviewSelectionInput())

    assertEquals(listOf(ReviewSelection(1000, startDate = "2026-10-08", endDate = "2026-10-08")), pr.reviews.selections)
  }

  @Test
  fun `a default batch after yesterday is covered holds the day before`() {
    pr.reviews.pullRequests += pullRequestsMergedOn(YESTERDAY, 1, first = 20) + pullRequestsMergedOn(YESTERDAY.minusDays(1), 1, first = 10)
    pr.coverDates(day(YESTERDAY))

    val batch = pr.analysis.prepareBatch(pr.coordinator, ReviewSelectionInput())
    assertEquals(listOf(listOf(10 to "10")), pr.items(batch))
  }

  @Test
  fun `a default batch leaves out the pull requests already analyzed by number`() {
    pr.reviews.pullRequests += pullRequestsMergedOn(YESTERDAY, 2, first = 20)
    pr.coverNumbers(21)

    val batch = pr.analysis.prepareBatch(pr.coordinator, ReviewSelectionInput())
    assertEquals(listOf(listOf(20 to "20")), pr.items(batch))
  }

  @Test
  fun `a default batch is empty and fetches nothing once every date since 1970 is covered`() {
    pr.coverDates(PrAnalysisDateRange("1970-01-01", YESTERDAY.toString()))

    val batch = pr.analysis.prepareBatch(pr.coordinator, ReviewSelectionInput())
    assertEquals(0, batch.availableChunkCount)
    assertEquals(emptyList(), pr.reviews.selections)
  }

  @Test
  fun `publishing the empty batch of an exhausted history covers nothing`() {
    pr.coverDates(PrAnalysisDateRange("1970-01-01", YESTERDAY.toString()))
    val batch = pr.analysis.prepareBatch(pr.coordinator, ReviewSelectionInput())

    assertEquals(
      PrPublicationReceipt(batch.batchId, 0, 0, emptyList(), emptyList(), emptyList(), 0, 0, 0, emptyList(), emptyList(), false),
      pr.analysis.publishValidated(pr.coordinator, batch.batchId),
    )
  }

  @Test
  fun `publishing a date without pull requests covers the date`() {
    val batch = pr.analysis.prepareBatch(pr.coordinator, ReviewSelectionInput())

    assertEquals(
      PrPublicationReceipt(batch.batchId, 0, 0, emptyList(), emptyList(), emptyList(), 0, 0, 0, listOf(day(YESTERDAY)), emptyList(), true),
      pr.analysis.publishValidated(pr.coordinator, batch.batchId),
    )
  }

  @Test
  fun `a published date is recorded in the coverage`() {
    val batch = pr.analysis.prepareBatch(pr.coordinator, ReviewSelectionInput())

    pr.analysis.publishValidated(pr.coordinator, batch.batchId)
    assertEquals(RepositoryPrAnalysisCoverage(KOTLIN, listOf(day(YESTERDAY))), pr.store.getPrAnalysisCoverage(pr.coordinator, KOTLIN))
  }

  @Test
  fun `a date batch fetches its whole range at once`() {
    pr.analysis.prepareBatch(pr.coordinator, ReviewSelectionInput(startDate = "2026-10-06", endDate = "2026-10-07"))

    assertEquals(listOf(ReviewSelection(1000, startDate = "2026-10-06", endDate = "2026-10-07")), pr.reviews.selections)
  }

  @Test
  fun `a date batch ignores the coverage of its dates`() {
    pr.reviews.pullRequests += pullRequestsMergedOn(YESTERDAY, 1, first = 20)
    pr.coverDates(day(YESTERDAY))

    val batch = pr.analysis.prepareBatch(pr.coordinator, ReviewSelectionInput(startDate = "2026-10-08", endDate = "2026-10-08"))
    assertEquals(listOf(listOf(20 to "20")), pr.items(batch))
  }

  @Test
  fun `a date batch ending today is refused before fetching`() {
    val error = assertFailsWith<IllegalArgumentException> {
      pr.analysis.prepareBatch(pr.coordinator, ReviewSelectionInput(startDate = "2026-10-08", endDate = "2026-10-09"))
    }

    assertEquals("Only complete UTC dates before today can be selected", error.message)
    assertEquals(emptyList(), pr.reviews.selections)
  }

  @Test
  fun `a date range of 1000 pull requests or more is fetched again in halves, keeping every pull request`() {
    pr.reviews.pullRequests += pullRequestsMergedOn(TODAY.minusDays(2), 600) + pullRequestsMergedOn(YESTERDAY, 600, first = 1001)

    val batch = pr.analysis.prepareBatch(pr.coordinator, ReviewSelectionInput(startDate = "2026-10-07", endDate = "2026-10-08"))
    assertEquals(
      listOf(
        ReviewSelection(1000, startDate = "2026-10-07", endDate = "2026-10-08"),
        ReviewSelection(1000, startDate = "2026-10-08", endDate = "2026-10-08"),
        ReviewSelection(1000, startDate = "2026-10-07", endDate = "2026-10-07"),
      ),
      pr.reviews.selections,
    )
    assertEquals(150, batch.availableChunkCount)
  }

  @Test
  fun `a date range is fetched again in halves when 1000 pull requests were found, even if one was left out`() {
    // The 1000th pull request lacks exact revisions, so only 999 come back.
    pr.reviews.pullRequests += pullRequestsMergedOn(TODAY.minusDays(2), 500) + pullRequestsMergedOn(YESTERDAY, 499, first = 501) +
      pullRequest(1000, base = "f550b76", mergedOn = YESTERDAY)

    pr.analysis.prepareBatch(pr.coordinator, ReviewSelectionInput(startDate = "2026-10-07", endDate = "2026-10-08"))

    assertEquals(
      listOf(
        ReviewSelection(1000, startDate = "2026-10-07", endDate = "2026-10-08"),
        ReviewSelection(1000, startDate = "2026-10-08", endDate = "2026-10-08"),
        ReviewSelection(1000, startDate = "2026-10-07", endDate = "2026-10-07"),
      ),
      pr.reviews.selections,
    )
  }

  @Test
  fun `a date of 1000 pull requests cannot be proven fetched completely`() {
    pr.reviews.pullRequests += pullRequestsMergedOn(YESTERDAY, 1000)

    val error = assertFailsWith<IllegalArgumentException> { pr.analysis.prepareBatch(pr.coordinator, ReviewSelectionInput()) }
    assertEquals("Cannot prove that UTC date 2026-10-08 was fetched completely: it contains at least 1000 PRs", error.message)
  }

  @Test
  fun `a number batch fetches exactly those numbers`() {
    pr.analysis.prepareBatch(pr.coordinator, ReviewSelectionInput(prNumbers = listOf(20, 21)))

    assertEquals(listOf(ReviewSelection(2, prNumbers = listOf(20, 21))), pr.reviews.selections)
  }

  @Test
  fun `a number batch ignores the coverage of its numbers`() {
    pr.reviews.pullRequests += pullRequestsMergedOn(YESTERDAY, 1, first = 20)
    pr.coverNumbers(20)

    val batch = pr.analysis.prepareBatch(pr.coordinator, ReviewSelectionInput(prNumbers = listOf(20)))
    assertEquals(listOf(listOf(20 to "20")), pr.items(batch))
  }

  @Test
  fun `a PR number given twice is refused`() {
    val error = assertFailsWith<IllegalArgumentException> {
      pr.analysis.prepareBatch(pr.coordinator, ReviewSelectionInput(prNumbers = listOf(7, 7)))
    }

    assertEquals("Require distinct positive PR numbers within maxPrs, or dates", error.message)
  }

  @Test
  fun `more than 1000 PR numbers are refused`() {
    val error = assertFailsWith<IllegalArgumentException> {
      pr.analysis.prepareBatch(pr.coordinator, ReviewSelectionInput(prNumbers = (1..1001).toList()))
    }

    assertEquals("maxPrs must be 1..1000", error.message)
  }

  @Test
  fun `a start date without an end date is refused`() {
    val error = assertFailsWith<IllegalArgumentException> {
      pr.analysis.prepareBatch(pr.coordinator, ReviewSelectionInput(startDate = "2026-10-07"))
    }

    assertEquals("Specify both startDate and endDate, or neither", error.message)
  }

  @Test
  fun `PR numbers together with dates are refused`() {
    val error = assertFailsWith<IllegalArgumentException> {
      pr.analysis.prepareBatch(pr.coordinator, ReviewSelectionInput(listOf(7), "2026-10-07", "2026-10-08"))
    }

    assertEquals("Select either PR numbers or a date range", error.message)
  }

  @Test
  fun `a start date after the end date is refused`() {
    val error = assertFailsWith<IllegalArgumentException> {
      pr.analysis.prepareBatch(pr.coordinator, ReviewSelectionInput(startDate = "2026-10-08", endDate = "2026-10-07"))
    }

    assertEquals("startDate must not follow endDate", error.message)
  }

  @Test
  fun `a batch is identified by its selection and pull requests`() {
    pr.reviews.pullRequests += CORRECTION

    assertEquals(PrBatchSummary("bdc2539b928934128fd72e23", 1), pr.analysis.prepareBatch(pr.coordinator, ReviewSelectionInput()))
  }

  @Test
  fun `another selection of the same pull request gives another batch`() {
    val byDate = pr.correctionBatch()

    assertNotEquals(byDate.batchId, pr.analysis.prepareBatch(pr.coordinator, ReviewSelectionInput(prNumbers = listOf(7))).batchId)
  }

  @Test
  fun `a work item holds its pull request without threads, and the thread, under an ID of both`() {
    val batch = pr.correctionBatch()

    assertEquals(
      PrItem("pr-7-f85d6d74f4cd0de5", KOTLIN, CORRECTION.copy(threads = emptyList()), CORRECTION.threads.single()),
      pr.item(batch),
    )
  }

  @Test
  fun `work items are chunked eight at a time`() {
    pr.reviews.pullRequests += pullRequestsMergedOn(YESTERDAY, 9)

    val batch = pr.analysis.prepareBatch(pr.coordinator, ReviewSelectionInput())
    assertEquals(listOf((9 downTo 2).map { it to "$it" }, listOf(1 to "1")), pr.items(batch))
  }

  @Test
  fun `each thread of a pull request is its own work item`() {
    pr.reviews.pullRequests += pullRequest(7, thread("a"), thread("b", start = 5, end = 6))

    val batch = pr.analysis.prepareBatch(pr.coordinator, ReviewSelectionInput())
    assertEquals(listOf(listOf(7 to "a", 7 to "b")), pr.items(batch))
  }

  @Test
  fun `a PR analysis without configured reviews is refused`() {
    val unconfigured = PrAnalysis(pr.store, null) { TODAY }

    val error = assertFailsWith<IllegalArgumentException> { unconfigured.prepareBatch(pr.coordinator, ReviewSelectionInput()) }
    assertEquals("PR extraction requires complete edict.ci configuration", error.message)
  }

  @Test
  fun `an analysis worker cannot prepare a batch, and nothing is fetched for it`() {
    val error = assertFailsWith<IllegalStateException> {
      pr.analysis.prepareBatch(pr.worker, ReviewSelectionInput(prNumbers = listOf(7)))
    }

    assertEquals("PR analysis requires a running PR-analysis task", error.message)
    assertEquals(emptyList(), pr.reviews.selections)
  }

  @Test
  fun `the coordinator cannot read a chunk, which is its workers' to read`() {
    val batch = pr.correctionBatch()

    val error = assertFailsWith<IllegalArgumentException> { pr.analysis.chunk(pr.coordinator, batch.batchId, 0) }
    assertEquals("edict-pr-signal-analysis is not allowed to perform this managed mutation", error.message)
  }

  @Test
  fun `an analysis worker of another coordinator cannot read a chunk`() {
    val batch = pr.correctionBatch()

    val error = assertFailsWith<IllegalStateException> { pr.analysis.chunk(pr.commitAnalysisWorker(), batch.batchId, 0) }
    assertEquals("PR analysis requires a running PR-analysis task or its signal-analysis worker", error.message)
  }

  @Test
  fun `an unknown chunk index is refused`() {
    val batch = pr.correctionBatch()

    val error = assertFailsWith<IllegalArgumentException> { pr.analysis.chunk(pr.worker, batch.batchId, 1) }
    assertEquals("Unknown chunk index in PR batch", error.message)
  }

  @Test
  fun `an unknown batch is refused`() {
    val error = assertFailsWith<IllegalStateException> { pr.analysis.chunk(pr.worker, "bdc2539b928934128fd72e23", 0) }

    assertEquals("Unknown PR batch; prepare again after restart", error.message)
  }

  @Test
  fun `an unknown work item is refused`() {
    val batch = pr.correctionBatch()

    val error = assertFailsWith<IllegalStateException> { pr.analysis.get(pr.worker, batch.batchId, "pr-8-f85d6d74f4cd0de5") }
    assertEquals("Unknown work item in PR batch", error.message)
  }

  @Test
  fun `a file is read at a revision of its work item`() {
    val batch = pr.correctionBatch(correctionFiles())

    assertEquals("fun a() {\n  good()\n}\n", pr.analysis.file(pr.worker, batch.batchId, pr.item(batch).workItemId, HEAD, CORRECTED_PATH))
  }

  @Test
  fun `a file at a revision outside its work item is refused`() {
    val batch = pr.correctionBatch()

    val error = assertFailsWith<IllegalArgumentException> {
      pr.analysis.file(pr.worker, batch.batchId, pr.item(batch).workItemId, OTHER_REVISION, CORRECTED_PATH)
    }
    assertEquals("Revision does not belong to work item", error.message)
  }

  @Test
  fun `the diff of a work item is the provider's diff from the base to the head`() {
    val batch = pr.correctionBatch(correctionFiles())

    // One hunk replacing bad() with good() on line 2 of src/Example.kt, with a context line either side.
    assertEquals(
      diff("correction"),
      pr.analysis.diff(pr.worker, batch.batchId, pr.item(batch).workItemId, BASE, HEAD, CORRECTED_PATH, CORRECTED_PATH),
    )
  }

  @Test
  fun `a diff that does not end at the head is refused`() {
    val batch = pr.correctionBatch()

    val error = assertFailsWith<IllegalArgumentException> {
      pr.analysis.diff(pr.worker, batch.batchId, pr.item(batch).workItemId, BASE, COMMENTED, CORRECTED_PATH, CORRECTED_PATH)
    }
    assertEquals("Diff must compare prepared base/comment revision to head", error.message)
  }

  @Test
  fun `evidence of a thread holds its file's diff from the comment to the head and the commented code`() {
    val batch = pr.correctionBatch(correctionFiles())

    // One hunk of src/Example.kt replacing the bad() and log() lines with a single good() line.
    assertEquals(
      PrItemEvidence(
        CORRECTED_PATH, COMMENTED, HEAD, diff("correction-from-comment"), "    1| fun a() {\n    2|   bad()\n    3|   log()\n    4| }",
      ),
      pr.analysis.evidence(pr.item(batch)),
    )
  }

  @Test
  fun `evidence of a thread on the whole file has the whole diff and no commented code`() {
    pr.reviews.pullRequests += pullRequest(7, thread("r1", start = null, end = null))
    pr.reviews.files += correctionFiles()
    val batch = pr.analysis.prepareBatch(pr.coordinator, ReviewSelectionInput())

    // One hunk replacing the bad() and log() lines with a single good() line.
    assertEquals(PrItemEvidence(CORRECTED_PATH, COMMENTED, HEAD, diff("correction-from-comment")), pr.analysis.evidence(pr.item(batch)))
  }

  @Test
  fun `evidence of a large diff holds only the hunks near the comment, saying so`() {
    pr.reviews.pullRequests += correctionAnchoredAt(100)
    pr.reviews.files += largeDiffFiles()
    val batch = pr.analysis.prepareBatch(pr.coordinator, ReviewSelectionInput())

    val evidence = pr.analysis.evidence(pr.item(batch))
    // A 41-line hunk around the change of bad() to good() at line 100: 20 context lines either side.
    assertEquals(diff("large-evidence-near-100"), evidence.diff)
    assertEquals(listOf("the diff shows only changes near the comment; edict_pr_file_diff has the whole file"), evidence.notes)
  }

  /** Pins today's behaviour; see the report: the evidence then shows bare file headers rather than "no changes". */
  @Test
  fun `evidence of a diff without a hunk near the comment holds only the file headers`() {
    val batch = pr.correctionBatch(largeDiffFiles())

    val evidence = pr.analysis.evidence(pr.item(batch))
    assertEquals("--- a/src/Example.kt\n+++ b/src/Example.kt\n", evidence.diff)
    assertEquals(listOf("the diff shows only changes near the comment; edict_pr_file_diff has the whole file"), evidence.notes)
  }

  @Test
  fun `evidence of a diff over 600 lines holds its first 600 and says the rest is left out`() {
    val batch = pr.correctionBatch(longDiffFiles())

    val evidence = pr.analysis.evidence(pr.item(batch))
    assertEquals(LONG_DIFF_FIRST_600_LINES, evidence.diff)
    assertEquals(emptyList(), evidence.notes)
  }

  @Test
  fun `evidence of a file unchanged since the comment says there are no changes near it`() {
    val batch = pr.correctionBatch(correctionFiles() + ((HEAD to CORRECTED_PATH) to COMMENTED_SOURCE))

    assertEquals(
      PrItemEvidence(CORRECTED_PATH, COMMENTED, HEAD, NO_CHANGES, "    1| fun a() {\n    2|   bad()\n    3|   log()\n    4| }"),
      pr.analysis.evidence(pr.item(batch)),
    )
  }

  @Test
  fun `evidence of a file the provider does not have says why each part is unavailable`() {
    val batch = pr.correctionBatch(OTHER_FILES_ONLY)

    assertEquals(
      PrItemEvidence(
        CORRECTED_PATH, COMMENTED, HEAD, NO_CHANGES, null,
        listOf("diff unavailable: File absent at both revisions", "commented code unavailable: src/Example.kt"),
      ),
      pr.analysis.evidence(pr.item(batch)),
    )
  }

  @Test
  fun `an empty chunk stores every work item as successful`() {
    val batch = pr.correctionBatch()

    assertEquals(PrReceipt(batch.batchId, 0, 1, 1, 0, 0, emptyList()), pr.storeEmptyChunk(batch))
  }

  @Test
  fun `a chunk that lists its work items out of order is refused`() {
    pr.reviews.pullRequests += pullRequest(7, thread("a"), thread("b", start = 5, end = 6))
    val batch = pr.analysis.prepareBatch(pr.coordinator, ReviewSelectionInput())
    val reversed = pr.analysis.chunk(pr.worker, batch.batchId, 0).workItemIds.reversed()

    val error = assertFailsWith<IllegalArgumentException> {
      pr.analysis.store(pr.worker, batch.batchId, 0, reversed, emptyList(), emptyList(), emptyList())
    }
    assertEquals("Incomplete or unordered PR chunk coverage", error.message)
  }

  @Test
  fun `a chunk of an unknown index is refused`() {
    val batch = pr.correctionBatch()

    val error = assertFailsWith<IllegalArgumentException> {
      pr.analysis.store(pr.worker, batch.batchId, 1, emptyList(), emptyList(), emptyList(), emptyList())
    }
    assertEquals("Unknown chunk index in PR batch", error.message)
  }

  @Test
  fun `a failed work item outside the chunk is refused`() {
    val batch = pr.correctionBatch()
    val item = pr.item(batch).workItemId

    val error = assertFailsWith<IllegalArgumentException> {
      pr.analysis.store(pr.worker, batch.batchId, 0, listOf(item), listOf("pr-8-f85d6d74f4cd0de5"), emptyList(), listOf(TIMEOUT))
    }
    assertEquals("Failed work-item IDs must be distinct members of the inspected chunk", error.message)
  }

  @Test
  fun `a failed work item listed twice is refused`() {
    val batch = pr.correctionBatch()
    val item = pr.item(batch).workItemId

    val error = assertFailsWith<IllegalArgumentException> {
      pr.analysis.store(pr.worker, batch.batchId, 0, listOf(item), listOf(item, item), emptyList(), listOf(TIMEOUT))
    }
    assertEquals("Failed work-item IDs must be distinct members of the inspected chunk", error.message)
  }

  @Test
  fun `a failed work item without a problem is refused`() {
    val batch = pr.correctionBatch()
    val item = pr.item(batch).workItemId

    val error = assertFailsWith<IllegalArgumentException> {
      pr.analysis.store(pr.worker, batch.batchId, 0, listOf(item), listOf(item), emptyList(), emptyList())
    }
    assertEquals("Every failed work item requires a problem, and problems require failed work-item IDs", error.message)
  }

  @Test
  fun `a problem holding a capability token is refused`() {
    val batch = pr.correctionBatch()
    val item = pr.item(batch).workItemId

    val error = assertFailsWith<IllegalArgumentException> {
      pr.analysis.store(pr.worker, batch.batchId, 0, listOf(item), listOf(item), emptyList(), listOf("Failed as ${pr.worker}"))
    }
    assertEquals("Persisted content must not contain capability tokens", error.message)
  }

  @Test
  fun `a failed work item with its problem is stored as failed`() {
    val batch = pr.correctionBatch()
    val item = pr.item(batch).workItemId

    assertEquals(
      PrReceipt(batch.batchId, 0, 1, 0, 1, 0, emptyList(), problems = listOf(TIMEOUT)),
      pr.analysis.store(pr.worker, batch.batchId, 0, listOf(item), listOf(item), emptyList(), listOf(TIMEOUT)),
    )
  }

  @Test
  fun `storing a chunk after publication is refused`() {
    val batch = pr.correctionBatch()
    pr.publishEmpty(batch)

    val error = assertFailsWith<IllegalStateException> { pr.storeEmptyChunk(batch) }
    assertEquals("Cannot store a PR chunk after batch publication", error.message)
  }

  @Test
  fun `storing a chunk again replaces its earlier result`() {
    val batch = pr.correctionBatch()
    val item = pr.item(batch).workItemId
    pr.analysis.store(pr.worker, batch.batchId, 0, listOf(item), listOf(item), emptyList(), listOf(TIMEOUT))

    assertEquals(
      PrPublicationReceipt(batch.batchId, 1, 1, emptyList(), emptyList(), emptyList(), 1, 1, 0, listOf(day(YESTERDAY)), emptyList(), true),
      pr.publishEmpty(batch),
    )
  }

  @Test
  fun `a draft is stored as a Signal keyed by its work item, ordinal, label, path, revision and lines`() {
    val batch = pr.correctionBatch(correctionFiles())
    val item = pr.item(batch)

    assertEquals(
      PrReceipt(batch.batchId, 0, 1, 1, 0, 1, listOf(stableSignalId("${item.workItemId}|0|POSITIVE|src/Example.kt|$BASE|2:2"))),
      pr.storeDrafts(batch, draft(item, POSITIVE)),
    )
  }

  @Test
  fun `a published draft is a FromPR Signal completed from its work item`() {
    val batch = pr.correctionBatch(correctionFiles())
    val item = pr.item(batch)
    val key = "${item.workItemId}|0|POSITIVE|src/Example.kt|$BASE|2:2"
    pr.storeDrafts(batch, draft(item, POSITIVE))

    pr.analysis.publishValidated(pr.coordinator, batch.batchId)
    // One hunk replacing bad() with good() on line 2.
    assertEquals(prSignal(item, batch.batchId, POSITIVE, key, diff("correction")), pr.store.inboxSignal(stableSignalId(key)))
  }

  @Test
  fun `drafts of one work item on the same lines are told apart by their ordinal`() {
    val batch = pr.correctionBatch(correctionFiles())
    val item = pr.item(batch)

    assertEquals(
      listOf(
        stableSignalId("${item.workItemId}|0|POSITIVE|src/Example.kt|$BASE|2:2"),
        stableSignalId("${item.workItemId}|1|POSITIVE|src/Example.kt|$BASE|2:2"),
      ),
      pr.storeDrafts(batch, draft(item, POSITIVE), draft(item, POSITIVE)).signalIds,
    )
  }

  @Test
  fun `a negative draft is stored at the head`() {
    val batch = pr.correctionBatch(correctionFiles())
    val item = pr.item(batch)

    assertEquals(
      listOf(stableSignalId("${item.workItemId}|0|NEGATIVE|src/Example.kt|$HEAD|2:2")),
      pr.storeDrafts(batch, draft(item, NEGATIVE)).signalIds,
    )
  }

  @Test
  fun `the drafts of one correction are recorded as a pair and other numbered drafts stay separate`() {
    val batch = pr.correctionBatch(correctionFiles())
    val item = pr.item(batch)

    val receipt = pr.storeDrafts(
      batch,
      draft(item, POSITIVE, correction = 0),
      draft(item, NEGATIVE, correction = 0),
      draft(item, POSITIVE, correction = 1),
      draft(item, NEGATIVE),
    )

    val (positive, negative, lonePositive, loneNegative) = receipt.signalIds
    assertEquals(negative, pr.pairs.sibling(positive))
    assertEquals(positive, pr.pairs.sibling(negative))
    assertNull(pr.pairs.sibling(lonePositive))
    assertNull(pr.pairs.sibling(loneNegative))
    assertEquals(listOf("Correction 1 of ${item.workItemId}: 1 draft(s) share it"), receipt.unpairedCorrections)
  }

  @Test
  fun `a positive draft diffed from the comment revision is stored at it`() {
    val batch = pr.correctionBatch(correctionFiles())
    val item = pr.item(batch)

    assertEquals(
      listOf(stableSignalId("${item.workItemId}|0|POSITIVE|src/Example.kt|$COMMENTED|2:2")),
      pr.storeDrafts(batch, draft(item, POSITIVE, before = COMMENTED)).signalIds,
    )
  }

  @Test
  fun `a draft diffed from a revision outside its work item is refused`() {
    val batch = pr.correctionBatch()
    val item = pr.item(batch)
    val key = "${item.workItemId}|0|POSITIVE|src/Example.kt|$OTHER_REVISION|2:2"

    assertEquals(
      PrReceipt(
        batch.batchId, 0, 1, 0, 1, 0, emptyList(),
        listOf(PrSignalStoreFailure(0, stableSignalId(key), item.workItemId, "Diff must compare prepared base/comment revision to head")),
      ),
      pr.storeDrafts(batch, draft(item, POSITIVE, before = OTHER_REVISION)),
    )
  }

  @Test
  fun `a weak draft is refused before publication, which takes only strong Signals`() {
    val batch = pr.correctionBatch(correctionFiles())
    val item = pr.item(batch)

    assertEquals(
      listOf(
        PrSignalStoreFailure(
          0, stableSignalId("${item.workItemId}|0|POSITIVE|src/Example.kt|$BASE|2:2"), item.workItemId,
          "Only STRONG Signals can be published to the inbox",
        ),
      ),
      pr.storeDrafts(batch, draft(item, POSITIVE).copy(strength = EdictSignalStrength.WEAK)).failures,
    )
  }

  @Test
  fun `a positive draft on the new path of a moved file is refused, as it belongs to the old one`() {
    val batch = pr.correctionBatch()
    val item = pr.item(batch)

    assertEquals(
      listOf(
        PrSignalStoreFailure(
          0, stableSignalId("${item.workItemId}|0|POSITIVE|src/Example.kt|$BASE|2:2"), item.workItemId,
          "Signal path must match its diff side",
        ),
      ),
      pr.storeDrafts(batch, draft(item, POSITIVE, beforePath = "src/Old.kt")).failures,
    )
  }

  @Test
  fun `a positive draft at the head is refused as on the wrong side`() {
    val batch = pr.correctionBatch(correctionFiles())
    val item = pr.item(batch)

    assertEquals(
      listOf(
        PrSignalStoreFailure(
          0, stableSignalId("${item.workItemId}|0|POSITIVE|src/Example.kt|$HEAD|2:2"), item.workItemId,
          "Signal revision must match its diff side",
        ),
      ),
      pr.storeDrafts(batch, draft(item, POSITIVE, at = HEAD)).failures,
    )
  }

  @Test
  fun `a positive draft at the comment revision but diffed from the base is refused`() {
    // Both revisions belong to the work item, but its lines would be checked in the base's file.
    val batch = pr.correctionBatch(correctionFiles())
    val item = pr.item(batch)

    assertEquals(
      listOf(
        PrSignalStoreFailure(
          0, stableSignalId("${item.workItemId}|0|POSITIVE|src/Example.kt|$COMMENTED|2:2"), item.workItemId,
          "Signal revision must match its diff side",
        ),
      ),
      pr.storeDrafts(batch, draft(item, POSITIVE, at = COMMENTED)).failures,
    )
  }

  @Test
  fun `a draft on a large diff keeps only the changes near its lines`() {
    val batch = pr.correctionBatch(largeDiffFiles())
    val item = pr.item(batch)
    val id = pr.storeDrafts(batch, draft(item, POSITIVE, EdictLineRange(100, 100))).signalIds.single()

    pr.analysis.publishValidated(pr.coordinator, batch.batchId)
    // The hunk around line 100 replacing bad() with good(), with three context lines either side.
    assertEquals(diff("large-draft-excerpt-100"), (pr.store.inboxSignal(id)?.source as EdictSignalSource.FromPR).diffPositiveToNegative)
  }

  @Test
  fun `a draft on a large diff also keeps the related lines it names`() {
    val batch = pr.correctionBatch(largeDiffFiles())
    val item = pr.item(batch)
    val drafted = draft(item, POSITIVE, EdictLineRange(99, 100), relatedAfter = listOf(EdictLineRange(210, 210)))
    val id = pr.storeDrafts(batch, drafted).signalIds.single()

    pr.analysis.publishValidated(pr.coordinator, batch.batchId)
    val excerpt = (pr.store.inboxSignal(id)?.source as EdictSignalSource.FromPR).diffPositiveToNegative
    assertContains(excerpt, "+added 210 ")
    assertFalse("+added 230 " in excerpt)
  }

  @Test
  fun `a draft whose changes cannot fit 4000 characters is refused`() {
    val batch = pr.correctionBatch(largeDiffFiles())
    val item = pr.item(batch)

    assertEquals(
      listOf(
        PrSignalStoreFailure(
          0,
          stableSignalId("${item.workItemId}|0|POSITIVE|src/Example.kt|$BASE|200:250"),
          item.workItemId,
          "The changes at the Signal's lines do not fit 4000 characters; narrow its ranges",
        ),
      ),
      pr.storeDrafts(batch, draft(item, POSITIVE, EdictLineRange(200, 250))).failures,
    )
  }

  @Test
  fun `supplied Signals are validated one by one, keeping the valid ones`() {
    val batch = pr.correctionBatch()
    val item = pr.item(batch)
    val positive = prSignal(item, batch.batchId, POSITIVE)
    val negative = prSignal(item, batch.batchId, NEGATIVE)
    val invalid = prSignal(item, batch.batchId, POSITIVE, key = "undescribed", description = "")

    assertEquals(
      PrReceipt(
        batch.batchId, 0, 1, 0, 1, 2, listOf(positive.id, negative.id),
        listOf(
          PrSignalStoreFailure(
            2, invalid.id, item.workItemId,
            "Invalid signal 'inbox/${invalid.id}.json': description: must be one concise line of at most 1000 characters",
          ),
        ),
      ),
      pr.storeSignals(batch, positive, negative, invalid),
    )
  }

  @Test
  fun `a supplied Signal given twice is kept once`() {
    val batch = pr.correctionBatch()
    val item = pr.item(batch)
    val signal = prSignal(item, batch.batchId, POSITIVE)

    assertEquals(
      PrReceipt(
        batch.batchId, 0, 1, 0, 1, 1, listOf(signal.id), listOf(PrSignalStoreFailure(1, signal.id, item.workItemId, "Duplicate signal")),
      ),
      pr.storeSignals(batch, signal, signal),
    )
  }

  @Test
  fun `a supplied Signal of a work item outside the chunk is refused without failing the chunk's item`() {
    val batch = pr.correctionBatch()
    val signal = prSignal(pr.item(batch), batch.batchId, POSITIVE, workItemId = "pr-8-f85d6d74f4cd0de5")

    assertEquals(
      PrReceipt(
        batch.batchId, 0, 1, 1, 0, 0, emptyList(),
        listOf(PrSignalStoreFailure(0, signal.id, "pr-8-f85d6d74f4cd0de5", "Signal work item does not belong to this chunk")),
      ),
      pr.storeSignals(batch, signal),
    )
  }

  @Test
  fun `a supplied Signal of another batch is refused`() {
    val batch = pr.correctionBatch()
    val item = pr.item(batch)
    val signal = prSignal(item, "000000000000000000000000", POSITIVE)

    assertEquals(
      listOf(PrSignalStoreFailure(0, signal.id, item.workItemId, "Signal provenance must preserve the analysis batch")),
      pr.storeSignals(batch, signal).failures,
    )
  }

  @Test
  fun `a supplied Signal with another title than its pull request's is refused`() {
    val batch = pr.correctionBatch()
    val item = pr.item(batch)
    val signal = prSignal(item, batch.batchId, POSITIVE, title = "KT-7 Another title")

    assertEquals(
      listOf(
        PrSignalStoreFailure(0, signal.id, item.workItemId, "Signal source must preserve prepared PR number, title and discussion URL"),
      ),
      pr.storeSignals(batch, signal).failures,
    )
  }

  @Test
  fun `a supplied Signal with other discussion messages than its thread's is refused`() {
    val batch = pr.correctionBatch()
    val item = pr.item(batch)
    val signal = prSignal(item, batch.batchId, POSITIVE, messages = listOf(REVIEWER_MESSAGE, "Done"))

    assertEquals(
      listOf(PrSignalStoreFailure(0, signal.id, item.workItemId, "Preserve all prepared discussion messages in order")),
      pr.storeSignals(batch, signal).failures,
    )
  }

  @Test
  fun `a supplied negative Signal at the base is refused as on the wrong side`() {
    val batch = pr.correctionBatch()
    val item = pr.item(batch)
    val signal = prSignal(item, batch.batchId, NEGATIVE, at = BASE)

    assertEquals(
      listOf(PrSignalStoreFailure(0, signal.id, item.workItemId, "Evidence revision does not match its PR side")),
      pr.storeSignals(batch, signal).failures,
    )
  }

  @Test
  fun `a supplied Signal over 8 MiB is refused`() {
    val batch = pr.correctionBatch()
    val item = pr.item(batch)
    val signal = prSignal(item, batch.batchId, POSITIVE, diff = "x".repeat(8 * 1024 * 1024))

    assertEquals(
      listOf(PrSignalStoreFailure(0, signal.id, item.workItemId, "Signal exceeds 8 MiB")),
      pr.storeSignals(batch, signal).failures,
    )
  }

  @Test
  fun `a date batch whose work items all succeed covers its date`() {
    val batch = pr.correctionBatch()

    assertEquals(
      PrPublicationReceipt(batch.batchId, 1, 1, emptyList(), emptyList(), emptyList(), 1, 1, 0, listOf(day(YESTERDAY)), emptyList(), true),
      pr.publishEmpty(batch),
    )
  }

  @Test
  fun `a date batch with most of its work items successful covers its date despite a chunk not stored`() {
    pr.reviews.pullRequests += pullRequestsMergedOn(YESTERDAY, 9)
    val batch = pr.analysis.prepareBatch(pr.coordinator, ReviewSelectionInput())
    pr.storeEmptyChunk(batch, 0)

    assertEquals(
      PrPublicationReceipt(
        batch.batchId, 2, 1, listOf(1), emptyList(), emptyList(), 9, 8, 1, listOf(day(YESTERDAY)), emptyList(), true,
        listOf("Chunk 1 did not store a result"),
      ),
      pr.analysis.publishValidated(pr.coordinator, batch.batchId),
    )
  }

  @Test
  fun `a date batch with exactly half of its work items successful covers nothing`() {
    pr.reviews.pullRequests += pullRequestsMergedOn(YESTERDAY, 16)
    val batch = pr.analysis.prepareBatch(pr.coordinator, ReviewSelectionInput())
    pr.storeEmptyChunk(batch, 0)

    assertEquals(
      PrPublicationReceipt(
        batch.batchId, 2, 1, listOf(1), emptyList(), emptyList(), 16, 8, 8, listOf(day(YESTERDAY)), emptyList(), false,
        listOf("Chunk 1 did not store a result"),
      ),
      pr.analysis.publishValidated(pr.coordinator, batch.batchId),
    )
  }

  @Test
  fun `a date batch without work items covers nothing when the provider reported a problem`() {
    pr.reviews.problems += "Space review discovery page 1 data[3]: expected object"
    val batch = pr.analysis.prepareBatch(pr.coordinator, ReviewSelectionInput())

    assertEquals(
      PrPublicationReceipt(
        batch.batchId, 0, 0, emptyList(), emptyList(), emptyList(), 0, 0, 0, listOf(day(YESTERDAY)), emptyList(), false,
        listOf("Space review discovery page 1 data[3]: expected object"),
      ),
      pr.analysis.publishValidated(pr.coordinator, batch.batchId),
    )
  }

  @Test
  fun `a number batch whose work items all succeed covers its numbers`() {
    pr.reviews.pullRequests += pullRequestsMergedOn(YESTERDAY, 2, first = 20)
    val batch = pr.analysis.prepareBatch(pr.coordinator, ReviewSelectionInput(prNumbers = listOf(20, 21)))

    assertEquals(
      PrPublicationReceipt(batch.batchId, 1, 1, emptyList(), emptyList(), emptyList(), 2, 2, 0, emptyList(), listOf(20, 21), true),
      pr.publishEmpty(batch),
    )
  }

  @Test
  fun `published numbers are recorded in the coverage`() {
    pr.reviews.pullRequests += pullRequestsMergedOn(YESTERDAY, 2, first = 20)

    pr.publishEmpty(pr.analysis.prepareBatch(pr.coordinator, ReviewSelectionInput(prNumbers = listOf(20, 21))))
    assertEquals(
      RepositoryPrAnalysisCoverage(KOTLIN, analyzedPrNumbers = listOf(20, 21)),
      pr.store.getPrAnalysisCoverage(pr.coordinator, KOTLIN),
    )
  }

  @Test
  fun `a number batch with a failed work item covers nothing`() {
    pr.reviews.pullRequests += CORRECTION
    val batch = pr.analysis.prepareBatch(pr.coordinator, ReviewSelectionInput(prNumbers = listOf(7)))
    val item = pr.item(batch).workItemId
    pr.analysis.store(pr.worker, batch.batchId, 0, listOf(item), listOf(item), emptyList(), listOf(TIMEOUT))

    assertEquals(
      PrPublicationReceipt(
        batch.batchId, 1, 1, emptyList(), emptyList(), emptyList(), 1, 0, 1, emptyList(), listOf(7), false, listOf(TIMEOUT),
      ),
      pr.analysis.publishValidated(pr.coordinator, batch.batchId),
    )
  }

  @Test
  fun `a number batch covers nothing when a pull request lacks exact revisions`() {
    pr.reviews.pullRequests += listOf(pullRequest(7, thread("r1"), head = "")) + pullRequestsMergedOn(YESTERDAY, 1, first = 20)
    val batch = pr.analysis.prepareBatch(pr.coordinator, ReviewSelectionInput(prNumbers = listOf(7, 20)))

    assertEquals(
      PrPublicationReceipt(
        batch.batchId, 1, 1, emptyList(), emptyList(), emptyList(), 1, 1, 0, emptyList(), listOf(20), false,
        listOf("PR 7: lacks exact base/head revisions"),
      ),
      pr.publishEmpty(batch),
    )
  }

  @Test
  fun `a publication lists the provider's problems, then each chunk's problems and failures, then the chunks not stored`() {
    pr.reviews.pullRequests += pullRequestsMergedOn(YESTERDAY, 9) + pullRequest(30, thread("30", path = "../Outside.kt", pullRequest = 30))
    val batch = pr.analysis.prepareBatch(pr.coordinator, ReviewSelectionInput())
    val ids = pr.analysis.chunk(pr.worker, batch.batchId, 0).workItemIds
    val failing = draft(pr.item(batch, 0, 1), POSITIVE, before = OTHER_REVISION)
    pr.analysis.store(pr.worker, batch.batchId, 0, ids, listOf(ids[0]), emptyList(), listOf(TIMEOUT), listOf(failing))

    assertEquals(
      listOf(
        "PR 30 discussion 30: invalid path, revision or anchors",
        TIMEOUT,
        "${ids[1]}: Diff must compare prepared base/comment revision to head",
        "Chunk 1 did not store a result",
      ),
      pr.analysis.publishValidated(pr.coordinator, batch.batchId).problems,
    )
  }

  @Test
  fun `publishing a batch creates its Signals, and publishing it again creates none`() {
    val batch = pr.correctionBatch(correctionFiles())
    val id = pr.storeDrafts(batch, draft(pr.item(batch), POSITIVE)).signalIds.single()

    assertEquals(listOf(id), pr.analysis.publishValidated(pr.coordinator, batch.batchId).createdSignalIds)
    assertEquals(
      PrPublicationReceipt(batch.batchId, 1, 1, emptyList(), listOf(id), emptyList(), 1, 1, 0, listOf(day(YESTERDAY)), emptyList(), true),
      pr.analysis.publishValidated(pr.coordinator, batch.batchId),
    )
  }

  @Test
  fun `a published Signal replaces an earlier version in the inbox without counting as created`() {
    val batch = pr.correctionBatch(correctionFiles())
    val item = pr.item(batch)
    // One hunk replacing bad() with good() on line 2.
    val signal = prSignal(item, batch.batchId, POSITIVE, "${item.workItemId}|0|POSITIVE|src/Example.kt|$BASE|2:2", diff("correction"))
    pr.store.publishSignal(pr.coordinator, signal.copy(description = "Calls bad()"))
    pr.storeDrafts(batch, draft(item, POSITIVE))

    assertEquals(emptyList(), pr.analysis.publishValidated(pr.coordinator, batch.batchId).createdSignalIds)
    assertEquals(signal, pr.store.inboxSignal(signal.id))
  }

  @Test
  fun `a Signal ID stored in two chunks is refused at publication`() {
    pr.reviews.pullRequests += pullRequestsMergedOn(YESTERDAY, 9)
    val batch = pr.analysis.prepareBatch(pr.coordinator, ReviewSelectionInput())
    pr.storeSignals(batch, prSignal(pr.item(batch, 0), batch.batchId, POSITIVE, key = "shared"), chunkIndex = 0)
    pr.storeSignals(batch, prSignal(pr.item(batch, 1), batch.batchId, POSITIVE, key = "shared"), chunkIndex = 1)

    val error = assertFailsWith<IllegalArgumentException> { pr.analysis.publishValidated(pr.coordinator, batch.batchId) }
    assertEquals("Duplicate Signal IDs across stored PR chunks", error.message)
  }

  @Test
  fun `completing before any batch is refused`() {
    val error = assertFailsWith<IllegalArgumentException> { pr.analysis.complete() }

    assertEquals("Prepare and inspect requested PR selection before finishing", error.message)
  }

  @Test
  fun `completing with an unpublished batch is refused`() {
    pr.analysis.prepareBatch(pr.coordinator, ReviewSelectionInput())

    val error = assertFailsWith<IllegalArgumentException> { pr.analysis.complete() }
    assertEquals("Publish stored PR chunks before finishing", error.message)
  }

  @Test
  fun `completing after a published Signal changed in the inbox is refused`() {
    val batch = pr.correctionBatch(correctionFiles())
    val item = pr.item(batch)
    pr.storeDrafts(batch, draft(item, POSITIVE))
    pr.analysis.publishValidated(pr.coordinator, batch.batchId)
    val key = "${item.workItemId}|0|POSITIVE|src/Example.kt|$BASE|2:2"
    // One hunk replacing bad() with good() on line 2.
    pr.store.publishSignal(pr.coordinator, prSignal(item, batch.batchId, POSITIVE, key, diff("correction"), description = "Calls bad()"))

    val error = assertFailsWith<IllegalArgumentException> { pr.analysis.complete() }
    assertEquals("Stored PR Signal is missing or differs from receipt", error.message)
  }

  @Test
  fun `fetching a batch through the tool refuses a repository named in the call`() {
    val result = pr.tools.call("edict_fetch_pr_batch", buildJsonObject {
      put("token", pr.coordinator)
      put("provider", "space")
      put("owner", "attacker")
      put("repo", "other")
      put("prNumbers", JsonArray(listOf(JsonPrimitive(7))))
    })

    assertEquals(toolError("Unknown tool argument"), result)
    assertEquals(emptyList(), pr.reviews.selections)
  }

  @Test
  fun `the coordinator cannot publish a Signal directly`() {
    val batch = pr.correctionBatch()
    val signal = wireJson.encodeToJsonElement(prSignal(pr.item(batch), batch.batchId, POSITIVE))

    assertEquals(
      toolError("edict-pr-signal-analysis is not allowed to perform this managed mutation"),
      pr.tools.call("edict_publish_signal", buildJsonObject { put("token", pr.coordinator); put("signal", signal) }),
    )
  }

  @Test
  fun `an analysis worker cannot publish a batch through the tool`() {
    assertEquals(
      toolError("PR analysis requires a running PR-analysis task"),
      pr.tools.call("edict_publish_validated_pr_signals", buildJsonObject {
        put("token", pr.worker)
        put("batchId", "bdc2539b928934128fd72e23")
      }),
    )
  }

  @Test
  fun `the coordinator completes despite a failed worker when half of the chunks were stored`() {
    pr.reviews.pullRequests += pullRequestsMergedOn(YESTERDAY, 9)
    pr.publishThroughTools(0)

    val result = pr.tools.call("edict_task_finish", buildJsonObject {
      put("token", pr.coordinator)
      put("status", "completed")
      put("result", "Published one of two chunks")
    })
    assertEquals(JsonPrimitive(false), result["isError"])
    assertEquals(
      listOf(PR_ANALYSIS to "completed", SIGNAL_ANALYSIS to "completed", SIGNAL_ANALYSIS to "failed"),
      pr.store.plan()?.tasks?.map { it.skill to it.status },
    )
  }

  @Test
  fun `the coordinator cannot complete when more than half of the chunks failed`() {
    pr.reviews.pullRequests += pullRequestsMergedOn(YESTERDAY, 9)
    pr.publishThroughTools()

    assertEquals(
      toolError("More than half of PR analysis chunks failed after retry: 2 of 2"),
      pr.tools.call("edict_task_finish", buildJsonObject {
        put("token", pr.coordinator)
        put("status", "completed")
        put("result", "Published no chunk")
      }),
    )
  }
}

/** A full revision SHA that belongs to no fixture pull request. */
private const val OTHER_REVISION = "cccccccccccccccccccccccccccccccccccccccc"

/** A worker's problem with a failed work item. */
private const val TIMEOUT = "Timed out reading the discussion"

/** The evidence diff of a file without changes near the comment. */
private const val NO_CHANGES = "(no changes to this file near the comment)"
