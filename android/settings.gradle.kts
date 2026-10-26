// Top-level build file for the hlops Companion APK (issue #37 / CI in #39).
// The app talks to Termux only through the `hlops bridge` JSON contract
// (docs/companion-app-bridge.md) — no backup logic lives here.
pluginManagement {
    // AGP is published on Google's Maven, not the Gradle Plugin Portal,
    // so the plugin resolution repositories must be configured here.
    repositories {
        google()
        mavenCentral()
        gradlePluginPortal()
    }
}

plugins {
    id("com.android.application") version "8.5.2" apply false
    id("org.jetbrains.kotlin.android") version "1.9.23" apply false // pairs with Compose Compiler 1.5.13
}

dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories {
        google()
        mavenCentral()
    }
}

rootProject.name = "hlops-companion"
include(":app")
