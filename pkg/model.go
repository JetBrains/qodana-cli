package pkg

type Linter string

// TODO: Consider if needed
//const (
//	JVM        Linter = "jvm"
//	Python            = "py"
//	JavaScript        = "js"
//	Go                = "go"
//	PHP               = "php"
//)

type LinterOptions struct {
	ImageName   string
	ReportPath  string
	CachePath   string
	ProjectPath string
}

func NewLinterOptions() *LinterOptions {
	return &LinterOptions{}
}
