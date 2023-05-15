# Contributing

By participating in this project, you agree to abide our [Code of conduct](.github/CODE_OF_CONDUCT.md).

## Set up your machine

`qodana` CLI is written in [Go](https://golang.org/).

Prerequisites:

- [Go 1.16+](https://golang.org/doc/install)

Other things you might need to develop:

- [GoLand](https://www.jetbrains.com/go/) (it's [free for open-source development](https://www.jetbrains.com/community/opensource/))

Clone `qodana` anywhere:

```sh
git clone git@github.com:JetBrains/qodana.git
```

`cd` into the directory and run for debug:

```sh
go run main.go
```

Build a binary with

```sh
go build -o qodana main.go
```

Lint your code with `golangci-lint`:

```sh
golangci-lint run
```

Test your code with coverage:
```sh
go test -v ./... -coverprofile cover.out
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

Push your branch to your `qodana` fork and open a pull request against the
main branch.


## Release a new version

If you are a core maintainer and want to release a new version, all you need is just running the following command:

```shell
git tag -a vX.X.X -m "vX.X.X" && git push origin vX.X.X
```

And goreleaser will do the rest.
