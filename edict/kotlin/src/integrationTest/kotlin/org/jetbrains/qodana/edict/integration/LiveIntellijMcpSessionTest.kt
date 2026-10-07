package org.jetbrains.qodana.edict.integration

import kotlinx.coroutines.delay
import kotlinx.coroutines.runBlocking
import org.jetbrains.qodana.edict.edictnext.IntellijMcpServerLifecycle
import org.jetbrains.qodana.edict.edictnext.IntellijMcpServerService
import org.jetbrains.qodana.edict.integration.support.IntegrationTest
import org.jetbrains.qodana.edict.integration.support.inspection.InspectionServer
import org.junit.jupiter.api.Test
import java.net.URI
import java.util.concurrent.atomic.AtomicInteger
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

    private companion object {
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
