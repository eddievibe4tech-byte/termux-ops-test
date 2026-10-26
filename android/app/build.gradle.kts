plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

android {
    namespace = "dev.hlops.companion"
    compileSdk = 34

    defaultConfig {
        applicationId = "dev.hlops.companion"
        minSdk = 26          // Termux requires Android 7+; we target 8+ for notifications channels
        targetSdk = 34
        versionCode = 1
        versionName = "0.1.0"
    }

    // Release signing comes from CI secrets (issue #39): the keystore is
    // base64-decoded into KEYPATH by the android-ci workflow before gradle runs.
    // Without those env vars we fall back to the debug signing config (AGP 8.x
    // cannot assembleRelease unsigned), so local builds never need a checked-in
    // keystore.
    signingConfigs {
        create("ci") {
            val ksPath = System.getenv("KEYPATH")
            if (!ksPath.isNullOrEmpty()) {
                storeFile = file(ksPath)
                storePassword = System.getenv("KEYSTORE_PASSWORD")
                keyAlias = System.getenv("KEY_ALIAS") ?: "hlops"
                keyPassword = System.getenv("KEY_PASSWORD")
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            val ciConfig = signingConfigs.getByName("ci")
            // AGP 8.x refuses to assembleRelease without a signing config, so
            // fall back to the debug key when CI secrets are missing (local
            // builds / fork PRs) instead of producing an unsigned artifact.
            signingConfig = if (ciConfig.storeFile != null) ciConfig else signingConfigs.getByName("debug")
        }
    }

    lint {
        abortOnError = false     // warnings (hardcoded strings, translations) must not fail CI
        checkReleaseBuilds = false
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions {
        jvmTarget = "17"
    }

    buildFeatures {
        compose = true
    }
    composeOptions {
        kotlinCompilerExtensionVersion = "1.5.13" // matches Kotlin 1.9.23 (compatibility map)
    }
}

dependencies {
    val composeBom = platform("androidx.compose:compose-bom:2024.06.00")
    implementation(composeBom)
    implementation("androidx.activity:activity-compose:1.9.0")
    implementation("androidx.compose.material3:material3")
    implementation("androidx.compose.ui:ui")
    implementation("androidx.compose.ui:ui-tooling-preview")
    debugImplementation("androidx.compose.ui:ui-tooling")
    implementation("androidx.lifecycle:lifecycle-runtime-ktx:2.8.2")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.8.1")
    // Bridge JSON is parsed with org.json (built into Android) to keep the
    // dependency surface minimal — see docs/companion-app-bridge.md.
}
