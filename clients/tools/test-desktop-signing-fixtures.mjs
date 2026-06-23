import { execFileSync } from "node:child_process";
import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { defaultPfxPassEnv } from "./desktop-signing-profile.mjs";

export const testPfxPassEnv = "NEXUSIM_TEST_DESKTOP_PFX_PASS";
export const testPfxValue = "NexusIM-Test-Code-Signing-Value-2026!";

export function createTemporaryCodeSigningPfx(tempRoot, filename = "nexusim-test-signing.pfx") {
  const pfxPath = join(tempRoot, filename);
  if (process.platform !== "win32") {
    writeFileSync(pfxPath, "non-windows placeholder pfx");
    return {
      pfxPath,
      pfxPassEnv: testPfxPassEnv,
      env: {
        [defaultPfxPassEnv]: testPfxValue,
        [testPfxPassEnv]: testPfxValue
      },
      pfxCertificateProbe: usablePfxCertificateProbe
    };
  }
  const script = [
    "$ErrorActionPreference = 'Stop'",
    "$path = $env:NEXUSIM_TEST_PFX_PATH",
    "$value = $env:NEXUSIM_TEST_PFX_VALUE",
    "$rsa = [System.Security.Cryptography.RSA]::Create(2048)",
    "$hash = [System.Security.Cryptography.HashAlgorithmName]::SHA256",
    "$padding = [System.Security.Cryptography.RSASignaturePadding]::Pkcs1",
    "$req = [System.Security.Cryptography.X509Certificates.CertificateRequest]::new('CN=NexusIM Test Code Signing', $rsa, $hash, $padding)",
    "$usage = [System.Security.Cryptography.X509Certificates.X509KeyUsageFlags]::DigitalSignature",
    "$req.CertificateExtensions.Add([System.Security.Cryptography.X509Certificates.X509KeyUsageExtension]::new($usage, $false))",
    "$oids = [System.Security.Cryptography.OidCollection]::new()",
    "[void]$oids.Add([System.Security.Cryptography.Oid]::new('1.3.6.1.5.5.7.3.3'))",
    "$req.CertificateExtensions.Add([System.Security.Cryptography.X509Certificates.X509EnhancedKeyUsageExtension]::new($oids, $false))",
    "$cert = $req.CreateSelfSigned([datetimeoffset]::UtcNow.AddDays(-1), [datetimeoffset]::UtcNow.AddDays(30))",
    "$bytes = $cert.Export([System.Security.Cryptography.X509Certificates.X509ContentType]::Pfx, $value)",
    "[System.IO.File]::WriteAllBytes($path, $bytes)"
  ].join("; ");
  execFileSync("powershell", ["-NoProfile", "-Command", script], {
    env: {
      ...process.env,
      NEXUSIM_TEST_PFX_PATH: pfxPath,
      NEXUSIM_TEST_PFX_VALUE: testPfxValue
    },
    windowsHide: true,
    stdio: ["ignore", "ignore", "pipe"]
  });
  return {
    pfxPath,
    pfxPassEnv: testPfxPassEnv,
    env: {
      [defaultPfxPassEnv]: testPfxValue,
      [testPfxPassEnv]: testPfxValue
    },
    pfxCertificateProbe: usablePfxCertificateProbe
  };
}

export function usablePfxCertificateProbe() {
  return {
    readable: true,
    signingKeyAvailable: true,
    notAfter: "2035-01-01T00:00:00.000Z"
  };
}
