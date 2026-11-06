// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.feedback

import org.jetbrains.qodana.edict.common.DEFAULT_EDICT_STATE_BRANCH
import org.jetbrains.qodana.edict.common.EdictLayout
import org.jetbrains.qodana.edict.common.support.GitOrigin
import org.jetbrains.qodana.edict.feedback.support.PULL_REQUEST
import org.jetbrains.qodana.edict.promotion.support.ClusterFixture
import org.jetbrains.qodana.edict.promotion.support.EvaluationFixture
import org.jetbrains.qodana.edict.promotion.support.INSPECTION
import org.jetbrains.qodana.edict.promotion.support.promotionRepository
import org.jetbrains.qodana.edict.promotion.support.store
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import java.nio.file.Path
import kotlin.io.path.createDirectories
import kotlin.io.path.createParentDirectories
import kotlin.io.path.exists
import kotlin.io.path.isRegularFile
import kotlin.io.path.listDirectoryEntries
import kotlin.io.path.name
import kotlin.io.path.readText
import kotlin.io.path.relativeTo
import kotlin.io.path.walk
import kotlin.io.path.writeText
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse

class FeedbackPreparationTest {
  @TempDir
  lateinit var directory: Path

  private val origin by lazy { GitOrigin(directory) }

  @Test
  fun `copies the selected inspections of the state on origin into the output directory`() {
    origin.publishState("routine", stateFiles(ClusterFixture(emptyList())))

    prepare(ActionsPullRequest.Own(PULL_REQUEST))

    assertEquals(mapOf("rule.inspection.kts" to INSPECTION), outputDirectory.listDirectoryEntries().associate { it.name to it.readText() })
  }

  @Test
  fun `copies into an output directory that exists and is empty`() {
    origin.publishState("routine", stateFiles(ClusterFixture(emptyList())))
    outputDirectory.createDirectories()

    prepare(ActionsPullRequest.Own(PULL_REQUEST))

    assertEquals(mapOf("rule.inspection.kts" to INSPECTION), outputDirectory.listDirectoryEntries().associate { it.name to it.readText() })
  }

  @Test
  fun `copies nothing when no inspection is selected`() {
    origin.publishState("routine", stateFiles(ClusterFixture(emptyList(), evaluation = EvaluationFixture(precision = 0.5))))

    prepare(ActionsPullRequest.Own(PULL_REQUEST))

    assertFalse(outputDirectory.exists())
  }

  @Test
  fun `refuses an output directory inside the project`() {
    val project = origin.clone("ci")
    val output = project.resolve("feedback")

    val failure = assertFailsWith<IllegalArgumentException> {
      prepareFeedback(EdictLayout(project, stateDirectory), DEFAULT_EDICT_STATE_BRANCH, ActionsPullRequest.Own(PULL_REQUEST), output)
    }

    assertEquals("The directory for the feedback inspections must be outside the project: $output", failure.message)
  }

  @Test
  fun `refuses an output directory that already holds files`() {
    outputDirectory.resolve("stale.inspection.kts").createParentDirectories().writeText(INSPECTION)

    val failure = assertFailsWith<IllegalArgumentException> { prepare(ActionsPullRequest.Own(PULL_REQUEST)) }

    assertEquals("The directory for the feedback inspections is not empty: $outputDirectory", failure.message)
  }

  @Test
  fun `leaves a pull request from a fork alone, without checking the state out`() {
    origin.publishState("routine", stateFiles(ClusterFixture(emptyList())))

    prepare(ActionsPullRequest.FromFork(PULL_REQUEST))

    assertFalse(stateDirectory.exists())
    assertFalse(outputDirectory.exists())
  }

  @Test
  fun `leaves a run for no pull request alone, without checking the state out`() {
    origin.publishState("routine", stateFiles(ClusterFixture(emptyList())))

    prepare(ActionsPullRequest.None)

    assertFalse(stateDirectory.exists())
    assertFalse(outputDirectory.exists())
  }

  /** The state directory, outside the project, as a CI run puts it. */
  private val stateDirectory: Path by lazy { directory.resolve("state") }

  /** The directory the inspections to run for feedback go to, outside the project as well. */
  private val outputDirectory: Path by lazy { directory.resolve("feedback-inspections") }

  /** Prepares the feedback of [event] in a fresh clone of origin. */
  private fun prepare(event: ActionsPullRequest) {
    prepareFeedback(EdictLayout(origin.clone("ci"), stateDirectory), DEFAULT_EDICT_STATE_BRANCH, event, outputDirectory)
  }

  /** The files of a state holding [cluster], contents by their path in the state. */
  private fun stateFiles(cluster: ClusterFixture): Map<String, String> {
    val state = promotionRepository(directory.resolve("routine-state")).apply { store(cluster) }.paths.root
    return state.walk().filter(Path::isRegularFile).associate { it.relativeTo(state).toString() to it.readText() }
  }
}
