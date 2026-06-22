# NexusIM Android Local APK Build Baseline

Date: 2026-06-22

Scope: first local Android debug APK build for the client platform MVP.

This report proves that the Android native WebView shell can be packaged on the
Windows host using the local F-drive toolchain. It does not prove real-device
WebView metadata, login, PullInbox, WebSocket notify or AckDelivery; those are
the next smoke steps after explicit APK install.

## Environment

- Toolchain root: `F:\IM\toolchains`
- JDK: Temurin 17
- Gradle: 8.10.2
- Android SDK packages:
  - `platform-tools`
  - `platforms;android-35`
  - `build-tools;35.0.0`
- Gradle cache root: `F:\IM\toolchains\gradle-user-home`

The persistent user environment now points `JAVA_HOME`, `GRADLE_HOME`,
`ANDROID_HOME`, `ANDROID_SDK_ROOT` and `GRADLE_USER_HOME` at the F-drive
toolchain. Current Codex shells may still need inline env injection until the
parent process is restarted.

## Commands

Focused checks:

```powershell
npm --prefix clients run check:no-toolchain
npm --prefix clients run check:build-prereqs
npm --prefix clients run report:android-platform-readiness
```

Build:

```powershell
npm --prefix clients run build:android-apk:collect
```

Install plan:

```powershell
npm --prefix clients run plan:artifact-install
```

## Result

`check:build-prereqs` reported:

```text
desktopArtifactReady=true
androidApkReady=true
java>=17=true
gradle=true
ANDROID_HOME=true
ANDROID_SDK_ROOT=true
```

`report:android-platform-readiness` reported:

```text
localToolchain.ready=true
canBuildApkLocally=true
device.readyForInstallSmoke=true
```

`build:android-apk:collect` completed successfully and collected:

```text
manifest: clients/artifacts/2026-06-22T034017Z/manifest.json
artifact: nexusim-android-debug.apk
bytes: 1337087
sha256: f931053736f0e4168417b1187fe2c6058b86dc8db8dbca7a1e9b1fec1a901dba
```

`plan:artifact-install` reported Android `artifactReady=true` and
`readyForInstall=true`, and printed manual checklist commands for `adb install`
and package verification.

## Code Fixes Required During Bring-Up

- Windows command probes now resolve `.cmd` / `.bat` recoverys before marking
  Gradle missing.
- Android APK build wrapper now invokes `gradle.bat` through `cmd.exe` on
  Windows when no Gradle wrapper is present.
- Android native project now enables AndroidX because it depends on
  `androidx.webkit`.
- Android Java / Kotlin compile targets are aligned on JDK 17.

## Limits

- The APK was built and collected, but not installed by automation.
- No Android Activity was started.
- No `adb reverse` / `adb forward` was opened by this build report.
- No Android WebView metadata or login-level smoke is claimed here.
- Docker Android builder image was not built or used.

## Next Step

With user confirmation, run the Android device path:

```powershell
npm --prefix clients run smoke:android-webview-metadata
npm --prefix clients run smoke:android-webview-login -- --fixture <clientweb-fixture.json>
```

Those commands are device-touching and may install / start the app, so they
remain outside the safe no-toolchain gate.
