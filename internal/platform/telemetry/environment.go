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
	"cmp"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
)

const gb = 1 << 30

// ideConfigDir matches IDE config directories like IntelliJIdea2024.3 or PyCharmCE2023.1.
var ideConfigDir = regexp.MustCompile(`^[A-Za-z]+\d{4}\.\d+$`)

func environment() (system map[string]any, software map[string]any) {
	system = map[string]any{"os": runtime.GOOS, "arch": runtime.GOARCH, "cpu_count": powerOfTwo(uint64(runtime.NumCPU()), cpuCounts)}
	if platform, _, platformVersion, err := host.PlatformInformation(); err == nil {
		platformVersion, _, _ = strings.Cut(platformVersion, " ")
		system["os_version"] = versionName(platformVersion)
		if runtime.GOOS == "linux" {
			system["distro"] = distro(platform)
		}
	}
	if memory, err := mem.VirtualMemory(); err == nil {
		total := memory.Total
		if limit, ok := containerMemory(); ok && limit < memory.Total {
			total = limit
		}
		system["ram_gb"] = powerOfTwo(total/gb, memorySizes)
	}
	cacheDir, _ := os.UserCacheDir()
	if usage, err := disk.Usage(cmp.Or(cacheDir, ".")); err == nil {
		system["disk_free_gb"] = bucket(int(usage.Free / gb))
	}

	software = map[string]any{
		"git":     isInstalled("git"),
		"docker":  isInstalled("docker"),
		"java":    isInstalled("java") || os.Getenv("JAVA_HOME") != "",
		"tar":     isInstalled("tar"),
		"unzip":   isInstalled("unzip"),
		"toolbox": isDir(toolboxDir()),
		"ide":     hasIde(),
	}
	if shell := os.Getenv("SHELL"); shell != "" {
		software["shell"] = oneOf(strings.TrimSuffix(filepath.Base(shell), ".exe"), shells)
	}
	return system, software
}

// distro names the Linux distribution by gopsutil's platform, like opensuse-leap for openSUSE.
func distro(platform string) string {
	if strings.HasPrefix(platform, "opensuse") {
		return "opensuse"
	}
	return oneOf(platform, distros)
}

// containerMemory returns the memory limit of the container the CLI runs in, which can't use the memory of the host
// beyond the limit. There is none outside a container or without a limit.
func containerMemory() (limit uint64, ok bool) {
	for _, file := range []string{
		"/sys/fs/cgroup/memory.max",                   // v2
		"/sys/fs/cgroup/memory/memory.limit_in_bytes", // v1
	} {
		if limit, err := readUint(file); err == nil {
			return limit, true
		}
	}
	return 0, false
}

func readUint(file string) (uint64, error) {
	content, err := os.ReadFile(file)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(strings.TrimSpace(string(content)), 10, 64)
}

func isInstalled(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func toolboxDir() string {
	var base string
	switch runtime.GOOS {
	case "darwin":
		base, _ = os.UserConfigDir() // ~/Library/Application Support
	case "windows":
		base, _ = os.UserCacheDir() // %LOCALAPPDATA%
	default:
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "JetBrains", "Toolbox")
}

// hasIde reports whether any JetBrains IDE has been run: each one creates its config directory.
func hasIde() bool {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return false
	}
	entries, _ := os.ReadDir(filepath.Join(configDir, "JetBrains"))
	for _, entry := range entries {
		if entry.IsDir() && ideConfigDir.MatchString(entry.Name()) {
			return true
		}
	}
	return false
}
