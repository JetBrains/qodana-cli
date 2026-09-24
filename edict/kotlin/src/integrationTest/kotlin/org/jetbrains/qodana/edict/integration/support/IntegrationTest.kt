// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.
package org.jetbrains.qodana.edict.integration.support

import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Tag
import org.junit.jupiter.api.TestInfo
import java.nio.file.Files
import java.nio.file.Path

/** Inherited by every integration test, including scripted MCP, provider HTTP fixtures and the installed CLI. */
@Tag("integration")
abstract class IntegrationTest {
    internal lateinit var workspace: IntegrationWorkspace
    protected val directory: Path get() = workspace.output
    protected open val fixtureRevision: String = historyCommit
    protected open val fixtureProject: String = historyProject

    @BeforeEach
    fun prepareIntegration(test: TestInfo) {
        val configured = System.getProperty("edict.integrationOutput")
        val output = if (configured != null) Path.of(configured) else {
            val cwd = Path.of("").toAbsolutePath()
            val project = generateSequence(cwd) { it.parent }.flatMap { sequenceOf(it, it.resolve("edict/kotlin")) }
                .firstOrNull { Files.isRegularFile(it.resolve("build.gradle.kts")) && Files.isDirectory(it.resolve("src/main/resources/skills")) }
                ?: error("Cannot locate Kotlin subproject; set edict.integrationOutput")
            project.resolve("out/integration")
        }
        workspace = IntegrationWorkspace.open(
            output, test.testClass.orElseThrow().simpleName, test.testMethod.orElseThrow().name,
            revision = fixtureRevision, projectPath = fixtureProject
        )
        println("Integration artifacts: ${workspace.output}")
    }

    @AfterEach
    fun retainIntegrationArtifacts() {
        if (::workspace.isInitialized) try {
            workspace.assertCheckoutUnchanged()
        } finally {
            workspace.close()
        }
    }
}
