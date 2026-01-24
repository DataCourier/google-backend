plugins {
    kotlin("multiplatform") version "1.9.22"
    kotlin("plugin.serialization") version "1.9.22"
    id("com.android.library") version "8.2.0"
    `maven-publish`
}

group = "com.example"
version = "0.1.0"

kotlin {
    // Android
    androidTarget {
        compilations.all {
            kotlinOptions {
                jvmTarget = "1.8"
            }
        }
        publishLibraryVariants("release", "debug")
    }

    // iOS
    iosX64()
    iosArm64()
    iosSimulatorArm64()

    // JVM (for tests)
    jvm()

    sourceSets {
        val commonMain by getting {
            dependencies {
                // Ktor client
                implementation("io.ktor:ktor-client-core:2.3.7")
                implementation("io.ktor:ktor-client-content-negotiation:2.3.7")
                implementation("io.ktor:ktor-serialization-kotlinx-json:2.3.7")

                // Kotlinx
                implementation("org.jetbrains.kotlinx:kotlinx-serialization-json:1.6.2")
                implementation("org.jetbrains.kotlinx:kotlinx-datetime:0.5.0")
                implementation("org.jetbrains.kotlinx:kotlinx-coroutines-core:1.7.3")

                // Reflection (for annotations)
                implementation(kotlin("reflect"))
            }
        }

        val commonTest by getting {
            dependencies {
                implementation(kotlin("test"))
                implementation("org.jetbrains.kotlinx:kotlinx-coroutines-test:1.7.3")
            }
        }

        val androidMain by getting {
            dependencies {
                implementation("io.ktor:ktor-client-android:2.3.7")
            }
        }

        val iosX64Main by getting
        val iosArm64Main by getting
        val iosSimulatorArm64Main by getting
        val iosMain by creating {
            dependsOn(commonMain)
            iosX64Main.dependsOn(this)
            iosArm64Main.dependsOn(this)
            iosSimulatorArm64Main.dependsOn(this)
            dependencies {
                implementation("io.ktor:ktor-client-darwin:2.3.7")
            }
        }

        val jvmMain by getting {
            dependencies {
                implementation("io.ktor:ktor-client-cio:2.3.7")
            }
        }

        val jvmTest by getting {
            dependencies {
                implementation("io.ktor:ktor-client-mock:2.3.7")
            }
        }
    }
}

android {
    namespace = "com.example.buckets"
    compileSdk = 34

    defaultConfig {
        minSdk = 24
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_1_8
        targetCompatibility = JavaVersion.VERSION_1_8
    }
}

// Publishing configuration
publishing {
    publications {
        // Publications are automatically created by the Kotlin Multiplatform plugin
    }

    repositories {
        maven {
            name = "local"
            url = uri("${rootProject.buildDir}/repo")
        }
    }
}
