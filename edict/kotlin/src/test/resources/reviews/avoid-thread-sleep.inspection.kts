import com.intellij.psi.PsiJavaFile
import com.intellij.psi.PsiMethodCallExpression
import com.intellij.psi.util.PsiTreeUtil

val avoidThreadSleepInspection = localInspection { psiFile, inspection ->
    val javaFile = psiFile as? PsiJavaFile ?: return@localInspection
    PsiTreeUtil.collectElementsOfType(javaFile, PsiMethodCallExpression::class.java).forEach { call ->
        val method = call.methodExpression
        if (method.referenceName == "sleep" && method.qualifierExpression?.text == "Thread") {
            inspection.registerProblem(call, "Replace Thread.sleep with explicit synchronization")
        }
    }
}

listOf(
    InspectionKts(
        id = "AvoidThreadSleepForSynchronization",
        localTool = avoidThreadSleepInspection,
        name = "Avoid Thread.sleep for synchronization",
        htmlDescription = "<html><body>Reports Thread.sleep used as synchronization.</body></html>",
    )
)
