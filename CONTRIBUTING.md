# Contributing

By participating in this project, you agree to abide our [Code of conduct](.github/CODE_OF_CONDUCT.md).

## Set up your machine

`qd` is written in [Go](https://golang.org/).

Prerequisites:

- [Go 1.21+](https://golang.org/doc/install)

Other things you might need to develop:

- [GoLand](https://www.jetbrains.com/go/) (it's [free for open-source development](https://www.jetbrains.com/community/opensource/))

Clone the project anywhere:

```sh
git clone git@github.com:JetBrains/qodana-cli.git
```

Prepare embedded tools:

1. `cd` into the `tooling` directory
2. Run `go run scripts/download-resource.go config-loader-cli.jar` and `go run scripts/download-resource.go publisher-cli.jar`
3. Either run if you don't test the related functionality:
   ```
   touch baseline-cli.jar intellij-report-converter.jar qodana-fuser.jar
   ```
   or go to the [latest qodana-cli build](https://buildserver.labs.intellij.net/buildConfiguration/StaticAnalysis_Cli_Nightlyclimain) 
   (internal only), go to `dependencies` tab and download the artifacts you need.
4. `cd` back to the root directory

`cd` into the `cli` directory and run for debug:

```sh
go run main.go
```

Build a binary with

```sh
go build -o qd main.go
```

Test your code with coverage:
```sh
go test -v $(go list -f '{{.Dir}}/...' -m)
```

Test your code with a human-readable report (requires `go install github.com/mfridman/tparse@latest`):
```shell
export GITHUB_ACTIONS=true # skip third-party linter tests
set -o pipefail && go test -json -v $(go list -f '{{.Dir}}/...' -m) | tparse -all
```

Dry-run goreleaser:

```sh
goreleaser release --snapshot --clean
```

## Test 3rd party linters

Inside 3rd party linters docker image a different qodana-cli executable is used. To build it:
1. `cd` into the 3rd party linter directory (for this example, we will use cdnet - clang is the same)
2. Download the linter binary from the [latest qodana-cdnet build](https://buildserver.labs.intellij.net/buildConfiguration/ijplatform_master_QodanaCdNetBinary#all-projects) (internal only).
   To do this, open the latest build, go to `dependencies` tab, download `*.nupkg` file from the first dependency, place it in the current directory and rename it to `clt.zip`
3. Run `go generate`
4. Change the `buildDateStr` variable in [main.go](cdnet/main.go) to a more recent date (e.g., update it from "2023-12-05T10:52:23Z" to today's date in the same format) to avoid EAP expiration errors.
5. Build the executable `env GOOS=linux CGO_ENABLED=0 go build -o qd-custom`
6. To replace the executable in docker image, see `'Patching' an existing Qodana image` section below. Note that the `qodana-cdnet` image has qodana executable in `/opt/qodana/qodana` path.

## Create a commit

Commit messages should be well formatted, and to make that "standardized", we are using Gitmoji.

You can follow the documentation on
[their website](https://gitmoji.dev).


## Submit a pull request

Push your branch to your repository fork and open a pull request against the
main branch.

## 'Patching' an existing Qodana image

For testing purposes, it can be necessary to patch an existing Qodana image with a custom qodana-cli build.
To achieve that, first build a linux binary:
```shell
# assume we're in the cli directory
env GOOS=linux CGO_ENABLED=0 go build -o qd-custom
```

Then build a new docker image, replacing the bundled qodana-cli with the newly built one:
```dockerfile
# Use any existing qodana image
FROM registry.jetbrains.team/p/sa/containers/qodana-go:latest
COPY qd-custom /opt/idea/bin/qodana
```
```shell
docker build . -t qd-image
```

And lastly run the custom image with the custom binary:
```shell
/path/to/qodana-cli/cli/qd-custom scan --linter="docker.io/library/qd-image" --skip-pull
```

## Release a new version

If you are a core maintainer and want to release a new version, all you need to release a new version is:

1. Tag release **in the release branch** (e.g. `241`)
  ```
  git checkout 241 && git tag -a vX.X.X -m "vX.X.X" && git push origin vX.X.X
  ```
2. Trigger [release job](https://buildserver.labs.intellij.net/buildConfiguration/StaticAnalysis_Cli_Releasecli) **in the release branch** (e.g. `241`)
3. The release will be published to:
- [`JetBrains/qodana-cli`](https://github.com/JetBrains/qodana-cli/releases/) release page
- [Chocolatey](https://community.chocolatey.org/packages/qodana) registry
- GitHub's repositories that are used for package managers:
  - external (updates are done via pull requests): [`Microsoft/winget-pkgs`](https://github.com/microsoft/winget-pkgs/pulls?q=JetBrains.QodanaCLI)
  - internal (updates are done via direct commits): [`JetBrains/scoop-utils`](https://github.com/jetbrains/scoop-utils) and [`JetBrains/homebrew-utils`](https://github.com/jetbrains/homebrew-utils)
4. For all CIs: the update will be done automatically via pull request, read https://github.com/JetBrains/qodana-action/blob/main/CONTRIBUTING.md#release-a-new-version

### Troubleshooting `choco` releases

Releases through `choco` channel can be unstable sometimes depending on the Chocolatey services,
so if you have any issues with it on release, upload the package manually:

- Set up [`goreleaser`](https://goreleaser.com/install/) and [`choco`](https://chocolatey.org/install) (for non-Windows systems – look at [ci.yml]([.github/workflows/ci.yml](https://github.com/JetBrains/qodana-cli/blob/ca90ffe4ca0b33fda19b471cc80c7390c7e0bfd9/.github/workflows/ci.yml#L69)) for details)
- Run the following commands:
  - Check out the wanted tag
  - Release the package locally to generate all metadata files and executables
  - Set the correct checksum for the already published package (can be obtained from the release page)
  - Set up `choco` API key and publish

```shell
git checkout v2025.1.2
goreleaser release --skip-publish --clean
vim dist/qodana.choco/tools/chocolateyinstall.ps1
choco apikey --key <YOUR_API_KEY> --source https://push.chocolatey.org/
cd dist/qodana.choco && choco pack && choco push qodana.2024.1.2.nupkg --source https://push.chocolatey.org/
```
