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
    protected open val fixtureSource: String = IntegrationWorkspace.defaultSource()
    protected open val fixtureStatePath: String? = null
    protected open fun checkPrerequisites() = Unit

    @BeforeEach
    fun prepareIntegration(test: TestInfo) {
        checkPrerequisites()
        val configured = System.getProperty("edict.integrationOutput")
        val output = configured?.let(Path::of) ?: kotlinProjectDirectory().resolve("out/integration")
        workspace = IntegrationWorkspace.open(
            output, test.testClass.orElseThrow().simpleName, test.testMethod.orElseThrow().name,
            source = fixtureSource, revision = fixtureRevision, projectPath = fixtureProject,
            initialStatePath = fixtureStatePath,
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

abstract class LiveIntegrationTest : IntegrationTest() {
    protected lateinit var qodanaCli: Path
        private set

    override fun checkPrerequisites() {
        val executableName = if (System.getProperty("os.name").startsWith("Windows")) "qodana.exe" else "qodana"
        val executable = System.getProperty("edict.qodanaCli")?.let(Path::of)
            ?: kotlinProjectDirectory().resolve("../../$executableName").normalize()
        check(Files.isRegularFile(executable) && Files.isExecutable(executable)) {
            "Missing assembled Qodana CLI at $executable. Run ./gradlew assembleQodanaCli from edict/kotlin."
        }
        qodanaCli = executable.toAbsolutePath()
    }
}

private fun kotlinProjectDirectory(): Path = generateSequence(Path.of("").toAbsolutePath()) { it.parent }
    .flatMap { sequenceOf(it, it.resolve("edict/kotlin")) }
    .firstOrNull { Files.isRegularFile(it.resolve("build.gradle.kts")) && Files.isDirectory(it.resolve("src/main/resources/skills")) }
    ?: error("Cannot locate the edict/kotlin project directory")
