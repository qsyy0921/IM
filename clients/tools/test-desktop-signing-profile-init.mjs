import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { initDesktopSigningProfile } from "./init-desktop-signing-profile.mjs";
import { readDesktopSigningProfile } from "./desktop-signing-profile.mjs";

const scriptPath = fileURLToPath(new URL("./init-desktop-signing-profile.mjs", import.meta.url));

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

function runCLI(args) {
  const output = execFileSync(process.execPath, [scriptPath, ...args], {
    encoding: "utf8"
  });
  return JSON.parse(output);
}

const tempRoot = mkdtempSync(join(tmpdir(), "nexusim-desktop-signing-profile-init-"));
try {
  const pfxOutput = join(tempRoot, "signing-profile.local.json");
  const pfxPlan = initDesktopSigningProfile({
    source: "pfx-file",
    output: pfxOutput
  });
  assert(pfxPlan.readyToWrite === true, "pfx init should be ready");
  assert(pfxPlan.wroteProfile === true, "pfx init should write the profile");
  assert(pfxPlan.output.existsAfterWrite === true, "pfx output should exist after write");
  assert(existsSync(pfxOutput), "pfx output file missing");
  const pfxProfile = readDesktopSigningProfile(pfxOutput);
  assert(pfxProfile.certFile === "<local-nexusim-code-signing.pfx>", "pfx output should contain local placeholder");
  assert(pfxProfile.pfxPassEnv === "NEXUSIM_DESKTOP_SIGN_PFX_PASS", "pfx output should keep standard pfx env");

  const duplicate = initDesktopSigningProfile({
    source: "pfx-file",
    output: pfxOutput
  });
  assert(duplicate.readyToWrite === false, "duplicate output should not be ready");
  assert(duplicate.blockers.includes("output-already-exists"), "duplicate output should report blocker");
  assert(duplicate.wroteProfile === false, "duplicate output must not overwrite");

  writeFileSync(pfxOutput, "not json", "utf8");
  const overwritten = initDesktopSigningProfile({
    source: "windows-cert-store",
    output: pfxOutput,
    overwrite: true
  });
  assert(overwritten.readyToWrite === true, "explicit overwrite should be ready");
  const storeProfile = readDesktopSigningProfile(pfxOutput);
  assert(storeProfile.certFile === "", "cert-store output must not set pfx file");
  assert(storeProfile.certSHA1 === "00112233445566778899AABBCCDDEEFF00112233", "cert-store output should normalize thumbprint");

  const dryRunOutput = join(tempRoot, "dry-run-profile.local.json");
  const dryRun = runCLI([
    "--source",
    "pfx-file",
    "--output",
    dryRunOutput,
    "--dry-run"
  ]);
  assert(dryRun.readyToWrite === true, "dry-run should report ready");
  assert(dryRun.executionPolicy.dryRun === true, "dry-run policy missing");
  assert(dryRun.wroteProfile === false, "dry-run must not write");
  assert(!existsSync(dryRunOutput), "dry-run output should not exist");
  const dryRunSerialized = JSON.stringify(dryRun);
  assert(!dryRunSerialized.includes(tempRoot), "dry-run report leaked absolute temp path");
  assert(!dryRunSerialized.match(/[A-Z]:\\\\/), "dry-run report leaked a Windows absolute path");
  assert(!dryRunSerialized.match(/token|secret|password|credential|private/i), "dry-run report leaked sensitive names");

  const missingSource = initDesktopSigningProfile({
    output: join(tempRoot, "missing-source.json")
  });
  assert(missingSource.readyToWrite === false, "missing source should not be ready");
  assert(missingSource.blockers.includes("source-required"), "missing source blocker missing");

  const badOutput = initDesktopSigningProfile({
    source: "pfx-file",
    output: join(tempRoot, "signing-profile.pfx.example.json")
  });
  assert(badOutput.readyToWrite === false, "example output should be blocked");
  assert(badOutput.blockers.includes("output-must-not-be-example-profile"), "example output blocker missing");

  const nestedOutput = join(tempRoot, "nested", "signing-profile.local.json");
  const nested = initDesktopSigningProfile({
    source: "pfx-file",
    output: nestedOutput
  });
  assert(nested.wroteProfile === true, "nested output should be created");
  assert(existsSync(nestedOutput), "nested output file missing");
  assert(readFileSync(nestedOutput, "utf8").includes("<local-nexusim-code-signing.pfx>"), "nested output content mismatch");
} finally {
  rmSync(tempRoot, { recursive: true, force: true });
}

console.log("desktop signing profile init ok");
