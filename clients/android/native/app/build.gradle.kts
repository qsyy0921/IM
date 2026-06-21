plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

val npmExecutable = if (System.getProperty("os.name").lowercase().contains("windows")) "npm.cmd" else "npm"

android {
    namespace = "com.nexusim.android"
    compileSdk = 35

    defaultConfig {
        applicationId = "com.nexusim.android"
        minSdk = 26
        targetSdk = 35
        versionCode = 1
        versionName = "0.1.0"
    }
}

dependencies {
    implementation("androidx.webkit:webkit:1.12.1")
}

tasks.register<Exec>("prepareNexusIMWebAssets") {
    workingDir = file("../../..")
    commandLine(npmExecutable, "run", "build:shell-assets:android")
}

tasks.named("preBuild") {
    dependsOn("prepareNexusIMWebAssets")
}
