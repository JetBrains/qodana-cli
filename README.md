# Qodana CLI

> 💡 **Note**: This is experimental project, so it's not guaranteed to work correctly.
> Use it at your own risk. For running Qodana stably and reliably, please use [Qodana Docker Images](https://www.jetbrains.com/help/qodana/docker-images.html).

`qodana` is a command line interface for [Qodana](https://jetbrains.com/qodana). 
It allows you to run Qodana inspections on your local machine (or a CI agent) easily, by running [Qodana Docker Images](https://www.jetbrains.com/help/qodana/docker-images.html).

## Prerequisites

The Qodana CLI is distributed and run as a binary. The actual linters with inspections are Docker Images. 
To support this, you must have Docker installed and running locally.

## Installation

We have installation scripts for Linux, macOS and Windows. Alternatively, you can install the latest binary from [GitHub Releases](https://github.com/tiulpin/qodana/releases/latest).

### Linux and macOS

Install and run `qodana` to `/urs/local/bin`:

```shell
curl -fsSL https://raw.githubusercontent.com/tiulpin/qodana/main/install | bash
```

If you want to install some specific version, add the version number (e.g. `0.2.0`) **in the end** of the command.

### Windows

Install and run `qodana` to `$Home\bin`:

```shell
iwr https://raw.githubusercontent.com/tiulpin/qodana/main/install.ps1 -useb | iex
```

If you want to install some specific version, add the version number (e.g. `$v="0.2.0";`) **in the beginning** of the command.

## Usage

### Project configuration

Before you start using Qodana, you need to configure your project – choose a linter to use. 
If you know what linter do you want to use, you can skip this step. 
Qodana CLI can do that for you, just run the following command in your project root:

```sh
qodana init
```

### Project analysis

Right after you configured your project (or remembered linter name you want to run), you can run Qodana inspections simply by invoking the following command in your project root:

```sh
qodana scan
```

- After the first Qodana run, the following runs will be faster because of the saved Qodana cache in your project (defaults to `./.qodana/cache`)
- Latest Qodana report will be saved to `./.qodana/results` – you can find qodana.sarif.json and other Qodana artifacts (like logs) in this directory.

### Show report

After analysis is done, the results are saved to `./.qodana/results` by default. Inside the directory `./.qodana/results/report`, you can find Qodana HTML report.
To view it in the browser, run the following command from your project root:

```shell
qodana show
```

You can serve any Qodana HTML report regardless of the project, if you provide the correct report path.

### Disable telemetry

To disable [Qodana user statistics](https://www.jetbrains.com/help/qodana/qodana-jvm-docker-readme.html#Usage+statistics) and [CLI Sentry crash reports](https://blog.sentry.io/2016/02/09/what-is-crash-reporting), export the `DO_NOT_TRACK` environment variable to `1` before running the CLI:

```sh
export DO_NOT_TRACK=1
```


### More configuration

To find more CLI options, run `qodana ...` commands with the `--help` flag. Currently there are not many options, if you want to configure Qodana or checks inside Qodana, consider using [`qodana.yaml` ](https://www.jetbrains.com/help/qodana/qodana-yaml.html).
