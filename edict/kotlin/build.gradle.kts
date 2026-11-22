import java.util.Properties

plugins {
    kotlin("jvm") version "2.3.21"
    kotlin("plugin.serialization") version "2.3.21"
    application
}

group = "org.jetbrains.qodana"
version = "1.0.0"

allprojects {
    repositories {
        mavenCentral()
        maven("https://packages.jetbrains.team/maven/p/ij/intellij-dependencies") {
            content { includeGroup("org.jetbrains.qodana"); includeGroup("com.jetbrains.qodana") }
        }
    }
}

dependencies {
    implementation("org.jetbrains.qodana:publisher:3.1.14")
    implementation("org.jetbrains.qodana:qodana-cloud-kotlin-client:0.2.17")
    implementation("com.jetbrains.qodana:qodana-sarif:1.0.57")
    implementation("org.jetbrains.kotlinx:kotlinx-serialization-json:1.10.0")
    implementation("org.tomlj:tomlj:1.3.1")
    implementation("io.github.oshai:kotlin-logging-jvm:8.0.7")
    runtimeOnly("ch.qos.logback:logback-classic:1.6.6")
    implementation("org.yaml:snakeyaml:2.7")
    implementation("org.eclipse.jgit:org.eclipse.jgit:7.8.1.202611041320-r")
    testImplementation(kotlin("test-junit5"))
    testImplementation(kotlin("reflect"))
    testImplementation("org.junit-pioneer:junit-pioneer:2.3.0")
    testImplementation("org.junit.jupiter:junit-jupiter-params")
    testRuntimeOnly("org.junit.platform:junit-platform-launcher")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-core:1.11.0")
    implementation("io.modelcontextprotocol:kotlin-sdk-server:0.15.0")
    implementation("io.ktor:ktor-server-cio:3.6.1")
    implementation("ai.djl:api:0.38.0")
    implementation("com.microsoft.onnxruntime:onnxruntime:1.22.0")
}

kotlin { jvmToolchain(21) }
kotlin.sourceSets.test { kotlin.srcDir("src/integrationTest/kotlin") }
application { mainClass = "org.jetbrains.qodana.edict.MainKt" }

// The Go CLI embeds this self-contained executable alongside its other Java tools.
val bundledJar by tasks.registering(Jar::class) {
    archiveFileName = "edict-cli.jar"
    duplicatesStrategy = DuplicatesStrategy.EXCLUDE
    isPreserveFileTimestamps = false
    isReproducibleFileOrder = true
    manifest { attributes("Main-Class" to application.mainClass.get(), "Multi-Release" to "true") }
    from(sourceSets.main.get().output)
    from(configurations.runtimeClasspath.map { files -> files.map { if (it.isDirectory) it else zipTree(it) } })
    exclude(
        "META-INF/*.SF",
        "META-INF/*.RSA",
        "META-INF/*.DSA",
        "module-info.class",
        "META-INF/versions/**/module-info.class",
        // ONNX Runtime ships native debug symbols; loading the libraries does not need them.
        "ai/onnxruntime/native/**/*.pdb",
        "ai/onnxruntime/native/**/*.dSYM/**",
    )
}

val qodanaCliRoot = layout.projectDirectory.dir("../..")
val qodanaCli = qodanaCliRoot.file(if (System.getProperty("os.name").startsWith("Windows")) "qodana.exe" else "qodana")

val prepareQodanaCli by tasks.registering(Exec::class) {
    dependsOn(bundledJar)
    workingDir(qodanaCliRoot.asFile)
    commandLine("go", "generate", "./internal/tooling/...")
    // Reuse this build's JAR instead of starting another Gradle build in the same checkout.
    environment("QODANA_EDICT_BUNDLED_JAR", bundledJar.get().archiveFile.get().asFile.absolutePath)
}

val assembleQodanaCli by tasks.registering(Exec::class) {
    group = "build"
    description = "Assemble the parent Qodana CLI for Edict live tests"
    dependsOn(prepareQodanaCli)
    workingDir(qodanaCliRoot.asFile)
    commandLine("go", "build", "-o", qodanaCli.asFile.absolutePath, "./cli")
}

val managedSkills = layout.projectDirectory.dir("src/main/resources/skills")
val skillIndex = layout.buildDirectory.file("generated/skill-index/index.txt")
val generateSkillIndex by tasks.registering {
    inputs.dir(managedSkills)
    outputs.file(skillIndex)
    doLast {
        val entries = managedSkills.asFileTree.files
            .map { it.relativeTo(managedSkills.asFile).invariantSeparatorsPath }
            .sorted()
        skillIndex.get().asFile.apply {
            parentFile.mkdirs()
            writeText(entries.joinToString("\n", postfix = "\n"))
        }
    }
}

tasks.processResources {
    from(generateSkillIndex) { into("skills") }
}

tasks.test {
    val excludeIntegrationTests =
        providers.gradleProperty("excludeIntegrationTests").map(String::toBoolean).getOrElse(false)
    inputs.property("excludeIntegrationTests", excludeIntegrationTests)
    useJUnitPlatform { if (excludeIntegrationTests) excludeTags("integration") }
    // JUnit Pioneer's @SetEnvironmentVariable rewrites the JDK's environment map reflectively.
    jvmArgs("--add-opens", "java.base/java.util=ALL-UNNAMED", "--add-opens", "java.base/java.lang=ALL-UNNAMED")
    // Tests answer Edict's trust check with a fake; live runs and the trust contract test use the real Codex.
    environment("CODEX_BIN", layout.projectDirectory.file("src/test/fake-codex/codex").asFile.absolutePath)
    environment("EDICT_REAL_CODEX", System.getenv("CODEX_BIN") ?: "codex")
    if (!excludeIntegrationTests) {
        dependsOn(assembleQodanaCli)
        systemProperty("edict.qodanaCli", qodanaCli.asFile.absolutePath)
        val linterFeed = providers.environmentVariable("QD_PRODUCT_INTERNAL_FEED")
            .getOrElse("https://packages.jetbrains.team/files/p/sa/qdist")
        environment("QD_PRODUCT_INTERNAL_FEED", linterFeed)
        val linterVersion = providers.gradleProperty("qodanaLinterVersion")
            .orElse(providers.environmentVariable("QD_PRODUCT_INTERNAL_VERSION"))
            .getOrElse("2027.1")
        environment("QD_PRODUCT_INTERNAL_VERSION", linterVersion)
        val localPropertiesFile = layout.projectDirectory.file("local.properties").asFile
        doFirst {
            val authVariable = "QD_PRODUCT_INTERNAL_AUTH"
            if (System.getenv(authVariable).isNullOrBlank() && localPropertiesFile.isFile) {
                val properties = Properties().apply { localPropertiesFile.inputStream().use(::load) }
                properties.getProperty(authVariable)?.trim()?.takeIf(String::isNotEmpty)?.let {
                    (this as Test).environment(authVariable, it)
                }
            }
        }
        // Clone/runtime/provider state is external to Gradle's input snapshot.
        outputs.upToDateWhen { false }
        testLogging { showStandardStreams = true }
    }
    dependsOn(tasks.installDist)
    systemProperty("edict.executable", layout.buildDirectory.file("install/edict/bin/edict").get().asFile.absolutePath)
    systemProperty("edict.integrationOutput", layout.projectDirectory.dir("out/integration").asFile.absolutePath)
}
