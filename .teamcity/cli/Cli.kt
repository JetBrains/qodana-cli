package cli

import jetbrains.buildServer.configs.kotlin.Project

object CLI : Project({
    id("Cli")
    name = "CLI"
    description = "Various products built from https://github.com/jetbrains/qodana-cli"
    buildType(GoReleaser("cli", "", arguments = listOf()))
    buildType(GoReleaser("cli", "main", arguments = listOf("--nightly", "--skip=chocolatey,nfpm,homebrew,scoop,snapcraft")))
})

const val clangId = "ijplatform_master_CIDR_ExternalTools_ClangdAll"

fun getDotNetId(branch: String): String {
    return if (branch == "241") {
        "ijplatform_IjPlatform241_Net20241_Deploy_TriggerForPublishing_TriggerRtm"
    } else {
        "ijplatform_master_NetTrunk_PostCompile_TriggerAllInstallers"
    }
}
