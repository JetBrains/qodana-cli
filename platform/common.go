package platform

import (
	"fmt"
)

// QodanaLogo prepares the info message for the tool
func QodanaLogo(toolDesc string, version string) string {
	return fmt.Sprintf(`
          _              _
         /\ \           /\ \        %s %s
        /  \ \         /  \ \       Documentation
       / /\ \ \       / /\ \ \      https://jb.gg/qodana-docs
      / / /\ \ \     / / /\ \ \     Contact us at
     / / /  \ \_\   / / /  \ \_\    qodana-support@jetbrains.com
    / / / _ / / /  / / /   / / /    Or via our issue tracker
   / / / /\ \/ /  / / /   / / /     https://jb.gg/qodana-issue
  / / /__\ \ \/  / / /___/ / /      Or share your feedback at our forum
 / / /____\ \ \ / / /____\/ /       https://jb.gg/qodana-forum
 \/________\_\/ \/_________/

`, toolDesc, version)
}
