plugins {
    kotlin("jvm")
    kotlin("plugin.serialization")
    application
}

repositories { mavenCentral() }

dependencies {
    implementation("org.jetbrains.kotlinx:kotlinx-serialization-json:1.10.0")
    testImplementation(kotlin("test-junit5"))
    testRuntimeOnly("org.junit.platform:junit-platform-launcher")
}

kotlin { jvmToolchain(21) }
application { mainClass = "org.jetbrains.qodana.edict.benchmark.MainKt" }
tasks.test { useJUnitPlatform() }

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
