package core

import (
	"path/filepath"
	"strings"

	"github.com/zalando/go-keyring"
)

var (
	defaultEndpoint = "qodana.cloud"
	defaultService  = "qodana-cli"
)

// getCloudTeamsPageUrl returns the team page URL on Qodana Cloud
func getCloudTeamsPageUrl(path string) string {
	name := filepath.Base(path)
	origin := gitRemoteUrl(path)

	return strings.Join([]string{"https://", defaultEndpoint, "/?origin=", origin, "&name=", name}, "")
}

// saveCloudToken saves token to the system keyring
func saveCloudToken(id string, token string) error {
	err := keyring.Set(defaultService, id, token)
	if err != nil {
		return err
	}
	return nil
}

// getCloudToken returns token from the system keyring
func getCloudToken(id string) (string, error) {
	secret, err := keyring.Get(defaultService, id)
	if err != nil {
		return "", err
	}
	return secret, nil
}
