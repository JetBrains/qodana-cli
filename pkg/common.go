package pkg

type LinterOptions struct {
	ReportPath  string
	CachePath   string
	ProjectPath string
}

func Contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}
