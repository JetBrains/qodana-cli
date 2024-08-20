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

package core

import (
	"github.com/JetBrains/qodana-cli/v2024/platform"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestQodanaOptions_determineRunScenario(t *testing.T) {
	type args struct {
		qodanaOptions *platform.QodanaOptions
		productCode   string
		hasStartHash  bool
	}
	tests := []struct {
		name string
		args args
		want RunScenario
	}{
		{
			name: "full history for .NET",
			args: args{
				qodanaOptions: &platform.QodanaOptions{
					FullHistory: true,
				},
				hasStartHash: true,
				productCode:  platform.QDNET,
			},
			want: runScenarioFullHistory,
		},
		{
			name: "full history for not .NET",
			args: args{
				qodanaOptions: &platform.QodanaOptions{
					FullHistory: true,
				},
				hasStartHash: true,
				productCode:  platform.QDJVM,
			},
			want: runScenarioFullHistory,
		},
		{
			name: "default .NET",
			args: args{
				qodanaOptions: &platform.QodanaOptions{},
				hasStartHash:  false,
				productCode:   platform.QDNET,
			},
			want: runScenarioDefault,
		},
		{
			name: "default not .NET",
			args: args{
				qodanaOptions: &platform.QodanaOptions{},
				hasStartHash:  false,
				productCode:   platform.QDJVM,
			},
			want: runScenarioDefault,
		},
		{
			name: "with start hash .NET",
			args: args{
				qodanaOptions: &platform.QodanaOptions{},
				hasStartHash:  true,
				productCode:   platform.QDNET,
			},
			want: runScenarioScoped,
		},
		{
			name: "with start hash not .NET",
			args: args{
				qodanaOptions: &platform.QodanaOptions{},
				hasStartHash:  true,
				productCode:   platform.QDJVM,
			},
			want: runScenarioScoped,
		},
		{
			name: "forced script",
			args: args{
				qodanaOptions: &platform.QodanaOptions{
					ForceLocalChangesScript: true,
				},
				hasStartHash: true,
				productCode:  platform.QDJVM,
			},
			want: runScenarioLocalChanges,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := Prod.Code
			Prod.Code = tt.args.productCode
			defer func() { Prod.Code = code }()
			o := QodanaOptions{QodanaOptions: tt.args.qodanaOptions}
			assert.Equalf(t, tt.want, o.determineRunScenario(tt.args.hasStartHash), "determineRunScenario(%v)", tt.args.hasStartHash)
		})
	}
}
