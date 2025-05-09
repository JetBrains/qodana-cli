package tooling

import _ "embed"

//go:generate go run scripts/download-resource.go publisher-cli.jar
//go:embed publisher-cli.jar
var PublisherCli []byte
