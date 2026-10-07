plugins {
    kotlin("jvm") version "2.3.20"
    kotlin("plugin.serialization") version "2.3.20"
    application
}

group = "org.jetbrains.qodana"
version = "1.0.0"

repositories { mavenCentral() }

dependencies {
    implementation("org.jetbrains.kotlinx:kotlinx-serialization-json:1.10.0")
    implementation("org.tomlj:tomlj:1.1.1")
    implementation("io.github.oshai:kotlin-logging-jvm:8.0.4")
    runtimeOnly("ch.qos.logback:logback-classic:1.6.3")
    implementation("org.yaml:snakeyaml:2.4")
    testImplementation(kotlin("test-junit5"))
    testImplementation(kotlin("reflect"))
    testImplementation("org.junit-pioneer:junit-pioneer:2.3.0")
    testRuntimeOnly("org.junit.platform:junit-platform-launcher")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-core:1.10.2")
    implementation("io.modelcontextprotocol:kotlin-sdk-server:0.15.0")
    implementation("io.ktor:ktor-server-cio:3.5.1")
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
        // Clone/runtime/provider state is external to Gradle's input snapshot.
        outputs.upToDateWhen { false }
        testLogging { showStandardStreams = true }
    }
    dependsOn(tasks.installDist)
    systemProperty("edict.executable", layout.buildDirectory.file("install/edict/bin/edict").get().asFile.absolutePath)
    systemProperty("edict.integrationOutput", layout.projectDirectory.dir("out/integration").asFile.absolutePath)
}
