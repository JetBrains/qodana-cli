package platform

import (
	"fmt"
	"golang.org/x/exp/maps"
	"os"
	"reflect"
	"testing"
)

func unsetGitHubVariables() {
	variables := []string{
		"GITHUB_SERVER_URL",
		"GITHUB_REPOSITORY",
		"GITHUB_RUN_ID",
		"GITHUB_HEAD_REF",
		"GITHUB_REF",
	}
	for _, v := range variables {
		_ = os.Unsetenv(v)
	}
}

func Test_ExtractEnvironmentVariables(t *testing.T) {
	revisionExpected := "1234567890abcdef1234567890abcdef12345678"
	branchExpected := "refs/heads/main"

	if os.Getenv("GITHUB_ACTIONS") == "true" {
		unsetGitHubVariables()
	}

	for _, tc := range []struct {
		ci                string
		variables         map[string]string
		jobUrlExpected    string
		envExpected       string
		remoteUrlExpected string
		revisionExpected  string
		branchExpected    string
	}{
		{
			ci:          "no CI detected",
			variables:   map[string]string{},
			envExpected: "cli:dev",
		},
		{
			ci: "User defined",
			variables: map[string]string{
				qodanaEnv:       "user-defined",
				qodanaJobUrl:    "https://qodana.jetbrains.com/never-gonna-give-you-up",
				QodanaRemoteUrl: "https://qodana.jetbrains.com/never-gonna-give-you-up",
				QodanaBranch:    branchExpected,
				QodanaRevision:  revisionExpected,
			},
			envExpected:       "user-defined",
			remoteUrlExpected: "https://qodana.jetbrains.com/never-gonna-give-you-up",
			jobUrlExpected:    "https://qodana.jetbrains.com/never-gonna-give-you-up",
			revisionExpected:  revisionExpected,
			branchExpected:    branchExpected,
		},
		{
			ci: "Space",
			variables: map[string]string{
				"JB_SPACE_EXECUTION_URL":       "https://space.jetbrains.com/never-gonna-give-you-up",
				"JB_SPACE_GIT_BRANCH":          branchExpected,
				"JB_SPACE_GIT_REVISION":        revisionExpected,
				"JB_SPACE_API_URL":             "jetbrains.team",
				"JB_SPACE_PROJECT_KEY":         "sa",
				"JB_SPACE_GIT_REPOSITORY_NAME": "entrypoint",
			},
			envExpected:       fmt.Sprintf("space:%s", Version),
			remoteUrlExpected: "ssh://git@git.jetbrains.team/sa/entrypoint.git",
			jobUrlExpected:    "https://space.jetbrains.com/never-gonna-give-you-up",
			revisionExpected:  revisionExpected,
			branchExpected:    branchExpected,
		},
		{
			ci: "GitLab",
			variables: map[string]string{
				"CI_JOB_URL":        "https://gitlab.jetbrains.com/never-gonna-give-you-up",
				"CI_COMMIT_BRANCH":  branchExpected,
				"CI_COMMIT_SHA":     revisionExpected,
				"CI_REPOSITORY_URL": "https://gitlab.jetbrains.com/sa/entrypoint.git",
			},
			envExpected:       fmt.Sprintf("gitlab:%s", Version),
			remoteUrlExpected: "https://gitlab.jetbrains.com/sa/entrypoint.git",
			jobUrlExpected:    "https://gitlab.jetbrains.com/never-gonna-give-you-up",
			revisionExpected:  revisionExpected,
			branchExpected:    branchExpected,
		},
		{
			ci: "Jenkins",
			variables: map[string]string{
				"BUILD_URL":        "https://jenkins.jetbrains.com/never-gonna-give-you-up",
				"GIT_LOCAL_BRANCH": branchExpected,
				"GIT_COMMIT":       revisionExpected,
				"GIT_URL":          "https://git.jetbrains.com/sa/entrypoint.git",
			},
			envExpected:       fmt.Sprintf("jenkins:%s", Version),
			jobUrlExpected:    "https://jenkins.jetbrains.com/never-gonna-give-you-up",
			remoteUrlExpected: "https://git.jetbrains.com/sa/entrypoint.git",
			revisionExpected:  revisionExpected,
			branchExpected:    branchExpected,
		},
		{
			ci: "GitHub",
			variables: map[string]string{
				"GITHUB_SERVER_URL": "https://github.jetbrains.com",
				"GITHUB_REPOSITORY": "sa/entrypoint",
				"GITHUB_RUN_ID":     "123456789",
				"GITHUB_SHA":        revisionExpected,
				"GITHUB_HEAD_REF":   branchExpected,
			},
			envExpected:       fmt.Sprintf("github-actions:%s", Version),
			jobUrlExpected:    "https://github.jetbrains.com/sa/entrypoint/actions/runs/123456789",
			remoteUrlExpected: "https://github.jetbrains.com/sa/entrypoint.git",
			revisionExpected:  revisionExpected,
			branchExpected:    branchExpected,
		},
		{
			ci: "GitHub push",
			variables: map[string]string{
				"GITHUB_SERVER_URL": "https://github.jetbrains.com",
				"GITHUB_REPOSITORY": "sa/entrypoint",
				"GITHUB_RUN_ID":     "123456789",
				"GITHUB_SHA":        revisionExpected,
				"GITHUB_REF":        branchExpected,
			},
			envExpected:       fmt.Sprintf("github-actions:%s", Version),
			jobUrlExpected:    "https://github.jetbrains.com/sa/entrypoint/actions/runs/123456789",
			remoteUrlExpected: "https://github.jetbrains.com/sa/entrypoint.git",
			revisionExpected:  revisionExpected,
			branchExpected:    branchExpected,
		},
		{
			ci: "CircleCI",
			variables: map[string]string{
				"CIRCLE_BUILD_URL":      "https://circleci.jetbrains.com/never-gonna-give-you-up",
				"CIRCLE_SHA1":           revisionExpected,
				"CIRCLE_BRANCH":         branchExpected,
				"CIRCLE_REPOSITORY_URL": "https://circleci.jetbrains.com/sa/entrypoint.git",
			},
			envExpected:       fmt.Sprintf("circleci:%s", Version),
			jobUrlExpected:    "https://circleci.jetbrains.com/never-gonna-give-you-up",
			remoteUrlExpected: "https://circleci.jetbrains.com/sa/entrypoint.git",
			revisionExpected:  revisionExpected,
			branchExpected:    branchExpected,
		},
		{
			ci: "Azure Pipelines",
			variables: map[string]string{
				"SYSTEM_TEAMFOUNDATIONCOLLECTIONURI": "https://dev.azure.com/jetbrains",
				"BUILD_BUILDURI":                     "https://dev.azure.com/jetbrains/never-gonna-give-you-up",
				"SYSTEM_TEAMPROJECT":                 "/sa",
				"BUILD_BUILDID":                      "123456789",
				"BUILD_SOURCEVERSION":                revisionExpected,
				"BUILD_SOURCEBRANCH":                 "refs/heads/" + branchExpected,
				"BUILD_REPOSITORY_URI":               "https://dev.azure.com/jetbrains/sa/entrypoint.git",
			},
			envExpected:       fmt.Sprintf("azure-pipelines:%s", Version),
			jobUrlExpected:    "https://dev.azure.com/jetbrains/sa/_build/results?buildId=123456789",
			remoteUrlExpected: "https://dev.azure.com/jetbrains/sa/entrypoint.git",
			revisionExpected:  revisionExpected,
			branchExpected:    branchExpected,
		},
	} {
		t.Run(tc.ci, func(t *testing.T) {
			opts := &QodanaOptions{}
			for k, v := range tc.variables {
				err := os.Setenv(k, v)
				if err != nil {
					t.Fatal(err)
				}
				opts.Setenv(k, v)
			}

			for _, environment := range []struct {
				name  string
				set   func(string, string)
				unset func(string)
				get   func(string) string
			}{
				{
					name: "Container",
					set:  opts.Setenv,
					get:  opts.Getenv,
				},
				{
					name: "Local",
					set:  SetEnv,
					get:  os.Getenv,
				},
			} {
				t.Run(environment.name, func(t *testing.T) {
					ExtractQodanaEnvironment(environment.set)
					currentQodanaEnv := environment.get(qodanaEnv)
					if currentQodanaEnv != tc.envExpected {
						t.Errorf("%s: Expected %s, got %s", environment.name, tc.envExpected, currentQodanaEnv)
					}
					if environment.get(qodanaJobUrl) != tc.jobUrlExpected {
						t.Errorf("%s: Expected %s, got %s", environment.name, tc.jobUrlExpected, environment.get(qodanaJobUrl))
					}
					if environment.get(QodanaRemoteUrl) != tc.remoteUrlExpected {
						t.Errorf("%s: Expected %s, got %s", environment.name, tc.remoteUrlExpected, environment.get(QodanaRemoteUrl))
					}
					if environment.get(QodanaRevision) != tc.revisionExpected {
						t.Errorf("%s: Expected %s, got %s", environment.name, revisionExpected, environment.get(QodanaRevision))
					}
					if environment.get(QodanaBranch) != tc.branchExpected {
						t.Errorf("%s: Expected %s, got %s", environment.name, branchExpected, environment.get(QodanaBranch))
					}
				})
			}

			for _, k := range append(maps.Keys(tc.variables), []string{qodanaJobUrl, qodanaEnv, QodanaRemoteUrl, QodanaRevision, QodanaBranch}...) {
				err := os.Unsetenv(k)
				if err != nil {
					t.Fatal(err)
				}
				opts.Unsetenv(k)
			}
		})
	}
}

func TestDirLanguagesExcluded(t *testing.T) {
	expected := []string{"Go", "Shell", "Dockerfile"}
	actual, err := recognizeDirLanguages("../")
	if err != nil {
		return
	}
	if !reflect.DeepEqual(expected, actual) {
		t.Fatalf("expected \"%s\" got \"%s\"", expected, actual)
	}
}
