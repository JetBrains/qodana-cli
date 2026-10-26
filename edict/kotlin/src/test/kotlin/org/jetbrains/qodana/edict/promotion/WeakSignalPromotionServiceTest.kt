package org.jetbrains.qodana.edict.promotion

import kotlinx.coroutines.runBlocking
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import org.jetbrains.qodana.edict.ci.*
import org.jetbrains.qodana.edict.common.EdictCIConfiguration
import org.jetbrains.qodana.edict.common.EdictConfiguration
import org.jetbrains.qodana.edict.common.PromotionConfiguration
import org.jetbrains.qodana.edict.common.EdictLayout
import org.jetbrains.qodana.edict.common.flag
import org.jetbrains.qodana.edict.edictnext.*
import org.jetbrains.qodana.edict.support.edictNextToolset
import org.jetbrains.qodana.edict.support.launch
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import java.time.Clock
import java.time.Instant
import java.time.ZoneOffset
import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit
import kotlin.io.path.createDirectories
import kotlin.io.path.readText
import kotlin.io.path.writeText
import kotlin.test.*

class WeakSignalPromotionServiceTest {
  @TempDir lateinit var directory: Path
  private val revision = "a".repeat(40)
  private val digest = sha256Hex("inspection")
  private val configuration = EdictConfiguration(
    ci = EdictCIConfiguration("https://github.com/owner/repo"),
    promotion = PromotionConfiguration("reviewer", "main"),
  )

  @Test
  fun `selects at most five distinct true positives only from eligible generated inspections`() {
    val repository = repository()
    cluster(repository, "weak", findings = (1..7).map(::finding) + finding(1))
    cluster(repository, "strong", strongSignals = 5)
    cluster(repository, "same-file", strongSignals = 6, sourceFiles = 1)
    cluster(repository, "pending", status = EdictNextClusterStatus.Pending)
    cluster(repository, "no-tp", findings = emptyList())
    // Strong evidence without external references also cannot pass normal promotion.
    cluster(repository, "unreferenced", strongSignals = 6, references = false)
    val result = service(repository).prepare()

    assertTrue(result.failures.isEmpty())
    assertEquals(listOf("same-file", "unreferenced", "weak"), result.candidates.map { it.clusterId })
    val selected = result.candidates.single { it.clusterId == "weak" }
    assertEquals((1..5).toList(), selected.findings.map { it.fileRevision.expectedRanges!!.first().start })
    assertEquals(revision, selected.projectRevision)
    assertEquals("Avoid busy waiting", selected.inspectionDescription)
    assertEquals(digest, selected.inspectionDigest)
  }

  @Test
  fun `normal and weak promotion use the same positive source file threshold`() {
    val repository = repository()
    cluster(repository, "mixed", strongSignals = 5, negativeSignals = 1)
    cluster(repository, "negative-only", strongSignals = 6, negativeSignals = 6)
    cluster(repository, "positive-only", strongSignals = 5)
    val provider = FakeReviews()

    val normal = PromotionService({ repository }, configuration, provider).promote()
    assertTrue(normal.failures.isEmpty())
    assertEquals(listOf("inspections/positive-only.inspection.kts"), normal.created.map { it.targetPath })

    val weak = service(repository, provider)
    val selection = weak.prepare()
    assertTrue(selection.failures.isEmpty())
    assertEquals(listOf("mixed", "negative-only"), selection.candidates.map { it.clusterId })
    val fixes = listOf(WeakSignalFix("src/Worker.kt", listOf(1), "fixed", "Fix waiting"))
    weak.publish("mixed", digest, fixes, "Reviewed diff")
    assertEquals(mapOf("src/Worker.kt" to "fixed"), provider.requests.last().sourceFix!!.files)
    assertFailsWith<IllegalArgumentException> {
      weak.publish("positive-only", digest, fixes, "Reviewed diff")
    }
  }

  @Test
  fun `publishes source improvements with inspection explanation and acceptance request once across restarts`() {
    val repository = repository()
    cluster(repository, "weak")
    val provider = FakeReviews()
    val fixes = listOf(WeakSignalFix("src/Worker.kt", listOf(1, 2), "fun waitForResult() = latch.await()", "Wait for completion instead of spinning"))

    val promotion = service(repository, provider).publish("weak", digest, fixes, "Reviewed diff; focused tests passed")
    assertEquals(PromotionPrState.ON_REVIEW, promotion.state)
    assertEquals(promotion, repository.loadActiveWeakSignalPromotions()["weak"].orEmpty().single())
    assertTrue(repository.loadCluster("weak").manifest.promotions.isEmpty())
    assertEquals(EdictNextClusterStatus.Generated, repository.loadCluster("weak").manifest.status)
    val request = provider.requests.single()
    assertEquals(mapOf("src/Worker.kt" to fixes.single().content), request.sourceFix!!.files)
    assertEquals(revision, request.sourceFix.baseRevision)
    assertEquals(configuration.ci!!.reviewRepository, request.repository)
    assertEquals("reviewer", request.reviewer)
    assertContains(request.description, "Avoid busy waiting")
    assertContains(request.description, "accept/merge this PR if you agree")
    assertContains(request.description, "focused tests passed")
    assertContains(request.description, "original lines 1, 2")

    val restarted = service(EdictRepository(EdictRepositoryDirectory(repository.paths.root)), provider)
    assertTrue(restarted.prepare().candidates.isEmpty())
    assertEquals(promotion, restarted.publish("weak", digest, fixes, "Already validated"))
    assertEquals(1, provider.requests.size)
  }

  @Test
  fun `rejects unselected paths and lines more than five locations duplicate files and changed inspection`() {
    val repository = repository()
    cluster(repository, "weak", findings = (1..8).map(::finding))
    val provider = FakeReviews()
    val service = service(repository, provider)
    val fix = WeakSignalFix("src/Worker.kt", listOf(1), "fixed", "Fix waiting")
    val invalid = listOf(
      emptyList(),
      listOf(fix.copy(path = "src/Other.kt")),
      listOf(fix.copy(path = "../Other.kt")),
      listOf(fix.copy(lines = listOf(6))),
      listOf(fix.copy(lines = (1..6).toList())),
      listOf(fix.copy(lines = listOf(1, 1))),
      listOf(fix, fix.copy(lines = listOf(2))),
      listOf(fix.copy(content = "")),
    )
    invalid.forEach { fixes ->
      assertFailsWith<IllegalArgumentException> { service.publish("weak", digest, fixes, "Reviewed diff") }
    }
    repository.paths.inspectionPath("weak").writeText("changed inspection")
    assertFailsWith<IllegalArgumentException> { service.publish("weak", digest, listOf(fix), "Reviewed diff") }
    assertTrue(provider.requests.isEmpty())
  }

  @Test
  fun `missing and stale persisted reviews are failures without blocking independent candidates`() {
    val repository = repository()
    cluster(repository, "good")
    cluster(repository, "missing", review = null)
    cluster(repository, "stale", evaluationDigest = sha256Hex("older inspection"))
    val result = service(repository).prepare(listOf("missing", "good", "unknown", "stale"))

    assertEquals(listOf("good"), result.candidates.map { it.clusterId })
    assertEquals(setOf("missing", "unknown", "stale"), result.failures.map { it.clusterId }.toSet())
    assertFailsWith<IllegalArgumentException> { service(repository).prepare(emptyList()) }
  }

  @Test
  fun `provider failures leave no durable proposal and can be retried`() {
    val repository = repository()
    cluster(repository, "weak")
    val provider = FakeReviews().apply { fail = true }
    val fixes = listOf(WeakSignalFix("src/Worker.kt", listOf(1), "fixed", "Fix waiting"))
    assertFailsWith<IllegalStateException> { service(repository, provider).publish("weak", digest, fixes, "Reviewed diff") }
    assertTrue(repository.loadActiveWeakSignalPromotions()["weak"].orEmpty().isEmpty())
    provider.fail = false
    service(repository, provider).publish("weak", digest, fixes, "Reviewed diff")
    assertEquals(1, repository.loadActiveWeakSignalPromotions()["weak"].orEmpty().size)
  }

  @Test
  fun `changed fixes use a different branch after interrupted publication across restarts`() {
    val repository = repository()
    cluster(repository, "weak", findings = listOf(
      finding(1),
      finding(2).let { it.copy(fileRevision = it.fileRevision.copy(path = "src/Other.kt")) },
    ))
    val provider = FakeReviews().apply { failAfterCreation = true }
    val fixes = listOf(
      WeakSignalFix("src/Worker.kt", listOf(1), "first fix", "Fix waiting"),
      WeakSignalFix("src/Other.kt", listOf(2), "other fix", "Fix another wait"),
    )
    assertFailsWith<IllegalStateException> {
      service(repository, provider).publish("weak", digest, fixes, "Reviewed diff")
    }
    assertTrue(repository.loadActiveWeakSignalPromotions()["weak"].orEmpty().isEmpty())

    val restarted = service(EdictRepository(repository.paths), provider)
    assertFailsWith<IllegalStateException> {
      restarted.publish("weak", digest, fixes.reversed().map { it.copy(explanation = "Rechecked fix") }, "Rechecked diff")
    }
    assertEquals(provider.requests[0].branch, provider.requests[1].branch)

    provider.failAfterCreation = false
    val changedFixes = fixes.map { if (it.path == "src/Worker.kt") it.copy(content = "different fix") else it }
    val promotion = service(EdictRepository(repository.paths), provider).publish("weak", digest, changedFixes, "Reviewed new diff")
    assertNotEquals(provider.requests[0].branch, provider.requests[2].branch)
    assertTrue(provider.requests[2].operationId.startsWith("w-"))
    assertEquals(promotion, repository.loadActiveWeakSignalPromotions()["weak"].orEmpty().single())
    assertTrue(restarted.prepare().candidates.isEmpty())
    assertEquals(promotion, restarted.publish("weak", digest, fixes, "Already published"))
    assertEquals(3, provider.requests.size)
  }

  @Test
  fun `signal extraction is independent of unchecked and failed promotion reviews across restarts`() {
    val repository = repository()
    cluster(repository, "weak")
    val provider = FakeReviews()
    service(repository, provider).publish("weak", digest, listOf(
      WeakSignalFix("src/Worker.kt", listOf(1), "fixed", "Fix waiting"),
    ), "Reviewed diff")
    repeat(2) { run ->
      EdictNextRepositoryState.open(repository.paths.root).use { store ->
        val promotion = PromotionService({ repository }, configuration, provider)
        val layout = EdictLayout(directory, repository.paths.root)
        val management = EdictManagementService(store, layout, reviewProvider = provider, reviewRepository = configuration.ci!!.reviewRepository)
        edictNextToolset(layout, management, configuration = configuration, promotion = promotion).createServer()
        val plan = store.createPlan("Review signals", listOf(EdictNextRepositoryState.Step("edict-pr-signal-analysis", "Review signals")))
        val worker = store.launch(plan.token, plan.plan.tasks.single().id, "edict-pr-signal-analysis")
        val input = buildJsonObject {
          put("token", worker.token)
          put("prNumbers", JsonArray(listOf(JsonPrimitive(42))))
        }
        assertFalse(management.call("edict_fetch_pr_batch", input).flag("isError") == true)
        assertEquals(run * 2 + 1, provider.fetchCalls)
        provider.failReviews = true
        assertEquals(1, promotion.checkWeakSignalReviews().failures.size)
        assertFalse(management.call("edict_fetch_pr_batch", input).flag("isError") == true)
        assertEquals(run * 2 + 2, provider.fetchCalls)
        provider.failReviews = false

        val checked = promotion.checkWeakSignalReviews()
        assertTrue(checked.failures.isEmpty())
        assertEquals(PromotionPrState.ON_REVIEW, checked.reviews.single().promotion.state)
      }
    }
    val persisted = repository.loadActiveWeakSignalPromotions()["weak"].orEmpty().single()
    assertEquals(PromotionPrState.ON_REVIEW, persisted.state)
  }

  @Test
  fun `checks review states returning transient feedback without storing it on the promotion`() {
    val repository = repository()
    cluster(repository, "weak")
    val provider = FakeReviews()
    service(repository, provider).publish("weak", digest, listOf(WeakSignalFix("src/Worker.kt", listOf(1), "fixed", "Fix waiting")), "Reviewed diff")
    val promotion = PromotionService({ repository }, configuration, provider)
    assertFailsWith<IllegalArgumentException> {
      service(repository, provider).completeReview("weak", repository.loadActiveWeakSignalPromotions().getValue("weak").single().pullRequest)
    }
    val expected = listOf(
      ReviewState.OPEN to PromotionPrState.ON_REVIEW,
      ReviewState.MERGED to PromotionPrState.ACCEPTED,
      ReviewState.OPEN to PromotionPrState.ON_REVIEW,
      ReviewState.CLOSED_UNMERGED to PromotionPrState.CLOSED,
    )
    for ((remote, state) in expected) {
      provider.reviewStatus = ReviewStatus(remote, "Feedback for $remote")
      val result = promotion.checkWeakSignalReviews()
      assertTrue(result.failures.isEmpty())
      assertEquals(state, result.reviews.single().promotion.state)
      assertEquals("Feedback for $remote", result.reviews.single().reviewEvidence)
      assertFalse("reviewEvidence" in repository.loadCluster("weak").directory.manifestPath.readText())
      assertEquals(EdictNextClusterStatus.Generated, repository.loadCluster("weak").manifest.status)
      assertEquals(EdictNextSignalStrength.STRONG, repository.loadCluster("weak").signals.single().strength)
    }
    assertEquals(4, provider.reviewCalls)
    assertTrue(repository.loadActiveWeakSignalPromotions().isEmpty())
    assertEquals(PromotionPrState.CLOSED, repository.loadWeakSignalPromotionHistory("weak").single().state)
    assertTrue(promotion.checkWeakSignalReviews().reviews.isEmpty())
    assertEquals(4, provider.reviewCalls)
  }

  @Test
  fun `transient review evidence is redacted and never persisted`() {
    val repository = repository()
    cluster(repository, "weak")
    val provider = FakeReviews()
    service(repository, provider).publish("weak", digest, listOf(WeakSignalFix("src/Worker.kt", listOf(1), "fixed", "Fix waiting")), "Reviewed diff")
    provider.reviewStatus = ReviewStatus(ReviewState.OPEN, "Do not expose secret-token")
    val result = PromotionService({ repository }, configuration, provider).checkWeakSignalReviews {
      it.replace("secret-token", "[redacted]")
    }

    assertTrue(result.failures.isEmpty())
    assertEquals("Do not expose [redacted]", result.reviews.single().reviewEvidence)
    assertFalse("secret-token" in repository.loadCluster("weak").directory.manifestPath.readText())
  }

  @Test
  fun `failed review checks preserve state and transient evidence is bounded`() {
    val repository = repository()
    cluster(repository, "weak")
    val provider = FakeReviews()
    service(repository, provider).publish("weak", digest, listOf(WeakSignalFix("src/Worker.kt", listOf(1), "fixed", "Fix waiting")), "Reviewed diff")
    val promotion = PromotionService({ repository }, configuration, provider)
    provider.reviewStatus = ReviewStatus(ReviewState.OPEN, "x".repeat(40_000))
    val result = promotion.checkWeakSignalReviews()
    assertTrue(result.failures.isEmpty())
    val previous = repository.loadActiveWeakSignalPromotions()["weak"].orEmpty().single()
    assertEquals(32_000, result.reviews.single().reviewEvidence.length)
    provider.failReviews = true
    assertEquals(1, promotion.checkWeakSignalReviews().failures.size)
    assertEquals(previous, repository.loadActiveWeakSignalPromotions()["weak"].orEmpty().single())
    provider.failReviews = false
    assertTrue(promotion.checkWeakSignalReviews().failures.isEmpty())
  }

  @Test
  fun `inspection and weak-signal review checks affect only their own promotion kinds`() {
    val repository = repository()
    cluster(repository, "weak")
    cluster(repository, "other")
    val other = repository.loadCluster("other").manifest
    val provider = FakeReviews()
    val weak = service(repository, provider).publish("weak", digest,
      listOf(WeakSignalFix("src/Worker.kt", listOf(1), "fixed", "Fix waiting")), "Reviewed diff")
    val now = Instant.parse("2026-10-25T12:00:00Z")
    val inspectionPromotion = InspectionPromotion(
      "p-${"1".repeat(24)}", digest, "main", "inspections/weak.inspection.kts",
      weak.pullRequest.copy(id = "43", url = "https://github.com/owner/repo/pull/43"),
      PromotionPrState.ON_REVIEW, now.toString(), now.toString(),
    )
    repository.appendPromotion("weak", digest, inspectionPromotion)
    provider.reviewStatus = ReviewStatus(ReviewState.MERGED, "Accepted source fix")
    val promotion = PromotionService({ repository }, configuration, provider, Clock.fixed(now, ZoneOffset.UTC))

    val normalResult = promotion.checkReviews()
    assertEquals(listOf(inspectionPromotion.id), normalResult.accepted.map { it.id })
    assertEquals(1, provider.reviewCalls)
    assertEquals(weak, repository.loadActiveWeakSignalPromotions()["weak"].orEmpty().single())
    val before = repository.loadCluster("weak")
    val history = before.historyPath.readText()

    val weakResult = promotion.checkWeakSignalReviews()
    assertTrue(weakResult.failures.isEmpty())
    assertEquals("weak", weakResult.reviews.single().clusterId)
    val updated = weakResult.reviews.single().promotion
    assertEquals(weak.copy(state = PromotionPrState.ACCEPTED), updated)
    val after = repository.loadCluster("weak")
    assertEquals(before.manifest, after.manifest)
    assertEquals(before.signals, after.signals)
    assertEquals(history, after.historyPath.readText())
    assertEquals("inspection", repository.paths.inspectionPath("weak").readText())
    assertEquals(other, repository.loadCluster("other").manifest)
    assertTrue(repository.loadInboxSignals().isEmpty())
    assertEquals(2, provider.reviewCalls)
  }

  @Test
  fun `signal extraction proceeds while a weak-signal review check is in flight`() {
    val repository = repository()
    cluster(repository, "weak")
    val provider = FakeReviews()
    service(repository, provider).publish("weak", digest,
      listOf(WeakSignalFix("src/Worker.kt", listOf(1), "fixed", "Fix waiting")), "Reviewed diff")
    val checking = CountDownLatch(1)
    val release = CountDownLatch(1)
    provider.beforeReview = {
      checking.countDown()
      check(release.await(10, TimeUnit.SECONDS)) { "Review check was not released" }
    }
    val executor = Executors.newFixedThreadPool(2)
    try {
      EdictNextRepositoryState.open(repository.paths.root).use { store ->
        val layout = EdictLayout(directory, repository.paths.root)
        val promotion = PromotionService({ repository }, configuration, provider)
        val management = EdictManagementService(store, layout, reviewProvider = provider, reviewRepository = configuration.ci!!.reviewRepository)
        edictNextToolset(layout, management, configuration = configuration, promotion = promotion).createServer()
        val plan = store.createPlan("Extract signals while checking feedback",
          listOf(EdictNextRepositoryState.Step("edict-pr-signal-analysis", "Review signals")))
        val worker = store.launch(plan.token, plan.plan.tasks.single().id, "edict-pr-signal-analysis")
        val check = executor.submit<WeakSignalPromotionCheckReviewsResponse> { promotion.checkWeakSignalReviews() }
        try {
          assertTrue(checking.await(5, TimeUnit.SECONDS), "Review check must be in flight")
          val extraction = executor.submit {
            val response = management.call("edict_fetch_pr_batch", buildJsonObject {
              put("token", worker.token)
              put("prNumbers", JsonArray(listOf(JsonPrimitive(42))))
            })
            assertFalse(response.flag("isError") == true)
          }
          extraction.get(5, TimeUnit.SECONDS)
          assertEquals(1, provider.fetchCalls)
          assertFalse(check.isDone, "Extraction must not wait for review checking")
        } finally {
          release.countDown()
        }
        assertTrue(check.get(5, TimeUnit.SECONDS).failures.isEmpty())
      }
    } finally {
      release.countDown()
      executor.shutdownNow()
    }
  }

  private fun repository() = EdictRepository(EdictRepositoryDirectory(directory.resolve("state").createDirectories()))

  @Test
  fun `promotion serializes only the review reference and state and loads legacy records`() {
    val reference = ReviewReference(CiProviderId.GITHUB, "owner", "repo", "42", "https://github.com/owner/repo/pull/42")
    val old = """{"id":"old","inspectionDigest":"obsolete","projectRevision":"obsolete","findings":[],"reviewEvidence":"old feedback","lastCheckedAt":"old","pullRequest":${EdictNextJson.encodeToString(reference)},"state":"ACCEPTED"}"""
    val promotion = EdictNextJson.decodeFromString<WeakSignalPromotion>(old)
    val json = EdictNextJson.encodeToString(promotion)
    assertEquals(WeakSignalPromotion(reference, PromotionPrState.ACCEPTED), promotion)
    assertFalse(listOf("inspectionDigest", "projectRevision", "findings", "reviewEvidence", "lastCheckedAt").any { it in json })
  }

  @Test
  fun `active checks survive restarts without loading unrelated clusters or completed history`() {
    val repository = repository()
    cluster(repository, "weak")
    val provider = FakeReviews()
    val promotion = publishForReview(repository, provider)
    // Neither a broken unrelated cluster nor unreadable history is relevant to an active check.
    repository.paths.clustersDirectory.resolve("broken").createDirectories().resolve("cluster.json").writeText("not JSON")
    repository.paths.weakSignalPromotionHistoryPath("weak").parent.createDirectories()
    repository.paths.weakSignalPromotionHistoryPath("weak").writeText("not JSON")
    val result = service(EdictRepository(repository.paths), provider).checkReviews()
    assertTrue(result.failures.isEmpty())
    assertEquals(listOf(promotion.pullRequest), result.reviews.map { it.promotion.pullRequest })
    assertFalse("weakSignalPromotions" in repository.loadCluster("weak").directory.manifestPath.readText())
    assertContains(repository.paths.activeWeakSignalPromotionsPath.readText(), "activePromotions")
  }

  @Test
  fun `completed reviews are archived and no longer polled including a merged no-op`() {
    val repository = repository()
    cluster(repository, "weak")
    val provider = FakeReviews()
    val promotion = publishForReview(repository, provider)
    service(repository, provider).completeReview("weak", promotion.pullRequest)
    val calls = provider.reviewCalls
    val restarted = service(EdictRepository(repository.paths), provider)
    assertTrue(restarted.checkReviews().reviews.isEmpty())
    restarted.completeReview("weak", promotion.pullRequest)
    assertEquals(calls, provider.reviewCalls)
    assertTrue(repository.loadActiveWeakSignalPromotions().isEmpty())
    assertEquals(listOf(promotion.copy(state = PromotionPrState.ACCEPTED)), repository.loadWeakSignalPromotionHistory("weak"))
    assertEquals(1, repository.loadCluster("weak").signals.size)
  }

  @Test
  fun `legacy cluster records migrate once to standalone active state without changing other fields`() {
    val repository = repository()
    cluster(repository, "weak")
    val manifest = repository.loadCluster("weak").directory.manifestPath
    val reference = ReviewReference(CiProviderId.GITHUB, "owner", "repo", "42", "https://github.com/owner/repo/pull/42")
    val promotion = WeakSignalPromotion(reference)
    val before = manifest.readText()
    manifest.writeText(before.trimEnd().dropLast(1) + ",\"weakSignalPromotions\":[${EdictNextJson.encodeToString(promotion)}]}")
    assertEquals(listOf(promotion), repository.loadActiveWeakSignalPromotions()["weak"])
    assertFalse("weakSignalPromotions" in manifest.readText())
    assertEquals(EdictNextJson.parseToJsonElement(before), EdictNextJson.parseToJsonElement(manifest.readText()))
    assertEquals(listOf(promotion), EdictRepository(repository.paths).loadActiveWeakSignalPromotions()["weak"])
  }

  @Test
  fun `actual merged replacements go directly to source cluster and covered signals are processed across restarts`() {
    val repository = repository()
    cluster(repository, "weak")
    cluster(repository, "other")
    val provider = FakeReviews()
    val promotion = publishForReview(repository, provider)
    // The existing strong negative example has already been validated by generation.
    val negative = repository.loadCluster("weak").signals.single().copy(id = "existing-negative", label = EdictNextSignalLabel.NEGATIVE, syntheticExampleId = "negative")
    repository.loadCluster("weak").directory.signalPath(negative.id).writeText(EdictNextJson.encodeToString(negative))
    val drafts = listOf(draft(provider), draft(provider, EdictNextSignalLabel.NEGATIVE))
    val signals = service(repository, provider).recordSignals("weak", promotion.pullRequest, digest, drafts)
    val cluster = repository.loadCluster("weak")
    assertEquals(EdictNextClusterStatus.Generated, cluster.manifest.status)
    assertEquals(cluster.signalIds, cluster.manifest.processedSignalIds)
    assertTrue(runBlocking { validateCluster(cluster, "Generated fixture") }.isEmpty(), "Processed evidence must preserve Generated cluster validation")
    assertTrue(repository.loadInboxSignals().isEmpty())
    assertEquals(1, repository.loadCluster("other").signals.size)
    assertTrue(signals.all { it.strength == EdictNextSignalStrength.STRONG && it.source is EdictNextSignalSource.FromPR })
    assertEquals(listOf(provider.merged.beforeRevision, provider.merged.mergedRevision), signals.map { it.fileRevision.revision })
    assertEquals(signals, service(EdictRepository(repository.paths), provider).recordSignals("weak", promotion.pullRequest, digest, drafts))
    assertEquals(cluster, repository.loadCluster("weak"))
  }

  @Test
  fun `modified merged code not covered by the rule uses regular inbox publication without changing its source`() {
    val repository = repository()
    cluster(repository, "weak")
    val provider = FakeReviews()
    val promotion = publishForReview(repository, provider)
    val covered = draft(provider)
    service(repository, provider).recordSignals("weak", promotion.pullRequest, digest, listOf(covered))
    val beforeInbox = repository.loadCluster("weak")
    val uncovered = draft(provider, EdictNextSignalLabel.NEGATIVE).copy(syntheticExampleId = null)
    publishInbox(repository, uncovered)
    assertEquals(listOf(uncovered), repository.loadInboxSignals())
    assertEquals(beforeInbox, repository.loadCluster("weak"))
    assertEquals(EdictNextClusterStatus.Generated, beforeInbox.manifest.status)
    assertContains(beforeInbox.manifest.processedSignalIds, covered.id)
    assertFalse(uncovered.id in beforeInbox.signalIds)
    assertEquals("inspection", repository.paths.inspectionPath("weak").readText())
    service(EdictRepository(repository.paths), provider).completeReview("weak", promotion.pullRequest)
    assertTrue(repository.loadActiveWeakSignalPromotions().isEmpty())
    assertEquals(PromotionPrState.ACCEPTED, repository.loadWeakSignalPromotionHistory("weak").single().state)
  }

  @Test
  fun `proposed-only code forged excerpts stale inspection and wrong source cluster cannot publish`() {
    val repository = repository()
    cluster(repository, "weak")
    cluster(repository, "other")
    val provider = FakeReviews()
    val promotion = publishForReview(repository, provider)
    val original = repository.loadCluster("weak")
    val correct = draft(provider)
    val invalid = listOf(
      correct.copy(fileRevision = correct.fileRevision.copy(revision = "c".repeat(40))),
      correct.copy(fileRevision = correct.fileRevision.copy(path = "src/ProposedOnly.kt")),
      correct.copy(fileRevision = correct.fileRevision.copy(expectedRanges = listOf(EdictNextLineRange(2, 2)))),
      correct.copy(source = (correct.source as EdictNextSignalSource.FromPR).let { it.copy(diffPositiveToNegative = it.diffPositiveToNegative.replace("latch.await()", "proposedButNotMerged()")) }),
      correct.copy(source = (correct.source as EdictNextSignalSource.FromPR).copy(title = "Invented review title")),
      correct.copy(strength = EdictNextSignalStrength.WEAK),
    )
    invalid.forEach { signal ->
      assertFails { service(repository, provider).recordSignals("weak", promotion.pullRequest, digest, listOf(signal)) }
    }
    assertFails { service(repository, provider).recordSignals("other", promotion.pullRequest, digest, listOf(correct)) }
    assertFails { service(repository, provider).recordSignals("weak", promotion.pullRequest, sha256Hex("stale"), listOf(correct)) }
    assertFails { service(repository, provider).recordSignals("weak", promotion.pullRequest, digest, listOf(correct, correct)) }
    assertEquals(original, repository.loadCluster("weak"))
    assertTrue(repository.loadInboxSignals().isEmpty())
  }

  @Test
  fun `unavailable actual merge evidence is retryable without inventing signals`() {
    val repository = repository()
    cluster(repository, "weak")
    val provider = FakeReviews()
    val promotion = publishForReview(repository, provider)
    provider.failMerge = true
    val result = service(repository, provider).checkReviews()
    assertEquals(PromotionPrState.ACCEPTED, result.reviews.single().promotion.state)
    assertNull(result.reviews.single().mergedReview)
    assertEquals(1, result.failures.size)
    assertEquals(1, repository.loadCluster("weak").signals.size)
    assertFails { service(repository, provider).recordSignals("weak", promotion.pullRequest, digest, listOf(draft(provider))) }
    provider.failMerge = false
    assertTrue(service(EdictRepository(repository.paths), provider).checkReviews().failures.isEmpty())
    service(repository, provider).recordSignals("weak", promotion.pullRequest, digest, listOf(draft(provider)))
    assertEquals(2, repository.loadCluster("weak").signals.size)
  }

  @Test
  fun `covered evidence without an existing representative example must stay unprocessed`() {
    val repository = repository()
    cluster(repository, "weak")
    val provider = FakeReviews()
    val promotion = publishForReview(repository, provider)
    val withoutExample = draft(provider).copy(syntheticExampleId = null)
    assertFailsWith<IllegalArgumentException> {
      service(repository, provider).recordSignals("weak", promotion.pullRequest, digest, listOf(withoutExample))
    }
    val before = repository.loadCluster("weak")
    publishInbox(repository, withoutExample)
    val signal = repository.loadInboxSignals().single()
    assertNull(signal.syntheticExampleId)
    assertFalse(signal.id in repository.loadCluster("weak").manifest.processedSignalIds)
    assertEquals(before, repository.loadCluster("weak"))
  }

  @Test
  fun `unmerged reviews never produce signals and conflicting retries do not overwrite evidence`() {
    val repository = repository()
    cluster(repository, "weak")
    val provider = FakeReviews()
    val promotion = service(repository, provider).publish("weak", digest,
      listOf(WeakSignalFix("src/Worker.kt", listOf(1), "fixed", "Fix waiting")), "Reviewed diff")
    for (state in listOf(ReviewState.OPEN)) {
      provider.reviewStatus = ReviewStatus(state)
      service(repository, provider).checkReviews()
      assertFails { service(repository, provider).recordSignals("weak", promotion.pullRequest, digest, listOf(draft(provider))) }
    }
    provider.reviewStatus = ReviewStatus(ReviewState.MERGED)
    service(repository, provider).checkReviews()
    service(repository, provider).recordSignals("weak", promotion.pullRequest, digest, listOf(draft(provider)))
    val before = repository.loadCluster("weak")
    assertFails { service(repository, provider).recordSignals("weak", promotion.pullRequest, digest,
      listOf(draft(provider).copy(description = "Conflicting reinterpretation"))) }
    assertEquals(before, repository.loadCluster("weak"))
  }

  private fun publishForReview(repository: EdictRepository, provider: FakeReviews): WeakSignalPromotion {
    val promotion = service(repository, provider).publish("weak", digest,
      listOf(WeakSignalFix("src/Worker.kt", listOf(1), "proposed fix", "Fix waiting")), "Reviewed diff")
    provider.reviewStatus = ReviewStatus(ReviewState.MERGED, "Use a blocking wait")
    assertTrue(service(repository, provider).checkReviews().failures.isEmpty())
    return promotion
  }

  private fun draft(provider: FakeReviews, label: EdictNextSignalLabel = EdictNextSignalLabel.POSITIVE): EdictNextSignal {
    val workItem = "weak-promotion:GITHUB:owner/repo:42:${provider.merged.mergedRevision}"
    val revision = if (label == EdictNextSignalLabel.POSITIVE) provider.merged.beforeRevision else provider.merged.mergedRevision
    val key = "$workItem:$label:src/Worker.kt:$revision:1-1"
    return EdictNextSignal(stableSignalId(key), key, EdictNextFileRevision("src/Worker.kt", revision, listOf(EdictNextLineRange(1, 1))),
      EdictNextSignalSource.FromPR(42, provider.merged.title, provider.merged.discussionMessages, provider.canonicalDiff, "https://github.com/owner/repo/pull/42"),
      label, "Use a blocking wait instead of spinning", syntheticExampleId = if (label == EdictNextSignalLabel.POSITIVE) "positive" else "negative",
      provenance = EdictNextSignalProvenance(workItem))
  }

  private fun publishInbox(repository: EdictRepository, signal: EdictNextSignal) {
    EdictNextRepositoryState.open(repository.paths.root).use { store ->
      val layout = EdictLayout(directory, repository.paths.root)
      val management = EdictManagementService(store, layout)
      edictNextToolset(layout, management).createServer()
      val plan = store.createPlan("Check weak reviews", listOf(EdictNextRepositoryState.Step("edict-check-weak-signal-promotion", "Check")))
      val worker = store.launch(plan.token, plan.plan.tasks.single().id, "edict-check-weak-signal-promotion")
      assertFalse(management.call("edict_publish_signal", buildJsonObject {
        put("token", worker.token)
        put("signal", EdictNextJson.encodeToJsonElement(EdictNextSignal.serializer(), signal))
      }).flag("isError") == true)
    }
  }

  private fun service(repository: EdictRepository, reviews: FakeReviews = FakeReviews()) =
    WeakSignalPromotionService({ repository }, configuration, reviews)

  private fun finding(line: Int) = EdictNextProjectFinding(
    EdictNextFileRevision("src/Worker.kt", revision, listOf(EdictNextLineRange(line, line))), "Busy waiting",
  )

  private fun cluster(
    repository: EdictRepository,
    id: String,
    strongSignals: Int = 1,
    references: Boolean = true,
    status: EdictNextClusterStatus = EdictNextClusterStatus.Generated,
    findings: List<EdictNextProjectFinding> = (1..3).map(::finding),
    review: EdictNextProjectReview? = EdictNextProjectReview(revision, findings.size, findings.size, 0, 0, 0, "Avoid busy waiting", findings),
    evaluationDigest: String = digest,
    sourceFiles: Int = strongSignals,
    negativeSignals: Int = 0,
  ) {
    val root = repository.paths.clustersDirectory.resolve(id).createDirectories()
    root.resolve("cluster.json").writeText(EdictNextJson.encodeToString(EdictNextClusterManifest(id, EdictNextLanguage.Kotlin, status)))
    root.resolve("signals").createDirectories()
    repeat(strongSignals) { index ->
      val path = if (index % sourceFiles == 0) "src/Worker.kt" else "src/Worker${index % sourceFiles}.kt"
      val label = if (index < negativeSignals) EdictNextSignalLabel.NEGATIVE else EdictNextSignalLabel.POSITIVE
      val signal = EdictNextSignal("signal-$index", fileRevision = EdictNextFileRevision(path, revision),
        source = EdictNextSignalSource.FromCommit(revision, if (references) "https://example.com/commit/$revision" else null),
        label = label, description = "Avoid busy waiting", syntheticExampleId = if (label == EdictNextSignalLabel.POSITIVE) "positive" else "negative")
      root.resolve("signals/signal-$index.json").writeText(EdictNextJson.encodeToString(signal))
    }
    listOf(EdictNextSignalLabel.POSITIVE, EdictNextSignalLabel.NEGATIVE).forEach { label ->
      val name = if (label == EdictNextSignalLabel.POSITIVE) "positive" else "negative"
      val example = root.resolve("synthetic-examples/$name").createDirectories()
      example.resolve("metadata.json").writeText(EdictNextJson.encodeToString(EdictNextCodeExampleMetadata(name, "Example.kt", label,
        if (label == EdictNextSignalLabel.POSITIVE) listOf(EdictNextLineRange(1, 1)) else null)))
      example.resolve("project").createDirectories().resolve("Example.kt").writeText("fun example() {}\n")
    }
    root.resolve("evaluation.json").writeText(EdictNextJson.encodeToString(
      EdictNextEvaluation(evaluationDigest, "examples", EdictNextInspectionAction.GENERATE, findings.size, 0, 0, 1.0, 1.0, emptyMap(), review),
    ))
    repository.paths.inspectionsDirectory.createDirectories()
    repository.paths.inspectionPath(id).writeText("inspection")
  }

  private class FakeReviews : ReviewManagementApi, ReviewExtractionApi {
    val requests = mutableListOf<CreateReviewRequest>()
    var fail = false
    var failAfterCreation = false
    var failReviews = false
    var reviewStatus = ReviewStatus(ReviewState.OPEN)
    var reviewCalls = 0
    var fetchCalls = 0
    var beforeReview: () -> Unit = {}
    var failMerge = false
    var merged = MergedReview("Fix busy waiting", listOf("Use a blocking wait"), "a".repeat(40), "b".repeat(40),
      listOf(ReviewFileChange("src/Worker.kt", "src/Worker.kt")))
    var canonicalDiff = "--- a/src/Worker.kt\n+++ b/src/Worker.kt\n@@ -1 +1 @@\n-while (!done) {}\n+latch.await()\n"
    override fun resolveTargetBranch(repository: ReviewRepository, configured: String?) = configured ?: "main"
    override fun ensureReview(request: CreateReviewRequest): CreatedReview {
      if (fail) error("Provider unavailable")
      requests += request
      if (failAfterCreation) error("Provider response lost after branch creation")
      return CreatedReview("42", "https://github.com/owner/repo/pull/42")
    }
    override fun review(reference: ReviewReference): ReviewStatus {
      beforeReview()
      reviewCalls++
      if (failReviews) error("Review unavailable")
      return reviewStatus
    }
    override fun fetch(selection: ReviewSelection): ReviewFetchResult {
      fetchCalls++
      return ReviewFetchResult(emptyList())
    }
    override fun mergedReview(reference: ReviewReference): MergedReview {
      if (failMerge) error("Exact merge evidence unavailable")
      check(reviewStatus.state == ReviewState.MERGED)
      return merged
    }
    override fun file(repository: ReviewRepository, revision: String, path: String): String =
      if (revision == merged.beforeRevision) "while (!done) {}\n" else "latch.await()\n"
    override fun diff(repository: ReviewRepository, before: String, after: String, beforePath: String, afterPath: String, context: Int): String = canonicalDiff
  }
}
