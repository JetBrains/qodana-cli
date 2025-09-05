package cli

import jetbrains.buildServer.configs.kotlin.AbsoluteId
import jetbrains.buildServer.configs.kotlin.Dependencies

internal const val THIRD_PARTY_LICENSES_RULE = "**/*third-party-libraries.json=>licenses/"

fun Dependencies.getQodanaToolingArtifacts(tool: String = "cli") {
    artifacts(AbsoluteId("StaticAnalysis_Base_Build_baseline_cli")) {
        buildRule = lastSuccessful()
        artifactRules = """
            baseline-cli-*.jar => tooling/baseline-cli.jar
        """.trimIndent()
        if (tool != "cli") {
            artifactRules += "\n\n$THIRD_PARTY_LICENSES_RULE"
        }
    }
    artifacts(AbsoluteId("StaticAnalysis_Base_Build_fuser")) {
        buildRule = lastSuccessful()
        artifactRules = """
            qodana-fuser-*.jar => tooling/qodana-fuser.jar
        """.trimIndent()
        if (tool != "cli") {
            artifactRules += "\n\n$THIRD_PARTY_LICENSES_RULE"
        }
    }
    artifacts(AbsoluteId("StaticAnalysis_Build_UiAndConverter")) {
        buildRule = tag("readyForTest", "+:*")
        artifactRules = """
            intellij-report-converter.jar=>tooling/intellij-report-converter.jar
        """.trimIndent()
        if (tool != "cli") {
            artifactRules += "\n\n$THIRD_PARTY_LICENSES_RULE"
        }
    }
}

fun Dependencies.getClangArtifacts() {
    artifacts(AbsoluteId(clangId)) {
        buildRule = lastSuccessful()
        artifactRules = """
            clang-*-linux-aarch64.tar.gz=>clang/clang-linux-aarch64.tar.gz
            clang-*-linux-x64.tar.gz=>clang/clang-linux-x64.tar.gz
            clang-*-mac-aarch64.tar.gz=>clang/clang-mac-aarch64.tar.gz
            clang-*-mac-x64.tar.gz=>clang/clang-mac-x64.tar.gz
            clang-*-win-aarch64.zip=>clang/clang-win-aarch64.zip
            clang-*-win-x64.zip=>clang/clang-win-x64.zip
        """.trimIndent()
    }
}

fun Dependencies.getDotNetArtifacts(branch: String = "main", tool: String = "cli") {
    artifacts(AbsoluteId(getDotNetId(branch))) {
        buildRule = lastSuccessful("+:*")
        artifactRules = """
            Artifacts.InstallersPortablesZips/JetBrains.ReSharper.GlobalTools.*.nupkg=>cdnet/clt.zip
        """.trimIndent()
        if (tool != "cli") {
            artifactRules += "\n\nArtifacts.InstallersPortablesZips/JetBrains.ReSharper-*-third-party-libraries.json=>licenses/"
        }
    }
}