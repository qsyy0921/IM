import { execFileSync } from "node:child_process";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

const clientsRoot = fileURLToPath(new URL("..", import.meta.url));
const checkPlan = [
  {
    name: "no-toolchain-plan-self-check",
    script: "test:no-toolchain-check",
    reason: "guards this aggregate gate against unsafe operations"
  },
  {
    name: "client-workspace-validation",
    script: "validate",
    reason: "guards client workspace skeleton and package boundaries"
  },
  {
    name: "shell-smoke-plan",
    script: "test:shell-smoke-plan",
    reason: "guards browser, desktop and Android shell smoke checklist shape"
  },
  {
    name: "build-prereqs-report-contract",
    script: "test:build-prereqs",
    reason: "guards low-sensitive local build prerequisite reporting without building artifacts"
  },
  {
    name: "artifact-readiness-contract",
    script: "test:artifact-readiness",
    reason: "guards low-sensitive native artifact readiness reporting without building artifacts"
  },
  {
    name: "artifact-install-plan-contract",
    script: "test:artifact-install-plan",
    reason: "guards collected artifact install plans without installing packages or contacting devices"
  },
  {
    name: "artifact-builder-contract",
    script: "test:artifact-builders",
    reason: "guards desktop and Android artifact builder dry-run plans"
  },
  {
    name: "artifact-collector-contract",
    script: "test:artifact-collector",
    reason: "guards low-sensitive collected artifact manifest semantics"
  },
  {
    name: "client-builder-profile-contract",
    script: "validate:builder-profile",
    reason: "guards Android builder profile files without running the builder"
  },
  {
    name: "android-builder-wrapper-contract",
    script: "test:android-docker-builder",
    reason: "guards Android builder wrapper dry-run output"
  },
  {
    name: "clientweb-smoke-hooks",
    script: "test:clientweb-smoke-hooks",
    reason: "guards clientweb run-local-smoke WebView opt-in hooks without starting services"
  },
  {
    name: "client-start-scripts",
    script: "test:client-start-scripts",
    reason: "guards local Web/backend start scripts without starting services"
  },
  {
    name: "web-pwa-contract",
    script: "test:web-pwa",
    reason: "guards browser manifest and service-worker cache boundary"
  },
  {
    name: "shell-config-contract",
    script: "test:shell-config",
    reason: "guards desktop and Android shell config parsing without privileged fields"
  },
  {
    name: "client-typescript-workspace-contract",
    script: "typecheck",
    reason: "guards protocol, core, Web, desktop and Android TypeScript contracts"
  },
  {
    name: "desktop-native-skeleton-contract",
    script: "validate:desktop-tauri",
    reason: "guards desktop native shell skeleton without building the artifact"
  },
  {
    name: "android-native-skeleton-contract",
    script: "validate:android-native",
    reason: "guards Android native shell skeleton without building an APK"
  },
  {
    name: "web-platform-contract",
    script: "test:web-platform",
    reason: "guards browser and WebView platform adapter boundaries"
  },
  {
    name: "local-message-store-contract",
    script: "test:local-message-store",
    reason: "guards shared local message cache semantics"
  },
  {
    name: "indexeddb-message-store-contract",
    script: "test:indexeddb-store",
    reason: "guards browser IndexedDB message cache semantics"
  },
  {
    name: "key-value-message-store-contract",
    script: "test:key-value-store",
    reason: "guards persistent key-value message cache semantics"
  },
  {
    name: "http-bff-client-contract",
    script: "test:http-bff-client",
    reason: "guards BFF payload decoding and shared HTTP client mapping"
  },
  {
    name: "native-store-readiness-contract",
    script: "test:native-store-readiness",
    reason: "guards native SQLite bridge readiness contract"
  },
  {
    name: "runtime-lifecycle-contract",
    script: "test:runtime-lifecycle",
    reason: "guards shared desktop and Android runtime lifecycle without services"
  },
  {
    name: "web-shell-lifecycle-contract",
    script: "test:web-shell-actions",
    reason: "guards Web shell lifecycle actions through shared client-core"
  },
  {
    name: "web-shell-automation-contract",
    script: "test:web-shell-automation",
    reason: "guards stable Web shell selectors for browser and WebView smoke automation"
  },
  {
    name: "web-shell-smoke-report-contract",
    script: "test:web-shell-smoke-report",
    reason: "guards loopback-only Web shell metadata smoke report without starting services"
  },
  {
    name: "shell-web-assets",
    script: "test:shell-web-assets",
    reason: "guards target shell asset manifest and PWA asset propagation"
  },
  {
    name: "shell-asset-prep-wrapper-contract",
    script: "test:shell-asset-prep-wrapper",
    reason: "guards shell asset prep wrapper without native builds"
  },
  {
    name: "desktop-shell-action-assets",
    script: "test:desktop-shell-action-assets",
    reason: "guards desktop WebView assets without Tauri or installer"
  },
  {
    name: "desktop-artifact-launch-runner-contract",
    script: "test:desktop-artifact-launch-smoke",
    reason: "guards desktop artifact launch smoke dry-run output without launching the desktop artifact"
  },
  {
    name: "desktop-composed-smoke-runner-contract",
    script: "test:desktop-composed-smoke",
    reason: "guards desktop composed smoke dry-run summary without starting services"
  },
  {
    name: "desktop-webview-metadata-runner-contract",
    script: "test:desktop-webview-metadata-smoke",
    reason: "guards desktop WebView metadata smoke dry-run output without building or launching the desktop artifact"
  },
  {
    name: "desktop-webview-login-runner-contract",
    script: "test:desktop-webview-login-smoke",
    reason: "guards desktop login-level WebView smoke dry-run output without building or launching the desktop artifact"
  },
  {
    name: "android-shell-action-assets",
    script: "test:android-shell-action-assets",
    reason: "guards Android WebView assets without Gradle, SDK, APK or device"
  },
  {
    name: "android-webview-metadata-runner-contract",
    script: "test:android-webview-metadata-smoke",
    reason: "guards Android metadata WebView smoke dry-run output without APK, Docker or device execution"
  },
  {
    name: "android-webview-login-plan",
    script: "test:android-webview-login-smoke-plan",
    reason: "guards Android login-level WebView smoke plan and safe preflight without APK, Docker or device execution"
  },
  {
    name: "android-webview-login-runner-contract",
    script: "test:android-webview-login-smoke",
    reason: "guards Android login-level WebView smoke dry-run output and native-store readiness parsing without APK, Docker or device execution"
  },
  {
    name: "android-platform-readiness-contract",
    script: "test:android-platform-readiness",
    reason: "guards low-sensitive Android platform readiness schema"
  },
  {
    name: "android-device-readiness-contract",
    script: "test:android-device-readiness",
    reason: "guards low-sensitive Android ADB device readiness parsing without installing or launching"
  },
  {
    name: "android-webview-devtools-readiness-contract",
    script: "test:android-webview-devtools-readiness",
    reason: "guards low-sensitive Android WebView devtools readiness parsing without opening device tunnels"
  },
  {
    name: "android-webview-devtools-parser-contract",
    script: "test:android-webview-devtools-parser",
    reason: "guards Android WebView devtools socket parser contract"
  },
  {
    name: "android-platform-readiness-report",
    script: "report:android-platform-readiness",
    reason: "reports local Android toolchain, Docker builder and ADB state without downloading"
  }
];

function main() {
  const args = new Set(process.argv.slice(2));
  if (args.has("--dry-run")) {
    process.stdout.write(`${JSON.stringify(buildDryRunPlan(), null, 2)}\n`);
    return;
  }

  for (const check of checkPlan) {
    console.log(`\n[client-no-toolchain] ${check.name}: npm --prefix clients run ${check.script}`);
    runNpmScript(check.script);
  }
  console.log("\nclient no-toolchain checks ok");
}

export function buildDryRunPlan() {
  return {
    schemaVersion: "nexusim.client-no-toolchain-check.v1",
    executionPolicy: dryRunExecutionPolicy(),
    downloadsToolchain: false,
    readsDeviceReadiness: true,
    installsArtifacts: false,
    startsDeviceActivities: false,
    opensAdbReverse: false,
    startsServices: false,
    checks: checkPlan.map(check => ({
      name: check.name,
      command: `npm --prefix clients run ${check.script}`,
      reason: check.reason
    }))
  };
}

function dryRunExecutionPolicy() {
  return {
    planOnly: true,
    describesFocusedGate: true,
    executesChecks: false,
    runsNpmScripts: false,
    readsDeviceReadiness: false,
    installsArtifacts: false,
    startsDeviceActivities: false,
    opensAdbReverse: false,
    startsServices: false,
    startsDocker: false,
    downloadsToolchain: false
  };
}

function runNpmScript(script) {
  const npmExecPath = process.env.npm_execpath;
  if (npmExecPath) {
    execFileSync(process.execPath, [npmExecPath, "--prefix", clientsRoot, "run", script], {
      cwd: clientsRoot,
      stdio: "inherit"
    });
    return;
  }

  const npm = process.platform === "win32" ? "npm.cmd" : "npm";
  execFileSync(npm, ["--prefix", clientsRoot, "run", script], {
    cwd: clientsRoot,
    stdio: "inherit",
    shell: process.platform === "win32"
  });
}

const thisFile = fileURLToPath(import.meta.url);
if (resolve(process.argv[1] ?? "") === thisFile) {
  try {
    main();
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 2;
  }
}
