plugins {
    kotlin("jvm")
    kotlin("plugin.serialization")
    application
}

repositories { mavenCentral() }

dependencies {
    implementation(project(":"))
    implementation("org.jetbrains.kotlinx:kotlinx-serialization-json:1.10.0")
    testImplementation(kotlin("test-junit5"))
    testRuntimeOnly("org.junit.platform:junit-platform-launcher")
}

kotlin { jvmToolchain(21) }
application { mainClass = "org.jetbrains.qodana.edict.benchmark.MainKt" }
tasks.test { useJUnitPlatform() }

tasks.register<Jar>("runnerJar") {
    archiveFileName = "benchmark-runner.jar"
    duplicatesStrategy = DuplicatesStrategy.EXCLUDE
    isPreserveFileTimestamps = false
    isReproducibleFileOrder = true
    manifest { attributes("Main-Class" to "org.jetbrains.qodana.edict.benchmark.GenerationKt", "Multi-Release" to "true") }
    from(sourceSets.main.get().output)
    from(configurations.runtimeClasspath.map { files -> files.map { if (it.isDirectory) it else zipTree(it) } })
    exclude("META-INF/*.SF", "META-INF/*.RSA", "META-INF/*.DSA", "module-info.class", "META-INF/versions/**/module-info.class")
}

tasks.register<JavaExec>("configureTeamCity") {
    group = "build setup"
    classpath = sourceSets.main.get().runtimeClasspath
    mainClass = "org.jetbrains.qodana.edict.benchmark.ConfigureTeamCityKt"
    args(rootProject.projectDir.resolve("../../scripts/edict-benchmark").absolutePath)
    doFirst { args(providers.gradleProperty("benchmarkRevision").get()) }
}

// This task only consumes completed generation artifacts; it never starts an agent or IDE.
tasks.register<JavaExec>("compare") {
    group = "verification"
    description = "Compare generated inspection SARIF with benchmark specifications and gold SARIF"
    classpath = sourceSets.main.get().runtimeClasspath
    mainClass = application.mainClass
    doFirst {
        fun required(name: String) = providers.gradleProperty(name).orNull
            ?: error("Supply -P$name=<path>")
        val generation = required("generationDir")
        args("--benchmark-dir", required("benchmarkDir"), "--generation-dir", generation,
             "--output-dir", providers.gradleProperty("benchmarkOutputDir").getOrElse(generation))
        providers.gradleProperty("analysisSarif").orNull?.let { args("--analysis-sarif", it) }
    }
}
