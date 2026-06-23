# NexusIM Desktop Local Signing Smoke

Date: 2026-06-23

Scope:

- Windows desktop local development signing path.
- Uses the collected Windows desktop executable manifest:
  `clients/artifacts/2026-06-22T214826Z/manifest.json`.
- Signs a temporary copy of the collected artifact only.
- Creates a temporary CurrentUser code-signing certificate, temporarily imports
  it into CurrentUser trusted root, verifies Authenticode status, then removes
  both temporary certificate entries and temporary files.
- Does not mutate the collected artifact, build installers, install artifacts,
  launch the desktop app, start services, start Docker or download toolchains.
- Not a production code-signing certificate, production release signature, MSI /
  NSIS installer smoke or public distribution approval.

Command:

```powershell
npm --prefix clients run smoke:desktop-local-signing -- --manifest clients/artifacts/2026-06-22T214826Z/manifest.json --execute --allow-local-trust-store --require-valid
```

Key evidence:

```text
schemaVersion=nexusim.desktop-local-signing-smoke.v1
artifactKind=desktop-executable
readyToExecute=true
validSignedArtifactCopy=true
setStatus=Valid
verifyStatus=Valid
signer.subject=CN=NexusIM Local Development Signing Smoke
cleanup.currentUserMyRemoved=true
cleanup.currentUserRootRemoved=true
mutatesCollectedArtifact=false
```

Post-run verification:

```text
remaining temporary certificates with subject "CN=NexusIM Local Development Signing Smoke" = 0
original collected artifact sha256 still matches manifest
```

Limits:

- This proves the local Windows Authenticode signing and verification mechanism
  can produce a valid signed temporary copy.
- Release signing still requires a real certificate source, explicit signing
  profile, `sign:desktop-artifact`, `verify:desktop-signature --require-valid`
  and installer-specific signing / verification.
