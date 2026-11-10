package org.jetbrains.qodana.edict.core

import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.encodeToJsonElement
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.jetbrains.qodana.edict.signals.SignalValidation
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import kotlin.io.path.createDirectories
import kotlin.io.path.exists
import kotlin.io.path.readText
import kotlin.io.path.writeText
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertIs
import kotlin.test.assertTrue

class EdictRepositoryMutationTest {
  @TempDir
  lateinit var directory: Path

  @Test
  fun `canonical signal reads existing submitted feedback`() {
    val content = checkNotNull(javaClass.getResource("/signals/submitted-feedback.json")).readText()
    val signal = SignalValidation.validate("inbox/s-0b1840a3d440.json", content)

    assertIs<EdictSignalSource.SubmittedFeedback>(signal.source)
    assertEquals("benchmark/StaticInitializerReferencesSubClass/specification.json#negativeExamples/0", signal.deduplicationKey)
  }

  @Test
  fun `renamed signal variants retain their wire discriminator`() {
    val signal = EdictSignal(
      id = "s-652d0c8f933a",
      fileRevision = EdictFileRevision("ProcessTree.java", "abc", listOf(EdictLineRange(1, 1))),
      source = EdictSignalSource.SubmittedFeedback(),
      label = EdictSignalLabel.POSITIVE,
      description = "Thread.sleep in a loop",
    )
    val context = EdictJson.encodeToJsonElement<EdictDistributionContextResponse>(
      EdictDistributionContextResponse.SignalContext(signal),
    ).jsonObject
    val candidate = EdictJson.encodeToJsonElement<EdictSignalCandidate>(
      EdictSignalCandidate.SignalCandidate(signal.id, "/inbox/${signal.id}.json", 0.25, signal.label, signal.fileRevision.path, signal.description),
    ).jsonObject

    assertEquals("signal", context.getValue("type").jsonPrimitive.content)
    assertEquals(signal.id, context.getValue("signal").jsonObject.getValue("id").jsonPrimitive.content)
    assertEquals("signal", candidate.getValue("type").jsonPrimitive.content)
  }

  @Test
  fun `new evidence snapshots legacy Generated membership before reopening the cluster`() {
    val repository = EdictRepository(EdictRepositoryDirectory(directory.resolve("state").createDirectories()))
    val signal = EdictSignal(
      id = "s-original",
      fileRevision = EdictFileRevision("Example.java", "abc"),
      source = EdictSignalSource.SubmittedFeedback(),
      label = EdictSignalLabel.POSITIVE,
      description = "Original evidence",
    )
    val clusterDirectory = repository.paths.clustersDirectory.resolve("legacy").createDirectories()
    clusterDirectory.resolve("cluster.json").writeText(
      """{"id":"legacy","language":"Java","status":"Generated"}""",
    )
    clusterDirectory.resolve("signals").createDirectories().resolve("${signal.id}.json")
      .writeText(EdictJson.encodeToString(signal))
    repository.paths.inspectionPath("legacy").also {
      it.parent.createDirectories()
      it.writeText("// inspection")
    }
    val inbox = repository.paths.inboxDirectory.createDirectories()
    for (id in listOf("s-new", "s-later")) {
      inbox.resolve("$id.json").writeText(EdictJson.encodeToString(signal.copy(id = id)))
      repository.addSignalToCluster(id, "legacy")
      val reloaded = EdictRepository(repository.paths).loadCluster("legacy")
      assertEquals(EdictClusterStatus.Pending, reloaded.manifest.status)
      assertEquals("legacy", reloaded.manifest.predecessorId)
      assertEquals(setOf(signal.id), reloaded.manifest.processedSignalIds)
    }
  }

  @Test
  fun `managed generation mutations persist examples assignments candidates and history`() {
    val clusterId = "busy-wait"
    val signalId = "s-652d0c8f933a"
    val root = directory.resolve("state").createDirectories()
    val cluster = root.resolve("clusters/$clusterId").createDirectories()
    cluster.resolve("signals").createDirectories()
    cluster.resolve("cluster.json").writeText(
      EdictJson.encodeToString(
        EdictClusterManifest(clusterId, EdictLanguage.Java, EdictClusterStatus.Pending),
      ),
    )
    cluster.resolve("history.md").writeText("Created.\n")
    val signalPath = cluster.resolve("signals/$signalId.json")
    val serializedSignal = EdictJson.encodeToString(
      EdictSignal(
        id = signalId,
        fileRevision = EdictFileRevision("ProcessTree.java", "abc", listOf(EdictLineRange(1, 1))),
        source = EdictSignalSource.SubmittedFeedback(),
        label = EdictSignalLabel.POSITIVE,
        description = "Thread.sleep in a loop",
      ),
    )
    signalPath.writeText(
      serializedSignal.replaceFirst(
        "{",
        """{
          |    "idempotencyKey": "repository:work-item:positive",
          |    "provenance": { "workItemId": "work-item" },
          |""".trimMargin(),
      ),
    )

    val repository = EdictRepository(EdictRepositoryDirectory(root))
    val metadata = EdictCodeExampleMetadata(
      id = "busy-wait-positive",
      fileName = "BusyWait.java",
      label = EdictSignalLabel.POSITIVE,
      expectedRanges = listOf(EdictLineRange(4, 4)),
    )
    repository.saveCodeExample(clusterId, metadata.id, metadata, "class BusyWait { void f() throws Exception { while (true) Thread.sleep(1); } }")
    repository.assignCodeExample(clusterId, signalId, metadata.id)
    repository.saveCandidateInspection(clusterId, "val candidate = true")
    repository.appendHistory(clusterId, "Candidate stored.")

    val loaded = repository.loadCluster(clusterId)
    assertEquals(metadata.id, loaded.signals.single().syntheticExampleId)
    val assignedSignal = EdictJson.parseToJsonElement(signalPath.readText()).jsonObject
    assertEquals("repository:work-item:positive", assignedSignal.getValue("idempotencyKey").jsonPrimitive.content)
    assertEquals("work-item", assignedSignal.getValue("provenance").jsonObject.getValue("workItemId").jsonPrimitive.content)
    assertEquals(metadata.id, loaded.examples.single().metadata.id)
    assertEquals("val candidate = true", loaded.candidateInspectionPath.readText())
    assertTrue(cluster.resolve("history.md").readText().endsWith("Candidate stored.\n"))

    val unused = metadata.copy(id = "unused-positive", fileName = "Unused.java")
    repository.saveCodeExample(clusterId, unused.id, unused, "class Unused {}")
    repository.deleteCodeExample(clusterId, unused.id)
    assertFalse(cluster.resolve("synthetic-examples/${unused.id}").exists())

    // Deleting an assigned example unassigns its Signal and keeps every other Signal field.
    repository.deleteCodeExample(clusterId, metadata.id)
    assertFalse(cluster.resolve("synthetic-examples/${metadata.id}").exists())
    val unassignedSignal = EdictJson.parseToJsonElement(signalPath.readText()).jsonObject
    assertEquals(assignedSignal - "syntheticExampleId", unassignedSignal)
  }
}
