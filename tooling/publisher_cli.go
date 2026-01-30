package tooling

import _ "embed"

//go:generate go run download-resource.go -artifact publisher-cli
//go:embed publisher-cli.jar
var PublisherCli []byte
