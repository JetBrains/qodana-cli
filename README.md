# Qodana CLI
[![Test](https://github.com/tiulpin/qodana/actions/workflows/build-test.yml/badge.svg)][gh:test]
[![Docker Hub](https://img.shields.io/docker/pulls/jetbrains/qodana.svg)][jb:docker]
[![Slack](https://img.shields.io/badge/Slack-%23qodana-blue)][jb:slack]
[![Twitter Follow](https://img.shields.io/twitter/follow/QodanaEvolves?style=social&logo=twitter)][jb:twitter]

> ⚠️ This is an experimental project, so it's not guaranteed to work correctly.
> Use it at your own risk. For running Qodana stably and reliably, please use [Qodana Docker Images](https://www.jetbrains.com/help/qodana/docker-images.html).

**Table of Contents**

<!-- toc -->

- [Qodana CLI](#qodana-cli)
  - [Usage](#usage)
    - [Installation](#installation)
    - [Running](#running)
  - [Configuration](#configuration)

<!-- tocstop -->

`qodana` is a command-line interface for [Qodana](https://jetbrains.com/qodana).
It allows you to run Qodana inspections on your local machine (or a CI agent) easily by running [Qodana Docker Images](https://www.jetbrains.com/help/qodana/docker-images.html).

## Usage

### Installation

> 💡 The Qodana CLI is distributed and run as a binary. The actual linters with inspections are [Docker Images]((https://www.jetbrains.com/help/qodana/docker-images.html)).
You must have Docker installed and running locally to support this: https://www.docker.com/get-started.

We have installation scripts for Linux, macOS, and Windows.

#### Install in the terminal on Linux and macOS
```shell
curl -fsSL https://raw.githubusercontent.com/tiulpin/qodana/main/install | bash
```

#### Install in the terminal on Windows
```shell
iwr https://raw.githubusercontent.com/tiulpin/qodana/main/install.ps1 -useb | iex
```

If you want to install some specific version:
- **macOS and Linux**: add the version number (e.g. `0.2.0`) **to the end** of the command
- **Windows**: add the version number (e.g. `$v="0.2.0";`) **to the beginning** of the command.

Alternatively, you can install the latest binary from [GitHub Releases](https://github.com/tiulpin/qodana/releases/latest).

### Running

#### tl;dr

If you know what you are doing:

```
qodana scan --show-report
```

You can also add the linter by its name with the `--linter` option (e.g. `--linter jetbrains/qodana`).

#### Project configuration

Before you start using Qodana, you need to configure your project – choose a linter to use.
If you know what linter you want to use, you can skip this step.
Qodana CLI can do that for you. Just run the following command in your project root:

```sh
qodana init
```

#### Project analysis

Right after you configured your project (or remembered linter name you want to run), you can run Qodana inspections simply by invoking the following command in your project root:

```sh
qodana scan
```

- After the first Qodana run, the following runs will be faster because of the saved Qodana cache in your project (defaults to `./.qodana/cache`)
- Latest Qodana report will be saved to `./.qodana/results` – you can find qodana.sarif.json and other Qodana artifacts (like logs) in this directory.

#### Show report

After the analysis, the results are saved to `./.qodana/results` by default. Inside the directory `./.qodana/results/report`, you can find Qodana HTML report.
To view it in the browser, run the following command from your project root:

```shell
qodana show
```

You can serve any Qodana HTML report regardless of the project if you provide the correct report path.

## Configuration

Find more CLI options, run `qodana ...` commands with the `--help` flag. Currently, there are not many options.
If you want to configure Qodana or a check inside Qodana, consider using [`qodana.yaml` ](https://www.jetbrains.com/help/qodana/qodana-yaml.html) to have the same configuration on any CI you use and your machine.

#### Disable telemetry

To disable [Qodana user statistics](https://www.jetbrains.com/help/qodana/qodana-jvm-docker-readme.html#Usage+statistics) and [CLI Sentry crash reporting](https://blog.sentry.io/2016/02/09/what-is-crash-reporting), export the `DO_NOT_TRACK` environment variable to `1` before running the CLI:

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
  -c, --cache-dir string          Cache directory (default ".qodana/cache")
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
  -o, --results-dir string        Directory to save Qodana inspection results to (default ".qodana/results")
      --run-promo                 Set to true to have the application run the inspections configured by the promo profile; set to false otherwise. By default, a promo run is enabled if the application is executed with the default profile and is disabled otherwise
  -s, --save-report               Generate HTML report (default true)
      --script string             Override the run scenario (default "default")
      --send-report               Send the inspection report to Qodana Cloud, requires the '--token' option to be specified
  -w, --show-report               Serve HTML report on port
  -d, --source-directory string   Directory inside the project-dir directory must be inspected. If not specified, the whole project is inspected.
      --stub-profile string       Absolute path to the fallback profile file. This option is applied in case the profile was not specified using any available options
  -t, --token string              Qodana Cloud token
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
  -h, --help                help for show
  -p, --port int            Specify port to serve report at (default 8080)
  -r, --report-dir string   Specify HTML report path (the one with index.html inside) (default ".qodana/results/report")
```

[gh:test]: https://github.com/tiulpin/qodana/actions/workflows/build-test.yml
[youtrack]: https://youtrack.jetbrains.com/issues/QD
[youtrack-new-issue]: https://youtrack.jetbrains.com/newIssue?project=QD&c=Platform%20GitHub%20Action
[jb:confluence-on-gh]: https://confluence.jetbrains.com/display/ALL/JetBrains+on+GitHub
[jb:slack]: https://jb.gg/qodana-slack
[jb:twitter]: https://twitter.com/QodanaEvolves
[jb:docker]: https://hub.docker.com/r/jetbrains/qodana
