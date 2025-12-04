module github.com/JetBrains/qodana-cli/v2025/cmd

go 1.25.2

require (
	github.com/boyter/scc/v3 v3.6.0
	github.com/sirupsen/logrus v1.9.3
	github.com/spf13/cobra v1.10.1
	github.com/spf13/viper v1.21.0
)

replace (
	github.com/JetBrains/qodana-cli/v2025/cloud => ../cloud
	github.com/JetBrains/qodana-cli/v2025/core => ../core
	github.com/JetBrains/qodana-cli/v2025/platform => ../platform
	github.com/JetBrains/qodana-cli/v2025/sarif => ../sarif
	github.com/JetBrains/qodana-cli/v2025/tooling => ../tooling
	google.golang.org/genproto => google.golang.org/genproto/googleapis/rpc v0.0.0-20250825161204-c5933d9347a5
)

require (
	github.com/google/uuid v1.6.0
	github.com/otiai10/copy v1.14.1
)

require (
	github.com/agnivade/levenshtein v1.2.2-0.20250519083737-420867539855 // indirect
	github.com/boyter/gocodewalker v1.5.1 // indirect
	github.com/clipperhouse/uax29/v2 v2.2.0 // indirect
	github.com/danwakefield/fnmatch v0.0.0-20160403171240-cbb64ac3d964 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/mattn/go-runewidth v0.0.19 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/otiai10/mint v1.6.3 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/sagikazarmark/locafero v0.11.0 // indirect
	github.com/sourcegraph/conc v0.3.1-0.20240121214520-5f936abd7ae8 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/crypto v0.43.0 // indirect
	golang.org/x/sync v0.17.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/text v0.30.0 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)
