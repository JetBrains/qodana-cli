package platform

import (
	"fmt"
)

// computeBaselinePrintResults runs SARIF analysis (compares with baseline and prints the result)=
func computeBaselinePrintResults(options *QodanaOptions, mountInfo *MountInfo) (int, error) {
	args := []string{QuoteForWindows(mountInfo.JavaPath), "-jar", QuoteForWindows(mountInfo.BaselineCli), "-r", QuoteForWindows(options.GetSarifPath())}
	if options.FailThreshold != "" {
		args = append(args, "-f", options.FailThreshold)
	}
	if options.Baseline != "" {
		args = append(args, "-b", QuoteForWindows(options.Baseline))
	}
	ret, err := LaunchAndLog(options, "baseline", args...)
	if err != nil {
		return -1, fmt.Errorf("error while running baseline-cli: %w", err)
	}
	if ret > 0 {
		if ret == 1 {
			return -1, fmt.Errorf("error in supplied arguments for baseline-cli")
		}
		return ret, nil
	}
	return 0, err
}
