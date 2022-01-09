# Contributing

[//]: # (By participating in this project, you agree to abide our [code of conduct]&#40;TODO:ADD CODE_OF_CONDUCT.md&#41;.)

## Setup your machine

`qodana` CLI is written in [Go](https://golang.org/).

Prerequisites:

- [Go 1.16+](https://golang.org/doc/install)

Other things you might need to develop:

- [GoLand](https://www.jetbrains.com/go/) (it's [free for open source](https://www.jetbrains.com/community/opensource/))

Clone `qodana` anywhere:

```sh
git clone git@github.com:tiulpin/qodana.git
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

Dry run goreleaser:

```sh
goreleaser release --snapshot --rm-dist
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
