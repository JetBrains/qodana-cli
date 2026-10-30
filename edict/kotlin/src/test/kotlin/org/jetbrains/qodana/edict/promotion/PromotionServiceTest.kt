package org.jetbrains.qodana.edict.promotion

import kotlinx.serialization.encodeToString
import org.jetbrains.qodana.edict.ci.CiProviderId
import org.jetbrains.qodana.edict.ci.CreateReviewRequest
import org.jetbrains.qodana.edict.ci.CreatedReview
import org.jetbrains.qodana.edict.ci.ReviewManagementApi
import org.jetbrains.qodana.edict.ci.ReviewReference
import org.jetbrains.qodana.edict.ci.ReviewRepository
import org.jetbrains.qodana.edict.ci.ReviewState
import org.jetbrains.qodana.edict.ci.ReviewStatus
import org.jetbrains.qodana.edict.common.EdictCIConfiguration
import org.jetbrains.qodana.edict.common.EdictConfiguration
import org.jetbrains.qodana.edict.common.PromotionConfiguration
import org.jetbrains.qodana.edict.edictnext.EdictNextClusterManifest
import org.jetbrains.qodana.edict.edictnext.EdictNextClusterStatus
import org.jetbrains.qodana.edict.edictnext.EdictNextFileRevision
import org.jetbrains.qodana.edict.edictnext.EdictNextEvaluation
import org.jetbrains.qodana.edict.edictnext.EdictNextInspectionAction
import org.jetbrains.qodana.edict.edictnext.exampleSetDigest
import org.jetbrains.qodana.edict.edictnext.EdictNextJson
import org.jetbrains.qodana.edict.edictnext.EdictNextLanguage
import org.jetbrains.qodana.edict.edictnext.EdictNextLineRange
import org.jetbrains.qodana.edict.edictnext.EdictNextRepositoryState
import org.jetbrains.qodana.edict.edictnext.EdictNextSignal
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalLabel
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalSource
import org.jetbrains.qodana.edict.edictnext.EdictNextSignalStrength
import org.jetbrains.qodana.edict.edictnext.EdictRepository
import org.jetbrains.qodana.edict.edictnext.EdictRepositoryDirectory
import org.jetbrains.qodana.edict.edictnext.InspectionPromotion
import org.jetbrains.qodana.edict.edictnext.PromotionPrState
import org.jetbrains.qodana.edict.edictnext.sha256Hex
import org.jetbrains.qodana.edict.support.launch
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import java.time.Clock
import java.time.Instant
import java.time.ZoneOffset
import kotlin.io.path.createDirectories
import kotlin.io.path.exists
import kotlin.io.path.readText
import kotlin.io.path.writeText
import kotlin.test.Test
import kotlin.test.assertContains
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue
import kotlin.test.assertFailsWith

class PromotionServiceTest {
  @TempDir
  lateinit var temporary: Path

  private val ci = EdictCIConfiguration("https://github.com/JetBrains/qodana-cli")
  private val configuration = PromotionConfiguration("reviewer", "main")
  private val clock = Clock.fixed(Instant.parse("2026-10-04T12:00:00Z"), ZoneOffset.UTC)

  @Test
  fun `promotes only eligible generated clusters`() {
    val repository = repository()
    cluster(repository, "six-strong", 6)
    cluster(repository, "five-strong", 5)
    cluster(repository, "four-strong", 4)
    cluster(repository, "four-positive-one-negative", 5)
    replaceSignal(repository, "four-positive-one-negative", "signal-4") {
      it.copy(label = EdictNextSignalLabel.NEGATIVE)
    }
    cluster(repository, "six-negative", 6)
    repeat(6) { index ->
      replaceSignal(repository, "six-negative", "signal-$index") { it.copy(label = EdictNextSignalLabel.NEGATIVE) }
    }
    cluster(repository, "six-weak", 0, weakSignals = 6)
    cluster(repository, "pending-signals", 6, EdictNextClusterStatus.Pending)
    val processed = repository.loadCluster("five-strong")
    processed.directory.manifestPath.writeText(EdictNextJson.encodeToString(
      processed.manifest.copy(processedSignalIds = processed.signalIds),
    ))
    val provider = FakeProvider()

    val result = service(repository, provider).promoteSelected()

    assertEquals(
      setOf("five-strong", "six-strong"),
      result.created.map { it.targetPath.substringAfterLast('/').removeSuffix(".inspection.kts") }.toSet(),
    )
    assertEquals(2, provider.ensureCalls)
    assertTrue(result.failures.isEmpty())
    for (id in listOf("five-strong", "six-strong")) {
      assertEquals(PromotionPrState.ON_REVIEW, repository.loadCluster(id).manifest.promotions.single().state)
    }
    assertEquals(processed.signalIds, repository.loadCluster("five-strong").manifest.processedSignalIds)
  }

  @Test
  fun `PR description includes only referenced strong positive evidence`() {
    val repository = repository()
    cluster(repository, "documented-rule", 7, weakSignals = 1)
    replaceSignal(repository, "documented-rule", "signal-0") {
      it.copy(description = "Avoid blocking the worker thread")
    }
    val commit = "a".repeat(40)
    replaceSignal(repository, "documented-rule", "signal-4") {
      it.copy(
        source = EdictNextSignalSource.FromCommit(commit, "https://github.com/source/project/commit/$commit"),
        label = EdictNextSignalLabel.NEGATIVE,
        description = "Use the completion latch",
        fileRevision = EdictNextFileRevision("Worker.kt", commit, listOf(EdictNextLineRange(12, 15))),
      )
    }
    replaceSignal(repository, "documented-rule", "signal-5") {
      it.copy(source = EdictNextSignalSource.Generated(), description = "Unreferenced generated evidence")
    }
    replaceSignal(repository, "documented-rule", "signal-6") {
      it.copy(
        source = EdictNextSignalSource.FromCommit(commit, "https://github.com/source/project/commit/$commit"),
        description = "Avoid spinning until completion",
        fileRevision = EdictNextFileRevision("SpinningWorker.kt", commit, listOf(EdictNextLineRange(12, 15))),
      )
    }
    replaceSignal(repository, "documented-rule", "weak-0") {
      it.copy(source = EdictNextSignalSource.FromPR(99, "Weak evidence", emptyList(), "", "https://github.com/source/project/pull/99"))
    }
    val provider = FakeProvider()

    val result = service(repository, provider).promoteSelected()

    assertEquals(1, result.created.size)
    assertTrue(result.failures.isEmpty())
    val description = provider.lastRequest!!.description
    assertContains(description, "Adds the generated Kotlin inspection `documented-rule` at `inspections/documented-rule.inspection.kts`.")
    assertContains(description, "Representative evidence: Avoid blocking the worker thread")
    assertContains(description, "Showing referenced STRONG POSITIVE signals from 5 of 5 distinct source files, one per file")
    assertContains(description, "**POSITIVE** — Avoid blocking the worker thread")
    assertContains(description, "[PR #1: Correction 0](<https://github.com/source/project/pull/1#discussion_r1>)")
    assertContains(description, "**POSITIVE** — Avoid spinning until completion")
    assertContains(description, "[Commit diff $commit](<https://github.com/source/project/commit/$commit>)")
    assertContains(description, "SpinningWorker.kt at `$commit`, lines 12-15; signal `signal-6`")
    assertContains(description, "Inspection SHA-256: `${sha256Hex("inspection")}`")
    assertFalse(description.contains("Unreferenced generated evidence"))
    assertFalse(description.contains("Weak evidence"))
    assertFalse(description.contains("weak 0"))
    assertFalse(description.contains("NEGATIVE"))
    assertFalse(description.contains("Use the completion latch"))
  }

  @Test
  fun `promotion fails before creating a PR when fewer than five strong signals have references`() {
    val repository = repository()
    cluster(repository, "missing-references", 7, weakSignals = 1)
    replaceSignal(repository, "missing-references", "signal-4") {
      it.copy(source = EdictNextSignalSource.FromCommit("a".repeat(40)))
    }
    replaceSignal(repository, "missing-references", "signal-5") {
      it.copy(source = EdictNextSignalSource.Generated())
    }
    replaceSignal(repository, "missing-references", "signal-6") {
      it.copy(source = EdictNextSignalSource.SubmittedFeedback(url = "https://example.com/feedback"))
    }
    val provider = FakeProvider()

    val result = service(repository, provider).promoteSelected()

    assertTrue(result.created.isEmpty())
    assertEquals(0, provider.ensureCalls)
    assertEquals(listOf(PromotionFailure(
      "missing-references", "Promotion requires strong positive Signals from at least 5 distinct source files with PR or commit diff URLs; found 4",
    )), result.failures)
    assertTrue(repository.loadCluster("missing-references").manifest.promotions.isEmpty())
  }

  @Test
  fun `invalid source URLs do not count toward referenced strong signals`() {
    val invalidUrls = listOf("", "not-a-url", "javascript:alert(1)", "https:///missing-host", "https://host/path\nextra", "https://user:password@host/path")
    invalidUrls.forEachIndexed { index, url ->
      val repository = repository("invalid-url-$index")
      cluster(repository, "invalid-link", 5)
      replaceSignal(repository, "invalid-link", "signal-4") {
        it.copy(source = (it.source as EdictNextSignalSource.FromPR).copy(url = url))
      }
      val provider = FakeProvider()

      val result = service(repository, provider).promoteSelected()

      assertTrue(result.created.isEmpty(), "Unexpected PR for '$url'")
      assertContains(result.failures.single().message, "found 4")
      assertEquals(0, provider.ensureCalls)
    }
  }

  @Test
  fun `large clusters produce a deterministic bounded evidence section`() {
    val repository = repository()
    cluster(repository, "large-rule", 14)
    replaceSignal(repository, "large-rule", "signal-0") {
      it.copy(description = "Do not use [unsafe](https://wrong.test) <markup> or `ticks`\nacross lines")
    }
    val provider = FakeProvider()

    assertEquals(1, service(repository, provider).promoteSelected().created.size)

    val description = provider.lastRequest!!.description
    assertContains(description, "Showing referenced STRONG POSITIVE signals from 10 of 14 distinct source files, one per file")
    assertEquals(10, Regex("(?m)^\\d+\\. ").findAll(description).count())
    assertContains(description, "Do not use \\[unsafe\\](https://wrong.test) &lt;markup&gt; or \\`ticks\\` across lines")
    assertEquals(description, promotionDescription(repository.loadCluster("large-rule"), provider.lastRequest!!.targetPath, provider.lastRequest!!.inspectionDigest))
  }

  @Test
  fun `promotion counts source files once across revisions labels and reviews`() {
    val repository = repository()
    cluster(repository, "one-file", 8, sourceFiles = 1)
    cluster(repository, "four-files", 10, weakSignals = 5, sourceFiles = 4)
    cluster(repository, "five-files", 5)
    repeat(8) { index ->
      replaceSignal(repository, "one-file", "signal-$index") {
        it.copy(
          fileRevision = it.fileRevision.copy(revision = index.toString().repeat(40)),
          label = if (index % 2 == 0) EdictNextSignalLabel.POSITIVE else EdictNextSignalLabel.NEGATIVE,
        )
      }
    }
    // Equal basenames in different directories are different source files.
    repeat(5) { index ->
      replaceSignal(repository, "five-files", "signal-$index") {
        it.copy(fileRevision = it.fileRevision.copy(path = "src/package$index/Worker.kt"))
      }
    }
    val provider = FakeProvider()

    val result = service(repository, provider).promoteSelected()

    assertEquals(listOf("inspections/five-files.inspection.kts"), result.created.map { it.targetPath })
    assertEquals(1, provider.ensureCalls)
    assertTrue(result.failures.isEmpty())
    assertFailsWith<IllegalArgumentException> {
      promotionDescription(repository.loadCluster("four-files"), "inspection.kts", "digest")
    }
  }

  @Test
  fun `description selects one referenced strong positive representative per file`() {
    val repository = repository()
    cluster(repository, "grouped-rule", 10, sourceFiles = 5)
    replaceSignal(repository, "grouped-rule", "signal-0") { it.copy(source = EdictNextSignalSource.Generated()) }
    replaceSignal(repository, "grouped-rule", "signal-1") { it.copy(label = EdictNextSignalLabel.NEGATIVE) }
    val provider = FakeProvider()

    assertEquals(1, service(repository, provider).promoteSelected().created.size)

    val cluster = repository.loadCluster("grouped-rule")
    val evidence = cluster.promotionEvidence()
    assertEquals(listOf("signal-2", "signal-3", "signal-4", "signal-5", "signal-6"), evidence.map { it.id })
    val description = provider.lastRequest!!.description
    assertContains(description, "from 5 of 5 distinct source files, one per file")
    assertEquals(5, Regex("(?m)^\\d+\\. ").findAll(description).count())
    evidence.forEach { assertContains(description, "signal `${it.id}`") }
    listOf("signal-0", "signal-1", "signal-7", "signal-8", "signal-9").forEach {
      assertFalse(description.contains("signal `$it`"))
    }
    assertEquals(description, promotionDescription(cluster.copy(signals = cluster.signals.reversed()), "inspections/grouped-rule.inspection.kts", sha256Hex("inspection")))
  }

  @Test
  fun `managed promotion publishes eligible inspections despite failed and cancelled upstream tasks`() {
    val repository = repository()
    cluster(repository, "earlier-rule", 6)
    cluster(repository, "new-rule", 6)
    cluster(repository, "failed-rule", 6, EdictNextClusterStatus.Pending)
    val provider = FakeProvider()
    EdictNextRepositoryState.open(repository.paths.root).use { state ->
      val manager = state.createPlan("Run the daily repository routine", emptyList())
      val enrichment = state.addTask(manager.token, "edict-git-history-signal-enrichment", "Enrich another rule")
      val enrichmentWorker = state.launch(manager.token, enrichment.id, enrichment.skill)
      state.finishTask(enrichmentWorker.token, "failed", "Historical search failed")

      val generation = state.addTask(manager.token, "edict-next-cluster-generation", "Generate failed-rule")
      val generationWorker = state.launch(manager.token, generation.id, generation.skill)
      val review = state.addTask(generationWorker.token, "edict-next-inspection-code-review", "Review candidate")
      val reviewer = state.launch(generationWorker.token, review.id, review.skill)
      state.finishTask(reviewer.token, "failed", "False positive remains unrecorded")
      state.finishTask(generationWorker.token, "failed", "Candidate review failed")

      val lost = state.addTask(manager.token, generation.skill, "Generate another rule")
      state.launch(manager.token, lost.id, lost.skill)
      state.cancelTask(manager.token, lost.id, "Worker lost")
      val successful = state.addTask(manager.token, generation.skill, "Generate new-rule")
      val successfulWorker = state.launch(manager.token, successful.id, successful.skill)
      state.finishTask(successfulWorker.token, "completed", "Generated new-rule")

      val promotion = state.addTask(manager.token, "edict-promote", "Publish eligible inspections")
      val worker = state.launch(manager.token, promotion.id, promotion.skill)
      state.requireSkill(worker.token, setOf("edict-promote"))
      val result = service(repository, provider).promoteSelected()

      assertTrue(result.failures.isEmpty())
      assertEquals(
        setOf("inspections/earlier-rule.inspection.kts", "inspections/new-rule.inspection.kts"),
        result.created.map { it.targetPath }.toSet(),
      )
      state.finishTask(worker.token, "completed", "Published earlier-rule and new-rule")
      val tasks = state.plan()!!.tasks.associateBy { it.id }
      assertEquals("failed", tasks.getValue(enrichment.id).status)
      assertEquals("failed", tasks.getValue(generation.id).status)
      assertEquals("failed", tasks.getValue(review.id).status)
      assertEquals("cancelled", tasks.getValue(lost.id).status)
      assertEquals("completed", tasks.getValue(promotion.id).status)
      assertTrue(repository.loadCluster("failed-rule").manifest.promotions.isEmpty())
      for (id in listOf("earlier-rule", "new-rule")) {
        assertEquals(PromotionPrState.ON_REVIEW, repository.loadCluster(id).manifest.promotions.single().state)
      }
    }
  }

  @Test
  fun `promotes only explicitly selected clusters when cluster ids are provided`() {
    val repository = repository()
    cluster(repository, "not-selected", 6)
    cluster(repository, "selected", 6)
    val provider = FakeProvider()

    val result = service(repository, provider).promote(listOf("missing", "selected"))

    assertEquals(listOf("selected"), result.created.map { it.targetPath.substringAfterLast('/').removeSuffix(".inspection.kts") })
    assertEquals(listOf(PromotionFailure("missing", "cluster does not exist")), result.failures)
    assertTrue(repository.loadCluster("not-selected").manifest.promotions.isEmpty())
    assertEquals(1, provider.ensureCalls)
  }

  @Test
  fun `uses and persists repository default branch when target branch is omitted`() {
    val repository = repository()
    cluster(repository, "default-branch", 6)
    val provider = FakeProvider().apply { defaultBranch = "trunk" }

    val result = service(repository, provider, PromotionConfiguration("reviewer")).promoteSelected()

    assertEquals("trunk", result.created.single().targetBranch)
    assertEquals("trunk", provider.lastRequest?.targetBranch)
  }

  @Test
  fun `same digest and target is idempotent in every PR state while a new digest is promotable`() {
    PromotionPrState.entries.forEachIndexed { index, state ->
      val repository = repository("state-$index")
      val content = "inspection-$index"
      val digest = sha256Hex(content)
      val existing = promotion("p-${index.toString(16).padStart(24, '0')}", digest, state)
      cluster(repository, "same-version", 6, promotions = listOf(existing), inspection = content)
      val provider = FakeProvider()

      val result = service(repository, provider).promoteSelected()

      assertTrue(result.created.isEmpty())
      assertTrue(result.failures.isEmpty())
      assertEquals(0, provider.ensureCalls)
      assertEquals(1, repository.loadCluster("same-version").manifest.promotions.size)
    }

    val repository = repository("new-version")
    cluster(repository, "changed-version", 6, promotions = listOf(promotion("p-${"a".repeat(24)}", sha256Hex("old"), PromotionPrState.ACCEPTED)))
    val provider = FakeProvider()
    val result = service(repository, provider).promoteSelected()
    assertEquals(1, result.created.size)
    assertEquals(2, repository.loadCluster("changed-version").manifest.promotions.size)
  }

  @Test
  fun `provider retry creates one persisted record`() {
    val repository = repository()
    cluster(repository, "retry-rule", 6)
    val provider = FakeProvider(failOnceAfterRemoteCreation = true)

    assertEquals(1, service(repository, provider).promoteSelected().failures.size)
    assertTrue(repository.loadCluster("retry-rule").manifest.promotions.isEmpty())
    assertEquals(1, service(repository, provider).promoteSelected().created.size)
    assertEquals(1, repository.loadCluster("retry-rule").manifest.promotions.size)
    assertEquals(2, provider.ensureCalls)
    assertEquals(1, provider.remotePullRequests)
  }

  @Test
  fun `review states transition and closed current version can be discontinued`() {
    val repository = repository()
    val digest = sha256Hex("inspection")
    cluster(repository, "accepted-rule", 5, promotions = listOf(promotion("p-${"1".repeat(24)}", digest, PromotionPrState.ON_REVIEW)))
    cluster(repository, "closed-rule", 5, promotions = listOf(promotion("p-${"2".repeat(24)}", digest, PromotionPrState.ON_REVIEW)))
    cluster(repository, "open-rule", 5, promotions = listOf(promotion("p-${"3".repeat(24)}", digest, PromotionPrState.ON_REVIEW)))
    val provider = FakeProvider().apply {
      reviews["1"] = ReviewStatus(ReviewState.MERGED)
      reviews["2"] = ReviewStatus(ReviewState.CLOSED_UNMERGED, "Please do not ship this rule")
      reviews["3"] = ReviewStatus(ReviewState.OPEN)
    }
    val first = service(repository, provider).checkReviews()
    assertEquals(PromotionPrState.ACCEPTED, first.accepted.single().state)
    assertEquals("p-${"1".repeat(24)}", first.accepted.single().id)
    assertEquals(PromotionPrState.ACCEPTED, repository.loadCluster("accepted-rule").manifest.promotions.single().state)
    assertContains(
      repository.loadCluster("accepted-rule").historyPath.readText(),
      "accepted after merged PR https://example/p-${"1".repeat(24)}",
    )
    assertEquals(PromotionPrState.ON_REVIEW, repository.loadCluster("closed-rule").manifest.promotions.single().state)
    assertEquals(PromotionPrState.ON_REVIEW, repository.loadCluster("open-rule").manifest.promotions.single().state)
    assertEquals("Please do not ship this rule", first.undecided.single().evidence)
    val refreshed = service(repository, provider).checkReviews()
    assertTrue(refreshed.accepted.isEmpty())
    assertEquals("closed-rule", refreshed.undecided.single().clusterId)

    val decision = PromotionReviewDecision(
      "closed-rule", "p-${"2".repeat(24)}", ClosedReviewDecision.DISCONTINUE_CLUSTER, "Reviewer rejected the rule itself",
    )
    val second = service(repository, provider).decideReviews(listOf(decision))
    assertTrue(second.failures.isEmpty())
    val cluster = repository.loadCluster("closed-rule")
    assertEquals(EdictNextClusterStatus.Discontinued, cluster.manifest.status)
    assertEquals(PromotionPrState.CLOSED, cluster.manifest.promotions.single().state)
    assertFalse(repository.paths.inspectionPath("closed-rule").exists())
    assertTrue(cluster.historyPath.readText().contains("Reviewer rejected the rule itself"))
    assertTrue(service(repository, provider).checkReviews().undecided.isEmpty())
  }

  @Test
  fun `decision can return current generated cluster to pending and records history`() {
    val repository = repository()
    val promotion = promotion("p-${"9".repeat(24)}", sha256Hex("inspection"), PromotionPrState.ON_REVIEW)
    cluster(repository, "revise-rule", 5, promotions = listOf(promotion))
    val provider = FakeProvider().apply {
      reviews[promotion.pullRequest.id] = ReviewStatus(ReviewState.CLOSED_UNMERGED, "Please revise")
    }

    val result = service(repository, provider).decideReviews(listOf(
      PromotionReviewDecision(
        "revise-rule",
        promotion.id,
        ClosedReviewDecision.MOVE_CLUSTER_TO_PENDING,
        "Reviewer requested a narrower generated rule",
      ),
    ))

    assertTrue(result.failures.isEmpty())
    val stored = repository.loadCluster("revise-rule")
    assertEquals(EdictNextClusterStatus.Pending, stored.manifest.status)
    assertEquals("revise-rule", stored.manifest.predecessorId)
    assertEquals(PromotionPrState.CLOSED, stored.manifest.promotions.single().state)
    assertEquals(stored.signalIds, stored.manifest.processedSignalIds)
    assertTrue(repository.paths.inspectionPath("revise-rule").exists())
    assertTrue(stored.historyPath.readText().contains("Reviewer requested a narrower generated rule"))
  }

  @Test
  fun `decision batches are limited to ten promotions`() {
    val decision = PromotionReviewDecision(
      "rule",
      "p-${"a".repeat(24)}",
      ClosedReviewDecision.MOVE_CLUSTER_TO_PENDING,
      "Rejected",
    )
    assertFailsWith<IllegalArgumentException> {
      service(repository(), FakeProvider()).decideReviews(List(11) { decision.copy(promotionId = "p-${it.toString(16).padStart(24, '0')}") })
    }
  }

  @Test
  fun `stale digest cannot discontinue newer generated version and older accepted promotion does not decide outcome`() {
    val repository = repository()
    val old = promotion("p-${"4".repeat(24)}", sha256Hex("old"), PromotionPrState.ACCEPTED)
    val closed = promotion("p-${"5".repeat(24)}", sha256Hex("inspection"), PromotionPrState.CLOSED)
    cluster(repository, "stale-rule", 5, promotions = listOf(old, closed))
    repository.paths.inspectionPath("stale-rule").writeText("newer inspection")

    val provider = FakeProvider().apply {
      reviews[closed.pullRequest.id] = ReviewStatus(ReviewState.CLOSED_UNMERGED, "Rejected")
    }
    val result = service(repository, provider).decideReviews(listOf(
      PromotionReviewDecision("stale-rule", closed.id, ClosedReviewDecision.DISCONTINUE_CLUSTER, "Reject current rule"),
    ))

    assertEquals(1, result.failures.size)
    val stored = repository.loadCluster("stale-rule")
    assertEquals(EdictNextClusterStatus.Generated, stored.manifest.status)
    assertEquals(PromotionPrState.CLOSED, stored.manifest.promotions.last().state)
    assertEquals(PromotionPrState.ACCEPTED, stored.manifest.promotions.first().state)
  }

  @Test
  fun `provider failure preserves on-review state`() {
    val repository = repository()
    cluster(repository, "provider-failure", 5, promotions = listOf(
      promotion("p-${"6".repeat(24)}", sha256Hex("inspection"), PromotionPrState.ON_REVIEW),
    ))
    val provider = FakeProvider().apply { reviewFailure = true }

    val result = service(repository, provider).checkReviews()

    assertEquals(1, result.failures.size)
    assertEquals(PromotionPrState.ON_REVIEW, repository.loadCluster("provider-failure").manifest.promotions.single().state)
  }

  @Test
  fun `decision refuses a review that is still open`() {
    val repository = repository()
    val promotion = promotion("p-${"c".repeat(24)}", sha256Hex("inspection"), PromotionPrState.ON_REVIEW)
    cluster(repository, "open-decision", 5, promotions = listOf(promotion))

    val result = service(repository, FakeProvider()).decideReviews(listOf(
      PromotionReviewDecision(
        "open-decision",
        promotion.id,
        ClosedReviewDecision.MOVE_CLUSTER_TO_PENDING,
        "Reviewer requested changes",
      ),
    ))

    assertEquals(1, result.failures.size)
    val stored = repository.loadCluster("open-decision")
    assertEquals(EdictNextClusterStatus.Generated, stored.manifest.status)
    assertEquals(PromotionPrState.ON_REVIEW, stored.manifest.promotions.single().state)
  }

  @Test
  fun `closed promotion remains undecided when evidence refresh fails`() {
    val repository = repository()
    val promotion = promotion("p-${"b".repeat(24)}", sha256Hex("inspection"), PromotionPrState.CLOSED)
    cluster(repository, "closed-provider-failure", 5, promotions = listOf(promotion))
    val provider = FakeProvider().apply { reviewFailure = true }

    val result = service(repository, provider).checkReviews()

    assertEquals(1, result.failures.size)
    assertEquals(promotion.id, result.undecided.single().promotionId)
    assertEquals(PromotionPrState.CLOSED, repository.loadCluster("closed-provider-failure").manifest.promotions.single().state)
  }

  @Test
  fun `illegal transitions and duplicate promotion ids are rejected`() {
    val repository = repository()
    val digest = sha256Hex("inspection")
    val accepted = promotion("p-${"7".repeat(24)}", digest, PromotionPrState.ACCEPTED)
    cluster(repository, "invalid-transition", 5, promotions = listOf(accepted))

    assertFailsWith<IllegalArgumentException> {
      repository.applyResolvedPromotion(
        clusterId = "invalid-transition",
        promotionId = accepted.id,
        resolvedState = PromotionPrState.CLOSED,
        updatedAt = clock.instant().toString(),
        expectedInspectionDigest = digest,
        targetStatus = EdictNextClusterStatus.Discontinued,
        rationale = "Invalid second resolution",
      )
    }
    assertFailsWith<IllegalArgumentException> {
      repository.appendPromotion(
        "invalid-transition",
        digest,
        accepted.copy(pullRequest = accepted.pullRequest.copy(id = "different")),
      )
    }
  }

  @Test
  fun `closed review evidence is bounded`() {
    val repository = repository()
    val promotion = promotion("p-${"8".repeat(24)}", sha256Hex("inspection"), PromotionPrState.ON_REVIEW)
    cluster(repository, "bounded-evidence", 5, promotions = listOf(promotion))
    val provider = FakeProvider().apply {
      reviews[promotion.pullRequest.id] = ReviewStatus(ReviewState.CLOSED_UNMERGED, "x".repeat(40_000))
    }
    val result = service(repository, provider).checkReviews()
    assertEquals(32_000, result.undecided.single().evidence.length)
    assertEquals(PromotionPrState.ON_REVIEW, repository.loadCluster("bounded-evidence").manifest.promotions.single().state)
  }

  private fun PromotionService.promoteSelected(clusterIds: List<String>? = null): PromotionPromoteResponse {
    val selection = select()
    val chosen = selection.candidates.filter { clusterIds == null || it.clusterId in clusterIds }
    val result = if (chosen.isEmpty()) PromotionPromoteResponse()
      else promote(chosen.map { it.clusterId })
    return result.copy(failures = selection.failures + result.failures)
  }

  @Test
  fun `selection is read only and publication accepts only the skills chosen subset`() {
    val repository = repository()
    cluster(repository, "first", 5)
    cluster(repository, "second", 5)
    val provider = FakeProvider()
    val service = service(repository, provider)

    val selection = service.select()
    assertEquals(setOf("first", "second"), selection.candidates.map { it.clusterId }.toSet())
    assertTrue(selection.candidates.all { it.evaluation.precision == 1.0 && it.f1 == 1.0 })
    assertEquals(0, provider.ensureCalls)
    assertTrue(repository.loadCluster("first").manifest.promotions.isEmpty())
    assertTrue(repository.loadCluster("second").manifest.promotions.isEmpty())

    val result = service.promote(listOf("second"))
    assertEquals(listOf("inspections/second.inspection.kts"), result.created.map { it.targetPath })
    assertTrue(repository.loadCluster("first").manifest.promotions.isEmpty())
    assertEquals(listOf("first"), service.select().candidates.map { it.clusterId })
  }

  @Test
  fun `selection count limits eligible candidates and reports configured thresholds`() {
    val repository = repository()
    cluster(repository, "a-ineligible", 5)
    evaluation(repository, "a-ineligible", 0.1, 0.1)
    cluster(repository, "b-eligible", 5)
    cluster(repository, "c-eligible", 5)
    val provider = FakeProvider()
    val targets = configuration.copy(targetPrecision = 0.75, targetRecall = 0.65, targetF1 = 0.2)
    val service = service(repository, provider, targets)

    val limited = service.select(count = 1)
    assertEquals(listOf("b-eligible"), limited.candidates.map { it.clusterId })
    assertEquals(PromotionThresholds(0.75, 0.65, 0.2), limited.thresholds)
    assertEquals(listOf("b-eligible", "c-eligible"), service.select().candidates.map { it.clusterId })
    assertEquals(2, service.select(count = 10).candidates.size)
    assertEquals(0, provider.ensureCalls)
    assertFailsWith<IllegalArgumentException> { service.select(count = 0) }
    assertFailsWith<IllegalArgumentException> { service.select(count = -1) }
    val empty = service(repository("empty-selection"), provider, targets).select()
    assertTrue(empty.candidates.isEmpty())
    assertEquals(limited.thresholds, empty.thresholds)
  }

  @Test
  fun `skill can publish generated inspections outside metric selection`() {
    val repository = repository()
    cluster(repository, "low-score", 5)
    evaluation(repository, "low-score", 0.1, 0.1)
    cluster(repository, "no-evaluation", 5)
    repository.loadCluster("no-evaluation").directory.evaluationPath.toFile().delete()
    val provider = FakeProvider()
    val service = service(repository, provider)
    assertTrue(service.select().candidates.isEmpty())

    val result = service.promote(listOf("low-score", "no-evaluation"))
    assertEquals(2, result.created.size)
    assertTrue(result.failures.isEmpty())
    assertFailsWith<IllegalArgumentException> { service.promote(emptyList()) }
    assertFailsWith<IllegalArgumentException> { service.promote(listOf("low-score", "low-score")) }
  }

  private fun repository(name: String = "state"): EdictRepository {
    val root = temporary.resolve(name).createDirectories()
    root.resolve("clusters").createDirectories()
    root.resolve("inspections").createDirectories()
    return EdictRepository(EdictRepositoryDirectory(root))
  }

  private fun cluster(
    repository: EdictRepository,
    id: String,
    positives: Int,
    status: EdictNextClusterStatus = EdictNextClusterStatus.Generated,
    promotions: List<InspectionPromotion> = emptyList(),
    inspection: String = "inspection",
    weakSignals: Int = 0,
    sourceFiles: Int = positives,
  ) {
    val root = repository.paths.clustersDirectory.resolve(id).createDirectories()
    root.resolve("signals").createDirectories()
    root.resolve("cluster.json").writeText(EdictNextJson.encodeToString(
      EdictNextClusterManifest(id, EdictNextLanguage.Kotlin, status, promotions = promotions),
    ))
    root.resolve("history.md").writeText("Created.\n")
    repeat(positives) { index ->
      root.resolve("signals/signal-$index.json").writeText(EdictNextJson.encodeToString(
        EdictNextSignal(
          "signal-$index",
          fileRevision = EdictNextFileRevision("Example${index % sourceFiles}.kt", "revision"),
          source = EdictNextSignalSource.FromPR(
            index + 1, "Correction $index", listOf("Fix the problem"), "",
            "https://github.com/source/project/pull/${index + 1}#discussion_r${index + 1}",
          ),
          label = EdictNextSignalLabel.POSITIVE,
          description = "positive $index",
        ),
      ))
    }
    repeat(weakSignals) { index ->
      root.resolve("signals/weak-$index.json").writeText(EdictNextJson.encodeToString(
        EdictNextSignal(
          "weak-$index",
          fileRevision = EdictNextFileRevision("Example.kt", "revision"),
          source = EdictNextSignalSource.Generated(),
          label = EdictNextSignalLabel.POSITIVE,
          description = "weak $index",
          strength = EdictNextSignalStrength.WEAK,
        ),
      ))
    }
    repository.paths.inspectionPath(id).writeText(inspection)
    evaluation(repository, id)
  }

  private fun evaluation(repository: EdictRepository, id: String, precision: Double = 1.0, recall: Double = 1.0) {
    repository.loadCluster(id).directory.evaluationPath.writeText(EdictNextJson.encodeToString(EdictNextEvaluation(
      inspectionHash = sha256Hex(repository.paths.inspectionPath(id).readText()),
      exampleSetDigest = exampleSetDigest(repository.loadCluster(id)),
      action = EdictNextInspectionAction.GENERATE,
      tp = 5, fp = 0, fn = 0, precision = precision, recall = recall,
      examples = emptyMap(), projectReview = null,
    )))
  }

  @Test
  fun `promotion requires all metrics strictly above their targets`() {
    val repository = repository()
    val provider = FakeProvider()
    cluster(repository, "quality-rule", 5)
    val targets = configuration.copy(targetPrecision = 0.7, targetRecall = 0.7, targetF1 = 0.8)
    for ((precision, recall) in listOf(0.6 to 1.0, 1.0 to 0.6, 0.7 to 1.0, 1.0 to 0.7, 0.75 to 0.75, 0.8 to 0.8, 0.0 to 0.0)) {
      evaluation(repository, "quality-rule", precision, recall)
      val result = service(repository, provider, targets).promoteSelected(listOf("quality-rule"))
      assertTrue(result.created.isEmpty(), "Unexpected promotion for $precision / $recall")
      assertTrue(result.failures.isEmpty())
      assertEquals(EdictNextClusterStatus.Generated, repository.loadCluster("quality-rule").manifest.status)
      assertTrue(repository.paths.inspectionPath("quality-rule").exists())
    }
    assertEquals(0, provider.ensureCalls)
    evaluation(repository, "quality-rule", 0.9, 0.9)
    assertEquals(1, service(repository, provider, targets).promoteSelected().created.size)
  }

  @Test
  fun `missing or stale evaluations leave inspections generated`() {
    val repository = repository()
    val provider = FakeProvider()
    cluster(repository, "missing", 5)
    repository.loadCluster("missing").directory.evaluationPath.toFile().delete()
    cluster(repository, "stale-inspection", 5)
    repository.paths.inspectionPath("stale-inspection").writeText("changed")
    cluster(repository, "stale-examples", 5)
    val cluster = repository.loadCluster("stale-examples")
    cluster.directory.evaluationPath.writeText(EdictNextJson.encodeToString(repository.loadEvaluation(cluster)!!.copy(exampleSetDigest = "stale")))

    val result = service(repository, provider).promoteSelected()
    assertTrue(result.created.isEmpty())
    assertTrue(result.failures.isEmpty())
    assertEquals(0, provider.ensureCalls)
    for (id in listOf("missing", "stale-inspection", "stale-examples")) {
      assertEquals(EdictNextClusterStatus.Generated, repository.loadCluster(id).manifest.status)
      assertTrue(repository.paths.inspectionPath(id).exists())
    }
  }

  private fun replaceSignal(
    repository: EdictRepository,
    clusterId: String,
    signalId: String,
    transform: (EdictNextSignal) -> EdictNextSignal,
  ) {
    val cluster = repository.loadCluster(clusterId)
    val signal = cluster.signals.single { it.id == signalId }
    cluster.directory.signalPath(signalId).writeText(EdictNextJson.encodeToString(transform(signal)))
  }

  private fun promotion(id: String, digest: String, state: PromotionPrState): InspectionPromotion = InspectionPromotion(
    id,
    digest,
    "main",
    "inspections/same-version.inspection.kts",
    ReviewReference(CiProviderId.GITHUB, "JetBrains", "qodana-cli", id.removePrefix("p-").take(1), "https://example/$id"),
    state,
    "2026-10-04T12:00:00Z",
    "2026-10-04T12:00:00Z",
  )

  private fun service(
    repository: EdictRepository,
    provider: FakeProvider,
    configuration: PromotionConfiguration = this.configuration,
  ) = PromotionService(
    repository = { repository },
    configuration = EdictConfiguration(ci = ci, promotion = configuration),
    reviews = provider,
    clock = clock,
  )

  private class FakeProvider(
    private val failOnceAfterRemoteCreation: Boolean = false,
  ) : ReviewManagementApi {
    var ensureCalls = 0
    var remotePullRequests = 0
    var reviewFailure = false
    var defaultBranch = "main"
    var lastRequest: CreateReviewRequest? = null
    val reviews = mutableMapOf<String, ReviewStatus>()
    private val remote = mutableMapOf<String, CreatedReview>()

    override fun resolveTargetBranch(repository: ReviewRepository, configured: String?): String = configured ?: defaultBranch

    override fun ensureReview(request: CreateReviewRequest): CreatedReview {
      lastRequest = request
      ensureCalls++
      val pullRequest = remote.getOrPut(request.operationId) {
        remotePullRequests++
        CreatedReview("$remotePullRequests", "https://example/pr/$remotePullRequests")
      }
      if (failOnceAfterRemoteCreation && ensureCalls == 1) error("connection lost after PR creation")
      return pullRequest
    }

    override fun review(reference: ReviewReference): ReviewStatus {
      if (reviewFailure) error("provider unavailable")
      return reviews[reference.id] ?: ReviewStatus(ReviewState.OPEN)
    }
  }
}
