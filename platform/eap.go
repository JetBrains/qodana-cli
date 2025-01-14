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

	//golang:noinspection GoDfaErrorMayBeNotNil
	deadline := buildDate.AddDate(0, 0, 60)
	now := time.Now()
	if now.After(deadline) {
		if IsContainer() {
			log.Fatal("EAP license of this Qodana image is expired. Please use \"docker pull\" to update image.")
		}
		log.Fatalf("EAP license of this Qodana linter is expired. Obtain the new one with the latest version of Qodana CLI.")
	} else {
		date := deadline.Format("January 02, 2006")
		if IsContainer() {
			println(fmt.Sprintf("\nBy using this Docker image, you agree to"+
				"\n- JetBrains Privacy Policy (https://jb.gg/jetbrains-privacy-policy)"+
				"\n- JETBRAINS EAP USER AGREEMENT (https://jb.gg/jetbrains-user-eap)"+
				"\n"+
				"\nThe Docker image includes an evaluation license."+
				"\nThe license will expire on %s."+
				"\nPlease ensure you pull a new image on time.", date))
		} else {
			println(fmt.Sprintf("\nBy using this linter, you agree to"+
				"\n- JetBrains Privacy Policy (https://jb.gg/jetbrains-privacy-policy)"+
				"\n- JETBRAINS EAP USER AGREEMENT (https://jb.gg/jetbrains-user-eap)"+
				"\n"+
				"\nThe linter includes an evaluation license."+
				"\nThe license will expire on %s."+
				"\nPlease ensure you obtain a new version on time.", date))
		}
	}
}
