module github.com/JetBrains/qodana-cli/v2025/platform

go 1.24.0

require (
	github.com/cucumber/ci-environment/go v0.0.0-20230911180507-bd001ebc644c
	github.com/go-enry/go-enry/v2 v2.9.2
	github.com/google/uuid v1.6.0
	github.com/liamg/clinch v1.6.6
	github.com/mattn/go-isatty v0.0.20
	github.com/otiai10/copy v1.14.1
	github.com/pterm/pterm v0.12.80
	github.com/sirupsen/logrus v1.9.3
	github.com/spf13/cobra v1.9.1
	github.com/stretchr/testify v1.10.0
	github.com/zalando/go-keyring v0.2.6
	gopkg.in/yaml.v3 v3.0.1
)

replace (
	github.com/JetBrains/qodana-cli/v2025/cloud => ../cloud
	github.com/JetBrains/qodana-cli/v2025/platform => ../platform
	github.com/JetBrains/qodana-cli/v2025/sarif => ../sarif
	github.com/JetBrains/qodana-cli/v2025/tooling => ../tooling
)

require (
	github.com/JetBrains/qodana-cli/v2025/cloud v0.0.0-00010101000000-000000000000
	github.com/JetBrains/qodana-cli/v2025/sarif v0.0.0-00010101000000-000000000000
	github.com/JetBrains/qodana-cli/v2025/tooling v0.0.0-00010101000000-000000000000
	github.com/docker/docker v28.0.0+incompatible
	github.com/reviewdog/go-bitbucket v0.0.0-20201024094602-708c3f6a7de0
	github.com/shirou/gopsutil/v3 v3.24.5
)

require (
	al.essio.dev/pkg/shellescape v1.5.1 // indirect
	atomicgo.dev/cursor v0.2.0 // indirect
	atomicgo.dev/keyboard v0.2.9 // indirect
	atomicgo.dev/schedule v0.1.0 // indirect
	github.com/Microsoft/go-winio v0.4.14 // indirect
	github.com/containerd/console v1.0.3 // indirect
	github.com/containerd/log v0.1.0 // indirect
	github.com/danieljoos/wincred v1.2.2 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/distribution/reference v0.6.0 // indirect
	github.com/docker/go-connections v0.5.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-enry/go-oniguruma v1.2.1 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/gookit/color v1.5.4 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/klauspost/cpuid/v2 v2.2.5 // indirect
	github.com/liamg/tml v0.3.0 // indirect
	github.com/lithammer/fuzzysearch v1.1.8 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/moby/term v0.5.2 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.0 // indirect
	github.com/otiai10/mint v1.6.3 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/rivo/uniseg v0.4.4 // indirect
	github.com/sergi/go-diff v1.3.2-0.20230802210424-5b0b94c5c0d3 // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/spf13/pflag v1.0.6 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.59.0 // indirect
	go.opentelemetry.io/otel v1.34.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.34.0 // indirect
	go.opentelemetry.io/otel/metric v1.34.0 // indirect
	go.opentelemetry.io/otel/sdk v1.34.0 // indirect
	go.opentelemetry.io/otel/trace v1.34.0 // indirect
	golang.org/x/crypto v0.31.0 // indirect
	golang.org/x/exp v0.0.0-20230905200255-921286631fa9 // indirect
	golang.org/x/oauth2 v0.18.0 // indirect
	golang.org/x/sync v0.10.0 // indirect
	golang.org/x/sys v0.29.0 // indirect
	golang.org/x/term v0.27.0 // indirect
	golang.org/x/text v0.21.0 // indirect
	google.golang.org/appengine v1.6.8 // indirect
	google.golang.org/protobuf v1.36.3 // indirect
	gotest.tools/v3 v3.5.2 // indirect
)
