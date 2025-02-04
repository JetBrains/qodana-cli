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

package startupargs

import (
	"github.com/JetBrains/qodana-cli/v2024/platform"
	"os"
	"path/filepath"
)

type Args struct {
	Linter                 string
	Ide                    string
	IsClearCache           bool
	CacheDir               string
	ProjectDir             string
	ResultsDir             string
	QodanaSystemDir        string
	Id                     string
	QodanaToken            string
	QodanaLicenseOnlyToken string
}

func (o Args) LogDir() string {
	return filepath.Join(o.ResultsDir, "log")
}

func (o Args) ConfDirPath() string {
	if conf, ok := os.LookupEnv(platform.QodanaConfEnv); ok {
		return conf
	}
	confDir := filepath.Join(o.GetLinterDir(), "config")
	return confDir
}

func (o Args) GetLinterDir() string {
	return filepath.Join(
		o.QodanaSystemDir,
		o.Id,
	)
}

/**
CloudTokenLoader
*/

func (o Args) GetQodanaToken() string            { return o.QodanaToken }
func (o Args) GetQodanaLicenseOnlyToken() string { return o.QodanaLicenseOnlyToken }
func (o Args) GetId() string                     { return o.Id }
func (o Args) GetIde() string                    { return o.Ide }
func (o Args) GetLinter() string                 { return o.Linter }
func (o Args) GetProjectDir() string             { return o.ProjectDir }
func (o Args) GetLogDir() string                 { return o.LogDir() }
