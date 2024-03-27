module github.com/JetBrains/qodana-cli/v2024/cloud

go 1.21

require (
	github.com/sirupsen/logrus v1.9.3
	github.com/stretchr/testify v1.9.0
)

require (
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	golang.org/x/sys v0.18.0 // indirect
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/JetBrains/qodana-cli/v2024/cloud => ../cloud
	github.com/JetBrains/qodana-cli/v2024/cmd => ../cmd
	github.com/JetBrains/qodana-cli/v2024/core => ../core
	github.com/JetBrains/qodana-cli/v2024/platform => ../platform
	github.com/JetBrains/qodana-cli/v2024/sarif => ../sarif
	github.com/JetBrains/qodana-cli/v2024/tooling => ../tooling
)
