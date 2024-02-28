# Contributing

By participating in this project, you agree to abide our [Code of conduct](.github/CODE_OF_CONDUCT.md).

## Set up your machine

`qd` is written in [Go](https://golang.org/).

Prerequisites:

- [Go 1.19+](https://golang.org/doc/install)

Other things you might need to develop:

- [GoLand](https://www.jetbrains.com/go/) (it's [free for open-source development](https://www.jetbrains.com/community/opensource/))

Clone the project anywhere:

```sh
git clone git@github.com:JetBrains/qodana-cli.git
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
go test -v $(go list -f '{{.Dir}}/...' -m | xargs)
```

Dry run goreleaser:

```sh
goreleaser release --snapshot --clean
```

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

If you are a core maintainer and want to release a new version, all you need is just running the following command:

```shell
git tag -a vX.X.X -m "vX.X.X" && git push origin vX.X.X
```

And goreleaser will do the rest.

### Troubleshooting `choco` releases

Releases through `choco` channel can be unstable, so if you have any issues with it on release, upload the package manually:

- Set up [`goreleaser`](https://goreleaser.com/install/) and [`choco`](https://chocolatey.org/install) (for non-Windows systems – look at [ci.yml]([.github/workflows/ci.yml](https://github.com/JetBrains/qodana-cli/blob/ca90ffe4ca0b33fda19b471cc80c7390c7e0bfd9/.github/workflows/ci.yml#L69)) for details)
- Run the following commands:
   - Check out to the wanted tag
   - Release the package locally to generate all metadata files and executables
   - Set the correct checksum for the already published package (can be obtained from the release page)
   - Set up `choco` API key and publish

```shell
git checkout v2023.2.5
goreleaser release --skip-publish --clean
vim dist/qodana.choco/tools/chocolateyinstall.ps1
choco apikey --key <YOUR_API_KEY> --source https://push.chocolatey.org/
cd dist/qodana.choco && choco pack && choco push qodana.2023.2.5.nupkg --source https://push.chocolatey.org/
```
