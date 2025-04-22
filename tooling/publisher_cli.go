package tooling

import _ "embed"

//go:generate go run scripts/download-publisher-cli.go 3.0.3

//go:embed publisher-cli.jar
var PublisherCli []byte
