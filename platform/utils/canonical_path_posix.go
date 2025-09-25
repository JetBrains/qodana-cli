//go:build unix

package utils

import (
	"os"
	"path/filepath"
	"strings"
)

// CanonicalPath produces a standard, or "canonical" form of a path.
// A canonical path has the following properties:
// - is absolute;
// - contains no directory traversal segments such as `.` and `..`;
// - contains no symbolic links;
// - has normalized case, in filesystems which are case-insensitive;
// - contains no repeated path separators.
// Since producing a canonical path implies resolution of symlinks, the path must exist to be canonicalized.
// On POSIX systems, this function provides pure Go implementation equivalent to `realpath`.
func CanonicalPath(path string) (result string, err error) {
	if !filepath.IsAbs(path) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		path = cwd + "/" + path
	}

	parts := strings.Split(path, string(filepath.Separator))

	filteredParts := []string{}
	for _, part := range parts {
		if part != "" {
			filteredParts = append(filteredParts, part)
		}
	}

	result, err = processPath(filteredParts)
	if err != nil {
		return "", err
	}

	result, err = getCaseSensitivePath(result)
	if err != nil {
		return "", err
	}

	return result, nil
}

// processPath processes the path components, handling symlinks and .. correctly
func processPath(parts []string) (string, error) {
	stack := []string{}

	i := 0
	for i < len(parts) {
		part := parts[i]

		if part == "." {
			i++
			continue
		}

		if part == ".." {
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			i++
			continue
		}

		stack = append(stack, part)

		currentPath := "/" + strings.Join(stack, "/")

		info, err := os.Lstat(currentPath)
		if err != nil {
			return "", err
		}

		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(currentPath)
			if err != nil {
				return "", err
			}

			stack = stack[:len(stack)-1]
			remainingParts := parts[i+1:]

			var targetParts []string
			if filepath.IsAbs(target) {
				targetParts = strings.Split(target, string(filepath.Separator))

				filteredTargetParts := []string{}
				for _, part := range targetParts {
					if part != "" {
						filteredTargetParts = append(filteredTargetParts, part)
					}
				}

				allParts := append(filteredTargetParts, remainingParts...)
				return processPath(allParts)
			} else {
				targetParts = strings.Split(target, string(filepath.Separator))

				filteredTargetParts := []string{}
				for _, part := range targetParts {
					if part != "" {
						filteredTargetParts = append(filteredTargetParts, part)
					}
				}

				allParts := append(stack, filteredTargetParts...)
				allParts = append(allParts, remainingParts...)
				return processPath(allParts)
			}
		}

		i++
	}

	result := "/"
	if len(stack) > 0 {
		result = "/" + strings.Join(stack, "/")
	}

	return result, nil
}

// getCaseSensitivePath returns the path with the actual case of each component as it exists on the filesystem.
func getCaseSensitivePath(path string) (string, error) {
	if path == "/" {
		return path, nil
	}

	parts := strings.Split(path[1:], "/")
	result := "/"

	for _, part := range parts {
		if part == "" {
			continue
		}

		entries, err := os.ReadDir(result)
		if err != nil {
			return "", err
		}

		found := false
		for _, entry := range entries {
			if strings.EqualFold(entry.Name(), part) {
				if result == "/" {
					result = "/" + entry.Name()
				} else {
					result = result + "/" + entry.Name()
				}
				found = true
				break
			}
		}

		if !found {
			return "", os.ErrNotExist
		}
	}

	return result, nil
}
