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
import org.jetbrains.qodana.edict.edictnext.EdictNextJson
import org.jetbrains.qodana.edict.edictnext.EdictNextLanguage
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
    cluster(repository, "six-weak", 0, weakSignals = 6)
    cluster(repository, "pending-signals", 6, EdictNextClusterStatus.Pending)
    val provider = FakeProvider()

    val result = service(repository, provider).promote()

    assertEquals(
      setOf("five-strong", "six-strong"),
      result.created.map { it.targetPath.substringAfterLast('/').removeSuffix(".inspection.kts") }.toSet(),
    )
    assertEquals(2, provider.ensureCalls)
    assertTrue(result.failures.isEmpty())
    for (id in listOf("five-strong", "six-strong")) {
      assertEquals(PromotionPrState.ON_REVIEW, repository.loadCluster(id).manifest.promotions.single().state)
    }
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
      val result = service(repository, provider).promote()

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

    val result = service(repository, provider, PromotionConfiguration("reviewer")).promote()

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

      val result = service(repository, provider).promote()

      assertTrue(result.created.isEmpty())
      assertTrue(result.failures.isEmpty())
      assertEquals(0, provider.ensureCalls)
      assertEquals(1, repository.loadCluster("same-version").manifest.promotions.size)
    }

    val repository = repository("new-version")
    cluster(repository, "changed-version", 6, promotions = listOf(promotion("p-${"a".repeat(24)}", sha256Hex("old"), PromotionPrState.ACCEPTED)))
    val provider = FakeProvider()
    val result = service(repository, provider).promote()
    assertEquals(1, result.created.size)
    assertEquals(2, repository.loadCluster("changed-version").manifest.promotions.size)
  }

  @Test
  fun `provider retry creates one persisted record`() {
    val repository = repository()
    cluster(repository, "retry-rule", 6)
    val provider = FakeProvider(failOnceAfterRemoteCreation = true)

    assertEquals(1, service(repository, provider).promote().failures.size)
    assertTrue(repository.loadCluster("retry-rule").manifest.promotions.isEmpty())
    assertEquals(1, service(repository, provider).promote().created.size)
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
          fileRevision = EdictNextFileRevision("Example.kt", "revision"),
          source = EdictNextSignalSource.Generated(),
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
