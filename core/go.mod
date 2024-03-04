module github.com/JetBrains/qodana-cli/v2024/core

go 1.21

require (
	github.com/cucumber/ci-environment/go v0.0.0-20230911180507-bd001ebc644c
	github.com/docker/cli v25.0.0+incompatible
	github.com/docker/docker v20.10.23+incompatible // DO NOT UPDATE: breaking changes
	github.com/otiai10/copy v1.14.0
	github.com/owenrumney/go-sarif/v2 v2.3.0
	github.com/pterm/pterm v0.12.79
	github.com/shirou/gopsutil/v3 v3.24.2
	github.com/sirupsen/logrus v1.9.3
	github.com/stretchr/testify v1.9.0
)

replace (
	github.com/JetBrains/qodana-cli/v2024/cloud => ../cloud
	github.com/JetBrains/qodana-cli/v2024/cmd => ../cmd
	github.com/JetBrains/qodana-cli/v2024/core => ../core
	github.com/JetBrains/qodana-cli/v2024/platform => ../platform
	github.com/JetBrains/qodana-cli/v2024/sarif => ../sarif
	github.com/JetBrains/qodana-cli/v2024/tooling => ../tooling
)

require (
	github.com/JetBrains/qodana-cli/v2024/cloud v0.0.0-00010101000000-000000000000
	github.com/JetBrains/qodana-cli/v2024/platform v0.0.0-00010101000000-000000000000
)

require (
	atomicgo.dev/cursor v0.2.0 // indirect
	atomicgo.dev/keyboard v0.2.9 // indirect
	atomicgo.dev/schedule v0.1.0 // indirect
	github.com/JetBrains/qodana-cli/v2024/sarif v0.0.0-00010101000000-000000000000 // indirect
	github.com/JetBrains/qodana-cli/v2024/tooling v0.0.0-00010101000000-000000000000 // indirect
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/alessio/shellescape v1.4.1 // indirect
	github.com/containerd/console v1.0.3 // indirect
	github.com/danieljoos/wincred v1.2.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/distribution/reference v0.5.0 // indirect
	github.com/docker/distribution v2.8.3+incompatible // indirect
	github.com/docker/docker-credential-helpers v0.8.0 // indirect
	github.com/docker/go-connections v0.5.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/go-enry/go-enry/v2 v2.8.6 // indirect
	github.com/go-enry/go-oniguruma v1.2.1 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/uuid v1.5.0 // indirect
	github.com/gookit/color v1.5.4 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/liamg/clinch v1.6.6 // indirect
	github.com/liamg/tml v0.3.0 // indirect
	github.com/lithammer/fuzzysearch v1.1.8 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.15 // indirect
	github.com/moby/term v0.5.0 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.2 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/rivo/uniseg v0.4.4 // indirect
	github.com/rogpeppe/go-internal v1.9.0 // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/spf13/cobra v1.8.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	github.com/zalando/go-keyring v0.2.3 // indirect
	golang.org/x/crypto v0.20.0 // indirect
	golang.org/x/mod v0.12.0 // indirect
	golang.org/x/sync v0.5.0 // indirect
	golang.org/x/sys v0.17.0 // indirect
	golang.org/x/term v0.17.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	golang.org/x/time v0.5.0 // indirect
	golang.org/x/tools v0.13.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	gotest.tools/v3 v3.5.1 // indirect
)
