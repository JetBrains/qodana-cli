/*
 * Copyright 2021-2024 JetBrains s.r.o.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package commoncontext

import (
	"fmt"
	"github.com/JetBrains/qodana-cli/v2025/cloud"
	"github.com/JetBrains/qodana-cli/v2025/platform/msg"
	"github.com/JetBrains/qodana-cli/v2025/platform/product"
	"github.com/JetBrains/qodana-cli/v2025/platform/qdenv"
	"github.com/JetBrains/qodana-cli/v2025/platform/qdyaml"
	"github.com/JetBrains/qodana-cli/v2025/platform/utils"
	"github.com/pterm/pterm"
	log "github.com/sirupsen/logrus"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

func GuessAnalyzerFromEnvAndCLI(ide string, linter string) product.Analyzer {
	dist, exists := os.LookupEnv(qdenv.QodanaDistEnv)
	if exists && dist != "" {
		analyzer, err := BuildPathNativeAnalyzer(dist)
		if err != nil {
			log.Fatalf("Env %s doesn't point to valid distribution: %s", qdenv.QodanaDistEnv, err)
		}
		return analyzer
	}

	return GuessAnalyzerFromParams(ide, linter)
}

func GuessAnalyzerFromParams(ide string, image string) product.Analyzer {
	if ide != "" {
		linter := product.FindLinterByProductCode(ide)
		if linter == product.UnknownLinter {
			//legacy
			log.Warnf(
				"--ide value %s is not recognised as product code, trying to interpret as path to distribution\n",
				ide,
			)
			analyzer, err := BuildPathNativeAnalyzer(ide)
			if err != nil {
				log.Fatalf("Flag --ide doesn't point to valid distribution: %s", err)
			}
			return analyzer
		}

		return &product.NativeAnalyzer{
			Linter: linter,
			Eap:    strings.Contains(ide, product.EapSuffix),
		}
	}

	if image != "" {
		return &product.DockerAnalyzer{
			Linter: product.FindLinterByImage(image),
			Image:  image,
		}
	}

	return nil
}

func BuildPathNativeAnalyzer(dist string) (product.Analyzer, error) {
	productInfo, err := product.ReadIdeProductInfo(dist)
	if err != nil {
		return nil, fmt.Errorf("can't read product-info.json: %v ", err)
	}

	info, err := product.FindLinterPropertiesByProductInfo(productInfo.ProductCode)
	if err != nil {
		return nil, fmt.Errorf("product dist %s is not recognised as valid: %v", dist, err)
	}
	return &product.PathNativeAnalyzer{
		Linter: info.Linter,
		Path:   dist,
		IsEap:  product.IsEap(productInfo),
	}, nil
}

// SelectAnalyzerForPath gets linter for the given path
func SelectAnalyzerForPath(path string, token string) product.Analyzer {
	var linters []product.Linter
	msg.PrintProcess(
		func(_ *pterm.SpinnerPrinter) {
			languages := readIdeaDir(path)
			if len(languages) == 0 {
				languages, _ = recognizeDirLanguages(path)
			}
			if len(languages) == 0 {
				msg.WarningMessage("No technologies detected (no source code files?)\n")
			} else {
				msg.WarningMessage("Detected technologies: " + strings.Join(languages, ", ") + "\n")
				for _, language := range languages {
					if i, err := product.LangsToLinters[language]; err {
						for _, l := range i {
							linters = append(linters, l)
						}
					}
				}
				if len(linters) == 0 {
					linters = product.AllLinters
				}
			}
			// breaking change will not be backported to 241
			if (slices.Contains(linters, product.AndroidCommunityLinter) ||
				slices.Contains(linters, product.AndroidLinter)) &&
				isAndroidProject(path) {

				filteredLinters := make([]product.Linter, 0, len(linters))
				for _, l := range linters {
					if l != product.AndroidLinter && l != product.AndroidCommunityLinter {
						filteredLinters = append(filteredLinters, l)
					}
				}
				linters = append(
					[]product.Linter{product.AndroidLinter, product.AndroidCommunityLinter},
					filteredLinters...,
				)
			}
		}, "Scanning project", "",
	)

	selector := func(choices []string) string {
		choice, err := msg.QodanaInteractiveSelect.WithOptions(choices).Show()
		if err != nil {
			msg.ErrorMessage("%s", err)
			return ""
		}
		return choice
	}

	interactive := msg.IsInteractive()
	linters = filterByLicensePlan(linters, token)
	analyzer := selectAnalyzer(path, linters, interactive, selector)
	if analyzer == nil {
		msg.ErrorMessage("Could not configure project as it is not supported by Qodana")
		msg.WarningMessage("See https://www.jetbrains.com/help/qodana/supported-technologies.html for more details")
		os.Exit(1)
	}
	msg.SuccessMessage("Selected '%s'", analyzer.GetLinter().PresentableName)
	return analyzer
}

func filterByLicensePlan(linters []product.Linter, token string) []product.Linter {
	if token == "" {
		return linters
	}
	cloud.SetupLicenseToken(token)
	if licensePlan := cloud.GetCloudApiEndpoints().GetLicensePlan(token); licensePlan == cloud.CommunityLicensePlan {
		var filteredCodes []product.Linter
		for _, linter := range linters {
			if !linter.IsPaid {
				filteredCodes = append(filteredCodes, linter)
			}
		}
		return filteredCodes
	}
	return linters
}

// GetAndSaveDotNetConfig gets .NET config for the given path and saves configName
func GetAndSaveDotNetConfig(projectDir string, qodanaYamlFullPath string) bool {
	possibleOptions := utils.FindFiles(projectDir, []string{".sln", ".csproj", ".vbproj", ".fsproj"})
	if len(possibleOptions) <= 1 {
		return false
	}
	msg.WarningMessage("Detected multiple .NET solution/project files, select the preferred one \n")
	choice, err := msg.QodanaInteractiveSelect.WithOptions(possibleOptions).WithDefaultText("Select solution/project").Show()
	if err != nil {
		msg.ErrorMessage("%s", err)
		return false
	}
	dotnet := &qdyaml.DotNet{}
	if strings.HasSuffix(choice, ".sln") {
		dotnet.Solution = filepath.Base(choice)
	} else {
		dotnet.Project = filepath.Base(choice)
	}
	return qdyaml.SetQodanaDotNet(qodanaYamlFullPath, dotnet)
}

func selectAnalyzer(
	path string,
	linters []product.Linter,
	interactive bool,
	selectFunc func([]string) string,
) product.Analyzer {
	var distribution product.Analyzer
	if len(linters) == 0 && !interactive {
		return nil
	}

	selection, choices := analyzerToSelect(linters, path)
	log.Debugf("Detected products: %s", strings.Join(choices, ", "))

	if len(choices) == 1 || !interactive {
		distribution = selection[choices[0]]
	} else {
		choice := selectFunc(choices)
		if choice == "" {
			return nil
		}
		distribution = selection[choice]
	}
	return distribution
}

func analyzerToSelect(linters []product.Linter, path string) (map[string]product.Analyzer, []string) {
	analyzersMap := make(map[string]product.Analyzer)
	analyzersList := make([]string, 0, len(linters))
	for _, linter := range linters {
		if linter.SupportNative {
			if isNativeRequired(path, linter) {
				key := linter.PresentableName + " (Native)"
				analyzersMap[key] = &product.NativeAnalyzer{
					Linter: linter,
					Eap:    linter.EapOnly,
				}
				analyzersList = append(analyzersList, key)
			}
		}
		dockerKey := linter.PresentableName + " (Docker)"
		analyzersMap[dockerKey] = &product.DockerAnalyzer{
			Linter: linter,
			Image:  linter.Image(),
		}
		analyzersList = append(analyzersList, dockerKey)
	}
	return analyzersMap, analyzersList
}

// ShowReport serves the Qodana report
func ShowReport(resultsDir string, reportPath string, port int) {
	cloudUrl := cloud.GetReportUrl(resultsDir)
	if cloudUrl != "" {
		openReport(cloudUrl, reportPath, port)
	} else {
		msg.WarningMessage("Press Ctrl+C to stop serving the report\n")
		msg.PrintProcess(
			func(_ *pterm.SpinnerPrinter) {
				if _, err := os.Stat(reportPath); os.IsNotExist(err) {
					log.Fatal("Qodana report not found. Get a report by running `qodana scan`")
				}
				openReport("", reportPath, port)
			},
			fmt.Sprintf("Showing Qodana report from %s", fmt.Sprintf("http://localhost:%d/", port)),
			"",
		)
	}
}

// openReport serves the report on the given port and opens the browser.
func openReport(cloudUrl string, path string, port int) {
	if cloudUrl != "" {
		resp, err := http.Get(cloudUrl)
		if err == nil && resp.StatusCode == 200 {
			err = utils.OpenBrowser(cloudUrl)
			if err != nil {
				return
			}
		}
		return
	} else {
		url := fmt.Sprintf("http://localhost:%d", port)
		go func() {
			resp, err := http.Get(url)
			if err == nil && resp.StatusCode == 200 {
				err := utils.OpenBrowser(url)
				if err != nil {
					return
				}
			}
		}()
		http.Handle("/", noCache(http.FileServer(http.Dir(path))))
		err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
		if err != nil {
			msg.WarningMessage("Problem serving report, %s\n", err.Error())
			return
		}
	}
	_, _ = fmt.Scan()
}

// noCache handles serving the static files with no cache headers.
func noCache(h http.Handler) http.Handler {
	etagHeaders := []string{
		"ETag",
		"If-Modified-Since",
		"If-Match",
		"If-None-Match",
		"If-Range",
		"If-Unmodified-Since",
	}
	epoch := time.Unix(0, 0).Format(time.RFC1123)
	noCacheHeaders := map[string]string{
		"Expires":         epoch,
		"Cache-Control":   "no-cache, private, max-age=0",
		"Pragma":          "no-cache",
		"X-Accel-Expires": "0",
	}
	fn := func(w http.ResponseWriter, r *http.Request) {
		for _, x := range etagHeaders {
			if r.Header.Get(x) != "" {
				r.Header.Del(x)
			}
		}
		for k, v := range noCacheHeaders {
			w.Header().Set(k, v)
		}
		h.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}

var InterruptChannel chan os.Signal
