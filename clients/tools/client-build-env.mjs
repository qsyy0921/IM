import { spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

export const toolsDir = dirname(fileURLToPath(import.meta.url));
export const workspaceRoot = dirname(toolsDir);

export function collectClientBuildPrereqs() {
  const checks = [
    checkCommand("rustc", ["--version"], "desktop", "Rust compiler"),
    checkCommand("cargo", ["--version"], "desktop", "Cargo"),
    checkCommand("cargo", ["tauri", "--version"], "desktop", "cargo-tauri CLI"),
    checkLocalNodeBin("tauri", ["--version"], "desktop", "local npm Tauri CLI"),
    checkJava(),
    checkCommand("gradle", ["--version"], "android", "Gradle"),
    checkEnvPath("ANDROID_HOME", "android", "Android SDK"),
    checkEnvPath("ANDROID_SDK_ROOT", "android", "Android SDK")
  ];

  return {
    desktopArtifactReady:
      isOK(checks, "rustc") &&
      isOK(checks, "cargo") &&
      (isOK(checks, "cargo tauri") || isOK(checks, "local:tauri")),
    androidApkReady:
      isOK(checks, "java>=17") &&
      isOK(checks, "gradle") &&
      (isOK(checks, "ANDROID_HOME") || isOK(checks, "ANDROID_SDK_ROOT")),
    checks
  };
}

export function localNodeBin(binaryName) {
  return join(
    workspaceRoot,
    "node_modules",
    ".bin",
    process.platform === "win32" ? `${binaryName}.cmd` : binaryName
  );
}

export function commandSucceeded(command, args) {
  return runCommand(command, args).status === 0;
}

export function runCommand(command, args, options = {}) {
  const result = spawnSync(command, args, {
    encoding: "utf8",
    ...options
  });
  if (process.platform !== "win32" || result.error?.code !== "ENOENT" || command.match(/[\\/]/)) {
    return result;
  }
  for (const extension of [".cmd", ".bat", ".exe"]) {
    const candidateCommand = `${command}${extension}`;
    const located = spawnSync("where.exe", [candidateCommand], {
      encoding: "utf8"
    });
    if (located.status !== 0) {
      continue;
    }
    const candidateResult = extension === ".cmd" || extension === ".bat"
      ? spawnSync("cmd.exe", ["/d", "/c", candidateCommand, ...args], {
        encoding: "utf8",
        ...options
      })
      : spawnSync(candidateCommand, args, {
        encoding: "utf8",
        ...options
      });
    if (candidateResult.error?.code !== "ENOENT") {
      return candidateResult;
    }
  }
  return result;
}

function isOK(checks, name) {
  return checks.some(check => check.name === name && check.ok);
}

function checkCommand(command, args, target, label) {
  const executed = runCommand(command, args);
  const name = [command, ...args.filter(arg => arg !== "--version")].join(" ");
  return {
    name,
    target,
    label,
    ok: executed.status === 0,
    command: [command, ...args].join(" "),
    detail: trimOutput(executed.stdout || executed.stderr || executed.error?.message || "")
  };
}

function checkLocalNodeBin(binaryName, args, target, label) {
  const binaryPath = localNodeBin(binaryName);
  if (!existsSync(binaryPath)) {
    return {
      name: `local:${binaryName}`,
      target,
      label,
      ok: false,
      command: binaryPath,
      detail: "not installed in clients/node_modules/.bin"
    };
  }
  const executed = process.platform === "win32"
    ? runCommand("cmd.exe", ["/d", "/c", binaryPath, ...args])
    : runCommand(binaryPath, args);
  return {
    name: `local:${binaryName}`,
    target,
    label,
    ok: executed.status === 0,
    command: [binaryPath, ...args].join(" "),
    detail: trimOutput(executed.stdout || executed.stderr || executed.error?.message || "")
  };
}

function checkJava() {
  const executed = runCommand("java", ["-version"]);
  const output = executed.stderr || executed.stdout || executed.error?.message || "";
  const major = parseJavaMajorVersion(output);
  return {
    name: "java>=17",
    target: "android",
    label: "JDK 17+",
    ok: executed.status === 0 && major !== null && major >= 17,
    command: "java -version",
    detail: trimOutput(output),
    detectedMajorVersion: major
  };
}

function checkEnvPath(name, target, label) {
  const value = process.env[name] ?? "";
  return {
    name,
    target,
    label,
    ok: value.trim() !== "",
    command: `env:${name}`,
    detail: value || "not set"
  };
}

function parseJavaMajorVersion(output) {
  const match = output.match(/version\s+"([^"]+)"/);
  if (!match) {
    return null;
  }
  const version = match[1];
  if (version.startsWith("1.")) {
    const legacy = Number.parseInt(version.split(".")[1] ?? "", 10);
    return Number.isNaN(legacy) ? null : legacy;
  }
  const major = Number.parseInt(version.split(".")[0] ?? "", 10);
  return Number.isNaN(major) ? null : major;
}

function trimOutput(output) {
  return output.trim().split(/\r?\n/).slice(0, 4).join("\n");
}
