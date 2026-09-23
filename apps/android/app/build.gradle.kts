plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.plugin.compose")
}

val releaseKeystore = rootProject.file("release-key.jks")
val releasePasswordFile = rootProject.file(".release-password")
val hasReleaseSigning = releaseKeystore.isFile && releasePasswordFile.isFile

android {
    namespace = "dev.bookharbor.app"
    compileSdk = 37

    defaultConfig {
        applicationId = "dev.bookharbor.app"
        minSdk = 26
        targetSdk = 36
        versionCode = 10
        versionName = "0.9.0"

        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    buildFeatures {
        compose = true
        buildConfig = false
    }

    signingConfigs {
        if (hasReleaseSigning) {
            create("release") {
                storeFile = releaseKeystore
                storePassword = releasePasswordFile.readText().trim()
                keyAlias = "bookharbor"
                keyPassword = storePassword
            }
        }
    }

    buildTypes {
        // Installs beside a release build, so testing never replaces (or wipes) the real app.
        debug {
            applicationIdSuffix = ".debug"
        }
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            if (hasReleaseSigning) {
                signingConfig = signingConfigs.getByName("release")
            }
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro",
            )
        }
        // Release-speed build (R8, not debuggable) signed with the debug key, installed beside the
        // real app: judge performance on this, never on a debug build.
        create("preview") {
            initWith(getByName("release"))
            applicationIdSuffix = ".preview"
            signingConfig = signingConfigs.getByName("debug")
            matchingFallbacks += listOf("release")
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
}

dependencies {
    val composeBom = platform("androidx.compose:compose-bom:2026.09.00")
    implementation(composeBom)
    androidTestImplementation(composeBom)

    implementation("androidx.activity:activity-compose:1.13.0")
    implementation("androidx.compose.material:material-icons-core")
    implementation("androidx.compose.material3:material3")
    implementation("androidx.compose.ui:ui")
    implementation("androidx.compose.ui:ui-tooling-preview")
    implementation("androidx.work:work-runtime-ktx:2.10.2")
    // Real-world EPUBs are frequently not well-formed XML (unclosed <br>/<img>, duplicate <html>
    // roots, etc.). Jsoup repairs that tag soup into well-formed markup before the strict XML
    // parser sees it; hand-rolling that repair with regexes is the fragile path, not the lazy one.
    implementation("org.jsoup:jsoup:1.18.1")

    debugImplementation("androidx.compose.ui:ui-tooling")
    debugImplementation("androidx.compose.ui:ui-test-manifest")

    testImplementation("junit:junit:4.13.2")
    // android.jar only ships org.json stubs; unit tests need the real implementation.
    testImplementation("org.json:json:20250517")
    androidTestImplementation("androidx.compose.ui:ui-test-junit4")
    androidTestImplementation("androidx.test.ext:junit:1.3.0")
}
