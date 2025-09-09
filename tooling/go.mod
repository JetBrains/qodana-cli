module github.com/JetBrains/qodana-cli/v2025/tooling

go 1.24.0

replace (
	github.com/JetBrains/qodana-cli/v2025/cloud => ../cloud
	github.com/JetBrains/qodana-cli/v2025/cmd => ../cmd
	github.com/JetBrains/qodana-cli/v2025/core => ../core
	github.com/JetBrains/qodana-cli/v2025/platform => ../platform
	github.com/JetBrains/qodana-cli/v2025/sarif => ../sarif
)

require github.com/JetBrains/qodana-cli/v2025/platform v0.0.0-00010101000000-000000000000

require (
	atomicgo.dev/cursor v0.2.0 // indirect
	atomicgo.dev/keyboard v0.2.9 // indirect
	atomicgo.dev/schedule v0.1.0 // indirect
	github.com/containerd/console v1.0.3 // indirect
	github.com/cucumber/ci-environment/go v0.0.0-20230911180507-bd001ebc644c // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/gookit/color v1.5.4 // indirect
	github.com/liamg/clinch v1.6.6 // indirect
	github.com/liamg/tml v0.3.0 // indirect
	github.com/lithammer/fuzzysearch v1.1.8 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/pterm/pterm v0.12.80 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/shirou/gopsutil/v3 v3.24.5 // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	golang.org/x/crypto v0.41.0 // indirect
	golang.org/x/exp v0.0.0-20230905200255-921286631fa9 // indirect
	golang.org/x/sys v0.35.0 // indirect
	golang.org/x/term v0.34.0 // indirect
	golang.org/x/text v0.28.0 // indirect
)
