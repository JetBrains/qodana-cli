package main

import (
	_ "embed"
	"fmt"

	"github.com/JetBrains/qodana-cli/internal/platform/thirdpartyscan"
)

type TestLinter struct {
}

func (l TestLinter) RunAnalysis(c thirdpartyscan.Context) error {
	fmt.Printf("Running Test Linter %s %s analysis...\n", c.LinterInfo().LinterName, c.LinterInfo().LinterVersion)
	fmt.Println("Qodana tip of the day:")
	fmt.Printf("NO PROJECT - NO PROBLEMS DETECTED.\n\n")
	return nil
}

func (l TestLinter) MountTools(path string) (map[string]string, error) {
	fmt.Printf("Skipping tools mounting from %s as Test Linter doesn't need them\n", path)
	return nil, nil
}
