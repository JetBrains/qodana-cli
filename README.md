# qodana-cli

## Goals

The main goal of this project is unifying running Qodana everywhere (in CI or locally) with as fewer efforts as possible.

Imagine if installing this binary is this easy:
```bash
curl https://get.qodana.com/ -fsSL | bash
```

Then configuring the project is done via CLI or via IDEA with Qodana IDE plugin installed:
```bash
qodana init
```

And then running the whole project analysis with qodana.yaml configured is like
```bash
qodana scan
```

### Basic CLI commands implementation

- [ ] qodana init
  - check if Docker is installed
    - warn user about problems of running Qodana if less than 4GB RAM is configured
  - analyze the project (with go-enry) and enable Qodana linters relevant to it
  - add all the needed information to qodana.yaml
  - pull the images
- [ ] qodana scan
    - docker run with all needed directories and configured options (don't forget the caches!)
- [ ] qodana fix
  - when https://youtrack.jetbrains.com/issue/QD-775 is done, this command will make sense

### Qodana runners integration

One of the most important project goals is to implement one protocol to communicate with all possible CI and build system plugins.
Currently, we already have the following runners:

- [GitHub Action](https://github.com/JetBrains/qodana-action)
- [Gradle plugin](https://github.com/JetBrains/gradle-qodana-plugin)
- [TeamCity plugin](https://jetbrains.team/p/sa/repositories/teamcity/files/README.md)

All of these depend on different type of configuration, difficult to maintain and update to the latest linters with the latest configuration options.


## Development

### Try

Run for debug with (go 1.16+ is required)

```go run main.go jvm```

### Build

Build a binary with
```go build -o qodana main.go```
