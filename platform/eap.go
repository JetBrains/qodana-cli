package platform

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"time"
)

func CheckEAP(buildDateStr string, isEap bool) {
	if !isEap {
		return
	}
	buildDate, err := time.Parse(time.RFC3339, buildDateStr)
	if err != nil {
		log.Fatal("Failed to parse build date")
	}

	deadline := buildDate.AddDate(0, 0, 60)
	now := time.Now()
	if now.After(deadline) {
		log.Fatal("Current date is two months after the build date. Exiting...")
	} else {
		if IsContainer() {
			println(fmt.Sprintf("\nBy using this Docker image, you agree to"+
				"\n- JetBrains Privacy Policy (https://jb.gg/jetbrains-privacy-policy)"+
				"\n- JETBRAINS EAP USER AGREEMENT (https://jb.gg/jetbrains-user-eap)"+
				"\n"+
				"\nThe Docker image includes an evaluation license."+
				"\nThe license will expire on %s."+
				"\nPlease ensure you pull a new image on time.", deadline.Format("2006-01-02")))
		} else {
			println(fmt.Sprintf("\nBy using this linter, you agree to"+
				"\n- JetBrains Privacy Policy (https://jb.gg/jetbrains-privacy-policy)"+
				"\n- JETBRAINS EAP USER AGREEMENT (https://jb.gg/jetbrains-user-eap)"+
				"\n"+
				"\nThe linter includes an evaluation license."+
				"\nThe license will expire on %s."+
				"\nPlease ensure you obtain a new version on time.", deadline.Format("2006-01-02")))
		}
	}
}
