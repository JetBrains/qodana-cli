// Copyright 2026 JetBrains s.r.o. Licensed under the Apache License, Version 2.0.

package review

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (c *Client) File(ctx context.Context, repo Repository, revision, path string) (string, error) {
	if !ValidRevision(revision) || !ValidPath(path) {
		return "", errors.New("source reads require a full Git revision and a repository-relative path")
	}
	root := repoEndpoint(repo)
	var encoded string
	var size int
	if repo.Provider == "github" {
		parts := strings.Split(path, "/")
		for i := range parts {
			parts[i] = url.PathEscape(parts[i])
		}
		var value struct {
			Type     string `json:"type"`
			Content  string `json:"content"`
			Encoding string `json:"encoding"`
			Size     int    `json:"size"`
		}
		_, err := c.get(ctx, "github", root+"/contents/"+strings.Join(parts, "/"), url.Values{"ref": {revision}}, &value)
		if err != nil {
			return "", err
		}
		if value.Type != "file" || value.Encoding != "base64" {
			return "", errors.New("GitHub did not return complete base64 file content")
		}
		encoded, size = value.Content, value.Size
	} else {
		var files []struct {
			Path string `json:"path"`
			Type string `json:"type"`
			Blob string `json:"blob"`
		}
		_, err := c.get(ctx, "space", root+"/files", url.Values{"commit": {revision}, "path": {path}}, &files)
		if err != nil {
			return "", err
		}
		blob := ""
		for _, file := range files {
			if file.Path == path && (file.Type == "FILE" || file.Type == "EXE_FILE") {
				blob = file.Blob
			}
		}
		if blob == "" {
			return "", os.ErrNotExist
		}
		var value struct {
			Content string `json:"partBase64"`
			Size    int    `json:"totalSize"`
		}
		_, err = c.get(ctx, "space", root+"/content", url.Values{"blobId": {blob}, "skip": {"0"}, "limit": {strconv.Itoa(maxResponse)}}, &value)
		if err != nil {
			return "", err
		}
		encoded, size = value.Content, value.Size
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(data) != size {
		return "", errors.New("provider file content is invalid or truncated")
	}
	if strings.ContainsRune(string(data), 0) {
		return "", errors.New("binary source files are unsupported")
	}
	return string(data), nil
}

// Diff runs Git on complete provider snapshots. Provider compare patches may be
// truncated, so they are never used as canonical signal evidence.
func (c *Client) Diff(ctx context.Context, repo Repository, before, after, beforePath, afterPath string) (string, error) {
	old, oldErr := c.File(ctx, repo, before, beforePath)
	if oldErr != nil && !errors.Is(oldErr, os.ErrNotExist) {
		return "", oldErr
	}
	newContent, newErr := c.File(ctx, repo, after, afterPath)
	if newErr != nil && !errors.Is(newErr, os.ErrNotExist) {
		return "", newErr
	}
	if oldErr != nil && newErr != nil {
		return "", errors.New("file is absent at both requested revisions")
	}
	directory, err := os.MkdirTemp("", "edict-review-diff-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(directory)
	if err = os.WriteFile(filepath.Join(directory, "before"), []byte(old), 0600); err != nil {
		return "", err
	}
	if err = os.WriteFile(filepath.Join(directory, "after"), []byte(newContent), 0600); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "git", "--no-pager", "diff", "--no-index", "--no-prefix", "--no-color", "--no-ext-diff", "--no-textconv", "--unified=200", "--", "before", "after")
	command.Dir = directory
	output, err := command.Output()
	var exit *exec.ExitError
	if err != nil && !(errors.As(err, &exit) && exit.ExitCode() == 1) {
		return "", fmt.Errorf("Git diff of provider snapshots failed: %w", err)
	}
	if len(output) == 0 {
		return "", nil
	}
	// Keep Git's hunks byte-for-byte; replace only the scratch-file headers with
	// the authentic repository-relative paths. This mirrors Ultimate's file_diff.
	hunk := strings.Index(string(output), "@@ ")
	if hunk < 0 {
		return "", errors.New("provider snapshot diff has no source hunks")
	}
	oldName, newName := "a/"+beforePath, "b/"+afterPath
	if oldErr != nil {
		oldName = "/dev/null"
	}
	if newErr != nil {
		newName = "/dev/null"
	}
	quote := func(name string) string {
		if strings.ContainsAny(name, "\t\"\\") {
			return strconv.Quote(name)
		}
		return name
	}
	return "--- " + quote(oldName) + "\n+++ " + quote(newName) + "\n" + string(output[hunk:]), nil
}
