import { existsSync, readFileSync } from "node:fs";
import { resolve } from "node:path";

export const desktopSigningProfileSchema = "nexusim.desktop-signing-profile.v1";
export const defaultPfxPassEnv = "NEXUSIM_DESKTOP_SIGN_PFX_PASS";
export const signingProfileEnv = "NEXUSIM_DESKTOP_SIGNING_PROFILE";

export function applyDesktopSigningProfile(options = {}, env = process.env) {
  const profilePath = stringValue(options.signingProfile || env[signingProfileEnv]);
  const merged = {
    ...options,
    pfxPassEnv: stringValue(options.pfxPassEnv) || defaultPfxPassEnv
  };
  if (!profilePath) {
    return withPfxEnvPresence(merged, env);
  }

  const profile = readDesktopSigningProfile(profilePath);
  merged.signingProfile = profilePath;
  merged.signToolPath = stringValue(merged.signToolPath) || profile.signToolPath;
  merged.timestampURL = stringValue(merged.timestampURL) || profile.timestampURL;
  merged.certFile = stringValue(merged.certFile) || profile.certFile;
  merged.certSHA1 = stringValue(merged.certSHA1) || profile.certSHA1;
  merged.expectedSignerSubjectContains = stringValue(merged.expectedSignerSubjectContains) || profile.expectedSignerSubjectContains;
  merged.pfxPassEnv = stringValue(merged.pfxPassEnv) || profile.pfxPassEnv || defaultPfxPassEnv;
  if (profile.pfxPassEnv && (!stringValue(options.pfxPassEnv) || stringValue(options.pfxPassEnv) === defaultPfxPassEnv)) {
    merged.pfxPassEnv = profile.pfxPassEnv;
  }
  return withPfxEnvPresence(merged, env);
}

export function readDesktopSigningProfile(profilePath) {
  const resolved = resolve(profilePath);
  if (!existsSync(resolved)) {
    throw new Error("desktop signing profile file missing");
  }
  const raw = readFileSync(resolved, "utf8");
  assertProfileText(raw);
  const profile = JSON.parse(raw);
  assertNoSensitiveKeys(profile);
  if (profile.schemaVersion !== desktopSigningProfileSchema) {
    throw new Error("desktop signing profile schema mismatch");
  }
  const certificate = profile.certificate ?? {};
  const source = stringValue(certificate.source);
  const certFile = stringValue(certificate.certFile);
  const certSHA1 = normalizeThumbprint(certificate.certSHA1);
  if (source !== "pfx-file" && source !== "windows-cert-store") {
    throw new Error("desktop signing profile certificate source invalid");
  }
  if (source === "pfx-file" && (!certFile || certSHA1)) {
    throw new Error("desktop signing profile pfx certificate invalid");
  }
  if (source === "windows-cert-store" && (!certSHA1 || certFile)) {
    throw new Error("desktop signing profile cert-store certificate invalid");
  }
  const timestampURL = stringValue(profile.timestampURL);
  if (timestampURL && !isSafeTimestampURL(timestampURL)) {
    throw new Error("desktop signing profile timestamp URL invalid");
  }
  const expectedSignerSubjectContains = stringValue(profile.signature?.expectedSignerSubjectContains);
  if (expectedSignerSubjectContains && !isSafeExpectedSignerSubject(expectedSignerSubjectContains)) {
    throw new Error("desktop signing profile expected signer subject invalid");
  }
  const pfxPassEnv = stringValue(certificate.pfxPassEnv || defaultPfxPassEnv);
  if (!pfxPassEnv.match(/^[A-Z][A-Z0-9_]{2,}$/)) {
    throw new Error("desktop signing profile pfx pass env invalid");
  }
  return {
    signToolPath: stringValue(profile.signToolPath),
    timestampURL,
    certFile,
    certSHA1,
    pfxPassEnv,
    expectedSignerSubjectContains
  };
}

export function isSafeTimestampURL(value) {
  try {
    const parsed = new URL(value);
    return (
      (parsed.protocol === "http:" || parsed.protocol === "https:") &&
      parsed.username === "" &&
      parsed.password === "" &&
      parsed.search === "" &&
      parsed.hash === ""
    );
  } catch {
    return false;
  }
}

function withPfxEnvPresence(options, env) {
  const pfxPassEnv = stringValue(options.pfxPassEnv) || defaultPfxPassEnv;
  return {
    ...options,
    pfxPassEnv,
    pfxPassEnvPresent: Boolean(env[pfxPassEnv])
  };
}

function isSafeExpectedSignerSubject(value) {
  return value.length >= 3 && value.length <= 160 && !value.match(/[\r\n]/);
}

function assertProfileText(raw) {
  if (raw.match(/BEGIN (RSA |EC |DSA |OPENSSH )?PRIVATE KEY/i)) {
    throw new Error("desktop signing profile must not contain private key material");
  }
}

function assertNoSensitiveKeys(value) {
  if (!value || typeof value !== "object") {
    return;
  }
  for (const [key, nested] of Object.entries(value)) {
    if (key.match(/(token|secret|password|credential|private)/i)) {
      throw new Error("desktop signing profile contains a sensitive field name");
    }
    assertNoSensitiveKeys(nested);
  }
}

function normalizeThumbprint(value) {
  return stringValue(value).replace(/\s+/g, "").toUpperCase();
}

function stringValue(value) {
  return typeof value === "string" ? value.trim() : "";
}
