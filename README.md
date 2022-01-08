# qodana

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

```shell
git tag -a v0.1.1 -m "v0.1.1" 
git push origin v0.1.1
```