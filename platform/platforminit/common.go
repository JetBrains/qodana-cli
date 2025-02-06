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

package platforminit

import (
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/cloud"
	"github.com/JetBrains/qodana-cli/v2024/platform/msg"
	"github.com/JetBrains/qodana-cli/v2024/platform/product"
	"github.com/JetBrains/qodana-cli/v2024/platform/qdyaml"
	"github.com/JetBrains/qodana-cli/v2024/platform/utils"
	"github.com/pterm/pterm"
	log "github.com/sirupsen/logrus"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// QodanaLogo prepares the info message for the tool
func QodanaLogo(toolDesc string, version string, eap bool) string {
	eapString := ""
	if eap {
		eapString = "EAP"
	}
	return fmt.Sprintf(
		`
          _              _
         /\ \           /\ \        %s %s %s
        /  \ \         /  \ \       Documentation
       / /\ \ \       / /\ \ \      https://jb.gg/qodana-docs
      / / /\ \ \     / / /\ \ \     Contact us at
     / / /  \ \_\   / / /  \ \_\    qodana-support@jetbrains.com
    / / / _ / / /  / / /   / / /    Or via our issue tracker
   / / / /\ \/ /  / / /   / / /     https://jb.gg/qodana-issue
  / / /__\ \ \/  / / /___/ / /      Or share your feedback at our forum
 / / /____\ \ \ / / /____\/ /       https://jb.gg/qodana-forum
 \/________\_\/ \/_________/

`, toolDesc, version, eapString,
	)
}

// GetAnalyzer gets linter for the given path
func GetAnalyzer(path string, token string) string {
	var analyzers []string
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
					if i, err := product.LangsProductCodes[language]; err {
						for _, l := range i {
							analyzers = utils.Append(analyzers, l)
						}
					}
				}
				if len(analyzers) == 0 {
					analyzers = product.AllCodes
				}
			}
			// breaking change will not be backported to 241
			if (utils.Contains(analyzers, product.QDAND) || utils.Contains(
				analyzers,
				product.QDANDC,
			)) && isAndroidProject(path) {
				analyzers = utils.Remove(analyzers, product.QDAND)
				analyzers = utils.Remove(analyzers, product.QDANDC)
				analyzers = append([]string{product.QDAND, product.QDANDC}, analyzers...)
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
	analyzers = filterByLicensePlan(analyzers, token)
	analyzer := selectAnalyzer(path, analyzers, interactive, selector)
	if analyzer == "" {
		msg.ErrorMessage("Could not configure project as it is not supported by Qodana")
		msg.WarningMessage("See https://www.jetbrains.com/help/qodana/supported-technologies.html for more details")
		os.Exit(1)
	}
	msg.SuccessMessage("Selected %s", analyzer)
	return analyzer
}

// filterCommunityCodes filters out codes that are available with a community license
func filterByLicensePlan(codes []string, token string) []string {
	if token == "" {
		return codes
	}
	cloud.SetupLicenseToken(token)
	if licensePlan := cloud.GetCloudApiEndpoints().GetLicensePlan(token); licensePlan == cloud.CommunityLicensePlan {
		var filteredCodes []string
		for _, code := range codes {
			if utils.Contains(product.AllSupportedFreeCodes, code) {
				filteredCodes = append(filteredCodes, code)
			}
		}
		return filteredCodes
	}
	return codes
}

// GetAndSaveDotNetConfig gets .NET config for the given path and saves configName
func GetAndSaveDotNetConfig(projectDir string, yamlName string) bool {
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
	return qdyaml.SetQodanaDotNet(projectDir, dotnet, yamlName)
}

// AllNativeCodes is a list of all supported Qodana linters product codes

func selectAnalyzer(path string, analyzers []string, interactive bool, selectFunc func([]string) string) string {
	var analyzer string
	if len(analyzers) == 0 && !interactive {
		return ""
	}

	selection, choices := analyzerToSelect(analyzers, path)
	log.Debugf("Detected products: %s", strings.Join(choices, ", "))

	if len(choices) == 1 || !interactive {
		analyzer = selection[choices[0]]
	} else {
		choice := selectFunc(choices)
		if choice == "" {
			return ""
		}
		analyzer = selection[choice]
	}
	return analyzer
}

func analyzerToSelect(analyzers []string, path string) (map[string]string, []string) {
	analyzersMap := make(map[string]string)
	analyzersList := make([]string, 0, len(analyzers))
	for _, a := range analyzers {
		if product.IsNativeAnalyzer(a) {
			if IsNativeRequired(path, a) {
				analyzersMap[a+" (Native)"] = a
				analyzersList = append(analyzersList, a+" (Native)")
			}
		}
		analyzersMap[product.Image(a)+" (Docker)"] = product.Image(a)
		analyzersList = append(analyzersList, product.Image(a)+" (Docker)")
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
