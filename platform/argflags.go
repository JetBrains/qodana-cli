package platform

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"os"
	"strings"
)

func ComputeFlags(cmd *cobra.Command, options *QodanaOptions) error {
	flags := cmd.Flags()
	flags.SortFlags = false

	if !IsContainer() {
		flags.StringVarP(&options.Linter, "linter", "l", "", "Use to run Qodana in a container (default). Choose linter (image) to use. Not compatible with --ide option. Available images are: "+strings.Join(AllImages, ", "))
	}
	flags.StringVar(&options.Ide, "ide", os.Getenv(QodanaDistEnv), fmt.Sprintf("Use to run Qodana without a container. Not compatible with --linter option. Available codes are %s, add -EAP part to obtain EAP versions", strings.Join(AllNativeCodes, ", ")))

	flags.StringVarP(&options.ProjectDir, "project-dir", "i", ".", "Root directory of the inspected project")
	flags.StringVarP(&options.ResultsDir, "results-dir", "o", "", "Override directory to save Qodana inspection results to (default <userCacheDir>/JetBrains/<linter>/results)")
	flags.StringVar(&options.CacheDir, "cache-dir", "", "Override cache directory (default <userCacheDir>/JetBrains/<linter>/cache)")
	flags.StringVarP(&options.ReportDir, "report-dir", "r", "", "Override directory to save Qodana HTML report to (default <userCacheDir>/JetBrains/<linter>/results/report)")

	flags.BoolVar(&options.PrintProblems, "print-problems", false, "Print all found problems by Qodana in the CLI output")
	flags.BoolVar(&options.ClearCache, "clear-cache", false, "Clear the local Qodana cache before running the analysis")
	flags.BoolVarP(&options.ShowReport, "show-report", "w", false, "Serve HTML report on port")
	flags.IntVar(&options.Port, "port", 8080, "Port to serve the report on")
	flags.StringVar(&options.YamlName, "yaml-name", FindQodanaYaml(options.ProjectDir), "Override qodana.yaml name to use: 'qodana.yaml' or 'qodana.yml'")

	flags.StringVarP(&options.AnalysisId, "analysis-id", "a", uuid.New().String(), "Unique report identifier (GUID) to be used by Qodana Cloud")
	flags.StringVarP(&options.Baseline, "baseline", "b", "", "Provide the path to an existing SARIF report to be used in the baseline state calculation")
	flags.BoolVar(&options.BaselineIncludeAbsent, "baseline-include-absent", false, "Include in the output report the results from the baseline run that are absent in the current run")
	flags.BoolVar(&options.FullHistory, "full-history", false, "Go through the full commit history and run the analysis on each commit. If combined with `--commit`, analysis will be started from the given commit. Could take a long time.")
	flags.StringVar(&options.Commit, "commit", "", "Base changes commit to reset to, resets git and runs linter with `--script local-changes`: analysis will be run only on changed files since the given commit. If combined with `--full-history`, full history analysis will be started from the given commit.")
	flags.StringVar(&options.FailThreshold, "fail-threshold", "", "Set the number of problems that will serve as a quality gate. If this number is reached, the inspection run is terminated with a non-zero exit code")
	flags.BoolVar(&options.DisableSanity, "disable-sanity", false, "Skip running the inspections configured by the sanity profile")
	flags.StringVarP(&options.SourceDirectory, "source-directory", "d", "", "Directory inside the project-dir directory must be inspected. If not specified, the whole project is inspected")
	flags.StringVarP(&options.ProfileName, "profile-name", "n", "", "Profile name defined in the project")
	flags.StringVarP(&options.ProfilePath, "profile-path", "p", "", "Path to the profile file")
	flags.StringVar(&options.RunPromo, "run-promo", "", "Set to 'true' to have the application run the inspections configured by the promo profile; set to 'false' otherwise (default: 'true' only if Qodana is executed with the default profile)")
	flags.StringVar(&options.Script, "script", "default", "Override the run scenario")
	flags.StringVar(&options.StubProfile, "stub-profile", "", "Absolute path to the fallback profile file. This option is applied in case the profile was not specified using any available options")
	flags.StringVar(&options.CoverageDir, "coverage-dir", "", "Directory with coverage data to process")

	flags.BoolVar(&options.ApplyFixes, "apply-fixes", false, "Apply all available quick-fixes, including cleanup")
	flags.BoolVar(&options.Cleanup, "cleanup", false, "Run project cleanup")
	flags.StringVar(&options.FixesStrategy, "fixes-strategy", "", "Set the strategy for applying quick-fixes. Available values: 'apply', 'cleanup', 'none'")

	flags.StringArrayVar(&options.Property, "property", []string{}, "Set a JVM property to be used while running Qodana using the --property property.name=value1,value2,...,valueN notation")
	flags.BoolVarP(&options.SaveReport, "save-report", "s", true, "Generate HTML report")

	if options.LinterSpecific != nil {
		if linterSpecific, ok := options.LinterSpecific.(ThirdPartyOptions); ok {
			linterSpecific.AddFlags(flags)
		}
	}

	if !IsContainer() {
		flags.StringArrayVarP(&options.Env, "env", "e", []string{}, "Only for container runs. Define additional environment variables for the Qodana container (you can use the flag multiple times). CLI is not reading full host environment variables and does not pass it to the Qodana container for security reasons")
		flags.StringArrayVarP(&options.Volumes, "volume", "v", []string{}, "Only for container runs. Define additional volumes for the Qodana container (you can use the flag multiple times)")
		flags.StringVarP(&options.User, "user", "u", GetDefaultUser(), "Only for container runs. User to run Qodana container as. Please specify user id – '$UID' or user id and group id $(id -u):$(id -g). Use 'root' to run as the root user (default: the current user)")
		flags.BoolVar(&options.SkipPull, "skip-pull", false, "Only for container runs. Skip pulling the latest Qodana container")
		cmd.MarkFlagsMutuallyExclusive("linter", "ide")
		cmd.MarkFlagsMutuallyExclusive("skip-pull", "ide")
		cmd.MarkFlagsMutuallyExclusive("volume", "ide")
		cmd.MarkFlagsMutuallyExclusive("user", "ide")
		cmd.MarkFlagsMutuallyExclusive("env", "ide")
	}

	cmd.MarkFlagsMutuallyExclusive("commit", "script")
	cmd.MarkFlagsMutuallyExclusive("profile-name", "profile-path")
	cmd.MarkFlagsMutuallyExclusive("apply-fixes", "cleanup")

	err := cmd.Flags().MarkDeprecated("fixes-strategy", "use --apply-fixes / --cleanup instead")
	if err != nil {
		return err
	}
	err = cmd.Flags().MarkDeprecated("stub-profile", "this option has no effect and no replacement")
	if err != nil {
		return err
	}
	return nil
}
