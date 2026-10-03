package org.jetbrains.qodana.edict.edictnext

import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import kotlin.io.path.createDirectories
import kotlin.io.path.exists
import kotlin.io.path.readText
import kotlin.io.path.writeText
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class EdictRepositoryMutationTest {
  @TempDir
  lateinit var directory: Path

  @Test
  fun `managed generation mutations persist examples assignments candidates and history`() {
    val clusterId = "busy-wait"
    val signalId = "s-652d0c8f933a"
    val root = directory.resolve("state").createDirectories()
    val cluster = root.resolve("clusters/$clusterId").createDirectories()
    cluster.resolve("signals").createDirectories()
    cluster.resolve("cluster.json").writeText(
      EdictNextJson.encodeToString(
        EdictNextClusterManifest(clusterId, EdictNextLanguage.Java, EdictNextClusterStatus.Pending),
      ),
    )
    cluster.resolve("history.md").writeText("Created.\n")
    val signalPath = cluster.resolve("signals/$signalId.json")
    val serializedSignal = EdictNextJson.encodeToString(
      EdictNextSignal(
        id = signalId,
        fileRevision = EdictNextFileRevision("ProcessTree.java", "abc", listOf(EdictNextLineRange(1, 1))),
        source = EdictNextSignalSource.SubmittedFeedback(),
        label = EdictNextSignalLabel.POSITIVE,
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
    val metadata = EdictNextCodeExampleMetadata(
      id = "busy-wait-positive",
      fileName = "BusyWait.java",
      label = EdictNextSignalLabel.POSITIVE,
      expectedRanges = listOf(EdictNextLineRange(4, 4)),
    )
    repository.saveCodeExample(clusterId, metadata.id, metadata, "class BusyWait { void f() throws Exception { while (true) Thread.sleep(1); } }")
    repository.assignCodeExample(clusterId, signalId, metadata.id)
    repository.saveCandidateInspection(clusterId, "val candidate = true")
    repository.appendHistory(clusterId, "Candidate stored.")

    val loaded = repository.loadCluster(clusterId)
    assertEquals(metadata.id, loaded.signals.single().syntheticExampleId)
    val assignedSignal = EdictNextJson.parseToJsonElement(signalPath.readText()).jsonObject
    assertEquals("repository:work-item:positive", assignedSignal.getValue("idempotencyKey").jsonPrimitive.content)
    assertEquals("work-item", assignedSignal.getValue("provenance").jsonObject.getValue("workItemId").jsonPrimitive.content)
    assertEquals(metadata.id, loaded.examples.single().metadata.id)
    assertEquals("val candidate = true", loaded.candidateInspectionPath.readText())
    assertTrue(cluster.resolve("history.md").readText().endsWith("Candidate stored.\n"))

    val unused = metadata.copy(id = "unused-positive", fileName = "Unused.java")
    repository.saveCodeExample(clusterId, unused.id, unused, "class Unused {}")
    repository.deleteCodeExample(clusterId, unused.id)
    assertFalse(cluster.resolve("synthetic-examples/${unused.id}").exists())
  }
}
