# Qodana CLI

> 💡 **Note**: This is experimental project, so it's not guaranteed to work correctly.
> Use it at your own risk. For running Qodana stably and reliably, please use [Qodana Docker Images](https://www.jetbrains.com/help/qodana/docker-images.html).

`qodana` is a command line interface for [Qodana](https://jetbrains.com/qodana). 
It allows you to run Qodana inspections on your local machine (or a CI agent) easily, by running [Qodana Docker Images](https://www.jetbrains.com/help/qodana/docker-images.html). You can 

## Prerequisites

The Qodana CLI is distributed and run as a binary. The actual linters with inspections are Docker Images. 
To support this, you must have Docker installed and running locally.

## Installation

Install and run `qodana` to `/urs/local/bin` (only Linux and macOS supported):

```shell
curl -fsSL https://raw.githubusercontent.com/tiulpin/qodana/main/install | bash # gets the latest version
```

Alternatively, you can install the latest binary from [GitHub Releases](https://github.com/tiulpin/qodana/releases/latest).

## Usage

### Project configuration

Before you start using Qodana, you need to configure your project. 
Qodana CLI can do that for you, by running the following command:

```shell
qodana init  # in your project root
```

### Project analysis

Right after you configured your project, you can run Qodana inspections simply by invoking the following command:

```shell
qodana scan # in your project root
```

- After the first Qodana run, the following runs will be faster because of the saved Qodana cache in your project (defaults to `./.qodana/cache`)
- Latest Qodana report will be saved to `./.qodana/report` – you can find qodana.sarif.json and other Qodana artifacts (like logs) in this directory.

### Show report

After analysis is done, the report is saved by default to `./.qodana/report`. Inside that directory, you can find Qodana HTML report.
To view it in the browser, run the following command:

```shell
qodana show # in your project root
```

You can serve any Qodana HTML report regardless of the project, if you provide the correct report path.
