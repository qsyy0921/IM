import { spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const toolsDir = dirname(fileURLToPath(import.meta.url));
const workspaceRoot = dirname(toolsDir);

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

const desktopReady =
  isOK("rustc") &&
  isOK("cargo") &&
  (isOK("cargo tauri") || isOK("local:tauri"));
const androidReady =
  isOK("java>=17") &&
  isOK("gradle") &&
  (isOK("ANDROID_HOME") || isOK("ANDROID_SDK_ROOT"));

const result = {
  desktopArtifactReady: desktopReady,
  androidApkReady: androidReady,
  checks
};

console.log(JSON.stringify(result, null, 2));

if (!desktopReady || !androidReady) {
  process.exitCode = 2;
}

function isOK(name) {
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
  const binaryPath = join(
    workspaceRoot,
    "node_modules",
    ".bin",
    process.platform === "win32" ? `${binaryName}.cmd` : binaryName
  );
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
    ? runCommand("cmd.exe", ["/d", "/s", "/c", quoteCommand(binaryPath, args)])
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

function runCommand(command, args) {
  return spawnSync(command, args, {
    encoding: "utf8"
  });
}

function quoteCommand(command, args) {
  const commandLine = [`"${command}"`, ...args.map(arg => `"${arg.replaceAll("\"", "\\\"")}"`)];
  return commandLine.join(" ");
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
