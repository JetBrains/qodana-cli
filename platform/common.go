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

package platform

import (
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/cloud"
	"github.com/pterm/pterm"
	log "github.com/sirupsen/logrus"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

//goland:noinspection GoUnnecessarilyExportedIdentifiers
var (
	QDJVMC         = "QDJVMC"
	QDJVM          = "QDJVM"
	QDAND          = "QDAND"
	QDPHP          = "QDPHP"
	QDPY           = "QDPY"
	QDPYC          = "QDPYC"
	QDJS           = "QDJS"
	QDGO           = "QDGO"
	QDNET          = "QDNET"
	QDNETC         = "QDNETC"
	QDANDC         = "QDANDC"
	QDRST          = "QDRST"
	QDRUBY         = "QDRUBY"
	QDCL           = "QDCL"
	QDCPP          = "QDCPP"
	DockerImageMap = map[string]string{
		QDAND:  "jetbrains/qodana-android:",
		QDANDC: "jetbrains/qodana-jvm-android:",
		QDPHP:  "jetbrains/qodana-php:",
		QDJS:   "jetbrains/qodana-js:",
		QDNET:  "jetbrains/qodana-dotnet:",
		QDNETC: "jetbrains/qodana-cdnet:",
		QDPY:   "jetbrains/qodana-python:",
		QDPYC:  "jetbrains/qodana-python-community:",
		QDGO:   "jetbrains/qodana-go:",
		QDJVM:  "jetbrains/qodana-jvm:",
		QDJVMC: "jetbrains/qodana-jvm-community:",
		QDCL:   "jetbrains/qodana-clang:",
		//QDRST:  "jetbrains/qodana-rust:",
	}
)

// QodanaLogo prepares the info message for the tool
func QodanaLogo(toolDesc string, version string, eap bool) string {
	eapString := ""
	if eap {
		eapString = "EAP"
	}
	return fmt.Sprintf(`
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

`, toolDesc, version, eapString)
}

// GetAnalyzer gets linter for the given path
func GetAnalyzer(path string, token string) string {
	var analyzers []string
	PrintProcess(func(_ *pterm.SpinnerPrinter) {
		languages := readIdeaDir(path)
		if len(languages) == 0 {
			languages, _ = recognizeDirLanguages(path)
		}
		if len(languages) == 0 {
			WarningMessage("No technologies detected (no source code files?)\n")
		} else {
			WarningMessage("Detected technologies: " + strings.Join(languages, ", ") + "\n")
			for _, language := range languages {
				if i, err := langsProductCodes[language]; err {
					for _, l := range i {
						analyzers = Append(analyzers, l)
					}
				}
			}
			if len(analyzers) == 0 {
				analyzers = AllCodes
			}
		}
		// breaking change will not be backported to 241
		if (Contains(analyzers, QDAND) || Contains(analyzers, QDANDC)) && isAndroidProject(path) {
			analyzers = Remove(analyzers, QDAND)
			analyzers = Remove(analyzers, QDANDC)
			analyzers = append([]string{QDAND, QDANDC}, analyzers...)
		}
	}, "Scanning project", "")

	selector := func(choices []string) string {
		choice, err := qodanaInteractiveSelect.WithOptions(choices).Show()
		if err != nil {
			ErrorMessage("%s", err)
			return ""
		}
		return choice
	}

	interactive := IsInteractive()
	analyzers = filterByLicensePlan(analyzers, token)
	analyzer := selectAnalyzer(path, analyzers, interactive, selector)
	if analyzer == "" {
		ErrorMessage("Could not configure project as it is not supported by Qodana")
		WarningMessage("See https://www.jetbrains.com/help/qodana/supported-technologies.html for more details")
		os.Exit(1)
	}
	SuccessMessage("Selected %s", analyzer)
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
			if Contains(AllSupportedFreeCodes, code) {
				filteredCodes = append(filteredCodes, code)
			}
		}
		return filteredCodes
	}
	return codes
}

// GetAndSaveDotNetConfig gets .NET config for the given path and saves configName
func GetAndSaveDotNetConfig(projectDir string, yamlName string) bool {
	possibleOptions := FindFiles(projectDir, []string{".sln", ".csproj", ".vbproj", ".fsproj"})
	if len(possibleOptions) <= 1 {
		return false
	}
	WarningMessage("Detected multiple .NET solution/project files, select the preferred one \n")
	choice, err := qodanaInteractiveSelect.WithOptions(possibleOptions).WithDefaultText("Select solution/project").Show()
	if err != nil {
		ErrorMessage("%s", err)
		return false
	}
	dotnet := &DotNet{}
	if strings.HasSuffix(choice, ".sln") {
		dotnet.Solution = filepath.Base(choice)
	} else {
		dotnet.Project = filepath.Base(choice)
	}
	return setQodanaDotNet(projectDir, dotnet, yamlName)
}

// AllNativeCodes is a list of all supported Qodana linters product codes
var AllNativeCodes = []string{QDNET, QDJVM, QDJVMC, QDGO, QDPY, QDPYC, QDJS, QDPHP}

func Image(code string) string {
	if val, ok := DockerImageMap[code]; ok {
		if //goland:noinspection GoBoolExpressions
		!isReleased {
			return val + ReleaseVersion + "-eap"
		}
		if code == QDNETC || code == QDCL {
			return val + ReleaseVersion + "-eap"
		}
		return val + ReleaseVersion
	} else {
		log.Fatal("Unknown code: " + code)
		return ""
	}
}

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

func IsNativeAnalyzer(analyzer string) bool {
	return Contains(AllNativeCodes, analyzer)
}

func analyzerToSelect(analyzers []string, path string) (map[string]string, []string) {
	analyzersMap := make(map[string]string)
	analyzersList := make([]string, 0, len(analyzers))
	for _, a := range analyzers {
		if IsNativeAnalyzer(a) {
			if IsNativeRequired(path, a) {
				analyzersMap[a+" (Native)"] = a
				analyzersList = append(analyzersList, a+" (Native)")
			}
		}
		analyzersMap[Image(a)+" (Docker)"] = Image(a)
		analyzersList = append(analyzersList, Image(a)+" (Docker)")
	}
	return analyzersMap, analyzersList
}

// ShowReport serves the Qodana report
func ShowReport(resultsDir string, reportPath string, port int) {
	cloudUrl := cloud.GetReportUrl(resultsDir)
	if cloudUrl != "" {
		openReport(cloudUrl, reportPath, port)
	} else {
		WarningMessage("Press Ctrl+C to stop serving the report\n")
		PrintProcess(
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
			err = OpenBrowser(cloudUrl)
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
				err := OpenBrowser(url)
				if err != nil {
					return
				}
			}
		}()
		http.Handle("/", noCache(http.FileServer(http.Dir(path))))
		err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
		if err != nil {
			WarningMessage("Problem serving report, %s\n", err.Error())
			return
		}
	}
	_, _ = fmt.Scan()
}

// OpenBrowser opens the default browser to the given url
func OpenBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "open"
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
	}
	args = append(args, url)
	return exec.Command(cmd, args...).Start()
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

// Version returns the version of the Qodana CLI, set during the GoReleaser build
var Version = "dev"
var InterruptChannel chan os.Signal
