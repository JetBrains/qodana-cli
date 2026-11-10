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

package tooling

import (
	"crypto/sha256"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGetQodanaJBRPath(t *testing.T) {
	// Get the JBR path
	javaPath := GetQodanaJBRPath(t.TempDir())

	if javaPath == "" {
		t.Fatal("Java path is empty")
	}

	t.Logf("Java executable path: %s", javaPath)

	// Execute java -version
	cmd := exec.Command(javaPath, "-version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to execute java -version: %v\nOutput: %s", err, string(output))
	}

	t.Logf("Java version output:\n%s", string(output))

	// Verify output contains expected content
	outputStr := string(output)

	if !strings.Contains(outputStr, "JDK Runtime Environment") {
		t.Error(`expected output to contain "JDK Runtime Environment"`)
	}

	if len(outputStr) == 0 {
		t.Error("java -version produced no output")
	}
}

func TestBundledJBRTrustStore(t *testing.T) {
	javaPath, err := computeQodanaJbrExecutablePath(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	javaHome := filepath.Dir(filepath.Dir(javaPath))
	keytool := filepath.Join(javaHome, "bin", "keytool")
	if runtime.GOOS == "windows" {
		keytool += ".exe"
	}
	output, err := exec.Command(keytool, "-list", "-rfc", "-keystore",
		filepath.Join(javaHome, "lib", "security", "cacerts"), "-storepass", "changeit").CombinedOutput()
	if err != nil {
		t.Fatalf("cannot read bundled cacerts: %v\n%s", err, output)
	}
	count := 0
	for {
		block, rest := pem.Decode(output)
		if block == nil {
			break
		}
		output = rest
		if block.Type != "CERTIFICATE" {
			continue
		}
		count++
		fingerprint := fmt.Sprintf("%X", sha256.Sum256(block.Bytes))
		if fingerprint == "86A1ECBA089C4A8D3BBE2734C612BA341D813E043CF9E8A862CD5C57A36BBE6B" ||
			fingerprint == "D7A7A0FB5D7E2731D771E9484EBCDEF71D5F0C3E0A2948782BC83EE0EA699EF4" {
			t.Errorf("blocked root present in bundled cacerts: %s", fingerprint)
		}
	}
	if count == 0 {
		t.Fatal("no certificates read from bundled cacerts")
	}
	t.Logf("Checked %d bundled root certificates", count)
	blocklist, err := os.ReadFile(filepath.Join(javaHome, "lib", "security", "blocked.certs"))
	if err != nil {
		t.Fatal(err)
	}
	lines := "\n" + strings.ToUpper(strings.ReplaceAll(string(blocklist), "\r\n", "\n")) + "\n"
	for _, fingerprint := range []string{
		"42C4ACCD12C4F59597925E1A08A7A90102608543809308D50EC427E118BA1493",
		"45A883F349AEDFD5F3A649B5498FFB121712BA8B448F3F2A91DBC5A766FCDDBE",
		"98B896625C0C230ADEEC3EB2CF6AAD5001ECF9156A68644E82374CE5833BC25B",
		"5F1DED4DD590AF74631290CF073B122A2AAC6A1FA479778A0BD43579BD832699",
		"E7568FE6E12331F4FD7DE61DA4C7559D0F8EB192A218D4741A3BA75184333838",
		"579480AEA166290BD3C643589E795FC0D0E54E86505D180F1DFF99EFD3199512",
		"0164D7CEAE5BB909C272AF151FF44B1177AE9B47D9424B94F1D9558C30934D19",
		"62B7F9868263F72F6911FC78CEAF72C1979158B7368BF648683BB2AA39B02E72",
		"CB6E15165AC802E9B3F993F16C25D41F4E81C21EBDE71AE1B012AC00BF9C7C3F",
		"7485B51C0ED192878FA1D2F948FB529AE1842CC5086C4B67F7BE5B2039DE9489",
		"432EC25F0D28DE9006D97BF752F17BBB2CD41F669933FCB072D9A1148D0261F3",
		"FD9D5C2DBBD4995AB69B8D50C439D9527B00529199C2FD01B2174EB58F07021F",
		"9E39DE2881273ACC225A149C72BA3E0F51424963669AB633EE1DC7DF5DB067B4",
		"A496E23AC0CAB8155FC9B117B97EAE368EFCBB9041AEC8804D5A308D2D01C2D8",
		"4B2460A466F95FD40113BCDCCA2CEB29B3B093921DE45474DA7DF48874C630B1",
		"84112972B8E59D14560AE7E7119654474AAD119990CF3722BFC1E3B91615E1FD",
	} {
		if !strings.Contains(lines, "\n"+fingerprint+"\n") {
			t.Errorf("compromised intermediate not blocked: %s", fingerprint)
		}
	}
}
