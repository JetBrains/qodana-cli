package org.jetbrains.qodana.edict.integration

import kotlinx.coroutines.delay
import kotlinx.coroutines.runBlocking
import org.jetbrains.qodana.edict.common.obj
import org.jetbrains.qodana.edict.common.text
import org.jetbrains.qodana.edict.core.HttpInspectionKtsClient
import org.jetbrains.qodana.edict.support.FakeIntellijMcpServerLifecycle
import org.jetbrains.qodana.edict.core.IntellijMcpServerService
import org.jetbrains.qodana.edict.integration.support.LiveIntegrationTest
import org.jetbrains.qodana.edict.integration.support.historyBefore
import org.jetbrains.qodana.edict.integration.support.historyPath
import org.jetbrains.qodana.edict.integration.support.historyProject
import org.jetbrains.qodana.edict.integration.support.inspection.InspectionServer
import org.junit.jupiter.api.Assumptions.assumeTrue
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.Timeout
import java.time.Duration
import java.util.concurrent.TimeUnit
import kotlin.test.assertContains
import kotlin.test.assertEquals
import kotlin.test.assertTrue
import kotlin.time.Duration.Companion.seconds

class LiveIntellijMcpSessionTest : LiveIntegrationTest() {

    /**
     * Agents call the IDE minutes apart, while some IDE builds drop a Streamable HTTP session after 15 idle seconds
     * unless its client keeps an event stream open. Each pause outlasts that, so a dropped session shows as an IDE restart.
     */
    @Test
    fun `production client keeps its IDE session across idle pauses without restarting the IDE`() = runBlocking {
        InspectionServer.start(qodanaCli, workspace.project, workspace.output).use { ide ->
            val original = workspace.repository.fileAt(historyBefore, historyPath)
            val inspection = ide.run(INSPECTION, historyPath.removePrefix("$historyProject/"), original)
            assertContains(inspection.problemLines, 5, "The IDE fixture must resolve Thread.sleep against its project JDK")
            // The surrounding test owns the IDE.
            val lifecycle = FakeIntellijMcpServerLifecycle(ide.streamableEndpoint)
            val service = IntellijMcpServerService(ide.source, serverLifecycle = lifecycle)
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
            assertEquals(1, lifecycle.starts, "IntelliJ MCP was restarted after its session was dropped")
        }
    }

    @Test
    @Timeout(value = 15, unit = TimeUnit.MINUTES)
    fun `project analysis can be submitted after the previous analysis times out`() = runBlocking {
        InspectionServer.start(qodanaCli, workspace.project, workspace.output).use { ide ->
            val supportsTimeout = ide.tools.any { tool ->
                tool.text("name") == "run_inspection_kts_project" &&
                    "timeoutMinutes" in tool.obj("inputSchema").obj("properties")
            }
            assumeTrue(supportsTimeout, "The installed Qodana linter does not support IDE-side project analysis timeouts")
            HttpInspectionKtsClient(
                endpoint = ide.streamableEndpoint.uri,
                projectPath = ide.source.toString(),
                requestTimeout = Duration.ofMinutes(2),
                projectAnalysisTimeoutMinutes = 1,
                authToken = ide.streamableEndpoint.authToken,
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
            import com.intellij.psi.PsiMethodCallExpression

            @Language("HTML")
            val htmlDescription = "<html><body>Reports calls to Thread.sleep.</body></html>"

            val threadSleepInspection = localInspection { psiFile, inspection ->
                if (psiFile !is PsiJavaFile) return@localInspection
                psiFile.descendantsOfType<PsiMethodCallExpression>()
                    .filter { call ->
                        call.methodExpression.referenceName == "sleep" &&
                            call.resolveMethod()?.containingClass?.qualifiedName == "java.lang.Thread"
                    }
                    .forEach { call -> inspection.registerProblem(call, "Thread.sleep call") }
            }

            listOf(
                InspectionKts(
                    id = "thread-sleep",
                    localTool = threadSleepInspection,
                    name = "Thread sleep call",
                    htmlDescription = htmlDescription,
                    level = HighlightDisplayLevel.WARNING,
                )
            )
        """.trimIndent()
    }
}
