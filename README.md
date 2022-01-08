# qodana

> **Note**: This is experimental project, so it's not guaranteed to work correctly.
> Use it at your own risk. For running Qodana stably and reliably, please use [Qodana Docker Images](https://www.jetbrains.com/help/qodana/docker-images.html).

## Usage

Install and run (only Linux and macOS supported):

```shell
curl https://i.jpillora.com/tiulpin/qodana\! | bash  # gets the latest version
qodana init  # in your project root
qodana scan  # in your project root
```

## Development

### Try

Run for debug with (go 1.16+ is required)

```shell
go run main.go
```

### Build

Build a binary with
```shell
go build -o qodana main.go
```

### Release a new version

Just run
```shell
v=v0.1.2 git tag -a $v -m "$v" && git push origin $v
```

goreleaser will do the rest.
