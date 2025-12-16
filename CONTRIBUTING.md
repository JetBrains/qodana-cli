# Contributing

By participating in this project, you agree to abide our [Code of conduct](.github/CODE_OF_CONDUCT.md).

## Set up your machine

`qd` is written in [Go](https://golang.org/).

Prerequisites:

- [Go 1.25+](https://golang.org/doc/install)
- [Docker](https://docs.docker.com/get-docker/)

Other things you might need to develop:

- [GoLand](https://www.jetbrains.com/go/) (it's [free for open-source development](https://www.jetbrains.com/community/opensource/))
- [IntelliJ IDEA](https://www.jetbrains.com/idea/) (it's [free for open-source development](https://www.jetbrains.com/community/opensource/))

Clone the project anywhere:

```sh
git clone git@github.com:JetBrains/qodana-cli.git
```

### Set up environment secrets

Create a `.env` file in the repository root (it's gitignored):

```sh
cp .env.example .env
```

Edit `.env` and add your tokens:
- `TEAMCITY_TOKEN` – for downloading closed-source dependencies (internal only, get from [TeamCity profile](https://buildserver.labs.intellij.net/profile.html?item=accessTokens))
- `QODANA_LICENSE_ONLY_TOKEN` – for running tests that require license validation (get a temporary token from Qodana Cloud)

### Prepare embedded tools

**For JetBrains employees (with VPN access):**

Run the download script to fetch all closed-source dependencies from TeamCity:
```sh
go run scripts/download-deps.go
```

Then download the public Maven JARs:
```sh
go generate ./internal/tooling/...
```

**For external contributors:**

1. Create empty stubs for closed-source JARs (tests using them will be skipped):
   ```sh
   touch internal/tooling/baseline-cli.jar internal/tooling/intellij-report-converter.jar internal/tooling/qodana-fuser.jar
   ```
2. Download public JARs via go generate:
   ```sh
   go generate ./internal/tooling/...
   ```

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
go test -v ./...
```

Test your code with a human-readable report (requires `go install github.com/mfridman/tparse@latest`):
```sh
go test -timeout 0 -json -v ./... > test.json 2>&1; tparse -all -file=test.json
```

To skip third-party linter tests (if you don't have cdnet/clang dependencies):
```sh
export GITHUB_ACTIONS=true
go test -v ./...
```

Dry-run goreleaser:

```sh
goreleaser release --snapshot --clean
```

## Test 3rd party linters

### Prerequisites

Install required tools via Homebrew (macOS):
```sh
brew install cmake dotnet openjdk@17
```

### Running tests locally

**For JetBrains employees:**

1. Ensure `.env` is configured with `TEAMCITY_TOKEN` and `QODANA_LICENSE_ONLY_TOKEN`
2. Download all dependencies:
   ```sh
   go run scripts/download-deps.go
   go generate ./...
   ```
3. Run all tests with Java 17:
   ```sh
   source .env
   go test -timeout 0 -v ./...
   ```

### Building a custom 3rd party linter

Inside 3rd party linters docker image a different qodana-cli executable is used. To build it:
1. `cd` into the 3rd party linter directory (e.g., `cdnet` or `clang`)
2. Ensure dependencies are downloaded (see above) and run `go generate`
3. Change the `buildDateStr` variable in [main.go](cdnet/main.go) to a more recent date (e.g., update it from "2023-12-05T10:52:23Z" to today's date in the same format) to avoid EAP expiration errors
4. Build the executable: `env GOOS=linux CGO_ENABLED=0 go build -o qd-custom`
5. To replace the executable in docker image, see `'Patching' an existing Qodana image` section below. Note that the `qodana-cdnet` image has qodana executable in `/opt/qodana/qodana` path

## Working with Docker images

### Build Docker images

With [Docker Bake](https://docs.docker.com/build/bake/) you can build all base images at once:

```shell
cd dockerfiles/base && docker buildx bake
```

### Verify product feed

`cd` into `.github/scripts` and run the script to check product feed if you edited something in `feed/releases.json`:

```shell
cd .github/scripts && node verifyChecksums.js
```

### Generate Dockerfiles

To generate Dockerfiles for a release:

```shell
./dockerfiles.py dockerfiles
```

To add a newly released product, check `dockerfiles/public.json`.

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
