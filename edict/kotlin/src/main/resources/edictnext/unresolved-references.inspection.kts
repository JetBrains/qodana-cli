import com.intellij.psi.PsiElement
import com.intellij.psi.PsiJavaCodeReferenceElement
import com.intellij.psi.PsiPolyVariantReference
import com.intellij.psi.PsiPackageStatement
import com.intellij.psi.util.PsiTreeUtil
import org.intellij.lang.annotations.Language
import org.jetbrains.kotlin.psi.KtFile
import org.jetbrains.kotlin.psi.KtNameReferenceExpression
import org.jetbrains.kotlin.psi.KtPackageDirective

@Language("HTML")
val htmlDescription = "<html><body>Reports every reference of the file that does not resolve.</body></html>"

/** Whether a reference of [element] has a target; an overloaded or expect/actual one may have several. */
fun resolves(element: PsiElement): Boolean = element.references.any { reference ->
  if (reference is PsiPolyVariantReference) reference.multiResolve(false).any { it.element != null }
  else reference.resolve() != null
}

val unresolvedReferences = localInspection { psiFile, inspection ->
  if (psiFile is KtFile) {
    PsiTreeUtil.collectElementsOfType(psiFile, KtNameReferenceExpression::class.java).forEach { reference ->
      if (PsiTreeUtil.getParentOfType(reference, KtPackageDirective::class.java) != null) return@forEach
      if (!resolves(reference)) {
        inspection.registerProblem(reference, "Unresolved reference '${reference.getReferencedName()}'")
      }
    }
  }
  else {
    PsiTreeUtil.collectElementsOfType(psiFile, PsiJavaCodeReferenceElement::class.java).forEach { reference ->
      if (PsiTreeUtil.getParentOfType(reference, PsiPackageStatement::class.java) != null) return@forEach
      if (!resolves(reference)) {
        inspection.registerProblem(reference, "Unresolved reference '${reference.referenceName}'")
      }
    }
  }
}

listOf(
  InspectionKts(
    id = "unresolved-references",
    localTool = unresolvedReferences,
    name = "Unresolved references",
    htmlDescription = htmlDescription,
    level = HighlightDisplayLevel.WARNING,
  )
)
