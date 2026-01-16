# Qodana CLI [<img src="https://api.producthunt.com/widgets/embed-image/v1/top-post-badge.svg?post_id=304841&theme=dark&period=daily" alt="" align="right" width="190" height="41">](https://www.producthunt.com/posts/jetbrains-qodana)

[![JetBrains project](https://jb.gg/badges/official.svg)](https://confluence.jetbrains.com/display/ALL/JetBrains+on+GitHub)
[![Qodana](https://github.com/JetBrains/qodana-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/JetBrains/qodana-cli/actions/workflows/ci.yml)
[![GoReport](https://goreportcard.com/badge/github.com/JetBrains/qodana-cli)][gh:goreport]
[![GitHub Discussions](https://img.shields.io/github/discussions/jetbrains/qodana)][jb:discussions]
[![Twitter Follow](https://img.shields.io/badge/follow-%40Qodana-1DA1F2?logo=twitter&style=social)][jb:twitter]

`qodana` is a simple cross-platform command-line tool to run [Qodana linters](https://www.jetbrains.com/help/qodana/docker-images.html) anywhere with minimum effort required.

#### tl;dr

[Install](https://github.com/JetBrains/qodana-cli/releases/latest) and run:

```console
qodana scan --show-report
```

You can also add the linter by its name with the `--linter` option (e.g. `--linter jetbrains/qodana-js`).

<!-- TOC -->
  * [Installation](#installation)
      * [macOS and Linux](#macos-and-linux)
      * [Windows](#windows)
      * [Anywhere else](#anywhere-else)
  * [Usage](#usage)
    * [Prepare your project](#prepare-your-project)
    * [Analyze your project](#analyze-your-project)
    * [View the report](#view-the-report)
  * [Configuration](#configuration)
  * [Why](#why)
<!-- TOC -->

![qodana](https://user-images.githubusercontent.com/13538286/151153050-934c0f41-e059-480a-a89f-cd4b2ca7a930.gif)

## Installation

> 💡 The Qodana CLI is distributed and run as a binary. The Qodana linters with inspections are [Docker Images](https://www.jetbrains.com/help/qodana/docker-images.html) or, starting from version `2023.2`, your local/downloaded by CLI IDE installations (experimental support).
> - To run Qodana with a container (the default mode in CLI), you must have Docker or Podman installed and running locally to support this: https://www.docker.com/get-started, and, if you are using Linux, you should be able to run Docker from the current (non-root) user (https://docs.docker.com/engine/install/linux-postinstall/#manage-docker-as-a-non-root-user)
> - To run Qodana without a container, you must have the IDE installed locally to provide the IDE installation path to the CLI or specify the product code, and CLI will try to download the IDE automatically (experimental support).

#### macOS and Linux
##### Install with [Homebrew](https://brew.sh) (recommended)
```shell
brew install jetbrains/utils/qodana
```
##### Install with our installer
```shell
curl -fsSL https://jb.gg/qodana-cli/install | bash
```
Also, you can install `nightly` or any other version the following way:
```
curl -fsSL https://jb.gg/qodana-cli/install | bash -s -- nightly
```

#### Windows
##### Install with [Windows Package Manager](https://learn.microsoft.com/en-us/windows/package-manager/winget/) (recommended)
```shell
winget install -e --id JetBrains.QodanaCLI
```
##### Install with [Chocolatey](https://chocolatey.org)
```shell
choco install qodana
```
##### Install with [Scoop](https://scoop.sh)
```shell
scoop bucket add jetbrains https://github.com/JetBrains/scoop-utils
scoop install qodana
```

#### Anywhere else
Alternatively,
you can install the latest binary (or the apt/rpm/deb package)
from [this page](https://github.com/JetBrains/qodana-cli/releases/latest).

## Usage

https://user-images.githubusercontent.com/13538286/233484685-b9225168-8379-41bf-b8c8-6149a324cea8.mp4

🎥 The "Get Started with Qodana CLI" video is [also available on YouTube](https://www.youtube.com/watch?v=RV1MFnURMP8).

### Prepare your project

Before you start using Qodana, you need to configure your project –
choose [a linter](https://www.jetbrains.com/help/qodana/linters.html) to use.
If you know what linter you want to use, you can skip this step.

Also, Qodana CLI can choose a linter for you. Just run the following command in your **project root**:

```shell
qodana init
```

### Analyze your project

Right after you configured your project (or remember linter's name you want to run),
you can run Qodana inspections simply by invoking the following command in your project root:

```shell
qodana scan
```

- After the first Qodana run, the following runs will be faster because of the saved Qodana cache in your project (defaults to `./<userCacheDir>/JetBrains/<linter>/cache`)
- The latest Qodana report will be saved to `./<userCacheDir>/JetBrains/<linter>/results` – you can find qodana.sarif.json and other Qodana artifacts (like logs) in this directory.

### View the report

After the analysis, the results are saved to `./<userCacheDir>/JetBrains/<linter>/results` by default.
Inside the directory `./<userCacheDir>/JetBrains/<linter>/results/report`, you can find a Qodana HTML report.
To view it in the browser, run the following command from your project root:

```shell
qodana show
```

You can serve any Qodana HTML report regardless of the project if you provide the correct report path.

## Configuration

To find more CLI options run `qodana ...` commands with the `--help` flag.
If you want to configure Qodana or a check inside Qodana,
consider
using [`qodana.yaml` ](https://www.jetbrains.com/help/qodana/qodana-yaml.html) to have the same configuration on any CI you use and your machine.

> In some flags help texts you can notice that the default path contains `<userCacheDir>/JetBrains`. The `<userCacheDir>` differs from the OS you are running Qodana with.
> - macOS: `~/Library/Caches/`
> - Linux: `~/.cache/`
> - Windows: `%LOCALAPPDATA%\`
> Also, you can just run `qodana show -d` to open the directory with the latest Qodana report.

## init

Configure a project for Qodana

### Synopsis

Configure a project for Qodana: prepare Qodana configuration file by analyzing the project structure and generating a default configuration qodana.yaml file.

```
qodana init [flags]
```

### Options

```
      --config string        Set a custom configuration file instead of 'qodana.yaml'. Relative paths in the configuration will be based on the project directory.
  -f, --force                Force initialization (overwrite existing valid qodana.yaml)
  -h, --help                 help for init
  -i, --project-dir string   Root directory of the project to configure (default ".")
```

### Options inherited from parent commands

```
      --disable-update-checks   Disable check for updates
      --log-level string        Set log-level for output (default "error")
```

## scan

Scan project with Qodana

### Synopsis

Scan a project with Qodana. It runs one of Qodana's Docker images (https://www.jetbrains.com/help/qodana/docker-images.html) and reports the results.

Note that most options can be configured via qodana.yaml (https://www.jetbrains.com/help/qodana/qodana-yaml.html) file.
But you can always override qodana.yaml options with the following command-line options.

```
qodana scan [flags]
```

### Options

```
  -l, --linter string             Defines the linter to be used for analysis. Default value is determined based on project files. 
                                  Available values: qodana-jvm-community, qodana-jvm, qodana-jvm-android, qodana-android, qodana-php, qodana-python-community, qodana-python, qodana-js, qodana-cdnet, qodana-dotnet, qodana-ruby, qodana-cpp, qodana-go, qodana-rust, qodana-clang. 
                                  !Legacy note!: Until version 2025.2 this parameter was used to define a docker image. This behavior is deprecated but supported for backward compatibility. Please use parameters --linter and --within-docker=true or --image instead.
      --within-docker string      Defines if analysis is performed within a docker container or not. 
                                  Set to 'false' for performing analysis in native mode. Set to 'true' for performing analysis within a docker container. 
                                  The image for container creation will be chosen automatically based on the value of the --linter param (e.g. jetbrains/qodana-jvm for --linter=qodana-jvm). 
                                  Default value is defined dynamically depending on the current environment and project.
      --image string              Defines an image to be used for analysis execution. 
                                  Sets --within-docker=true. Sets --linter to the one preinstalled within the image. 
                                  Available images are: jetbrains/qodana-jvm:2025.3-eap, jetbrains/qodana-dotnet:2025.3-eap, etc. Full list of images is available at https://hub.docker.com/u/jetbrains?search=qodana .
  -i, --project-dir string        Root directory of the inspected project (default ".")
      --repository-root string    Path to the root of the Git repository. This directory must be the same as --project-dir or contain the project directory inside it.
  -o, --results-dir string        Override directory to save Qodana inspection results to (default <userCacheDir>/JetBrains/<linter>/results)
      --cache-dir string          Override cache directory (default <userCacheDir>/JetBrains/<linter>/cache)
  -r, --report-dir string         Override directory to save Qodana HTML report to (default <userCacheDir>/JetBrains/<linter>/results/report)
      --print-problems            Print all found problems by Qodana in the CLI output
      --code-climate              Generate a Code Climate report in SARIF format (compatible with GitLab code Quality), will be saved to the results directory (default true if Qodana is executed on GitLab CI)
      --bitbucket-insights        Send the results BitBucket Code Insights, no additional configuration required if ran in BitBucket Pipelines (default true if Qodana is executed on BitBucket Pipelines)
      --clear-cache               Clear the local Qodana cache before running the analysis
  -w, --show-report               Serve HTML report on port
      --port int                  Port to serve the report on (default 8080)
      --config string             Set a custom configuration file instead of 'qodana.yaml'. Relative paths in the configuration will be based on the project directory.
  -a, --analysis-id string        Unique report identifier (GUID) to be used by Qodana Cloud
  -b, --baseline string           Provide the path to an existing SARIF report to be used in the baseline state calculation
      --baseline-include-absent   Include in the output report the results from the baseline run that are absent in the current run
      --full-history --commit     Go through the full commit history and run the analysis on each commit. If combined with --commit, analysis will be started from the given commit. Could take a long time.
      --commit --full-history     Base changes commit to reset to, resets git and starts a diff run: analysis will be run only on changed files since the given commit. If combined with --full-history, full history analysis will be started from the given commit.
      --fail-threshold string     Set the number of problems that will serve as a quality gate. If this number is reached, the inspection run is terminated with a non-zero exit code
      --disable-sanity            Skip running the inspections configured by the sanity profile
  -d, --only-directory string     Directory inside the project-dir directory must be inspected. If not specified, the whole project is inspected
  -n, --profile-name string       Profile name defined in the project
  -p, --profile-path string       Path to the profile file
      --run-promo string          Set to 'true' to have the application run the inspections configured by the promo profile; set to 'false' otherwise (default: 'true' only if Qodana is executed with the default profile)
      --script string             Override the run scenario (default "default")
      --coverage-dir string       Directory with coverage data to process
      --apply-fixes               Apply all available quick-fixes, including cleanup
      --cleanup                   Run project cleanup
      --property stringArray      Set a JVM property to be used while running Qodana using the --property property.name=value1,value2,...,valueN notation
  -s, --save-report               Generate HTML report (default true)
      --timeout int               Qodana analysis time limit in milliseconds. If reached, the analysis is terminated, process exits with code timeout-exit-code. Negative – no timeout (default -1)
      --timeout-exit-code int     See timeout option (default 1)
      --diff-start string         Commit to start a diff run from. Only files changed between --diff-start and --diff-end will be analysed.
      --diff-end string           Commit to end a diff run on. Only files changed between --diff-start and --diff-end will be analysed.
      --reverse                   Override the default run-scenario for diff runs to always use the reverse-scoped script
      --no-statistics             [qodana-clang/qodana-dotnet] Disable sending anonymous statistics
      --compile-commands string   [qodana-clang specific] Path to compile_commands.json. Should be relative to the project directory. (default "./build/compile_commands.json")
      --clang-args string         [qodana-clang specific] Additional arguments for clang
      --solution string           [qodana-cdnet specific] Relative path to solution file
      --project string            [qodana-cdnet specific] Relative path to project file
      --configuration string      [qodana-cdnet specific] Build configuration
      --platform string           [qodana-cdnet specific] Build platform
      --no-build                  [qodana-cdnet specific] Do not build the project before analysis
  -e, --env stringArray           Only for container runs. Define additional environment variables for the Qodana container (you can use the flag multiple times). CLI is not reading full host environment variables and does not pass it to the Qodana container for security reasons
  -v, --volume stringArray        Only for container runs. Define additional volumes for the Qodana container (you can use the flag multiple times)
  -u, --user string               Only for container runs. Override user inside the Qodana container. Format: uid[:gid] (e.g. '0:0' for root, '$(id -u):$(id -g)' for current user). Default: current system user, or root in privileged images (default "auto")
      --skip-pull                 Only for container runs. Skip pulling the latest Qodana container
  -h, --help                      help for scan
```

### Options inherited from parent commands

```
      --disable-update-checks   Disable check for updates
      --log-level string        Set log-level for output (default "error")
```

## show

Show a Qodana report

### Synopsis

Show (serve) the latest Qodana report. Or open the results directory if the flag is set.

Due to JavaScript security restrictions, the generated report cannot
be viewed via the file:// protocol (by double-clicking the index.html file).
https://www.jetbrains.com/help/qodana/html-report.html
This command serves the Qodana report locally and opens a browser to it.

```
qodana show [flags]
```

### Options

```
      --config string        Set a custom configuration file instead of 'qodana.yaml'. Relative paths in the configuration will be based on the project directory.
  -d, --dir-only             Open report directory only, don't serve it
  -h, --help                 help for show
  -l, --linter string        Override linter to use
  -p, --port int             Specify port to serve report at (default 8080)
  -i, --project-dir string   Root directory of the inspected project (default ".")
  -r, --report-dir string    Override directory to save Qodana HTML report to (default <userCacheDir>/JetBrains/<linter>/results/report)
  -o, --results-dir string   Override directory to save Qodana inspection results to (default <userCacheDir>/JetBrains/<linter>/results)
```

### Options inherited from parent commands

```
      --disable-update-checks   Disable check for updates
      --log-level string        Set log-level for output (default "error")
```

## send

Send a Qodana report to Cloud

### Synopsis

Send the report (qodana.sarif.json and other analysis results) to Qodana Cloud. 

If report directory is not specified, the latest report will be fetched from the default linter results location.

If you are using other Qodana Cloud instance than https://qodana.cloud/, override it by declaring the QODANA_ENDPOINT environment variable.

```
qodana send [flags]
```

### Options

```
  -a, --analysis-id string   Unique report identifier (GUID) to be used by Qodana Cloud
      --config string        Set a custom configuration file instead of 'qodana.yaml'. Relative paths in the configuration will be based on the project directory.
  -h, --help                 help for send
  -l, --linter string        Override linter to use
  -i, --project-dir string   Root directory of the inspected project (default ".")
  -r, --report-dir string    Override directory to save Qodana HTML report to (default <userCacheDir>/JetBrains/<linter>/results/report)
  -o, --results-dir string   Override directory to save Qodana inspection results to (default <userCacheDir>/JetBrains/<linter>/results)
```

### Options inherited from parent commands

```
      --disable-update-checks   Disable check for updates
      --log-level string        Set log-level for output (default "error")
```

## pull

Pull latest version of linter

### Synopsis

An alternative to pull an image.

```
qodana pull [flags]
```

### Options

```
      --config string        Set a custom configuration file instead of 'qodana.yaml'. Relative paths in the configuration will be based on the project directory.
  -h, --help                 help for pull
      --image string         Image to pull
  -l, --linter string        Override linter to use
  -i, --project-dir string   Root directory of the inspected project (default ".")
```

### Options inherited from parent commands

```
      --disable-update-checks   Disable check for updates
      --log-level string        Set log-level for output (default "error")
```

## view

View SARIF files in CLI

### Synopsis

Preview all problems found in SARIF files in CLI.

```
qodana view [flags]
```

### Options

```
  -h, --help                help for view
  -f, --sarif-file string   Path to the SARIF file (default "qodana.sarif.json")
```

### Options inherited from parent commands

```
      --disable-update-checks   Disable check for updates
      --log-level string        Set log-level for output (default "error")
```

## contributors

Calculate active project contributors

### Synopsis

A command-line helper for Qodana pricing[1] to calculate active contributor(s)[2] in the given local repositories.

[1] More information about available Qodana plans can be found at https://www.jetbrains.com/qodana/buy/

```
qodana contributors [flags]
```

### Options

```
  -d, --days int                  Number of days since when to calculate the number of active contributors (default 90)
  -h, --help                      help for contributors
  -o, --output string             Output format, can be tabular or json (default "tabular")
  -i, --project-dir stringArray   Project directory, can be specified multiple times to check multiple projects, if not specified, current directory will be used
```

### Options inherited from parent commands

```
      --disable-update-checks   Disable check for updates
      --log-level string        Set log-level for output (default "error")
```

## cloc

Calculate lines of code for projects with boyter/scc

### Synopsis

A command-line helper for project statistics: languages, lines of code. Powered by boyter/scc. For contributors, use "qodana contributors" command.

```
qodana cloc [flags]
```

### Options

```
  -h, --help                      help for cloc
  -o, --output string             Output format, can be [tabular, wide, json, csv, csv-stream, cloc-yaml, html, html-table, sql, sql-insert, openmetrics] (default "tabular")
  -i, --project-dir stringArray   Project directory, can be specified multiple times to check multiple projects, if not specified, current directory will be used
```

### Options inherited from parent commands

```
      --disable-update-checks   Disable check for updates
      --log-level string        Set log-level for output (default "error")
```

## Why

![Comics by Irina Khromova](https://user-images.githubusercontent.com/13538286/151377284-28d845d3-a601-4512-9029-18f99d215ee1.png)

> 🖼 [Irina Khromova painted the illustration](https://www.instagram.com/irkin_sketch/)

Qodana linters are distributed via Docker images –
which become handy for developers (us) and users to run code inspections in CI.

But to set up Qodana in CI, one wants to try it locally first,
as there is some additional configuration tuning required that differs from project to project
(and we try to be as much user-friendly as possible).

It's easy to try Qodana locally by running a _simple_ command:

```shell
docker run --rm -p 8080:8080 -v <source-directory>/:/data/project/ -v <output-directory>/:/data/results/ -v <caches-directory>/:/data/cache/ jetbrains/qodana-<linter> --show-report
```

**And that's not so simple**: you have to provide a few absolute paths, forward some ports, add a few Docker options...

- On Linux, you might want to set the proper permissions to the results produced after the container run – so you need to add an option like `-u $(id -u):$(id -g)`
- On Windows and macOS, when there is the default Docker Desktop RAM limit (2GB), your run might fail because of OOM (and this often happens on big Gradle projects on Gradle sync), and the only workaround, for now, is increasing the memory – but to find that out, one needs to look that up in the docs.
- That list could go on, but we've thought about these problems, experimented a bit, and created the CLI to simplify all of this.

**Isn't that a bit overhead to write a tool that runs Docker containers when we have Docker CLI already?** Our CLI, like Docker CLI, operates with Docker daemon via Docker Engine API using the official Docker SDK, so actually, our tool is our own tailored Docker CLI at the moment.

[gh:test]: https://github.com/JetBrains/qodana/actions/workflows/build-test.yml
[gh:goreport]: https://goreportcard.com/report/github.com/JetBrains/qodana-cli
[youtrack]: https://youtrack.jetbrains.com/issues/QD
[youtrack-new-issue]: https://youtrack.jetbrains.com/newIssue?project=QD&c=Platform%20GitHub%20Action
[jb:confluence-on-gh]: https://confluence.jetbrains.com/display/ALL/JetBrains+on+GitHub
[jb:discussions]: https://jb.gg/qodana-discussions
[jb:twitter]: https://twitter.com/Qodana
[jb:docker]: https://hub.docker.com/r/jetbrains/qodana
