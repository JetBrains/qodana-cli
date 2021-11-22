package pkg

type Linter string

// TODO: Update list of linters
const (
	JVM        Linter = "jvm"
	Python            = "py"
	JavaScript        = "js"
	Go                = "go"
	PHP               = "php"
)
