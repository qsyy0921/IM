import { createHash } from "node:crypto";
import { spawnSync } from "node:child_process";
import { existsSync, readFileSync, readdirSync, statSync } from "node:fs";
import { basename, dirname, isAbsolute, join, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";
import { workspaceRoot } from "./client-build-env.mjs";

const schemaVersion = "nexusim.client-artifact-install-plan.v1";
const artifactManifestSchema = "nexusim.client-artifacts.v1";
const artifactsRoot = join(workspaceRoot, "artifacts");
const targetNames = ["windows-desktop", "android"];

function main(argv) {
  const options = parseArgs(argv);
  const plan = buildClientArtifactInstallPlan(options);
  process.stdout.write(`${JSON.stringify(plan, null, 2)}\n`);
}

export function buildClientArtifactInstallPlan(options = {}) {
  const manifestPath = options.manifest ? resolve(options.manifest) : findLatestArtifactManifest(options.artifactsRoot ?? artifactsRoot);
  const installPrereqs = options.installPrereqs ?? collectInstallPrereqs();
  if (!manifestPath) {
    return emptyPlan(installPrereqs);
  }

  const manifest = readManifest(manifestPath);
  const manifestDir = dirname(manifestPath);
  const targets = Object.fromEntries(targetNames.map(target => [
    target,
    targetInstallPlan(target, manifest, manifestDir, installPrereqs)
  ]));
  const plan = {
    schemaVersion,
    generatedAt: new Date().toISOString(),
    artifactManifest: {
      present: true,
      manifestHint: safeHint(manifestPath),
      runId: stringValue(manifest.runId)
    },
    targets
  };
  assertLowSensitivePlan(plan);
  return plan;
}

function emptyPlan(installPrereqs) {
  return {
    schemaVersion,
    generatedAt: new Date().toISOString(),
    artifactManifest: {
      present: false,
      manifestHint: "clients/artifacts/<run-id>/manifest.json"
    },
    targets: Object.fromEntries(targetNames.map(target => [
      target,
      {
        artifactReady: false,
        readyForInstall: false,
        installPrereqs: targetInstallPrereqs(target, installPrereqs),
        missing: missingInstallInputs(target, installPrereqs, ["artifact-manifest"]),
        checklist: [
          {
            step: "build-and-collect-artifact",
            command: target === "android"
              ? "npm --prefix clients run build:android-apk:collect"
              : "npm --prefix clients run build:desktop-artifact:collect",
            evidence: "collector writes clients/artifacts/<run-id>/manifest.json"
          }
        ]
      }
    ]))
  };
}

function targetInstallPlan(target, manifest, manifestDir, installPrereqs) {
  const artifact = findArtifact(manifest, target);
  const prereqs = targetInstallPrereqs(target, installPrereqs);
  if (!artifact) {
    return {
      artifactReady: false,
      readyForInstall: false,
      installPrereqs: prereqs,
      missing: missingInstallInputs(target, installPrereqs, [`${target}-artifact`]),
      checklist: [
        {
          step: "build-and-collect-artifact",
          command: target === "android"
            ? "npm --prefix clients run build:android-apk:collect"
            : "npm --prefix clients run build:desktop-artifact:collect",
          evidence: `collector manifest includes target=${target}`
        }
      ]
    };
  }

  const artifactPath = join(manifestDir, artifact.filename);
  validateArtifactFile(artifact, artifactPath);
  const artifactHint = safeHint(artifactPath);
  const missing = missingInstallInputs(target, installPrereqs, []);
  return {
    artifactReady: true,
    readyForInstall: missing.length === 0,
    installPrereqs: prereqs,
    missing,
    artifact: {
      filename: artifact.filename,
      bytes: artifact.bytes,
      sha256: artifact.sha256,
      artifactHint
    },
    checklist: installChecklist(target, artifactHint)
  };
}

function targetInstallPrereqs(target, installPrereqs) {
  if (target === "android") {
    return {
      adbAvailable: Boolean(installPrereqs.adbAvailable)
    };
  }
  return {
    windowsInstallerLaunchSupported: Boolean(installPrereqs.windowsInstallerLaunchSupported)
  };
}

function missingInstallInputs(target, installPrereqs, baseMissing) {
  const missing = [...baseMissing];
  if (target === "android" && !installPrereqs.adbAvailable) {
    missing.push("adb");
  }
  if (target === "windows-desktop" && !installPrereqs.windowsInstallerLaunchSupported) {
    missing.push("windows-installer-launch");
  }
  return missing;
}

function collectInstallPrereqs() {
  return {
    adbAvailable: commandAvailable("adb", ["version"]),
    windowsInstallerLaunchSupported: process.platform === "win32"
  };
}

function commandAvailable(command, args) {
  const result = spawnSync(command, args, {
    encoding: "utf8",
    stdio: "ignore",
    timeout: 3000,
    windowsHide: true
  });
  return result.status === 0;
}

function installChecklist(target, artifactHint) {
  if (target === "android") {
    return [
      {
        step: "verify-adb-device",
        command: "adb devices",
        evidence: "target Android device appears as device, not unauthorized"
      },
      {
        step: "install-apk",
        command: `adb install -r ${artifactHint}`,
        evidence: "adb returns Success"
      },
      {
        step: "verify-installed-package",
        command: "adb shell pm path com.nexusim.android",
        evidence: "package manager returns a NexusIM package path"
      },
      {
        step: "run-client-smoke",
        evidence: "Android shell metadata reports target=android and can login, pull inbox, receive wakeup and ack"
      }
    ];
  }

  return [
    {
      step: "launch-installer",
      command: `Start-Process ${artifactHint}`,
      evidence: "Windows installer launches without SmartScreen or signer policy failures in the local test profile"
    },
    {
      step: "verify-installed-shell",
      evidence: "desktop shell metadata reports target=windows-desktop"
    },
    {
      step: "run-client-smoke",
      evidence: "desktop shell can login, pull inbox, receive wakeup and ack"
    }
  ];
}

function readManifest(manifestPath) {
  const raw = readFileSync(manifestPath, "utf8");
  if (raw.match(/[A-Za-z]:\\\\/) || raw.includes("\\\\?")) {
    throw new Error("artifact manifest leaked a local absolute path");
  }
  if (raw.match(/(token|secret|password|credential|private)/i)) {
    throw new Error("artifact manifest contains a sensitive field name");
  }
  const manifest = JSON.parse(raw);
  if (manifest.schemaVersion !== artifactManifestSchema) {
    throw new Error("artifact manifest schema mismatch");
  }
  if (!Array.isArray(manifest.artifacts)) {
    throw new Error("artifact manifest artifacts missing");
  }
  return manifest;
}

function findArtifact(manifest, target) {
  return manifest.artifacts.find(artifact => artifact?.target === target);
}

function validateArtifactFile(artifact, artifactPath) {
  if (typeof artifact.filename !== "string" || artifact.filename.length === 0) {
    throw new Error("artifact filename missing");
  }
  if (artifact.filename.includes("/") || artifact.filename.includes("\\") || isAbsolute(artifact.filename)) {
    throw new Error(`artifact filename is not relative-safe: ${artifact.filename}`);
  }
  if (artifact.filename.match(/(token|secret|password|credential|private)/i)) {
    throw new Error("artifact filename contains a sensitive field name");
  }
  if (!Number.isInteger(artifact.bytes) || artifact.bytes < 0) {
    throw new Error(`artifact byte size invalid: ${artifact.filename}`);
  }
  if (typeof artifact.sha256 !== "string" || !artifact.sha256.match(/^[a-f0-9]{64}$/)) {
    throw new Error(`artifact sha256 invalid: ${artifact.filename}`);
  }
  if (!existsSync(artifactPath) || !statSync(artifactPath).isFile()) {
    throw new Error(`artifact file missing: ${artifact.filename}`);
  }
  const bytes = readFileSync(artifactPath);
  const sha256 = createHash("sha256").update(bytes).digest("hex");
  if (bytes.length !== artifact.bytes) {
    throw new Error(`artifact byte size mismatch: ${artifact.filename}`);
  }
  if (sha256 !== artifact.sha256) {
    throw new Error(`artifact hash mismatch: ${artifact.filename}`);
  }
}

function findLatestArtifactManifest(root) {
  if (!existsSync(root)) {
    return "";
  }
  const candidates = [];
  collectManifestCandidates(root, candidates);
  candidates.sort((left, right) => right.mtimeMs - left.mtimeMs);
  return candidates[0]?.path ?? "";
}

function collectManifestCandidates(dir, candidates) {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const fullPath = join(dir, entry.name);
    if (!entry.isDirectory()) {
      if (entry.isFile() && entry.name === "manifest.json") {
        candidates.push({
          path: fullPath,
          mtimeMs: statSync(fullPath).mtimeMs
        });
      }
      continue;
    }
    collectManifestCandidates(fullPath, candidates);
  }
}

function safeHint(path) {
  const relativePath = relative(workspaceRoot, resolve(path)).split(sep).join("/");
  if (relativePath.startsWith("..") || isAbsolute(relativePath)) {
    return `${basename(path)}#${sha256Text(resolve(path)).slice(0, 12)}`;
  }
  return `clients/${relativePath}`;
}

function assertLowSensitivePlan(plan) {
  const serialized = JSON.stringify(plan);
  if (serialized.match(/[A-Za-z]:\\\\/) || serialized.includes("\\\\?")) {
    throw new Error("install plan leaked a local absolute path");
  }
  if (serialized.match(/(token|secret|password|credential|private)/i)) {
    throw new Error("install plan leaked a sensitive field name");
  }
}

function stringValue(value) {
  return typeof value === "string" ? value : "";
}

function sha256Text(value) {
  return createHash("sha256").update(value).digest("hex");
}

function parseArgs(argv) {
  const options = {
    manifest: ""
  };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (arg === "--manifest") {
      options.manifest = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    throw new Error(`unknown argument: ${arg}`);
  }
  return options;
}

function requiredValue(argv, index, name) {
  const value = argv[index + 1];
  if (!value || value.startsWith("--")) {
    throw new Error(`${name} requires a value`);
  }
  return value;
}

const thisFile = fileURLToPath(import.meta.url);
if (resolve(process.argv[1] ?? "") === thisFile) {
  try {
    main(process.argv.slice(2));
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 2;
  }
}
