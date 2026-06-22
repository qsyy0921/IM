import { execFileSync } from "node:child_process";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

const clientsRoot = fileURLToPath(new URL("..", import.meta.url));
const checkPlan = [
  {
    name: "shell-smoke-plan",
    script: "test:shell-smoke-plan",
    reason: "guards browser, desktop and Android shell smoke checklist shape"
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
    name: "clientweb-smoke-hooks",
    script: "test:clientweb-smoke-hooks",
    reason: "guards clientweb run-local-smoke WebView opt-in hooks without starting services"
  },
  {
    name: "web-pwa-contract",
    script: "test:web-pwa",
    reason: "guards browser manifest and service-worker cache boundary"
  },
  {
    name: "shell-web-assets",
    script: "test:shell-web-assets",
    reason: "guards target shell asset manifest and PWA asset propagation"
  },
  {
    name: "desktop-shell-action-assets",
    script: "test:desktop-shell-action-assets",
    reason: "guards desktop WebView assets without Tauri or installer"
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
