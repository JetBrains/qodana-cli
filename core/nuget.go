package core

import (
	"bufio"
	"fmt"
	"github.com/JetBrains/qodana-cli/v2023/cloud"
	"github.com/JetBrains/qodana-cli/v2023/platform"
	log "github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	nugetConfigName           = "nuget.config"
	nugetConfigNamePascalCase = "NuGet.Config"
)

func unsetNugetVariables() {
	variables := []string{qodanaNugetUser, qodanaNugetPassword, qodanaNugetName, qodanaNugetUrl}
	for _, variable := range variables {
		if err := os.Unsetenv(variable); err != nil {
			log.Fatal("couldn't unset env variable ", err.Error())
		}
	}
}

func warnIfPrivateFeedDetected(projectPath string) {
	if Prod.Code != QDNET && Prod.Code != QDNETC || qodanaNugetVarsSet() {
		return
	}
	configFileNames := []string{nugetConfigName, nugetConfigNamePascalCase}
	for _, fileName := range configFileNames {
		if _, err := os.Stat(filepath.Join(projectPath, fileName)); err == nil {
			if checkForPrivateFeed(filepath.Join(projectPath, fileName)) {
				_, _ = fmt.Fprintf(os.Stderr, "\nWarning: private NuGet feed detected. Please set %s, %s, %s and %s (optional) environment variables to provide credentials for the private feed.\n",
					qodanaNugetUser, qodanaNugetPassword, qodanaNugetUrl, qodanaNugetName)
			}
		}
	}
}

func checkForPrivateFeed(fileName string) bool {
	file, err := os.Open(fileName)
	if err != nil {
		fmt.Println("Failed to open file:", fileName)
		return false
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			log.Error("couldn't close file ", err.Error())
		}
	}(file)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "<add ") {
			match, _ := regexp.MatchString(`http(s)?://`, line)
			return match
		}
	}
	return false
}

func isNugetConfigNeeded() bool {
	return platform.IsContainer() && qodanaNugetVarsSet()
}

func qodanaNugetVarsSet() bool {
	return os.Getenv(qodanaNugetUrl) != "" && os.Getenv(qodanaNugetUser) != "" && os.Getenv(qodanaNugetPassword) != ""
}

func prepareNugetConfig(userPath string) {
	nugetConfig := filepath.Join(userPath, ".nuget", "NuGet")
	if _, err := os.Stat(nugetConfig); err != nil {
		// mkdir -p ~/.nuget/NuGet
		if err := os.MkdirAll(nugetConfig, os.ModePerm); err != nil {
			log.Fatal("couldn't create a directory ", err.Error())
		}
	}
	nugetConfig = filepath.Join(nugetConfig, "NuGet.Config")
	config := nugetWithPrivateFeed(cloud.GetEnvWithDefault(qodanaNugetName, "qodana"), os.Getenv(qodanaNugetUrl), os.Getenv(qodanaNugetUser), os.Getenv(qodanaNugetPassword))
	if err := os.WriteFile(nugetConfig, []byte(config), 0644); err != nil {
		log.Fatal("couldn't create a file ", err.Error())
	}
}
