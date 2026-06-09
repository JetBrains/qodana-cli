package downloaddeps

import "regexp"

var platformRe = regexp.MustCompile(`(linux|darwin|windows)-(amd64|arm64)`)

// selectFiles returns the subset of filenames relevant to the given runner: platform-agnostic
// files plus those matching goos/goarch. When all is true, every file is returned (used by the
// goreleaser build, which compiles every platform on one host). Input order is preserved.
func selectFiles(filenames []string, goos, goarch string, all bool) []string {
	if all {
		return filenames
	}
	var out []string
	for _, f := range filenames {
		fos, farch, specific := platformOf(f)
		if !specific || (fos == goos && farch == goarch) {
			out = append(out, f)
		}
	}
	return out
}

// platformOf extracts the GOOS/GOARCH a dependency archive targets from its filename.
// Names without a recognized <os>-<arch> segment (e.g. "clt.zip") are platform-agnostic
// and apply to every runner.
func platformOf(filename string) (goos, goarch string, platformSpecific bool) {
	m := platformRe.FindStringSubmatch(filename)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}
