module github.com/JetBrains/qodana-cli/v2024/cdnet

go 1.21

replace (
	github.com/JetBrains/qodana-cli/v2024/cmd => ../cmd
	github.com/JetBrains/qodana-cli/v2024/core => ../core
	github.com/JetBrains/qodana-cli/v2024/platform => ../platform
	github.com/JetBrains/qodana-cli/v2024/sarif => ../sarif
	github.com/JetBrains/qodana-cli/v2024/tooling => ../tooling
)
