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
	"github.com/JetBrains/qodana-cli/v2024/platform/scan"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestQodanaOptions_determineRunScenario(t *testing.T) {
	type args struct {
		c            scan.Context
		hasStartHash bool
	}
	tests := []struct {
		name string
		args args
		want scan.RunScenario
	}{
		{
			name: "full history for .NET",
			args: args{
				c: scan.Context{
					FullHistory: true,
					Prod: product.Product{
						Code: platform.QDNET,
					},
				},
				hasStartHash: true,
			},
			want: scan.RunScenarioFullHistory,
		},
		{
			name: "full history for not .NET",
			args: args{
				c: scan.Context{
					FullHistory: true,
					Prod: product.Product{
						Code: platform.QDJVM,
					},
				},
				hasStartHash: true,
			},
			want: scan.RunScenarioFullHistory,
		},
		{
			name: "default .NET",
			args: args{
				c: scan.Context{
					Prod: product.Product{
						Code: platform.QDNET,
					},
				},
				hasStartHash: false,
			},
			want: scan.RunScenarioDefault,
		},
		{
			name: "default not .NET",
			args: args{
				c: scan.Context{
					Prod: product.Product{
						Code: platform.QDJVM,
					},
				},
				hasStartHash: false,
			},
			want: scan.RunScenarioDefault,
		},
		{
			name: "with start hash .NET",
			args: args{
				c: scan.Context{
					Prod: product.Product{
						Code: platform.QDNET,
					},
				},
				hasStartHash: true,
			},
			want: scan.RunScenarioScoped,
		},
		{
			name: "with start hash not .NET",
			args: args{
				c: scan.Context{
					Prod: product.Product{
						Code: platform.QDJVM,
					},
				},
				hasStartHash: false,
			},
			want: scan.RunScenarioScoped,
		},
		{
			name: "forced script",
			args: args{
				c: scan.Context{
					ForceLocalChangesScript: true,
					Prod: product.Product{
						Code: platform.QDJVM,
					},
				},
				hasStartHash: true,
			},
			want: scan.RunScenarioLocalChanges,
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				assert.Equalf(
					t,
					tt.want,
					tt.args.c.DetermineRunScenario(tt.args.hasStartHash),
					"determineRunScenario(%v)",
					tt.args.hasStartHash,
				)
			},
		)
	}
}
