package cmd

import (
	"fmt"
	"github.com/docker/docker/client"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/tiulpin/qodana-cli/pkg"
	"os"
	"path/filepath"
	"strings"
)

// NewScanCommand returns a new instance of the scan command
func NewScanCommand() *cobra.Command { // TODO: handle Ctrl+C: container removing, etc.
	options := &pkg.QodanaOptions{}
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan project with Qodana",
		Long: `Scan a project with Qodana. It runs one of Qodana Docker's images (https://www.jetbrains.com/help/qodana/docker-images.html) and reports the results.

Note that most options can be configured via qodana.yaml (https://www.jetbrains.com/help/qodana/qodana-yaml.html) file.
But you can always override qodana.yaml options with the following command-line options.
`,
		PreRun: func(cmd *cobra.Command, args []string) {
			pkg.EnsureDockerRunning()
		},
		Run: func(cmd *cobra.Command, args []string) {
			var exitCode int64
			ctx := cmd.Context()
			if options.Linter == "" {
				qodanaYaml := pkg.GetQodanaYaml(options.ProjectDir)
				if qodanaYaml.Linters == nil {
					pkg.Warning.Println(
						"No valid qodana.yaml found. Have you run 'qodana init'? Running that for you...",
					)
					pkg.PrintProcess(func() { pkg.ConfigureProject(options.ProjectDir) }, "Configuring project", "project configuration")
					qodanaYaml = pkg.GetQodanaYaml(options.ProjectDir)
				}
				options.Linter = qodanaYaml.Linters[0]
			} else {
				if !strings.HasPrefix(options.Linter, pkg.OfficialDockerPrefix) {
					pkg.Warning.Printfln(
						"You are using an unofficial Qodana linter: %s", options.Linter,
					)
				}
				if options.Linter == "jetbrains/qodana-license-audit" {
					log.Fatal("jetbrains/qodana-license-audit is not supported")
				}
				if options.Linter == "jetbrains/qodana-clone-finder" {
					log.Fatal("jetbrains/qodana-clone-finder is not supported")
				}
			}
			if err := pkg.Greet(); err != nil {
				log.Fatal("couldn't print", err)
			}
			pkg.PrepareFolders(options)
			docker, err := client.NewClientWithOpts()
			if err != nil {
				log.Fatal("couldn't instantiate docker client", err)
			}
			pkg.PrintProcess(
				func() { pkg.PullImage(ctx, docker, options.Linter) },
				"Preparing images",
				"preparing images",
			)
			pkg.PrintProcess(
				func() {
					exitCode = pkg.RunLinter(cmd.Context(), docker, options)
				},
				fmt.Sprintf("Analyzing project with %s", options.Linter),
				"project analysis",
			)
			if exitCode != pkg.QodanaSuccessExitCode && exitCode != pkg.QodanaFailThresholdExitCode {
				log.Fatal("Linter failed, please check the logs in ", options.ResultsDir)
			}
			pkg.PrintSarif(options.ResultsDir)
			if options.ShowReport {
				reportPath := filepath.Join(options.ResultsDir, "report")
				message := fmt.Sprintf("Showing Qodana report at http://localhost:%d", options.Port)
				pkg.PrintProcess(func() { pkg.ShowReport(reportPath, options.Port) }, message, "report show")
			}
			if exitCode == pkg.QodanaFailThresholdExitCode {
				pkg.Error.Println("The number of problems exceeds the failThreshold")
				os.Exit(int(exitCode))
			}
		},
	}

	flags := cmd.Flags()
	// flags that define CLI behaviour
	flags.StringVarP(&options.Linter, "linter", "l", "", "Override linter to use")
	flags.StringVarP(&options.ProjectDir, "project-dir", "i", ".", "Root directory of the inspected project")
	flags.StringVarP(&options.ResultsDir, "results-dir", "o", "", "Override directory to save Qodana inspection results to (default .qodana/<linter>/results)")
	flags.StringVarP(&options.CacheDir, "cache-dir", "c", "", "Override cache directory (default .qodana/<linter>/cache)")
	flags.BoolVarP(&options.ShowReport, "show-report", "w", false, "Serve HTML report on port")
	flags.IntVar(&options.Port, "port", 8080, "Port to serve the report on")

	// flags that define Docker behaviour
	// no flags for volumes or any other thing because it seems that they are not needed (proper plugins downloading is on the way)
	flags.StringArrayVarP(&options.EnvVariables, "env", "e", []string{}, "Define additional environment variables for the Qodana container (you can use the flag multiple times). CLI is not reading full host environment variables and does not pass it to the Qodana container for security reasons")

	// flags that define Qodana behaviour
	flags.BoolVarP(&options.SaveReport, "save-report", "s", true, "Generate HTML report")
	flags.StringVarP(&options.SourceDirectory, "source-directory", "d", "", "Directory inside the project-dir directory must be inspected. If not specified, the whole project is inspected.")
	flags.BoolVar(&options.DisableSanity, "disable-sanity", false, "Skip running the inspections configured by the sanity profile")
	flags.StringVarP(&options.ProfileName, "profile-name", "n", "", "Profile name defined in the project")
	flags.StringVarP(&options.ProfilePath, "profile-path", "p", "", "Path to the profile file")
	flags.BoolVar(&options.RunPromo, "run-promo", false, "Set to true to have the application run the inspections configured by the promo profile; set to false otherwise. By default, a promo run is enabled if the application is executed with the default profile and is disabled otherwise")
	flags.StringVar(&options.StubProfile, "stub-profile", "", "Absolute path to the fallback profile file. This option is applied in case the profile was not specified using any available options")
	flags.StringVarP(&options.Baseline, "baseline", "b", "", "Provide the path to an existing SARIF report to be used in the baseline state calculation")
	flags.BoolVar(&options.BaselineIncludeAbsent, "baseline-include-absent", false, "Include in the output report the results from the baseline run that are absent in the current run")
	flags.StringVar(&options.Property, "property", "", "Set a JVM property to be used while running Qodana using the --property=property.name=value1,value2,...,valueN notation")
	flags.StringVar(&options.Script, "script", "default", "Override the run scenario")
	flags.StringVar(&options.FailThreshold, "fail-threshold", "", "Set the number of problems that will serve as a quality gate. If this number is reached, the inspection run is terminated with a non-zero exit code")
	flags.BoolVar(&options.Changes, "changes", false, "Override the docker image to be used for the analysis")
	flags.BoolVar(&options.SendReport, "send-report", false, "Send the inspection report to Qodana Cloud, requires the '--token' option to be specified")
	flags.StringVarP(&options.Token, "token", "t", "", "Qodana Cloud token")
	flags.StringVarP(&options.AnalysisId, "analysis-id", "a", "", "Unique report identifier (GUID) to be used by Qodana Cloud")
	return cmd
}
