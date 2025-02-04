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

package startup

import (
	"fmt"
)

func jdkTableXml(jdkPath string) string {
	return fmt.Sprintf(`<application>
  <component name="ProjectJdkTable">
    <jdk version="2">
      <name value="11" />
      <type value="JavaSDK" />
      <version value="java version &quot;11&quot;" />
      <homePath value="%[1]v" />
      <roots>
        <annotationsPath>
          <root type="composite">
            <root url="jar://$APPLICATION_HOME_DIR$/plugins/java/lib/jdkAnnotations.jar!/" type="simple" />
          </root>
        </annotationsPath>
        <classPath>
          <root type="composite">
            <root url="jrt://%[1]v!/java.base" type="simple" />
            <root url="jrt://%[1]v!/java.compiler" type="simple" />
            <root url="jrt://%[1]v!/java.datatransfer" type="simple" />
            <root url="jrt://%[1]v!/java.desktop" type="simple" />
            <root url="jrt://%[1]v!/java.instrument" type="simple" />
            <root url="jrt://%[1]v!/java.logging" type="simple" />
            <root url="jrt://%[1]v!/java.management" type="simple" />
            <root url="jrt://%[1]v!/java.management.rmi" type="simple" />
            <root url="jrt://%[1]v!/java.naming" type="simple" />
            <root url="jrt://%[1]v!/java.net.http" type="simple" />
            <root url="jrt://%[1]v!/java.prefs" type="simple" />
            <root url="jrt://%[1]v!/java.rmi" type="simple" />
            <root url="jrt://%[1]v!/java.scripting" type="simple" />
            <root url="jrt://%[1]v!/java.se" type="simple" />
            <root url="jrt://%[1]v!/java.security.jgss" type="simple" />
            <root url="jrt://%[1]v!/java.security.sasl" type="simple" />
            <root url="jrt://%[1]v!/java.smartcardio" type="simple" />
            <root url="jrt://%[1]v!/java.sql" type="simple" />
            <root url="jrt://%[1]v!/java.sql.rowset" type="simple" />
            <root url="jrt://%[1]v!/java.transaction.xa" type="simple" />
            <root url="jrt://%[1]v!/java.xml" type="simple" />
            <root url="jrt://%[1]v!/java.xml.crypto" type="simple" />
            <root url="jrt://%[1]v!/jdk.accessibility" type="simple" />
            <root url="jrt://%[1]v!/jdk.aot" type="simple" />
            <root url="jrt://%[1]v!/jdk.attach" type="simple" />
            <root url="jrt://%[1]v!/jdk.charsets" type="simple" />
            <root url="jrt://%[1]v!/jdk.compiler" type="simple" />
            <root url="jrt://%[1]v!/jdk.crypto.cryptoki" type="simple" />
            <root url="jrt://%[1]v!/jdk.crypto.ec" type="simple" />
            <root url="jrt://%[1]v!/jdk.dynalink" type="simple" />
            <root url="jrt://%[1]v!/jdk.hotspot.agent" type="simple" />
            <root url="jrt://%[1]v!/jdk.httpserver" type="simple" />
            <root url="jrt://%[1]v!/jdk.internal.ed" type="simple" />
            <root url="jrt://%[1]v!/jdk.internal.jvmstat" type="simple" />
            <root url="jrt://%[1]v!/jdk.internal.le" type="simple" />
            <root url="jrt://%[1]v!/jdk.internal.opt" type="simple" />
            <root url="jrt://%[1]v!/jdk.internal.vm.ci" type="simple" />
            <root url="jrt://%[1]v!/jdk.internal.vm.compiler" type="simple" />
            <root url="jrt://%[1]v!/jdk.internal.vm.compiler.management" type="simple" />
            <root url="jrt://%[1]v!/jdk.jcmd" type="simple" />
            <root url="jrt://%[1]v!/jdk.jdi" type="simple" />
            <root url="jrt://%[1]v!/jdk.jdwp.agent" type="simple" />
            <root url="jrt://%[1]v!/jdk.jfr" type="simple" />
            <root url="jrt://%[1]v!/jdk.jsobject" type="simple" />
            <root url="jrt://%[1]v!/jdk.localedata" type="simple" />
            <root url="jrt://%[1]v!/jdk.management" type="simple" />
            <root url="jrt://%[1]v!/jdk.management.agent" type="simple" />
            <root url="jrt://%[1]v!/jdk.management.jfr" type="simple" />
            <root url="jrt://%[1]v!/jdk.naming.dns" type="simple" />
            <root url="jrt://%[1]v!/jdk.naming.rmi" type="simple" />
            <root url="jrt://%[1]v!/jdk.net" type="simple" />
            <root url="jrt://%[1]v!/jdk.pack" type="simple" />
            <root url="jrt://%[1]v!/jdk.scripting.nashorn" type="simple" />
            <root url="jrt://%[1]v!/jdk.scripting.nashorn.shell" type="simple" />
            <root url="jrt://%[1]v!/jdk.sctp" type="simple" />
            <root url="jrt://%[1]v!/jdk.security.auth" type="simple" />
            <root url="jrt://%[1]v!/jdk.security.jgss" type="simple" />
            <root url="jrt://%[1]v!/jdk.unsupported" type="simple" />
            <root url="jrt://%[1]v!/jdk.xml.dom" type="simple" />
            <root url="jrt://%[1]v!/jdk.zipfs" type="simple" />
          </root>
        </classPath>
        <javadocPath>
          <root type="composite" />
        </javadocPath>
      </roots>
      <additional />
    </jdk>
  </component>
</application>
`, jdkPath)
}

func androidProjectDefaultXml(androidSdkPath string) string {
	return fmt.Sprintf(`<application>
  <component name="ProjectManager">
    <defaultProject>
      <component name="PropertiesComponent">
        <property name="android.sdk.path" value="%s" />
      </component>
    </defaultProject>
  </component>
</application>`, androidSdkPath)
}

const securityXml = `<application>
    <component name="PasswordSafe">
        <option name="PROVIDER" value="KEEPASS" />
    </component>
</application>`

const mavenSettingsXml = `<settings>
    <localRepository>/data/cache/.m2</localRepository>
    <mirrors>
        <mirror>
            <id>cache-central</id>
            <name>Maven Repository Manager running on repo.mycompany.com</name>
            <url>https://cache-redirector.jetbrains.com/maven-central</url>
            <mirrorOf>central</mirrorOf>
        </mirror>
        <mirror>
            <id>cache-intellij-third-party-dependencies</id>
            <name>IntelliJ Dependencies on Bintray</name>
            <url>https://cache-redirector.jetbrains.com/intellij-third-party-dependencies</url>
            <mirrorOf>intellij-third-party-dependencies</mirrorOf>
        </mirror>
        <mirror>
            <id>cache-jcenter</id>
            <name>JCenter on Bintray</name>
            <url>https://cache-redirector.jetbrains.com/jcenter</url>
            <mirrorOf>jcenter</mirrorOf>
        </mirror>
        <mirror>
            <id>cache-groovy</id>
            <name>Groovy Bintray Repository</name>
            <url>https://cache-redirector.jetbrains.com/dl.bintray.com/groovy/maven/</url>
            <mirrorOf>groovy</mirrorOf>
        </mirror>
        <mirror>
            <id>cache-jitpack</id>
            <name>jitpack</name>
            <url>https://cache-redirector.jetbrains.com/jitpack.io</url>
            <mirrorOf>jitpack</mirrorOf>
        </mirror>
    </mirrors>
</settings>
`

const mavenPathMacroxXml = `<application>
    <component name="PathMacrosImpl">
        <macro name="MAVEN_REPOSITORY" value="/data/cache/.m2" />
    </component>
</application>`

const userPrefsXml = `?xml version="1.0" encoding="UTF-8" standalone="no"?>
<!DOCTYPE map SYSTEM "http://java.sun.com/dtd/preferences.dtd">
<map MAP_XML_VERSION="1.0">
</map>`
