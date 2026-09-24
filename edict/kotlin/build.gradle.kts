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
    testImplementation(kotlin("test-junit5"))
    testRuntimeOnly("org.junit.platform:junit-platform-launcher")
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
        "META-INF/versions/**/module-info.class"
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
    if (!excludeIntegrationTests) {
        // Clone/runtime/provider state is external to Gradle's input snapshot.
        outputs.upToDateWhen { false }
        testLogging { showStandardStreams = true }
    }
    dependsOn(tasks.installDist)
    systemProperty("edict.executable", layout.buildDirectory.file("install/edict/bin/edict").get().asFile.absolutePath)
    systemProperty("edict.integrationOutput", layout.projectDirectory.dir("out/integration").asFile.absolutePath)
}
