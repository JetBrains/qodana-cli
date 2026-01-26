//go:build ignore

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const buildserver = "https://buildserver.labs.intellij.net"

var tcToken string

type artifact struct {
	buildTypeID string
	pattern     string
	subPath     string
	destPath    string
}

var clangArtifacts = []artifact{
	{
		"ijplatform_master_CIDR_ExternalTools_VanillaClangTidyAll",
		`clang-tidy-[^"]*-linux-x64\.tar\.gz`,
		"",
		"clang/clang-tidy-linux-amd64.tar.gz",
	},
	{
		"ijplatform_master_CIDR_ExternalTools_VanillaClangTidyAll",
		`clang-tidy-[^"]*-linux-aarch64\.tar\.gz`,
		"",
		"clang/clang-tidy-linux-arm64.tar.gz",
	},
	{
		"ijplatform_master_CIDR_ExternalTools_VanillaClangTidyAll",
		`clang-tidy-[^"]*-mac-x64\.tar\.gz`,
		"",
		"clang/clang-tidy-darwin-amd64.tar.gz",
	},
	{
		"ijplatform_master_CIDR_ExternalTools_VanillaClangTidyAll",
		`clang-tidy-[^"]*-mac-aarch64\.tar\.gz`,
		"",
		"clang/clang-tidy-darwin-arm64.tar.gz",
	},
	{
		"ijplatform_master_CIDR_ExternalTools_VanillaClangTidyAll",
		`clang-tidy-[^"]*-win-x64\.zip`,
		"",
		"clang/clang-tidy-windows-amd64.zip",
	},
	{
		"ijplatform_master_CIDR_ExternalTools_VanillaClangTidyAll",
		`clang-tidy-[^"]*-win-aarch64\.zip`,
		"",
		"clang/clang-tidy-windows-arm64.zip",
	},
}

var cdnetArtifacts = []artifact{
	{
		"ijplatform_master_Net_PostCompile_TriggerAllInstallers",
		`JetBrains\.ReSharper\.GlobalTools\.[^"]*\.nupkg`,
		"Artifacts.InstallersPortablesZips",
		"cdnet/clt.zip",
	},
}

func main() {
	repoRoot := findRepoRoot()
	loadEnv(filepath.Join(repoRoot, ".env"))
	tcToken = os.Getenv("TEAMCITY_TOKEN")
	if tcToken == "" {
		log.Fatal("TEAMCITY_TOKEN not set (check .env file)")
	}
	log.Printf("Downloading dependencies to %s", repoRoot)

	var all []artifact
	all = append(all, clangArtifacts...)
	all = append(all, cdnetArtifacts...)

	for _, a := range all {
		dest := filepath.Join(repoRoot, a.destPath)
		if err := download(a, dest); err != nil {
			log.Printf("WARN: %s: %v", a.destPath, err)
		}
	}
	log.Println("Done")
}

func loadEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		if k, v, ok := strings.Cut(line, "="); ok {
			if os.Getenv(k) == "" {
				os.Setenv(k, v)
			}
		}
	}
}

func download(a artifact, dest string) error {
	artifactName, err := findArtifact(a.buildTypeID, a.subPath, a.pattern)
	if err != nil {
		return err
	}

	urlPath := artifactName
	if a.subPath != "" {
		urlPath = a.subPath + "/" + artifactName
	}
	url := fmt.Sprintf(
		"%s/app/rest/builds/buildType:%s,status:SUCCESS,count:1/artifacts/content/%s",
		buildserver,
		a.buildTypeID,
		urlPath,
	)

	log.Printf("Downloading %s", a.destPath)
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+tcToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

func findArtifact(buildTypeID, subPath, pattern string) (string, error) {
	url := fmt.Sprintf("%s/app/rest/builds/buildType:%s,status:SUCCESS,count:1/artifacts", buildserver, buildTypeID)
	if subPath != "" {
		url += "/children/" + subPath
	}

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+tcToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var data struct {
		File []struct {
			Name string `json:"name"`
		} `json:"file"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		re := regexp.MustCompile(`"` + pattern + `"`)
		if match := re.FindString(string(body)); match != "" {
			return strings.Trim(match, `"`), nil
		}
		return "", fmt.Errorf("no match for %s", pattern)
	}

	re := regexp.MustCompile(pattern)
	for _, f := range data.File {
		if re.MatchString(f.Name) {
			return f.Name, nil
		}
	}
	return "", fmt.Errorf("no match for %s", pattern)
}

func findRepoRoot() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			log.Fatal("Could not find repository root (no go.mod found)")
		}
		dir = parent
	}
}
