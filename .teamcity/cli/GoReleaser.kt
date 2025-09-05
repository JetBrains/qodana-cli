package cli

import jetbrains.buildServer.configs.kotlin.BuildType
import jetbrains.buildServer.configs.kotlin.CheckoutMode
import jetbrains.buildServer.configs.kotlin.DslContext
import jetbrains.buildServer.configs.kotlin.ParameterDisplay
import jetbrains.buildServer.configs.kotlin.buildFeatures.commitStatusPublisher
import jetbrains.buildServer.configs.kotlin.buildFeatures.dockerRegistryConnections
import jetbrains.buildServer.configs.kotlin.buildSteps.ScriptBuildStep
import jetbrains.buildServer.configs.kotlin.buildSteps.python
import jetbrains.buildServer.configs.kotlin.buildSteps.qodana
import jetbrains.buildServer.configs.kotlin.buildSteps.script
import jetbrains.buildServer.configs.kotlin.triggers.schedule
import jetbrains.buildServer.configs.kotlin.triggers.vcs

const val CLI_GITHUB_REPO_URL = "https://github.com/JetBrains/qodana-cli"

enum class ReleaseType(val arguments: List<String>) {
    Nightly(listOf("--nightly")),
    Snapshot(listOf("--snapshot", "--skip=publish")),
    Release(emptyList());

    companion object {
        fun fromArguments(args: List<String>): ReleaseType =
            when {
                "--nightly" in args -> Nightly
                "--snapshot" !in args -> Release
                else -> Snapshot
            }
    }

    fun isNightlyOrRelease(): Boolean = this != Snapshot
    fun isRelease(): Boolean = this == Release
}

class GoReleaser(
    private val wd: String,
    private val branch: String = "main",
    private val buildPattern: String = "",
    private val arguments: List<String> = listOf("--snapshot", "--skip=publish"),
    private val qodanaToken: String = ""
) : BuildType({
    val isCli = wd == "cli"

    allowExternalStatus = true
    val releaseType = ReleaseType.fromArguments(arguments)
    id("${if (releaseType.isNightlyOrRelease()) releaseType.name else "Build"}$wd$branch")
    name = "${releaseType.name} qodana-$wd"
    description = "${releaseType.name} $arguments build of qodana-$wd for ($CLI_GITHUB_REPO_URL/$branch)"
    maxRunningBuildsPerBranch = if (releaseType != ReleaseType.Snapshot) "*:1" else "*:0"
    artifactRules = "${wd}/dist => ." + if (!isCli) "\n\n +:*-third-party-libraries.json" else ""

    if (buildPattern.isNotEmpty()) {
        buildNumberPattern = buildPattern
    }

    params {
        if (releaseType.isRelease() && isCli) {
            password("env.CHOCOLATEY_API_KEY", CHOCO_API_KEY, display = ParameterDisplay.HIDDEN)
        } else {
            param("env.CHOCOLATEY_API_KEY", "")
        }
        if (releaseType.isNightlyOrRelease() && isCli) {
            password("env.GITHUB_TOKEN", GH_JETBRAINS_PAT, display = ParameterDisplay.HIDDEN)
        }

        checkbox("skip.qodana", if (isCli || branch == "main") "false" else "true")
        checkbox("env.SIGN", "false")
        param("env.FINGERPRINT", CODESIGN_FINGERPRINT)
        password("env.SERVICE_ACCOUNT_TOKEN", CODESIGN_SERVICE_ACCOUNT_TOKEN, display = ParameterDisplay.HIDDEN)
        param("env.SERVICE_ACCOUNT_NAME", CODESIGN_SERVICE_ACCOUNT_NAME)
        password("env.GORELEASER_KEY", GORELEASER_KEY, display = ParameterDisplay.HIDDEN)
        param("env.VERSION", "%build.number%")
        param("env.QODANA_JOB_URL", "%env.BUILD_URL%")
        param("env.GO_TESTING", "true")
        param("env.DEVICEID", "200820300000000-0000-0000-0000-000000000000")
        password("env.QODANA_LICENSE_ONLY_TOKEN", QODANA_TOKEN, display = ParameterDisplay.HIDDEN)
        password("env.QODANA_TOKEN", QODANA_TOKEN, display = ParameterDisplay.HIDDEN)
    }

    vcs {
        root(DslContext.settingsRoot)
        cleanCheckout = true
        checkoutMode = CheckoutMode.ON_AGENT
        if (branch.isNotEmpty() && branch != "main") {
            branchFilter = "+:$branch"
        }
    }

    steps {
        if (releaseType.isNightlyOrRelease() && isCli) {
            script {
                scriptContent = """
                    git tag -d nightly || true
                    git fetch --tags
                """.trimIndent()
                workingDir = wd
            }
        }
        script {
            name = "Run 'go generate'"
            scriptContent = """
                go generate -v $(go list -f '{{.Dir}}/...' -m)
            """.trimIndent()

            useGoDevContainerDockerImage()
        }
        script {
            name = "Run GoReleaser"
            workingDir = wd
            scriptContent = """
                export GORELEASER_CURRENT_TAG=${'$'}(git describe --tags ${'$'}(git rev-list --tags --max-count=1))
                goreleaser release --clean ${arguments.joinToString(" ")} --skip=publish
                go test
            """.trimIndent()

            if ("--skip=publish" !in arguments) {
                scriptContent += "\n" + """
                    goreleaser release --clean ${arguments.joinToString(" ")}
                """.trimIndent()
            }

            useGoDevContainerDockerImage()
        }
        qodana {
            enabled = qodanaToken.isNotEmpty()
            conditions {
                equals("skip.qodana", "false")
            }
            id = "Qodana"
            linter = go {}
            additionalQodanaArguments = "-n qodana.single:CheckDependencyLicenses"
            additionalDockerArguments = "-e QODANA_LICENSE_ONLY_TOKEN=%env.QODANA_LICENSE_ONLY_TOKEN%"
            param("report-as-test-mode", "each-problem-is-test")
        }
        script {
            enabled = qodanaToken.isNotEmpty()
            conditions {
                equals("skip.qodana", "false")
            }
            id = "simpleRunner"
            scriptContent =
                "mv %system.teamcity.build.tempDir%/output/build/results/projectStructure/third-party-libraries.json licenses/$wd-third-party-libraries.json"
        }
        python {
            enabled = qodanaToken.isNotEmpty()
            conditions {
                equals("skip.qodana", "false")
            }
            name = "Generate licenses artifact"
            command = script {
                content = """
                from json import loads, dumps
                from pathlib import Path
                import os

                Path(f"${getProductCode(wd)}-%build.number%-third-party-libraries.json").write_text(
                    dumps(
                        [lib for file in Path("licenses").rglob("*third-party-libraries.json") for lib in loads(file.read_text())],
                        indent=4,
                    )
                )
                """.trimIndent()
            }
        }
        script {
            conditions {
                equals("skip.qodana", "true")
            }
            name = "Create fake third-party-libraries.json"
            scriptContent = "echo '[]' > licenses/${getProductCode(wd)}-third-party-libraries.json"
        }
    }

    triggers {
        vcs {
            branchFilter = "+:$branch"
        }
        if (releaseType.isNightlyOrRelease()) {
            schedule {
                schedulingPolicy = daily {
                    hour = 3
                }
                branchFilter = "+:$branch"
                triggerBuild = always()
                withPendingChangesOnly = true
            }
        }
    }

    features {
        dockerRegistryConnections {
            loginToRegistry = on {
                dockerRegistryId = "PROJECT_EXT_775"
            }
        }
        if (releaseType.isNightlyOrRelease() && isCli) {
            commitStatusPublisher {
                vcsRootExtId = "${DslContext.settingsRoot.id}"
                publisher = github {
                    githubUrl = "https://api.github.com"
                    authType = vcsRoot()
                }
            }
        }
    }
    dependencies {
        getQodanaToolingArtifacts(tool = wd)
        when (wd) {
            "clang" -> getClangArtifacts()
            "cdnet" -> getDotNetArtifacts(branch, tool = wd)
        }
    }

    requirements {
        contains("teamcity.agent.name", "qodana-linux-amd64-large", "RQ_3846")
    }

    disableSettings("RQ_3846")
})

private fun getProductCode(wd: String): String {
    return when (wd) {
        "clang" -> "QDCL"
        "cdnet" -> "QDNETC"
        else -> ""
    }
}

private fun ScriptBuildStep.useGoDevContainerDockerImage() {
    dockerImage = "registry.jetbrains.team/p/sa/containers/godevcontainer:latest"
    dockerImagePlatform = ScriptBuildStep.ImagePlatform.Linux
    dockerRunParameters =
        "--privileged -v /var/run/docker.sock:/var/run/docker.sock -e GOFLAGS=-buildvcs=false  -e VERSION=%build.number%"
}