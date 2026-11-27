package org.jetbrains.qodana.edict.history.enrichment

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.async
import kotlinx.coroutines.runBlocking
import org.jetbrains.qodana.edict.common.GitRepository
import org.jetbrains.qodana.edict.common.runProcess
import org.jetbrains.qodana.edict.core.EdictClusterStatus
import org.jetbrains.qodana.edict.core.EdictJson
import org.jetbrains.qodana.edict.core.EdictRepository
import org.jetbrains.qodana.edict.core.EdictRepositoryDirectory
import org.jetbrains.qodana.edict.support.fixtureSignals
import org.jetbrains.qodana.edict.support.gitFixture
import org.jetbrains.qodana.edict.support.revisions
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import java.time.Clock
import java.time.Duration
import java.time.Instant
import java.time.ZoneOffset
import java.util.concurrent.TimeUnit
import kotlin.io.path.bufferedWriter
import kotlin.io.path.createDirectories
import kotlin.io.path.exists
import kotlin.io.path.readLines
import kotlin.io.path.readText
import kotlin.io.path.writeText
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class EdictEnrichmentServiceTest {
  @TempDir
  lateinit var directory: Path

  /** The managed task's scratch, where the history range writes its revisions. */
  private val scratch: Path by lazy { directory.resolve("scratch") }

  @Test
  fun `standalone enrichment returns a range and publishes without persisting analysis state`() = runBlocking<Unit> {
    val (enrichment, repository, project) = fixture()
    val seed = repository.loadCluster("busy-wait").signals.single()
    val signal = fixtureSignals(project).last()
    val context = enrichment.getHistoryContext("busy-wait", scratch)
    val revisions = context.range.revisions()
    assertEquals("busy-wait", context.clusterId)
    assertEquals(project.resolve("HEAD"), context.range.headRevision)
    assertEquals(project.git("rev-list", "--topo-order", "HEAD").trim().lines(), revisions)
    assertEquals(revisions.size, context.range.revisionCount)
    assertEquals(scratch.resolve("history/busy-wait.revisions").toString(), context.range.revisionsFile)
    assertTrue(enrichment.publishClusterSignal("busy-wait", signal).created)
    assertFalse(enrichment.publishClusterSignal("busy-wait", signal).created)
    assertEquals(setOf(seed.id, signal.id), repository.clusterSignalIds("busy-wait"))
    assertTrue(repository.loadInboxSignals().isEmpty())

    assertFalse(repository.loadCluster("busy-wait").directory.root.resolve("history-analysis.json").exists())
    // Legacy files are neither read nor changed, and cannot suppress candidates after restart.
    val legacy = repository.loadCluster("busy-wait").directory.root.resolve("history-analysis.json")
    legacy.writeText("obsolete coverage is ignored")
    val restartedRepository = EdictRepository(repository.paths)
    val restarted = EdictEnrichmentService(restartedRepository, project.root)
    assertEquals(revisions, restarted.getHistoryContext("busy-wait", scratch).range.revisions())
    assertEquals("obsolete coverage is ignored", legacy.readText())
    assertFalse(repository.loadCluster("busy-wait").candidateInspectionPath.exists())
    assertFalse(repository.loadCluster("busy-wait").directory.evaluationPath.exists())
  }

  @Test
  fun `commit depth wins when the date window contains more commits`() = runBlocking<Unit> {
    val now = Instant.now().plus(Duration.ofDays(3650))
    val (enrichment, _, project) = fixture(clock = Clock.fixed(now, ZoneOffset.UTC))
    val revisions = commitsAt(project, now.minusSeconds(1), HISTORY_ANALYSIS_COMMIT_DEPTH + 3)
    val range = enrichment.getHistoryContext("busy-wait", scratch).range
    val window = range.revisions()
    assertEquals(revisions.takeLast(HISTORY_ANALYSIS_COMMIT_DEPTH).reversed(), window)
    assertEquals(revisions.last(), range.headRevision)
    assertEquals(now.minus(Duration.ofDays(HISTORY_ANALYSIS_DAY_DEPTH)).toString(), range.since)
    // A rare pickaxe hit outside the last N commits must not leak into the bounded search.
    assertTrue(project.git("log", "--no-walk=unsorted", "--format=%H", "-S", "Objects.equals", *window.toTypedArray(), "--").isBlank())
  }

  @Test
  fun `day depth wins when it contains fewer commits and includes the exact cutoff`() = runBlocking<Unit> {
    val now = Instant.parse("2040-01-01T12:00:00Z")
    val (enrichment, repository, project) = fixture(clock = Clock.fixed(now, ZoneOffset.UTC))
    val since = now.minus(Duration.ofDays(HISTORY_ANALYSIS_DAY_DEPTH))
    commitAt(project, since.minusSeconds(1))
    val boundary = commitAt(project, since)
    val recent = commitAt(project, now.minusSeconds(1))
    assertEquals(listOf(recent, boundary), enrichment.getHistoryContext("busy-wait", scratch).range.revisions())
    assertFalse(repository.loadCluster("busy-wait").directory.root.resolve("history-analysis.json").exists())
  }

  @Test
  fun `old history produces an empty range without hiding newer ancestors behind clock skew`() = runBlocking<Unit> {
    val now = Instant.parse("2040-01-01T12:00:00Z")
    val (enrichment, _, project) = fixture(clock = Clock.fixed(now, ZoneOffset.UTC))
    val empty = enrichment.getHistoryContext("busy-wait", scratch).range
    assertEquals(0, empty.revisionCount)
    assertEquals(emptyList(), empty.revisions())
    val recent = commitAt(project, now.minusSeconds(1))
    val oldHead = commitAt(project, now.minus(Duration.ofDays(HISTORY_ANALYSIS_DAY_DEPTH + 1)))
    val range = enrichment.getHistoryContext("busy-wait", scratch).range
    assertEquals(oldHead, range.headRevision)
    assertEquals(listOf(recent), range.revisions())
  }

  @Test
  fun `range includes eligible merged branch commits without assuming linear history`() = runBlocking<Unit> {
    val now = Instant.parse("2040-01-01T12:00:00Z")
    val (enrichment, _, project) = fixture(clock = Clock.fixed(now, ZoneOffset.UTC))
    val main = project.git("branch", "--show-current").trim()
    project.git("checkout", "-qb", "history-topic")
    val topic = commitAt(project, now.minusSeconds(3))
    project.git("checkout", main)
    val sibling = commitAt(project, now.minusSeconds(2))
    runProcess(project.root, listOf("git", "merge", "--no-ff", "-qm", "Merge history topic", "history-topic"), environment = mapOf(
      "GIT_AUTHOR_DATE" to now.toString(), "GIT_COMMITTER_DATE" to now.toString(),
    ))
    val range = enrichment.getHistoryContext("busy-wait", scratch).range
    val revisions = range.revisions()
    assertEquals(setOf(project.resolve("HEAD"), topic, sibling), revisions.toSet())
    assertEquals(range.headRevision, revisions.first())
  }

  @Test
  fun `enrichment requires an existing Pending cluster`() = runBlocking<Unit> {
    val (enrichment, repository, project) = fixture()
    assertFailsWith<IllegalArgumentException> { enrichment.getHistoryContext("missing", scratch) }
    assertFailsWith<IllegalArgumentException> { enrichment.getHistoryContext("../busy-wait", scratch) }
    val cluster = repository.loadCluster("busy-wait")
    cluster.directory.manifestPath.writeText(EdictJson.encodeToString(cluster.manifest.copy(status = EdictClusterStatus.Generated)))
    assertFailsWith<IllegalArgumentException> { enrichment.getHistoryContext(cluster.id, scratch) }
    assertFailsWith<IllegalArgumentException> { enrichment.publishClusterSignal(cluster.id, fixtureSignals(project).last()) }
  }

  @Test
  fun `concurrent identical publications store a Signal once`() = runBlocking<Unit> {
    val (enrichment, repository, project) = fixture()
    val cluster = repository.loadCluster("busy-wait")
    val signal = fixtureSignals(project).last()
    val publications = List(2) {
      async(Dispatchers.Default) { enrichment.publishClusterSignal(cluster.id, signal) }
    }
    assertEquals(listOf(false, true), publications.map { it.await().created }.sorted())
    assertEquals(cluster.signalIds + signal.id, repository.clusterSignalIds(cluster.id))
    assertEquals(cluster.id, enrichment.getHistoryContext(cluster.id, scratch).clusterId)
  }

  private fun commitAt(project: GitRepository, instant: Instant): String {
    runProcess(project.root, listOf("git", "commit", "--allow-empty", "-qm", "History candidate"), environment = mapOf(
      "GIT_AUTHOR_DATE" to instant.toString(), "GIT_COMMITTER_DATE" to instant.toString(),
    ))
    return project.resolve("HEAD")
  }

  /**
   * Appends [count] empty commits dated [instant] to the current branch and returns them oldest first. One `git fast-import`
   * writes them all: a `git commit` per commit takes minutes for the thousands that the commit depth needs.
   */
  private fun commitsAt(project: GitRepository, instant: Instant, count: Int): List<String> {
    val branch = project.git("symbolic-ref", "HEAD").trim()
    val parent = project.resolve("HEAD")
    val message = "History candidate"
    val stream = directory.resolve("history.fast-import")
    stream.bufferedWriter().use { out ->
      for (mark in 1..count) {
        out.write("commit $branch\nmark :$mark\n")
        out.write("committer Edict Test <edict-test@example.invalid> ${instant.epochSecond} +0000\n")
        out.write("data ${message.length}\n$message\n")
        if (mark == 1) out.write("from $parent\n")
        out.write("\n")
      }
    }
    val marks = directory.resolve("history.marks")
    val log = directory.resolve("history.log")
    val process = ProcessBuilder("git", "fast-import", "--quiet", "--export-marks=$marks")
      .directory(project.root.toFile())
      .redirectInput(stream.toFile())
      .redirectErrorStream(true)
      .redirectOutput(log.toFile())
      .start()
    check(process.waitFor(60, TimeUnit.SECONDS)) { process.destroyForcibly(); "git fast-import timed out" }
    check(process.exitValue() == 0) { "git fast-import exited ${process.exitValue()}: ${log.readText()}" }
    // Each line is ":<mark> <commit>"; the marks number the commits in the order they were written.
    return marks.readLines().map { it.split(' ') }.sortedBy { it[0].removePrefix(":").toInt() }.map { it[1] }
      .also { check(it.size == count && it.last() == project.resolve("HEAD")) { "git fast-import did not append $count commits" } }
  }

  private fun fixture(
    clock: Clock = Clock.systemUTC(),
  ): Triple<EdictEnrichmentService, EdictRepository, GitRepository> {
    val project = gitFixture(directory.resolve("project"))
    val repository = EdictRepository(EdictRepositoryDirectory(directory.resolve("state").createDirectories()))
    runProcess(repository.paths.root, listOf("git", "init", "--quiet"))
    val signal = fixtureSignals(project).first()
    repository.paths.inboxDirectory.createDirectories().resolve("${signal.id}.json").writeText(EdictJson.encodeToString(signal))
    repository.addSignalToCluster(signal.id, "busy-wait")
    return Triple(EdictEnrichmentService(repository, project.root, clock), repository, project)
  }
}
