package tooling

import _ "embed"

//go:generate go run scripts/download-resource.go config-loader-cli.jar
//go:embed config-loader-cli.jar
var ConfigLoaderCli []byte
