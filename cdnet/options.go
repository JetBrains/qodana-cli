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

package main

import (
	"fmt"
	"github.com/JetBrains/qodana-cli/v2024/platform"
	"github.com/spf13/pflag"
	"strconv"
	"strings"
)

type LocalOptions struct {
	*platform.QodanaOptions
}

type CltOptions struct {
	Solution      string
	Project       string
	Configuration string
	Platform      string
	NoBuild       bool
	MountInfo     *platform.MountInfo
	LinterInfo    *platform.LinterInfo
}

func (o *CltOptions) GetMountInfo() *platform.MountInfo {
	if o.MountInfo == nil {
		o.MountInfo = &platform.MountInfo{}
		o.MountInfo.CustomTools = make(map[string]string)
	}
	return o.MountInfo
}

func (o *CltOptions) GetInfo(_ *platform.QodanaOptions) *platform.LinterInfo {
	// todo: vary by release
	return o.LinterInfo
}

func (o *CltOptions) AddFlags(flags *pflag.FlagSet) {
	flags.StringVar(&o.Solution, "solution", "", "Relative path to solution file")
	flags.StringVar(&o.Project, "project", "", "Relative path to project file")
	flags.StringVar(&o.Configuration, "configuration", "", "Build configuration")
	flags.StringVar(&o.Platform, "platform", "", "Build platform")
	flags.BoolVar(&o.NoBuild, "no-build", false, "Do not build the project before analysis")
}

func (o *LocalOptions) GetCltOptions() *CltOptions {
	if v, ok := o.LinterSpecific.(*CltOptions); ok {
		return v
	}
	return &CltOptions{}
}

func (o *CltOptions) computeCdnetArgs(opts *platform.QodanaOptions, options *LocalOptions, yaml platform.QodanaYaml) ([]string, error) {
	target := getSolutionOrProject(options, yaml)
	if target == "" {
		return nil, fmt.Errorf("solution/project relative file path is not specified. Use --solution or --project flags or create qodana.yaml file with respective fields")
	}
	var props = ""
	for _, p := range opts.Property {
		if strings.HasPrefix(p, "log.") ||
			strings.HasPrefix(p, "idea.") ||
			strings.HasPrefix(p, "qodana.") ||
			strings.HasPrefix(p, "jetbrains.") {
			continue
		}
		if props != "" {
			props += ";"
		}
		props += p
	}
	if options.GetCltOptions().Configuration != "" {
		if props != "" {
			props += ";"
		}
		props += "Configuration=" + options.GetCltOptions().Configuration
	} else if yaml.DotNet.Configuration != "" {
		if props != "" {
			props += ";"
		}
		props += "Configuration=" + yaml.DotNet.Configuration
	}
	if options.GetCltOptions().Platform != "" {
		if props != "" {
			props += ";"
		}
		props += "Platform=" + options.GetCltOptions().Platform
	} else if yaml.DotNet.Platform != "" {
		if props != "" {
			props += ";"
		}
		props += "Platform=" + yaml.DotNet.Platform
	}
	if options.FailThreshold == "" && yaml.FailThreshold != nil {
		options.FailThreshold = strconv.Itoa(*yaml.FailThreshold)
	}
	mountInfo := o.GetMountInfo()
	if mountInfo == nil {
		return nil, fmt.Errorf("mount info is not set")
	}

	args := []string{
		"dotnet",
		platform.QuoteForWindows(mountInfo.CustomTools["clt"]),
		"inspectcode",
		platform.QuoteForWindows(target),
		"-o=\"" + options.GetSarifPath() + "\"",
		"-f=\"Qodana\"",
		"--LogFolder=\"" + options.LogDirPath() + "\"",
	}
	if props != "" {
		args = append(args, "--properties:"+props)
	}
	if options.NoStatistics {
		args = append(args, "--telemetry-optout")
	}
	if options.GetCltOptions().NoBuild {
		args = append(args, "--no-build")
	}
	return args, nil
}

func getSolutionOrProject(options *LocalOptions, yaml platform.QodanaYaml) string {
	var target = ""
	paths := [4]string{options.GetCltOptions().Solution, options.GetCltOptions().Project, yaml.DotNet.Solution, yaml.DotNet.Project}
	for _, path := range paths {
		if path != "" {
			target = path
			break
		}
	}
	return target
}
