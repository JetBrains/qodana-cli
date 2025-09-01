module github.com/JetBrains/qodana-cli/v2025/cloud

go 1.24.0

require (
	github.com/JetBrains/qodana-cli/v2025/platform v0.0.0-00010101000000-000000000000
	github.com/sirupsen/logrus v1.9.3
	github.com/stretchr/testify v1.11.1
)

require (
	github.com/cucumber/ci-environment/go v0.0.0-20230911180507-bd001ebc644c // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rogpeppe/go-internal v1.13.1 // indirect
	golang.org/x/sys v0.35.0 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/JetBrains/qodana-cli/v2025/cloud => ../cloud
	github.com/JetBrains/qodana-cli/v2025/cmd => ../cmd
	github.com/JetBrains/qodana-cli/v2025/core => ../core
	github.com/JetBrains/qodana-cli/v2025/platform => ../platform
	github.com/JetBrains/qodana-cli/v2025/sarif => ../sarif
	github.com/JetBrains/qodana-cli/v2025/tooling => ../tooling
)
