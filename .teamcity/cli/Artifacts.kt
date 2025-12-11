package cli

import jetbrains.buildServer.configs.kotlin.AbsoluteId
import jetbrains.buildServer.configs.kotlin.BuildSteps
import jetbrains.buildServer.configs.kotlin.Dependencies
import jetbrains.buildServer.configs.kotlin.buildSteps.script

/**
 * Default path suffix for artifacts where the target is supposed to be a file and not a directory.
 * @see BuildSteps.denestFileArtifacts
 */
const val DENEST_SUFFIX = ".denest-me"

/**
 * For each directory within [searchStartDir] find directories ending with [denestSuffix] and de-nest them: check that
 * there is a single file within the directory and move it so it has the same path as the directory except without the
 * suffix.
 *
 * Default behavior:
 * - rule: `artifact.txt => resources/artifact.txt`
 * - path: `resources/artifact.txt/artifact.txt`
 *
 * Using this function as a step:
 * - rule: `artifact.txt => resources/artifact.txt$DENEST_SUFFIX`
 * - path: `resources/artifact.txt`
 *
 * @see DENEST_SUFFIX
 */
fun BuildSteps.denestFileArtifacts(searchStartDir: String = "%teamcity.build.checkoutDir%", denestSuffix: String = DENEST_SUFFIX) {
    script {
        workingDir = searchStartDir
        name = "Denest file artifacts"
        scriptContent = """
            #!/usr/bin/env bash
            set -euo pipefail
            
            denest() {
                directory="$1"
                
                list_artifacts() {
                    find "${'$'}directory" -mindepth 1 -maxdepth 1 "$@"
                }
                
                n_files=$(list_artifacts -printf '.' | wc -c)
                if [ ${'$'}{n_files} -eq 0 ]; then
                    echo "No files found in '$(pwd)/${'$'}directory'. This is likely an error in BuildSteps.denestFileArtifacts." >&2
                    exit 2
                fi
                
                if [ ${'$'}{n_files} -ne 1 ]; then
                    echo "Found ${'$'}{n_files} files matching '$(pwd)/${'$'}directory':" >&2
                    list_artifacts >$2
                    echo "IJBuildType.loadSingleFileArtifact pattern should match a single file." >&2
                    exit 1
                fi
                
                targetPath="$(dirname "${'$'}directory")/$(basename "${'$'}directory" "$denestSuffix")"
                list_artifacts -exec mv -vT {} "${'$'}targetPath" \;
                rm -r "${'$'}directory"
            }
            
            # see also: https://stackoverflow.com/q/9612090
            find . -name "*$denestSuffix" -print0 | while IFS= read -r -d '' directory; do 
                denest "${'$'}directory"
            done
        """.trimIndent()
    }
}

internal const val THIRD_PARTY_LICENSES_RULE = "**/*third-party-libraries.json=>licenses/"

fun Dependencies.getQodanaToolingArtifacts(tool: String = "cli") {
    artifacts(AbsoluteId("StaticAnalysis_Base_Build_baseline_cli")) {
        buildRule = lastSuccessful()
        artifactRules = """
            baseline-cli-*.jar => internal/tooling/baseline-cli.jar$DENEST_SUFFIX/
        """.trimIndent()
        if (tool != "cli") {
            artifactRules += "\n\n$THIRD_PARTY_LICENSES_RULE"
        }
    }
    artifacts(AbsoluteId("StaticAnalysis_Base_Build_fuser")) {
        buildRule = lastSuccessful()
        artifactRules = """
            qodana-fuser-*.jar => internal/tooling/qodana-fuser.jar$DENEST_SUFFIX/
        """.trimIndent()
        if (tool != "cli") {
            artifactRules += "\n\n$THIRD_PARTY_LICENSES_RULE"
        }
    }
    artifacts(AbsoluteId("StaticAnalysis_Build_UiAndConverter")) {
        buildRule = tag("readyForTest", "+:*")
        artifactRules = """
            intellij-report-converter.jar=>internal/tooling/intellij-report-converter.jar$DENEST_SUFFIX/
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
            clang-tidy-*-linux-aarch64.tar.gz=>clang/clang-tidy-linux-arm64.tar.gz$DENEST_SUFFIX/
            clang-tidy-*-linux-x64.tar.gz=>clang/clang-tidy-linux-amd64.tar.gz$DENEST_SUFFIX/
            clang-tidy-*-mac-aarch64.tar.gz=>clang/clang-tidy-darwin-arm64.tar.gz$DENEST_SUFFIX/
            clang-tidy-*-mac-x64.tar.gz=>clang/clang-tidy-darwin-amd64.tar.gz$DENEST_SUFFIX/
            clang-tidy-*-win-aarch64.zip=>clang/clang-tidy-windows-arm64.zip$DENEST_SUFFIX/
            clang-tidy-*-win-x64.zip=>clang/clang-tidy-windows-amd64.zip$DENEST_SUFFIX/
        """.trimIndent()
    }
}

fun Dependencies.getDotNetArtifacts(branch: String = "main", tool: String = "cli") {
    artifacts(AbsoluteId(getDotNetId(branch))) {
        buildRule = lastSuccessful("+:*")
        artifactRules = """
            Artifacts.InstallersPortablesZips/JetBrains.ReSharper.GlobalTools.*.nupkg=>cdnet/clt.zip$DENEST_SUFFIX/
        """.trimIndent()
        if (tool != "cli") {
            artifactRules += "\n\nArtifacts.InstallersPortablesZips/JetBrains.ReSharper-*-third-party-libraries.json=>licenses/"
        }
    }
}