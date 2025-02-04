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
	"context"
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/sarif"
	bbapi "github.com/reviewdog/go-bitbucket" // adapted from https://raw.githubusercontent.com/reviewdog/reviewdog/master/LICENSE
	log "github.com/sirupsen/logrus"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

const (
	httpTimeout              = time.Second * 10
	bitBucketAnnotationLimit = 1000
	bitBucketReporter        = "JetBrains Qodana"
	bitBucketAvatar          = "https://avatars.githubusercontent.com/u/139879315"
	bitBucketReportFailed    = "FAILED"
	bitBucketReportPassed    = "PASSED"
	bitBucketReportType      = "BUG"
	bitBucketAnnotationType  = "CODE_SMELL"

	// https://developer.atlassian.com/cloud/bitbucket/rest/api-group-reports/#api-repositories-workspace-repo-slug-commit-commit-reports-reportid-annotations-annotationid-put-request
	bitBucketHigh   = "HIGH"
	bitBucketMedium = "MEDIUM"
	bitBucketLow    = "LOW"
	bitBucketInfo   = "INFO"

	pipelineProxyURL = "http://localhost:29418"
	pipeProxyURL     = "http://host.docker.internal:29418"
)

// toBitBucketSeverity maps SARIF and Qodana severity levels to BitBucket severity levels, levels are mapped to the closest match https://www.jetbrains.com/help/qodana/qodana-sarif-output.html#SARIF+severity
var (
	toBitBucketSeverity = map[string]string{
		sarifError:     bitBucketHigh,
		sarifWarning:   bitBucketMedium,
		sarifNote:      bitBucketLow,
		qodanaCritical: bitBucketHigh,
		qodanaHigh:     bitBucketHigh,
		qodanaModerate: bitBucketMedium,
		qodanaLow:      bitBucketLow,
		qodanaInfo:     bitBucketInfo,
	}
)

// sendBitBucketReport sends annotations to BitBucket code Insights
func sendBitBucketReport(annotations []bbapi.ReportAnnotation, toolName, cloudUrl, reportId string) error {
	client, ctx := getBitBucketClient(), getBitBucketContext()
	repoOwner, repoName, sha := getBitBucketRepoOwner(), getBitBucketRepoName(), getBitBucketCommit()
	_, resp, err := client.
		ReportsApi.CreateOrUpdateReport(ctx, repoOwner, repoName, sha, reportId).
		Body(buildReport(toolName, annotations, cloudUrl)).
		Execute()
	if err = checkBitBucketApiError(err, resp, http.StatusOK); err != nil {
		return fmt.Errorf("failed to create code insights report: %w", err)
	}
	totalAnnotations := len(annotations)
	if totalAnnotations != 0 {
		if totalAnnotations > bitBucketAnnotationLimit {
			totalAnnotations = bitBucketAnnotationLimit
			log.Debugf("Warning: Only first 1000 of %d annotations will be sent", len(annotations))
		}
		for i := 0; i < totalAnnotations; i += 100 {
			j := i + 100
			if j > totalAnnotations {
				j = totalAnnotations
			}
			_, resp, err := client.ReportsApi.
				BulkCreateOrUpdateAnnotations(ctx, repoOwner, repoName, sha, reportId).
				Body(annotations[i:j]).
				Execute()
			if err = checkBitBucketApiError(err, resp, http.StatusOK); err != nil {
				return fmt.Errorf("failed to create code insights annotations: %w", err)
			}
		}
	}
	return nil
}

// getBitBucketContext returns a context with BitBucket credentials (not required for runs in BitBucket Pipelines)
func getBitBucketContext() context.Context {
	ctx := context.Background()
	user, password, token :=
		os.Getenv("QD_BITBUCKET_USER"),
		os.Getenv("QD_BITBUCKET_PASSWORD"),
		os.Getenv("QD_BITBUCKET_TOKEN")
	if user != "" && password != "" {
		ctx = context.WithValue(ctx, bbapi.ContextBasicAuth,
			bbapi.BasicAuth{
				UserName: user,
				Password: password,
			})
	}
	if token != "" {
		ctx = context.WithValue(ctx, bbapi.ContextAccessToken, token)
	}
	return ctx
}

// buildReport builds a report to be sent to BitBucket code Insights
func buildReport(toolName string, annotations []bbapi.ReportAnnotation, cloudUrl string) bbapi.Report {
	var result string
	if len(annotations) == 0 {
		result = bitBucketReportPassed
	} else {
		result = bitBucketReportFailed
	}

	data := bbapi.NewReport()
	data.SetTitle(toolName + " ")
	data.SetReportType(bitBucketReportType)
	data.SetReporter(bitBucketReporter)
	data.SetLogoUrl(bitBucketAvatar)
	data.SetLink(cloudUrl)
	data.SetDetails(getProblemsFoundMessage(len(annotations)))
	data.SetResult(result)
	return *data
}

// buildAnnotation builds an annotation to be sent to BitBucket code Insights
func buildAnnotation(r *sarif.Result, ruleDescription string, reportLink string) bbapi.ReportAnnotation {
	bbSeverity, ok := toBitBucketSeverity[getSeverity(r)]
	if !ok {
		log.Debugf("Unknown SARIF severity: %s", getSeverity(r))
		bbSeverity = bitBucketLow
	}

	data := bbapi.NewReportAnnotation()
	data.SetExternalId(getFingerprint(r))
	data.SetAnnotationType(bitBucketAnnotationType)
	data.SetSummary(fmt.Sprintf("%s: %s", r.RuleId, r.Message.Text))
	data.SetDetails(ruleDescription)
	data.SetSeverity(bbSeverity)

	if r != nil && r.Locations != nil && len(r.Locations) > 0 && r.Locations[0].PhysicalLocation != nil {
		location := r.Locations[0].PhysicalLocation
		if location.Region != nil {
			data.SetLine(int32(location.Region.StartLine))
		}
		if location.ArtifactLocation != nil {
			data.SetPath(location.ArtifactLocation.Uri)
		}
	}
	data.SetLink(reportLink)
	return *data
}

// getBitBucketClient returns a BitBucket API client with proper configuration by bbapi package
func getBitBucketClient() *bbapi.APIClient {
	config := bbapi.NewConfiguration()
	config.HTTPClient = &http.Client{
		Timeout: httpTimeout,
	}
	server := bbapi.ServerConfiguration{
		URL:         "https://api.bitbucket.org/2.0",
		Description: `HTTPS API endpoint`,
	}
	if IsBitBucket() {
		var proxyURL *url.URL
		if isBitBucketPipe() {
			proxyURL, _ = url.Parse(pipeProxyURL)
		} else {
			proxyURL, _ = url.Parse(pipelineProxyURL)
		}
		config.HTTPClient.Transport = &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		}
		//goland:noinspection HttpUrlsUsage
		server = bbapi.ServerConfiguration{
			URL: "http://api.bitbucket.org/2.0",
		}
	}
	config.Servers = bbapi.ServerConfigurations{server}
	return bbapi.NewAPIClient(config)
}

// checkBitBucketApiError checks if the API call was successful
func checkBitBucketApiError(err error, resp *http.Response, expectedCode int) error {
	if err != nil {
		return fmt.Errorf("bitbucket Cloud API error: %w", err)
	}
	if resp != nil && resp.StatusCode != expectedCode {
		body, _ := io.ReadAll(resp.Body)
		log.Debugf("Unexpected response: %s", body)
		return fmt.Errorf("bitbucket Cloud API error: %w", err)
	}
	return nil
}
