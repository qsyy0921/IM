import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const toolsDir = dirname(fileURLToPath(import.meta.url));
const smokeScript = join(toolsDir, "smoke-desktop-composed.mjs");

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

function sha256(value) {
  return createHash("sha256").update(value).digest("hex");
}

const tempRoot = mkdtempSync(join(tmpdir(), "nexusim-desktop-composed-smoke-"));
try {
  const artifactDir = join(tempRoot, "artifact");
  const summaryDir = join(tempRoot, "summary");
  mkdirSync(artifactDir, { recursive: true });
  mkdirSync(summaryDir, { recursive: true });

  const exeBody = "fake exe bytes";
  writeFileSync(join(artifactDir, "nexusim-windows-desktop.exe"), exeBody);
  writeFileSync(join(artifactDir, "manifest.json"), `${JSON.stringify({
    schemaVersion: "nexusim.client-artifacts.v1",
    generatedAt: "2026-06-22T00:00:00.000Z",
    gitCommit: "test",
    runId: "desktop-composed-test",
    artifacts: [
      {
        target: "windows-desktop",
        filename: "nexusim-windows-desktop.exe",
        bytes: Buffer.byteLength(exeBody),
        sha256: sha256(exeBody),
        sourcePathHash: sha256("desktop-source"),
        sourceHint: "desktop/src-tauri/target/release/nexusim-desktop.exe"
      }
    ]
  }, null, 2)}\n`);

  const messageID = "msg_123";
  writeFileSync(join(summaryDir, "client-web-summary.json"), `${JSON.stringify({
    commit: "abcdef1",
    commit_full: "abcdef1234567890",
    git_dirty: false,
    success: true,
    result_dir: "H:\\NexusIM\\loadtest-results\\client-web-test",
    receiver_password: "must-not-leak",
    sender_login: {
      gateway_token_set: true,
      push_gateway_token_set: true,
      refresh_token_set: true
    },
    receiver_login: {
      gateway_token_set: true,
      push_gateway_token_set: true,
      refresh_token_set: true
    },
    server_hello: {
      op: "server.hello",
      resume_token: "must-not-leak"
    },
    send_message: {
      message_id: messageID,
      conversation_seq: 2
    },
    delivery_notify: {
      message_id: messageID,
      conversation_seq: 2
    },
    pull_inbox: {
      item_count: 1,
      max_seq: 2
    },
    list_conversations: {
      item_count: 1
    },
    ack_delivery: {
      last_received_seq: 2
    },
    postgres: {
      user_inbox_count: 1,
      device_delivery_cursor_seq: 2
    }
  }, null, 2)}\n`);

  const output = execFileSync(process.execPath, [
    smokeScript,
    "--clientweb-summary",
    join(summaryDir, "client-web-summary.json"),
    "--artifact-manifest",
    join(artifactDir, "manifest.json"),
    "--launch-dry-run",
    "--hold-ms",
    "1000"
  ], {
    encoding: "utf8"
  });
  const result = JSON.parse(output);
  const serialized = JSON.stringify(result);

  assert(result.schemaVersion === "nexusim.desktop-composed-smoke.v1", "desktop composed smoke schema mismatch");
  assert(result.mode.ranClientWeb === false, "dry fixture must not run clientweb");
  assert(result.mode.launchDryRun === true, "launch dry-run mode missing");
  assert(result.executionPolicy.planOnly === true, "dry fixture must be plan-only");
  assert(result.executionPolicy.readsClientWebSummary === true, "dry fixture should read an existing clientweb summary");
  assert(result.executionPolicy.runsClientWebSmoke === false, "dry fixture must not run clientweb smoke");
  assert(result.executionPolicy.startsServices === false, "dry fixture must not start services");
  assert(result.executionPolicy.readsArtifactManifest === true, "dry fixture should read the artifact manifest");
  assert(result.executionPolicy.validatesArtifactFile === true, "dry fixture should validate artifact bytes");
  assert(result.executionPolicy.runsDesktopLaunchSmoke === true, "dry fixture should run the nested desktop launch smoke");
  assert(result.executionPolicy.desktopLaunchDryRun === true, "dry fixture should run nested launch smoke in dry-run mode");
  assert(result.executionPolicy.startsDesktopArtifact === false, "dry fixture must not start the desktop artifact");
  assert(result.executionPolicy.opensNetworkConnection === false, "dry fixture must not open network connections");
  assert(result.executionPolicy.installsArtifacts === false, "dry fixture must not install artifacts");
  assert(result.executionPolicy.contactsDevice === false, "dry fixture must not contact devices");
  assert(result.executionPolicy.startsDocker === false, "dry fixture must not start Docker");
  assert(result.executionPolicy.downloadsToolchain === false, "dry fixture must not download toolchains");
  assert(result.clientWeb.success === true, "clientweb success missing");
  assert(result.clientWeb.cleanGit === true, "clientweb clean git missing");
  assert(result.clientWeb.flow.bffLogin === true, "BFF login proof missing");
  assert(result.clientWeb.flow.pushHello === true, "push hello proof missing");
  assert(result.clientWeb.flow.notifyMatchesSend === true, "notify/send proof missing");
  assert(result.clientWeb.flow.pullMatchesNotify === true, "pull/notify proof missing");
  assert(result.clientWeb.flow.ackMatchesPull === true, "ack/pull proof missing");
  assert(result.clientWeb.flow.cursorMatchesAck === true, "cursor/ack proof missing");
  assert(result.desktop.dryRun === true, "desktop launch dry-run missing");
  assert(result.desktop.artifact.filename === "nexusim-windows-desktop.exe", "desktop artifact filename mismatch");
  assert(result.verdict.clientWebPath === true, "clientweb verdict mismatch");
  assert(result.verdict.desktopLaunchPath === false, "dry-run launch must not claim desktop launch path");
  assert(result.verdict.composedEvidenceReady === true, "composed dry-run evidence should be ready");
  assert(result.verdict.loginLevelDesktopUISmoke === false, "composed smoke must not claim login-level desktop UI smoke");
  assert(!serialized.match(/[A-Z]:\\\\/), "desktop composed smoke leaked Windows absolute path");
  assert(!serialized.includes("\\\\?"), "desktop composed smoke leaked extended Windows path");
  assert(!serialized.match(/token|secret|password|credential|private/i), "desktop composed smoke leaked sensitive field name");

  console.log("desktop composed smoke ok");
} finally {
  rmSync(tempRoot, { recursive: true, force: true });
}
