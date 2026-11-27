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

package telemetry

import (
	"net/url"
	"regexp"
	"slices"
	"strings"
)

// JetBrains' own use of Qodana is not reported: a run of a JetBrains repository or with a Qodana Cloud of JetBrains is
// never sent, nor one of an internal organization in Qodana Cloud, see cloud.QdApiEndpoints.GetLicenseData.
var (
	// jetBrainsDomains are not jetbrains.space, which hosts the Space of customers
	jetBrainsDomains = []string{"jetbrains.com", "jetbrains.team", "intellij.net", "jb.gg"}
	// publicHostings host the JetBrains organizations: a self-hosted one may have a group of a customer named jetbrains
	publicHostings = []string{"github.com", "gitlab.com", "bitbucket.org"}
	// jetBrainsOwner is a JetBrains organization on a public hosting, like github.com/JetBrains or JetBrains-Research
	jetBrainsOwner = regexp.MustCompile(`(?i)^jetbrains(-.*)?$`)
)

func isJetBrainsRepository(remoteUrl string) bool {
	host, owner := parseUrl(remoteUrl)
	return isJetBrainsHost(host) || slices.Contains(publicHostings, host) && jetBrainsOwner.MatchString(owner)
}

// isJetBrainsHost takes a lowercase host, as parseUrl returns it.
func isJetBrainsHost(host string) bool {
	return slices.ContainsFunc(
		jetBrainsDomains,
		func(domain string) bool { return host == domain || strings.HasSuffix(host, "."+domain) },
	)
}

// parseUrl returns the lowercase host and the first path segment of a URL, also of one without a scheme and of a git remote in
// the scp syntax, user@host:owner/repository.
func parseUrl(rawUrl string) (host string, owner string) {
	if !strings.Contains(rawUrl, "://") {
		if before, after, scp := strings.Cut(rawUrl, ":"); scp && !strings.Contains(before, "/") {
			rawUrl = "ssh://" + before + "/" + after
		} else {
			rawUrl = "https://" + rawUrl
		}
	}
	parsed, err := url.Parse(rawUrl)
	if err != nil {
		return "", ""
	}
	owner, _, _ = strings.Cut(strings.TrimPrefix(parsed.Path, "/"), "/")
	return strings.ToLower(parsed.Hostname()), owner
}
