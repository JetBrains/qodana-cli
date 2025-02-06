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
	"github.com/JetBrains/qodana-cli/v2024/core/corescan"
	"github.com/JetBrains/qodana-cli/v2024/platform/product"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestQodanaOptions_determineRunScenario(t *testing.T) {
	type args struct {
		c            corescan.ContextBuilder
		hasStartHash bool
	}
	tests := []struct {
		name string
		args args
		want corescan.RunScenario
	}{
		{
			name: "full history for .NET",
			args: args{
				c: corescan.ContextBuilder{
					FullHistory: true,
					Prod: product.Product{
						Code: product.QDNET,
					},
				},
				hasStartHash: true,
			},
			want: corescan.RunScenarioFullHistory,
		},
		{
			name: "full history for not .NET",
			args: args{
				c: corescan.ContextBuilder{
					FullHistory: true,
					Prod: product.Product{
						Code: product.QDJVM,
					},
				},
				hasStartHash: true,
			},
			want: corescan.RunScenarioFullHistory,
		},
		{
			name: "default .NET",
			args: args{
				c: corescan.ContextBuilder{
					Prod: product.Product{
						Code: product.QDNET,
					},
				},
				hasStartHash: false,
			},
			want: corescan.RunScenarioDefault,
		},
		{
			name: "default not .NET",
			args: args{
				c: corescan.ContextBuilder{
					Prod: product.Product{
						Code: product.QDJVM,
					},
				},
				hasStartHash: false,
			},
			want: corescan.RunScenarioDefault,
		},
		{
			name: "with start hash .NET",
			args: args{
				c: corescan.ContextBuilder{
					Prod: product.Product{
						Code: product.QDNET,
					},
				},
				hasStartHash: true,
			},
			want: corescan.RunScenarioScoped,
		},
		{
			name: "with start hash not .NET",
			args: args{
				c: corescan.ContextBuilder{
					Prod: product.Product{
						Code: product.QDJVM,
					},
				},
				hasStartHash: true,
			},
			want: corescan.RunScenarioScoped,
		},
		{
			name: "forced script",
			args: args{
				c: corescan.ContextBuilder{
					ForceLocalChangesScript: true,
					Prod: product.Product{
						Code: product.QDJVM,
					},
				},
				hasStartHash: true,
			},
			want: corescan.RunScenarioLocalChanges,
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				assert.Equalf(
					t,
					tt.want,
					tt.args.c.Build().DetermineRunScenario(tt.args.hasStartHash),
					"determineRunScenario(%v)",
					tt.args.hasStartHash,
				)
			},
		)
	}
}
