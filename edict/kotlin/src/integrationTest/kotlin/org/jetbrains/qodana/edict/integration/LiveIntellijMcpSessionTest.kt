package org.jetbrains.qodana.edict.integration

import kotlinx.coroutines.delay
import kotlinx.coroutines.runBlocking
import org.jetbrains.qodana.edict.edictnext.HttpInspectionKtsClient
import org.jetbrains.qodana.edict.edictnext.IntellijMcpServerLifecycle
import org.jetbrains.qodana.edict.edictnext.IntellijMcpServerService
import org.jetbrains.qodana.edict.integration.support.IntegrationTest
import org.jetbrains.qodana.edict.integration.support.inspection.InspectionServer
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.Timeout
import java.net.URI
import java.nio.file.Files
import java.nio.file.Path
import java.time.Duration
import java.util.concurrent.atomic.AtomicInteger
import java.util.concurrent.TimeUnit
import kotlin.test.assertContains
import kotlin.test.assertEquals
import kotlin.test.assertTrue
import kotlin.time.Duration.Companion.seconds

class LiveIntellijMcpSessionTest : IntegrationTest() {

    /**
     * Agents call the IDE minutes apart, while some IDE builds drop a Streamable HTTP session after 15 idle seconds
     * unless its client keeps an event stream open. Each pause outlasts that, so a dropped session shows as an IDE restart.
     */
    @Test
    fun `production client keeps its IDE session across idle pauses without restarting the IDE`() = runBlocking {
        InspectionServer.start(workspace).use { ide ->
            val starts = AtomicInteger()
            val service = IntellijMcpServerService(
                ide.source,
                serverLifecycle = object : IntellijMcpServerLifecycle {
                    override suspend fun start(): URI = ide.streamableEndpoint.also { starts.incrementAndGet() }

                    // The surrounding test owns the IDE.
                    override suspend fun stop() = Unit
                },
            )
            try {
                repeat(3) { call ->
                    if (call > 0) delay(20.seconds)
                    val result = service.withClient { it.compile(INSPECTION) }
                    assertTrue(result.compilationSuccess, "Compilation failed: ${result.compilationErrorDetails}")
                }
            }
            finally {
                service.stop()
            }
            assertEquals(1, starts.get(), "IntelliJ MCP was restarted after its session was dropped")
        }
    }

    @Test
    @Timeout(value = 15, unit = TimeUnit.MINUTES)
    fun `project analysis can be submitted after the previous analysis times out`() = runBlocking {
        InspectionServer.start(workspace, ::prepareProjectForAnalysis).use { ide ->
            HttpInspectionKtsClient(
                endpoint = ide.streamableEndpoint,
                projectPath = ide.source.toString(),
                requestTimeout = Duration.ofMinutes(2),
                projectAnalysisTimeoutMinutes = 1,
            ).use { client ->
                val timedOut = client.analyzeProject(HANGING_INSPECTION)
                val timeoutError = timedOut.files.singleOrNull()?.executionError.orEmpty()
                assertContains(
                    timeoutError,
                    "Time limit",
                    message = "The first project analysis must reach the IDE-side timeout",
                )

                val completed = client.analyzeProject(INSPECTION)
                assertTrue(completed.compilation.compilationSuccess, completed.compilation.compilationErrorDetails)
                assertTrue(
                    completed.files.none { it.executionError != null },
                    "A submission after the timed-out batch must complete: ${completed.files}",
                )
            }
        }
    }

    private companion object {
        /** The Bazel MCP runner has an isolated IDE config, so give this copied fixture an explicit SDK and module. */
        fun prepareProjectForAnalysis(source: Path) {
            val idea = source.resolve(".idea")
            Files.createDirectories(idea)
            Files.writeString(
                idea.resolve("misc.xml"),
                """<?xml version="1.0" encoding="UTF-8"?>
                    |<project version="4">
                    |  <component name="ProjectRootManager" version="2" project-jdk-name="25" project-jdk-type="JavaSDK" />
                    |</project>
                    |""".trimMargin(),
            )
            Files.writeString(
                idea.resolve("modules.xml"),
                """<?xml version="1.0" encoding="UTF-8"?>
                    |<project version="4">
                    |  <component name="ProjectModuleManager">
                    |    <modules>
                    |      <module fileurl="file://${'$'}PROJECT_DIR${'$'}/inspection-project.iml" filepath="${'$'}PROJECT_DIR${'$'}/inspection-project.iml" />
                    |    </modules>
                    |  </component>
                    |</project>
                    |""".trimMargin(),
            )
            Files.writeString(
                source.resolve("inspection-project.iml"),
                """<?xml version="1.0" encoding="UTF-8"?>
                    |<module type="JAVA_MODULE" version="4">
                    |  <component name="NewModuleRootManager" inherit-compiler-output="true">
                    |    <exclude-output />
                    |    <content url="file://${'$'}MODULE_DIR${'$'}">
                    |      <sourceFolder url="file://${'$'}MODULE_DIR${'$'}/src/main/java" isTestSource="false" />
                    |    </content>
                    |    <orderEntry type="inheritedJdk" />
                    |    <orderEntry type="sourceFolder" forTests="false" />
                    |  </component>
                    |</module>
                    |""".trimMargin(),
            )
        }

        val HANGING_INSPECTION = """
            import com.intellij.openapi.progress.ProgressManager

            val hangingInspection = localInspection { _, _ ->
                while (true) {
                    ProgressManager.checkCanceled()
                    Thread.sleep(10)
                }
            }

            listOf(
                InspectionKts(
                    id = "timeout-regression-hanging",
                    localTool = hangingInspection,
                    name = "Timeout regression hanging inspection",
                    htmlDescription = "<html><body>Waits until project analysis is cancelled.</body></html>",
                    level = HighlightDisplayLevel.WARNING,
                )
            )
        """.trimIndent()

        val INSPECTION = """
            import org.intellij.lang.annotations.Language
            import com.intellij.psi.PsiJavaFile
            import com.intellij.psi.PsiLiteralExpression

            @Language("HTML")
            val htmlDescription = "<html><body>Reports octal integer literals.</body></html>"

            val octalLiteralInspection = localInspection { psiFile, inspection ->
                if (psiFile !is PsiJavaFile) return@localInspection
                psiFile.descendantsOfType<PsiLiteralExpression>()
                    .filter { literal -> Regex("0(?:_*[0-7])+[lL]?").matches(literal.text) }
                    .forEach { literal -> inspection.registerProblem(literal, "Octal integer literal") }
            }

            listOf(
                InspectionKts(
                    id = "octal-literal",
                    localTool = octalLiteralInspection,
                    name = "Octal integer literal",
                    htmlDescription = htmlDescription,
                    level = HighlightDisplayLevel.WARNING,
                )
            )
        """.trimIndent()
    }
}
