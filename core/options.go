package core

import (
	"github.com/JetBrains/qodana-cli/v2023/platform"
	log "github.com/sirupsen/logrus"
	"os"
	"path/filepath"
)

type QodanaOptions struct {
	*platform.QodanaOptions
}

func (o *QodanaOptions) fixesSupported() bool {
	return o.guessProduct() != platform.QDNET && o.guessProduct() != platform.QDNETC && o.guessProduct() != platform.QDCL
}

func (o *QodanaOptions) appInfoXmlPath(ideBinDir string) string {
	appInfoPath := filepath.Join(ideBinDir, qodanaAppInfoFilename)
	if _, err := os.Stat(appInfoPath); err != nil && o.AnalysisId != "FAKE" {
		log.Fatalf("%s should exist in IDE directory %s. Unsupported IDE detected, exiting.", qodanaAppInfoFilename, ideBinDir)
	}
	return appInfoPath
}

func (o *QodanaOptions) vmOptionsPath() string {
	return filepath.Join(o.ConfDirPath(), "ide.vmoptions")
}
