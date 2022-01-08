package pkg

type Linter string

type LinterOptions struct {
	ReportPath  string
	CachePath   string
	ProjectPath string
}

func NewLinterOptions() *LinterOptions {
	return &LinterOptions{}
}
