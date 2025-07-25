module github.com/JetBrains/qodana-cli/v2025/cli

go 1.24.0

require (
	github.com/boyter/scc/v3 v3.4.0 // indirect
	github.com/cucumber/ci-environment/go v0.0.0-20230911180507-bd001ebc644c // indirect
	github.com/docker/cli v25.0.0+incompatible // indirect
	github.com/docker/docker v25.0.6+incompatible // indirect; DO NOT UPDATE: breaking changes
	github.com/go-enry/go-enry/v2 v2.9.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/liamg/clinch v1.6.6 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/otiai10/copy v1.14.1 // indirect
	github.com/pterm/pterm v0.12.80 // indirect
	github.com/shirou/gopsutil/v3 v3.24.5 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/spf13/cobra v1.8.1 // indirect
	github.com/spf13/viper v1.19.0 // indirect
	github.com/zalando/go-keyring v0.2.6 // indirect
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

require (
	github.com/JetBrains/qodana-cli/v2025/cmd v0.0.0-00010101000000-000000000000
	github.com/JetBrains/qodana-cli/v2025/platform v0.0.0-00010101000000-000000000000
)

require (
	al.essio.dev/pkg/shellescape v1.5.1 // indirect
	atomicgo.dev/cursor v0.2.0 // indirect
	atomicgo.dev/keyboard v0.2.9 // indirect
	atomicgo.dev/schedule v0.1.0 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20210617225240-d185dfc1b5a1 // indirect
	github.com/JetBrains/qodana-cli/v2025/cloud v0.0.0-00010101000000-000000000000 // indirect
	github.com/JetBrains/qodana-cli/v2025/core v0.0.0-00010101000000-000000000000 // indirect
	github.com/JetBrains/qodana-cli/v2025/sarif v0.0.0-00010101000000-000000000000 // indirect
	github.com/JetBrains/qodana-cli/v2025/tooling v0.0.0-00010101000000-000000000000 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/boyter/gocodewalker v1.3.4 // indirect
	github.com/containerd/console v1.0.3 // indirect
	github.com/danieljoos/wincred v1.2.2 // indirect
	github.com/danwakefield/fnmatch v0.0.0-20160403171240-cbb64ac3d964 // indirect
	github.com/distribution/reference v0.5.0 // indirect
	github.com/docker/docker-credential-helpers v0.8.0 // indirect
	github.com/docker/go-connections v0.5.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/go-enry/go-oniguruma v1.2.1 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/gookit/color v1.5.4 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/liamg/tml v0.3.0 // indirect
	github.com/lithammer/fuzzysearch v1.1.8 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/moby/term v0.5.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.2 // indirect
	github.com/otiai10/mint v1.6.3 // indirect
	github.com/pelletier/go-toml/v2 v2.2.2 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/reviewdog/go-bitbucket v0.0.0-20201024094602-708c3f6a7de0 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/sagikazarmark/locafero v0.4.0 // indirect
	github.com/sagikazarmark/slog-shim v0.1.0 // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/sourcegraph/conc v0.3.0 // indirect
	github.com/spf13/afero v1.11.0 // indirect
	github.com/spf13/cast v1.6.0 // indirect
	github.com/spf13/pflag v1.0.6 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.49.0 // indirect
	go.opentelemetry.io/otel v1.37.0 // indirect
	go.opentelemetry.io/otel/metric v1.37.0 // indirect
	go.opentelemetry.io/otel/trace v1.37.0 // indirect
	go.uber.org/atomic v1.9.0 // indirect
	go.uber.org/multierr v1.9.0 // indirect
	golang.org/x/crypto v0.38.0 // indirect
	golang.org/x/exp v0.0.0-20230905200255-921286631fa9 // indirect
	golang.org/x/net v0.40.0 // indirect
	golang.org/x/oauth2 v0.30.0 // indirect
	golang.org/x/sync v0.15.0 // indirect
	golang.org/x/sys v0.33.0 // indirect
	golang.org/x/term v0.32.0 // indirect
	golang.org/x/text v0.26.0 // indirect
	golang.org/x/time v0.5.0 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)
