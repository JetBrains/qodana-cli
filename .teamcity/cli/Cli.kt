package cli

const val clangId = "ijplatform_master_CIDR_ExternalTools_ClangdAll"

fun getDotNetId(branch: String): String {
    return if (branch == "241") {
        "ijplatform_IjPlatform241_Net20241_Deploy_TriggerForPublishing_TriggerRtm"
    } else {
        "ijplatform_master_NetTrunk_PostCompile_TriggerAllInstallers"
    }
}
