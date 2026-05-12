package cli

import jetbrains.buildServer.configs.kotlin.*
import jetbrains.buildServer.configs.kotlin.buildFeatures.buildCache
import jetbrains.buildServer.configs.kotlin.buildFeatures.commitStatusPublisher
import jetbrains.buildServer.configs.kotlin.buildFeatures.dockerSupport
import jetbrains.buildServer.configs.kotlin.buildFeatures.gitHubAppBuildScopedToken
import jetbrains.buildServer.configs.kotlin.buildFeatures.sshAgent
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
    private val qodanaToken: String = "",
    private val enableTriggers: Boolean = true
) : BuildType({
    val isCli = wd == "cli"

    allowExternalStatus = true
    val releaseType = ReleaseType.fromArguments(arguments)
    id("${if (releaseType.isNightlyOrRelease()) releaseType.name else "Build"}$wd$branch")
    name = "${releaseType.name} qodana-$wd"
    description = "${releaseType.name} $arguments build of qodana-$wd for ($CLI_GITHUB_REPO_URL/$branch)"
    maxRunningBuildsPerBranch = if (releaseType != ReleaseType.Snapshot) "*:1" else "*:0"
    artifactRules = "dist => ." + if (!isCli) "\n\n +:*-third-party-libraries.json" else ""

    if (buildPattern.isNotEmpty()) {
        buildNumberPattern = buildPattern
    }

    params {
        password("env.CHOCOLATEY_API_KEY", CHOCO_API_KEY, display = ParameterDisplay.HIDDEN)

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
        denestFileArtifacts()

        if (releaseType.isNightlyOrRelease() && isCli) {
            script {
                scriptContent = """
                    git tag -d nightly || true
                    git fetch --tags
                    git remote remove origin && git remote add origin $CLI_GITHUB_REPO_URL.git
                """.trimIndent()
                workingDir = wd
            }
        }
        script {
            name = "Run GoReleaser"
            scriptContent = if (releaseType.isNightlyOrRelease()) {
                """
                    set -e

                    ARCH=${'$'}(uname -m)
                    case ${'$'}ARCH in
                        x86_64) ARCH_SUFFIX="amd64" ;;
                        aarch64|arm64) ARCH_SUFFIX="arm64" ;;
                        *) echo "Unsupported architecture: ${'$'}ARCH"; exit 1 ;;
                    esac
                    CODESIGN_BIN="codesign-client-linux-${'$'}ARCH_SUFFIX"
                    curl -fsSL -o /tmp/${'$'}CODESIGN_BIN https://codesign-distribution.labs.jb.gg/${'$'}CODESIGN_BIN
                    curl -fsSL -o /tmp/${'$'}CODESIGN_BIN.sha256 https://codesign-distribution.labs.jb.gg/${'$'}CODESIGN_BIN.sha256
                    curl -fsSL -o /tmp/${'$'}CODESIGN_BIN.sha256.asc https://codesign-distribution.labs.jb.gg/${'$'}CODESIGN_BIN.sha256.asc
                    curl -fsSL https://download-cdn.jetbrains.com/KEYS | gpg --import -
                    gpg --batch --verify /tmp/${'$'}CODESIGN_BIN.sha256.asc /tmp/${'$'}CODESIGN_BIN.sha256
                    (cd /tmp && sha256sum -c ${'$'}CODESIGN_BIN.sha256)
                    mv /tmp/${'$'}CODESIGN_BIN /usr/local/bin/codesign
                    chmod +x /usr/local/bin/codesign

                    # Serialize tooling downloads before goreleaser's per-target pre-hooks race on shared .part files (QD-14483)
                    if [ -d ./internal/tooling ]; then go generate ./internal/tooling; fi
                    if [ -d ./tooling ]; then go generate ./tooling; fi

                    goreleaser release --clean ${arguments.joinToString(" ")}
                """.trimIndent()
            } else {
                """
                    set -e

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

    if (enableTriggers) {
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
    }

    features {
        sshAgent {
            teamcitySshKey = "default teamcity key"
        }
        dockerSupport {
            loginToRegistry = on {
                dockerRegistryId = "PROJECT_EXT_775"
            }
        }
        if (isCli) {
            buildCache {
                name = "qodana-build-cache"
                publish = true
                publishOnlyChanged = true
                use = true
                rules = ".cache"
            }
        }
        if (releaseType.isNightlyOrRelease() && isCli) {
            commitStatusPublisher {
                publisher = github {
                    githubUrl = "https://api.github.com"
                    authType = vcsRoot()
                }
            }
        }
        gitHubAppBuildScopedToken {
            parameterName = "env.GITHUB_TOKEN" // add "env." prefix to make it an environmental variable
            connectionId = "PROJECT_EXT_2867" // GitHub App connection ID
            // The repository name format is "myRepo1" for "https://github.com/myUser/myRepo1"
            targetRepositories = """
                qodana-cli
                scoop-utils
                homebrew-utils
                qodana-action
                qodana-lsp
            """.trimIndent()
        }
    }
    dependencies {
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
        "clang" -> "QDCLC"
        "cdnet" -> "QDNETC"
        else -> ""
    }
}

private fun ScriptBuildStep.useGoDevContainerDockerImage() {
    dockerImage = "registry.jetbrains.team/p/sa/public/godevcontainer:latest"
    dockerImagePlatform = ScriptBuildStep.ImagePlatform.Linux
    dockerRunParameters =
        "--privileged -v /var/run/docker.sock:/var/run/docker.sock -e GOFLAGS=-buildvcs=false  -e VERSION=%build.number%"
}