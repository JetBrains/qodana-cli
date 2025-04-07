package main

import (
	"github.com/JetBrains/qodana-cli/v2025/platform"
	"github.com/JetBrains/qodana-cli/v2025/platform/thirdpartyscan"
)

func mergeSarifReports(c thirdpartyscan.Context) (int, error) {
	totalProblems, err := platform.MergeSarifReports(c, platform.GetDeviceIdSalt()[0])
	if err != nil {
		return 0, err
	}

	return totalProblems, nil
}
