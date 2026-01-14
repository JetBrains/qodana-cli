package msg

import (
	"testing"

	"github.com/pterm/pterm"
	"github.com/stretchr/testify/assert"
)

func TestInfoString(t *testing.T) {
	result := InfoString("1.0.0")
	assert.Contains(t, result, "Qodana CLI")
	assert.Contains(t, result, "1.0.0")
	assert.Contains(t, result, "https://jb.gg/qodana-cli")
}

func TestPrimary(t *testing.T) {
	result := Primary("Hello %s", "World")
	assert.Equal(t, "Hello World", result)
}

func TestPrimaryBold(t *testing.T) {
	result := PrimaryBold("Hello %s", "World")
	assert.Contains(t, result, "Hello World")
}

func TestGetProblemsFoundMessage(t *testing.T) {
	tests := []struct {
		count    int
		contains string
	}{
		{0, "No new problems found"},
		{1, "Found 1 new problem"},
		{5, "Found 5 new problems"},
	}

	for _, tt := range tests {
		t.Run(tt.contains, func(t *testing.T) {
			result := GetProblemsFoundMessage(tt.count)
			assert.Contains(t, result, tt.contains)
		})
	}
}

func TestGetTerminalWidth(t *testing.T) {
	width := getTerminalWidth()
	assert.Greater(t, width, 0)
}

func TestFormatMessageForCI(t *testing.T) {
	result := formatMessageForCI("warning", "test %s", "message")
	assert.Contains(t, result, "test message")
}

func TestIsInteractive(t *testing.T) {
	_ = IsInteractive()
}

func TestDisableColor(t *testing.T) {
	DisableColor()
}

func TestEmptyMessage(t *testing.T) {
	EmptyMessage()
}

func TestSuccessMessage(t *testing.T) {
	SuccessMessage("test %s", "message")
}

func TestWarningMessage(t *testing.T) {
	WarningMessage("test %s", "warning")
}

func TestWarningMessageCI(t *testing.T) {
	WarningMessageCI("test %s", "warning")
}

func TestErrorMessage(t *testing.T) {
	ErrorMessage("test %s", "error")
}

func TestPrintLinterLog(t *testing.T) {
	PrintLinterLog("test log message")
}

func TestUpdateText(t *testing.T) {
	UpdateText(nil, "new text")
}

func TestPrintProcess(t *testing.T) {
	PrintProcess(func(spinner *pterm.SpinnerPrinter) {
		// test callback
	}, "starting", "finished")
}

func TestPrintProblem(t *testing.T) {
	PrintProblem("TestRule", "warning", "Test message", "test.go", 10, 5, 8, "context line")
}

func TestPrintHeader(t *testing.T) {
	printHeader("warning", "TestRule", "test.go")
}

func TestPrintPath(t *testing.T) {
	printPath("test.go", 10, 5)
}

func TestPrintLines(t *testing.T) {
	printLines("line1\nline2\nline3", 5, 7, false)
}

func TestSpin(t *testing.T) {
	err := spin(func(spinner *pterm.SpinnerPrinter) {
		// test
	}, "test message")
	assert.NoError(t, err)
}

func TestStartQodanaSpinner(t *testing.T) {
	spinner, err := StartQodanaSpinner("Loading...")
	assert.NoError(t, err)
	if spinner != nil {
		_ = spinner.Stop()
	}
}

