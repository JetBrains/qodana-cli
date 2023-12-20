package linter

import (
	"fmt"
	tooling "github.com/JetBrains/qodana-cli/v2023/linter/tooling"
	"github.com/JetBrains/qodana-cli/v2023/platform"
	"os"
	"path/filepath"
)

func (o *CltOptions) MountTools(_ string, mountPath string, _ *platform.QodanaOptions) (map[string]string, error) {
	val := make(map[string]string)
	val["clt"] = filepath.Join(mountPath, "tools", "netcoreapp3.1", "any", "JetBrains.CommandLine.Products.dll")
	archive := "clt.zip"
	if _, err := os.Stat(val["clt"]); err != nil {
		if os.IsNotExist(err) {
			path := platform.ProcessAuxiliaryTool(archive, "clang", mountPath, tooling.Clt)
			if err := platform.Decompress(path, mountPath); err != nil {
				return nil, fmt.Errorf("failed to decompress clang archive: %w", err)
			}
		}
	}
	return val, nil
}
