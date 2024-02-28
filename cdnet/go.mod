module github.com/JetBrains/qodana-cli/v2024/cdnet

go 1.21

replace (
	github.com/JetBrains/qodana-cli/v2024/cmd => ../cmd
	github.com/JetBrains/qodana-cli/v2024/core => ../core
	github.com/JetBrains/qodana-cli/v2024/linter => ../cloud
	github.com/JetBrains/qodana-cli/v2024/platform => ../platform
	github.com/JetBrains/qodana-cli/v2024/sarif => ../sarif
	github.com/JetBrains/qodana-cli/v2024/tooling => ../tooling
)

require (
	github.com/JetBrains/qodana-cli/v2024/linter v0.0.0-00010101000000-000000000000
	github.com/sirupsen/logrus v1.9.3
)

require (
	golang.org/x/sys v0.16.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
