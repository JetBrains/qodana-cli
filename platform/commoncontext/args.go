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

package commoncontext

import (
	"github.com/JetBrains/qodana-cli/v2024/platform/qdenv"
	"os"
	"path/filepath"
)

type Context struct {
	Linter                 string
	Ide                    string
	IsClearCache           bool
	CacheDir               string
	ProjectDir             string
	ResultsDir             string
	ReportDir              string
	QodanaSystemDir        string
	Id                     string
	QodanaToken            string
	QodanaLicenseOnlyToken string
}

func (c Context) LogDir() string {
	return filepath.Join(c.ResultsDir, "log")
}

func (c Context) ConfDirPath() string {
	if conf, ok := os.LookupEnv(qdenv.QodanaConfEnv); ok {
		return conf
	}
	confDir := filepath.Join(c.GetLinterDir(), "config")
	return confDir
}

func (c Context) GetLinterDir() string {
	return filepath.Join(
		c.QodanaSystemDir,
		c.Id,
	)
}

/**
CloudTokenLoader
*/

func (c Context) GetQodanaToken() string            { return c.QodanaToken }
func (c Context) GetQodanaLicenseOnlyToken() string { return c.QodanaLicenseOnlyToken }
func (c Context) GetId() string                     { return c.Id }
func (c Context) GetIde() string                    { return c.Ide }
func (c Context) GetLinter() string                 { return c.Linter }
func (c Context) GetProjectDir() string             { return c.ProjectDir }
func (c Context) GetLogDir() string                 { return c.LogDir() }
