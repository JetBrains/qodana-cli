package downloaddeps

import "testing"

func TestPlatformOf(t *testing.T) {
	tests := []struct {
		filename             string
		wantGoos, wantGoarch string
		wantSpecific         bool
	}{
		{"clang-tidy-linux-amd64.tar.gz", "linux", "amd64", true},
		{"clang-tidy-linux-arm64.tar.gz", "linux", "arm64", true},
		{"clang-tidy-darwin-amd64.tar.gz", "darwin", "amd64", true},
		{"clang-tidy-darwin-arm64.tar.gz", "darwin", "arm64", true},
		{"clang-tidy-windows-amd64.zip", "windows", "amd64", true},
		{"clang-tidy-windows-arm64.zip", "windows", "arm64", true},
		{"clt.zip", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			goos, goarch, specific := platformOf(tt.filename)
			if goos != tt.wantGoos || goarch != tt.wantGoarch || specific != tt.wantSpecific {
				t.Errorf("platformOf(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tt.filename, goos, goarch, specific, tt.wantGoos, tt.wantGoarch, tt.wantSpecific)
			}
		})
	}
}

func TestSelectFiles(t *testing.T) {
	all := []string{
		"clang-tidy-linux-amd64.tar.gz",
		"clang-tidy-linux-arm64.tar.gz",
		"clang-tidy-darwin-arm64.tar.gz",
		"clt.zip",
	}
	t.Run("runner gets only its platform plus agnostic", func(t *testing.T) {
		got := selectFiles(all, "linux", "amd64", false)
		want := []string{"clang-tidy-linux-amd64.tar.gz", "clt.zip"}
		if !equalStrings(got, want) {
			t.Errorf("selectFiles(linux/amd64) = %v, want %v", got, want)
		}
	})
	t.Run("all flag returns everything", func(t *testing.T) {
		got := selectFiles(all, "linux", "amd64", true)
		if !equalStrings(got, all) {
			t.Errorf("selectFiles(all=true) = %v, want %v", got, all)
		}
	})
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
