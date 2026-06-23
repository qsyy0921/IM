import { createHash } from "node:crypto";
import { copyFileSync, existsSync, mkdirSync, statSync } from "node:fs";
import { basename, dirname, isAbsolute, join, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";
import { workspaceRoot } from "./client-build-env.mjs";
import { readDesktopSigningProfile } from "./desktop-signing-profile.mjs";

const schemaVersion = "nexusim.desktop-signing-profile-init.v1";
const templates = {
  "pfx-file": "signing-profile.pfx.example.json",
  "windows-cert-store": "signing-profile.cert-store.example.json"
};

function main(argv) {
  const options = parseArgs(argv);
  const result = initDesktopSigningProfile(options);
  process.stdout.write(`${JSON.stringify(result, null, 2)}\n`);
}

export function initDesktopSigningProfile(options = {}) {
  const plan = buildDesktopSigningProfileInitPlan(options);
  if (!plan.readyToWrite || plan.executionPolicy.dryRun) {
    return plan;
  }
  const outputPath = resolve(stringValue(options.output));
  const templatePath = templatePathForSource(stringValue(options.source));
  mkdirSync(dirname(outputPath), { recursive: true });
  copyFileSync(templatePath, outputPath);
  readDesktopSigningProfile(outputPath);
  return {
    ...plan,
    wroteProfile: true,
    output: {
      ...plan.output,
      existsAfterWrite: true
    },
    nextAction: "edit the local untracked profile with real local path or thumbprint before release signing"
  };
}

export function buildDesktopSigningProfileInitPlan(options = {}) {
  const source = stringValue(options.source);
  const outputPath = stringValue(options.output);
  const dryRun = Boolean(options.dryRun);
  const overwrite = Boolean(options.overwrite);
  const blockers = [];
  if (!source) {
    blockers.push("source-required");
  }
  if (source && !templates[source]) {
    blockers.push("source-invalid");
  }
  if (!outputPath) {
    blockers.push("output-required");
  }

  const templatePath = templatePathForSource(source);
  if (templatePath && !existsSync(templatePath)) {
    blockers.push("template-missing");
  }

  const outputResolved = outputPath ? resolve(outputPath) : "";
  if (outputResolved && isExampleProfile(outputResolved)) {
    blockers.push("output-must-not-be-example-profile");
  }
  if (outputResolved && existsSync(outputResolved) && !overwrite) {
    blockers.push("output-already-exists");
  }
  if (outputResolved && existsSync(outputResolved) && !statSync(outputResolved).isFile()) {
    blockers.push("output-not-file");
  }

  return {
    schemaVersion,
    generatedAt: new Date().toISOString(),
    executionPolicy: {
      dryRun,
      writesLocalProfile: !dryRun && blockers.length === 0,
      readsTemplateProfile: true,
      readsLocalCertificate: false,
      readsProtectedSigningMaterial: false,
      signsArtifacts: false,
      buildsInstaller: false,
      installsArtifacts: false,
      downloadsToolchain: false,
      startsServices: false
    },
    source,
    template: templatePath
      ? {
        filename: basename(templatePath),
        hint: safeHint(templatePath)
      }
      : null,
    output: outputResolved
      ? {
        hint: safeHint(outputResolved),
        existsBeforeWrite: existsSync(outputResolved),
        ignoredByRepoConvention: isIgnoredSigningProfileByConvention(outputResolved)
      }
      : null,
    overwrite,
    readyToWrite: blockers.length === 0,
    wroteProfile: false,
    blockers,
    nextAction: blockers.length === 0
      ? "write the local signing profile, then edit placeholders with real local signing inputs"
      : "provide explicit --source and --output and resolve blockers"
  };
}

function templatePathForSource(source) {
  return templates[source] ? join(workspaceRoot, "desktop", templates[source]) : "";
}

function parseArgs(argv) {
  const options = {
    source: "",
    output: "",
    dryRun: false,
    overwrite: false
  };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (arg === "--source") {
      options.source = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    if (arg === "--output") {
      options.output = requiredValue(argv, index, arg);
      index += 1;
      continue;
    }
    if (arg === "--dry-run") {
      options.dryRun = true;
      continue;
    }
    if (arg === "--overwrite") {
      options.overwrite = true;
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

function isExampleProfile(path) {
  return basename(path).includes(".example.");
}

function isIgnoredSigningProfileByConvention(path) {
  const rel = relative(workspaceRoot, path).split(sep).join("/");
  return rel.startsWith("desktop/signing-profile") &&
    rel.endsWith(".json") &&
    !basename(path).includes(".example.");
}

function safeHint(path) {
  const relativePath = relative(workspaceRoot, resolve(path)).split(sep).join("/");
  if (relativePath.startsWith("..") || isAbsolute(relativePath)) {
    return `${basename(path)}#${sha256Text(resolve(path)).slice(0, 12)}`;
  }
  return `clients/${relativePath}`;
}

function sha256Text(value) {
  return createHash("sha256").update(value).digest("hex");
}

function stringValue(value) {
  return typeof value === "string" ? value.trim() : "";
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
