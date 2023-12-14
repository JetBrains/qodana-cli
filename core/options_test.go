package core

import (
	"testing"
)

func TestQodanaOptions_guessProduct(t *testing.T) {
	tests := []struct {
		name     string
		ide      string
		linter   string
		expected string
	}{
		{"IDE defined", "QDNET", "", "QDNET"},
		{"IDE defined with EapSuffix", "QDNET-EAP", "", "QDNET"},
		{"IDE defined not in Products", "NEVERGONNAGIVEYOUUP", "", ""},
		{"Linter defined", "", "jetbrains/qodana-dotnet:2023.3-eap", "QDNET"},
		{"TC defined", "", "registry.jetbrains.team/p/sa/containers/qodana-php:2023.3-rc", "QDPHP"},
		{"Both defined", "QDNET", "jetbrains/qodana-php:2023.3-eap", "QDNET"},
		{"Unknown linter defined", "", "jetbrains/qodana-unknown:2023.3-eap", ""},
		{"None defined", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := QodanaOptions{
				Ide:    tt.ide,
				Linter: tt.linter,
			}
			if got := opts.guessProduct(); got != tt.expected {
				t.Errorf("QodanaOptions.guessProduct() = %v, want %v", got, tt.expected)
			}
		})
	}
}
