package org.jetbrains.qodana.edict.edictnext

import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import kotlin.test.Test
import kotlin.test.assertEquals

class EdictNextDistributionServiceTest {
  @TempDir
  lateinit var directory: Path

  @Test
  fun `submitted feedback with the same inspection identity reuses its cluster`() {
    val incoming = signal(
      id = "s-positive",
      inspectionName = "StaticInitializerReferencesSubClass",
      path = "Positive.java",
      label = EdictNextSignalLabel.POSITIVE,
    )
    val matching = cluster(
      "static-initializer-references-subclass",
      EdictNextLanguage.Java,
      signal(
        id = "s-negative",
        inspectionName = "StaticInitializerReferencesSubClass",
        path = "Negative.java",
        label = EdictNextSignalLabel.NEGATIVE,
      ),
    )
    val otherInspection = cluster(
      "other-inspection",
      EdictNextLanguage.Java,
      signal("s-other", "OtherInspection", "Other.java", EdictNextSignalLabel.POSITIVE),
    )
    val otherLanguage = cluster(
      "same-name-kotlin",
      EdictNextLanguage.Kotlin,
      signal("s-kotlin", "StaticInitializerReferencesSubClass", "Same.kt", EdictNextSignalLabel.POSITIVE),
    )

    assertEquals(
      listOf("static-initializer-references-subclass"),
      clustersSharingInspectionIdentity(incoming, listOf(otherInspection, otherLanguage, matching)),
    )
  }

  @Test
  fun `blank submitted inspection identity does not force cluster reuse`() {
    val incoming = signal("s-new", "", "New.java", EdictNextSignalLabel.POSITIVE)
    val cluster = cluster(
      "unnamed",
      EdictNextLanguage.Java,
      signal("s-existing", "", "Existing.java", EdictNextSignalLabel.NEGATIVE),
    )

    assertEquals(emptyList(), clustersSharingInspectionIdentity(incoming, listOf(cluster)))
  }

  private fun signal(
      id: String,
      inspectionName: String?,
      path: String,
      label: EdictNextSignalLabel,
  ) = EdictNextSignal(
    id = id,
    fileRevision = EdictNextFileRevision(path, "revision"),
    source = EdictNextSignalSource.SubmittedFeedback(inspectionName = inspectionName),
    label = label,
    description = "$label benchmark example",
  )

  private fun cluster(
      id: String,
      language: EdictNextLanguage,
      vararg signals: EdictNextSignal,
  ) = EdictNextStoredCluster(
    directory = EdictNextClusterDirectory(directory.resolve(id)),
    manifest = EdictNextClusterManifest(id, language, EdictNextClusterStatus.Pending),
    signals = signals.toList(),
    examples = emptyList(),
    candidateInspectionPath = directory.resolve("$id.inspection.kts"),
  )
}
