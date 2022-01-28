# Qodana CLI

[![JetBrains incubator project](https://jb.gg/badges/incubator.svg)](https://confluence.jetbrains.com/display/ALL/JetBrains+on+GitHub)
[![Test](https://github.com/JetBrains/qodana-cli/actions/workflows/build-test.yml/badge.svg)][gh:test]
[![GitHub Discussions](https://img.shields.io/github/discussions/jetbrains/qodana)][jb:discussions]
[![Twitter Follow](https://img.shields.io/twitter/follow/QodanaEvolves?style=social&logo=twitter)][jb:twitter]

> ⚠️ This is an experimental project, so it's not guaranteed to work correctly.
> Use it at your own risk. For running Qodana stably and reliably, please use [Qodana Docker Images](https://www.jetbrains.com/help/qodana/docker-images.html). For feature requests or bugs reports please use our YouTrack: https://jb.gg/qodana-cli/new-issue

`qodana` is a simple cross-platform command-line tool to run [Qodana linters](https://www.jetbrains.com/help/qodana/docker-images.html) anywhere with minimum effort required.

**Table of Contents**

<!-- toc -->

- [Usage](#usage)
   - [Installation](#installation)
   - [Running](#running)
   - [Update](#update)
- [Configuration](#configuration)
- [Why](#why)

<!-- tocstop -->

## Usage

### Installation

> 💡 The Qodana CLI is distributed and run as a binary. The Qodana linters with inspections are [Docker Images]((https://www.jetbrains.com/help/qodana/docker-images.html)).
You must have Docker installed and running locally to support this: https://www.docker.com/get-started.

We have installation scripts for Linux, macOS, and Windows.

#### Install on Linux and macOS
```shell
curl -fsSL https://jb.gg/qodana-cli/install | bash
```

#### Install on Windows
```powershell
iwr https://jb.gg/qodana-cli/install.ps1 -useb | iex
```

#### Install with [Homebrew](https://brew.sh)
```shell
brew tap jetbrains/utils
brew install qodana
```

#### Install with [Go](https://go.dev/doc/install)
```shell
go install github.com/JetBrains/qodana-cli@latest
```

If you want to install some specific version:
- **macOS and Linux**: add the version number (e.g. `0.5.0`) **to the end** of the command
- **Windows**: add the version number (e.g. `$v="0.5.0";`) **to the beginning** of the command.
- **Go**: change the `latest` to the version number.

Alternatively, you can install the latest binary (or the apt/rpm/deb package) from [the repository releases](https://github.com/JetBrains/qodana-cli/releases/latest).

### Running

![CleanShot 2022-01-26 at 13 58 28](https://user-images.githubusercontent.com/13538286/151153050-934c0f41-e059-480a-a89f-cd4b2ca7a930.gif)

#### tl;dr

If you know what you are doing:

```
qodana scan --show-report
```

You can also add the linter by its name with the `--linter` option (e.g. `--linter jetbrains/qodana-js`).

#### Configure Qodana

Before you start using Qodana, you need to configure your project – choose [a linter](https://www.jetbrains.com/help/qodana/linters.html) to use.
If you know what linter you want to use, you can skip this step.

Also, Qodana CLI can choose a linter for you. Just run the following command in your **project root**:

```sh
qodana init
```

#### Run Qodana

Right after you configured your project (or remember linter's name you want to run), you can run Qodana inspections simply by invoking the following command in your project root:

```sh
qodana scan
```

- After the first Qodana run, the following runs will be faster because of the saved Qodana cache in your project (defaults to `./<userCacheDir>/JetBrains/<linter>/cache`)
- The latest Qodana report will be saved to `./<userCacheDir>/JetBrains/<linter>/results` – you can find qodana.sarif.json and other Qodana artifacts (like logs) in this directory.

#### View the Qodana report

After the analysis, the results are saved to `./<userCacheDir>/JetBrains/<linter>/results` by default. Inside the directory `./<userCacheDir>/JetBrains/<linter>/results/report`, you can find Qodana HTML report.
To view it in the browser, run the following command from your project root:

```shell
qodana show
```

You can serve any Qodana HTML report regardless of the project if you provide the correct report path.

## Update

Update to the latest version depends on how you choose to install `qodana` on your machine.

#### Update on Linux and macOS
```shell
curl -fsSL https://jb.gg/qodana-cli/install | bash
```

#### Update on Windows
```powershell
iwr https://jb.gg/qodana-cli/install.ps1 -useb | iex
```

#### Update with [Homebrew](https://brew.sh)
```shell
brew upgrade qodana
```

#### Update with [Go](https://go.dev/doc/install)
```shell
go install github.com/JetBrains/qodana-cli@latest
```

Alternatively, you can install the latest binary (or the apt/rpm/deb package) from [the repository releases](https://github.com/JetBrains/qodana-cli/releases/latest).

## Configuration

Find more CLI options, run `qodana ...` commands with the `--help` flag. Currently, there are not many options.
If you want to configure Qodana or a check inside Qodana, consider using [`qodana.yaml` ](https://www.jetbrains.com/help/qodana/qodana-yaml.html) to have the same configuration on any CI you use and your machine.

> In some flags help texts you can notice that the default path contains `<userCacheDir>/JetBrains`. The `<userCacheDir>` differs from the OS you are running Qodana with.
> - macOS: ~/Library/Caches/
> - Linux: ~/.cache/
> - Windows: %LOCALAPPDATA%\

#### Disable telemetry

To disable [Qodana user statistics](https://www.jetbrains.com/help/qodana/qodana-jvm-docker-readme.html#Usage+statistics), export the `DO_NOT_TRACK` environment variable to `1` before running the CLI:

```sh
export DO_NOT_TRACK=1
```

### init

Configure project for Qodana

#### Synopsis

Configure project for Qodana: prepare Qodana configuration file by analyzing the project structure and generating a default configuration qodana.yaml file.

```
init [flags]
```

#### Options

```
  -h, --help                 help for init
  -i, --project-dir string   Root directory of the project to configure (default ".")
```

### scan

Scan project with Qodana

#### Synopsis

Scan a project with Qodana. It runs one of Qodana Docker's images (https://www.jetbrains.com/help/qodana/docker-images.html) and reports the results.

Note that most options can be configured via qodana.yaml (https://www.jetbrains.com/help/qodana/qodana-yaml.html) file.
But you can always override qodana.yaml options with the following command-line options.


```
scan [flags]
```

#### Options

```
  -a, --analysis-id string        Unique report identifier (GUID) to be used by Qodana Cloud
  -b, --baseline string           Provide the path to an existing SARIF report to be used in the baseline state calculation
      --baseline-include-absent   Include in the output report the results from the baseline run that are absent in the current run
  -c, --cache-dir string          Override cache directory (default <userCacheDir>/JetBrains/<linter>/cache)
      --changes                   Override the docker image to be used for the analysis
      --disable-sanity            Skip running the inspections configured by the sanity profile
  -e, --env stringArray           Define additional environment variables for the Qodana container (you can use the flag multiple times). CLI is not reading full host environment variables and does not pass it to the Qodana container for security reasons
      --fail-threshold string     Set the number of problems that will serve as a quality gate. If this number is reached, the inspection run is terminated with a non-zero exit code
  -h, --help                      help for scan
  -l, --linter string             Override linter to use
      --port int                  Port to serve the report on (default 8080)
  -n, --profile-name string       Profile name defined in the project
  -p, --profile-path string       Path to the profile file
  -i, --project-dir string        Root directory of the inspected project (default ".")
      --property string           Set a JVM property to be used while running Qodana using the --property=property.name=value1,value2,...,valueN notation
  -o, --results-dir string        Override directory to save Qodana inspection results to (default <userCacheDir>/JetBrains/<linter>/results)
      --run-promo                 Set to true to have the application run the inspections configured by the promo profile; set to false otherwise. By default, a promo run is enabled if the application is executed with the default profile and is disabled otherwise
  -s, --save-report               Generate HTML report (default true)
      --script string             Override the run scenario (default "default")
      --send-report               Send the inspection report to Qodana Cloud, requires the '--token' option to be specified
  -w, --show-report               Serve HTML report on port
  -d, --source-directory string   Directory inside the project-dir directory must be inspected. If not specified, the whole project is inspected.
      --stub-profile string       Absolute path to the fallback profile file. This option is applied in case the profile was not specified using any available options
  -t, --token string              Qodana Cloud token
  -u, --unveil-problems           Print all found problems by Qodana in the CLI output
```

### show

Show Qodana report

#### Synopsis

Show (serve locally) the latest Qodana report.

Due to JavaScript security restrictions, the generated report cannot
be viewed via the file:// protocol (by double-clicking the index.html file).
https://www.jetbrains.com/help/qodana/html-report.html
This command serves the Qodana report locally and opens a browser to it.

```
show [flags]
```

#### Options

```
  -d, --dir-only            Open report directory only, don't serve it
  -h, --help                help for show
  -p, --port int            Specify port to serve report at (default 8080)
  -r, --report-dir string   Specify HTML report path (the one with index.html inside) (default "<userCacheDir>/JetBrains/<linter>/results/report")
```

## Why

![image](https://user-images.githubusercontent.com/13538286/151377284-28d845d3-a601-4512-9029-18f99d215ee1.png)

> 🖼 The illustration is created by [Irina Khromova](https://www.instagram.com/irkin_sketch/)

Qodana linters are distributed via Docker images – which becomes handy for developers (us) and the users to run code inspections in CI.

But to set up Qodana in CI, one wants to try it locally first, as there is some additional configuration tuning required that differs from project to project (and we try to be as much user-friendly as possible).

It's easy to try Qodana locally by running a _simple_ command:

```shell
docker run --rm -it -p 8080:8080 -v <source-directory>/:/data/project/ -v <output-directory>/:/data/results/ -v <caches-directory>/:/data/cache/ jetbrains/qodana-<linter> --show-report
```

**And that's not so simple**: you have to provide a few absolute paths, forward some ports, add a few Docker options...

- On Linux, you might want to set the proper permissions to the results produced after the container run – so you need to add an option like `-u $(id -u):$(id -g)`
- On Windows and macOS, when there is the default Docker Desktop RAM limit (2GB), your run might fail because of OOM (and this often happens on big Gradle projects on Gradle sync), and the only workaround, for now, is increasing the memory – but to find that out, one needs to look that up in the docs.
- That list could go on, but we've thought about these problems, experimented a bit, and created the CLI to simplify all of this.

**Isn't that a bit overhead to write a tool that runs Docker containers when we have Docker CLI already?** Our CLI, like Docker CLI, operates with Docker daemon via Docker Engine API using the official Docker SDK, so actually, our tool is our own tailored Docker CLI at the moment. 


[gh:test]: https://github.com/JetBrains/qodana/actions/workflows/build-test.yml
[youtrack]: https://youtrack.jetbrains.com/issues/QD
[youtrack-new-issue]: https://youtrack.jetbrains.com/newIssue?project=QD&c=Platform%20GitHub%20Action
[jb:confluence-on-gh]: https://confluence.jetbrains.com/display/ALL/JetBrains+on+GitHub
[jb:discussions]: https://jb.gg/qodana-discussions
[jb:twitter]: https://twitter.com/QodanaEvolves
[jb:docker]: https://hub.docker.com/r/jetbrains/qodana
