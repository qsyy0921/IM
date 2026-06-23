import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import {
  applyDesktopSigningProfile,
  defaultPfxPassEnv,
  desktopSigningProfileSchema,
  isSafeTimestampURL,
  readDesktopSigningProfile,
  signingProfileEnv
} from "./desktop-signing-profile.mjs";

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

function writeProfile(path, profile) {
  writeFileSync(path, `${JSON.stringify(profile, null, 2)}\n`);
}

function expectError(fn, expectedMessage) {
  try {
    fn();
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    assert(message.includes(expectedMessage), `expected error ${expectedMessage}, got ${message}`);
    return;
  }
  throw new Error(`expected error: ${expectedMessage}`);
}

const tempRoot = mkdtempSync(join(tmpdir(), "nexusim-desktop-signing-profile-"));
try {
  const certDir = join(tempRoot, "private-path-is-not-a-secret-field");
  mkdirSync(certDir, { recursive: true });
  const fakeSignTool = join(tempRoot, "signtool.exe");
  const fakePfx = join(certDir, "nexusim-signing.pfx");
  writeFileSync(fakeSignTool, "fake signtool");
  writeFileSync(fakePfx, "fake pfx");

  const pfxProfile = join(tempRoot, "pfx-profile.json");
  writeProfile(pfxProfile, {
    schemaVersion: desktopSigningProfileSchema,
    signToolPath: fakeSignTool,
    timestampURL: "https://timestamp.example.test",
    signature: {
      expectedSignerSubjectContains: "NexusIM"
    },
    certificate: {
      source: "pfx-file",
      certFile: fakePfx,
      pfxPassEnv: "NEXUSIM_TEST_DESKTOP_PFX_PASS"
    }
  });
  const parsedPfx = readDesktopSigningProfile(pfxProfile);
  assert(parsedPfx.signToolPath === fakeSignTool, "pfx profile signtool should parse");
  assert(parsedPfx.certFile === fakePfx, "pfx profile cert file should parse");
  assert(parsedPfx.certSHA1 === "", "pfx profile must not set cert-store thumbprint");
  assert(parsedPfx.pfxPassEnv === "NEXUSIM_TEST_DESKTOP_PFX_PASS", "pfx profile env should parse");
  assert(parsedPfx.expectedSignerSubjectContains === "NexusIM", "expected signer subject should parse");
  assert(isSafeTimestampURL("https://timestamp.example.test/path"), "safe timestamp URL should pass");
  assert(!isSafeTimestampURL("https://timestamp.example.test/path?token=abc"), "timestamp URL query should fail closed");
  assert(!isSafeTimestampURL("https://user:pass@timestamp.example.test"), "timestamp URL credentials should fail closed");

  const mergedPfx = applyDesktopSigningProfile({ signingProfile: pfxProfile }, {
    NEXUSIM_TEST_DESKTOP_PFX_PASS: "present"
  });
  assert(mergedPfx.signToolPath === fakeSignTool, "pfx profile should merge signtool");
  assert(mergedPfx.certFile === fakePfx, "pfx profile should merge cert file");
  assert(mergedPfx.expectedSignerSubjectContains === "NexusIM", "pfx profile should merge expected signer subject");
  assert(mergedPfx.pfxPassEnv === "NEXUSIM_TEST_DESKTOP_PFX_PASS", "pfx profile should merge custom env");
  assert(mergedPfx.pfxPassEnvPresent === true, "pfx profile should report env presence");

  const envSelected = applyDesktopSigningProfile({}, {
    [signingProfileEnv]: pfxProfile,
    NEXUSIM_TEST_DESKTOP_PFX_PASS: "present"
  });
  assert(envSelected.signingProfile === pfxProfile, "env-selected profile path should be recorded");
  assert(envSelected.pfxPassEnvPresent === true, "env-selected profile should report custom env presence");

  const callerEnvOverride = applyDesktopSigningProfile({
    signingProfile: pfxProfile,
    pfxPassEnv: "NEXUSIM_CALLER_PFX_PASS"
  }, {
    NEXUSIM_CALLER_PFX_PASS: "present"
  });
  assert(callerEnvOverride.pfxPassEnv === "NEXUSIM_CALLER_PFX_PASS", "caller pfx env should override profile env");
  assert(callerEnvOverride.pfxPassEnvPresent === true, "caller pfx env should drive presence");

  const defaultEnvOnly = applyDesktopSigningProfile({}, {
    [defaultPfxPassEnv]: "present"
  });
  assert(defaultEnvOnly.pfxPassEnv === defaultPfxPassEnv, "default pfx env should be used without profile");
  assert(defaultEnvOnly.pfxPassEnvPresent === true, "default pfx env presence should be reported");

  const storeProfile = join(tempRoot, "store-profile.json");
  writeProfile(storeProfile, {
    schemaVersion: desktopSigningProfileSchema,
    signToolPath: fakeSignTool,
    timestampURL: "https://timestamp.example.test",
    certificate: {
      source: "windows-cert-store",
      certSHA1: "aa bb cc dd ee ff 00 11 22 33 44 55 66 77 88 99 aa bb cc dd"
    }
  });
  const parsedStore = readDesktopSigningProfile(storeProfile);
  assert(parsedStore.certFile === "", "store profile must not set pfx file");
  assert(parsedStore.certSHA1 === "AABBCCDDEEFF00112233445566778899AABBCCDD", "store thumbprint should normalize");
  assert(parsedStore.pfxPassEnv === defaultPfxPassEnv, "store profile should keep default pfx env for uniform output");

  const badSchema = join(tempRoot, "bad-schema.json");
  writeProfile(badSchema, {
    schemaVersion: "other",
    certificate: {
      source: "windows-cert-store",
      certSHA1: "AABBCCDDEEFF00112233445566778899AABBCCDD"
    }
  });
  expectError(() => readDesktopSigningProfile(badSchema), "schema mismatch");

  const mixedSource = join(tempRoot, "mixed-source.json");
  writeProfile(mixedSource, {
    schemaVersion: desktopSigningProfileSchema,
    certificate: {
      source: "pfx-file",
      certFile: fakePfx,
      certSHA1: "AABBCCDDEEFF00112233445566778899AABBCCDD"
    }
  });
  expectError(() => readDesktopSigningProfile(mixedSource), "pfx certificate invalid");

  const badEnv = join(tempRoot, "bad-env.json");
  writeProfile(badEnv, {
    schemaVersion: desktopSigningProfileSchema,
    certificate: {
      source: "pfx-file",
      certFile: fakePfx,
      pfxPassEnv: "bad-env-name"
    }
  });
  expectError(() => readDesktopSigningProfile(badEnv), "pfx pass env invalid");

  const unsafeTimestamp = join(tempRoot, "unsafe-timestamp.json");
  writeProfile(unsafeTimestamp, {
    schemaVersion: desktopSigningProfileSchema,
    signToolPath: fakeSignTool,
    timestampURL: "https://timestamp.example.test/path?token=do-not-store",
    certificate: {
      source: "pfx-file",
      certFile: fakePfx,
      pfxPassEnv: "NEXUSIM_TEST_DESKTOP_PFX_PASS"
    }
  });
  expectError(() => readDesktopSigningProfile(unsafeTimestamp), "timestamp URL invalid");

  const badExpectedSigner = join(tempRoot, "bad-expected-signer.json");
  writeProfile(badExpectedSigner, {
    schemaVersion: desktopSigningProfileSchema,
    signature: {
      expectedSignerSubjectContains: "CN=Bad\nSigner"
    },
    certificate: {
      source: "windows-cert-store",
      certSHA1: "AABBCCDDEEFF00112233445566778899AABBCCDD"
    }
  });
  expectError(() => readDesktopSigningProfile(badExpectedSigner), "expected signer subject invalid");

  const sensitiveField = join(tempRoot, "sensitive-field.json");
  writeProfile(sensitiveField, {
    schemaVersion: desktopSigningProfileSchema,
    certificate: {
      source: "pfx-file",
      certFile: fakePfx,
      pfxPassword: "do-not-store"
    }
  });
  expectError(() => readDesktopSigningProfile(sensitiveField), "sensitive field name");

  const privateKeyMaterial = join(tempRoot, "private-key-material.json");
  writeProfile(privateKeyMaterial, {
    schemaVersion: desktopSigningProfileSchema,
    signToolPath: "-----BEGIN PRIVATE KEY-----",
    certificate: {
      source: "windows-cert-store",
      certSHA1: "AABBCCDDEEFF00112233445566778899AABBCCDD"
    }
  });
  expectError(() => readDesktopSigningProfile(privateKeyMaterial), "private key material");
} finally {
  rmSync(tempRoot, { recursive: true, force: true });
}

console.log("desktop signing profile ok");
