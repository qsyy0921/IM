$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$serviceCmdRoot = Join-Path $repoRoot "services"

$cmdMainFiles = Get-ChildItem -LiteralPath $serviceCmdRoot -Recurse -Filter "main.go" -File |
    Where-Object { $_.FullName -match "\\cmd\\" }

function Get-RelativePath([string]$Path) {
    $root = (Resolve-Path -LiteralPath $repoRoot).Path.TrimEnd("\", "/")
    $fullPath = (Resolve-Path -LiteralPath $Path).Path
    $prefix = $root + [System.IO.Path]::DirectorySeparatorChar
    if ($fullPath.StartsWith($prefix, [System.StringComparison]::OrdinalIgnoreCase)) {
        return $fullPath.Substring($prefix.Length)
    }
    return $fullPath
}

function Add-Violation([System.Collections.Generic.List[string]]$Violations, [string]$Path, [string]$Message) {
    $Violations.Add("$(Get-RelativePath $Path): $Message")
}

$violations = [System.Collections.Generic.List[string]]::new()
foreach ($file in $cmdMainFiles) {
    $content = Get-Content -LiteralPath $file.FullName -Raw
    $testFile = Join-Path $file.DirectoryName "main_test.go"
    $testContent = ""
    if (Test-Path -LiteralPath $testFile) {
        $testContent = Get-Content -LiteralPath $testFile -Raw
    }

    $usesTrustedMetadataServerAuth = $content.Contains("newGRPCServerWithConfig") -and
        $content.Contains('case "metadata", "verified-metadata"') -and
        ($content -match '"NEXUSIM_[A-Z0-9_]+_AUTH_MODE"')
    if ($usesTrustedMetadataServerAuth) {
        if (-not $content.Contains("validateTrustedMetadataListenerConfig")) {
            Add-Violation $violations $file.FullName "trusted metadata server auth is missing validateTrustedMetadataListenerConfig"
        }
        if (-not $testContent.Contains("TestValidateTrustedMetadataListenerConfigRequiresMTLSForPublicAddress")) {
            Add-Violation $violations $file.FullName "trusted metadata server auth is missing public-address mTLS rejection test"
        }
        if (-not $testContent.Contains("TestValidateTrustedMetadataListenerConfigAllowsMTLSForPublicAddress")) {
            Add-Violation $violations $file.FullName "trusted metadata server auth is missing public-address mTLS allow test"
        }
    }

    $usesTrustedMetadataBackend = $content.Contains("validateTrustedMetadataBackendConfig(")
    if ($usesTrustedMetadataBackend) {
        if (-not $testContent.Contains("TestValidateTrustedMetadataBackendConfigRequiresMTLSForPublicAddress")) {
            Add-Violation $violations $file.FullName "trusted metadata backend config is missing public-address mTLS rejection test"
        }
        if (-not $testContent.Contains("TestValidateTrustedMetadataBackendConfigAllowsMTLSForPublicAddress")) {
            Add-Violation $violations $file.FullName "trusted metadata backend config is missing public-address mTLS allow test"
        }
    }

    $authListenerMatches = [regex]::Matches($content, "validate[A-Za-z0-9]+AuthListenerConfig\(")
    foreach ($match in $authListenerMatches) {
        $guardName = $match.Value.TrimEnd("(")
        if (-not ($testContent.Contains("RejectsPublicAddressForMock") -or $testContent.Contains("RejectsMockAuthOnPublicAddress"))) {
            Add-Violation $violations $file.FullName "$guardName is missing public-address mock-auth rejection test"
        }
        if (-not $testContent.Contains("RejectsPublicAddressForSignedAuthWithoutTLS")) {
            Add-Violation $violations $file.FullName "$guardName is missing public-address signed-auth without TLS rejection test"
        }
        if (-not $testContent.Contains("AllowsPublicAddressForSignedAuthWithTLS")) {
            Add-Violation $violations $file.FullName "$guardName is missing public-address signed-auth with TLS allow test"
        }
        break
    }

    $usesPolicyListenerGuard = $content.Contains("validatePolicyListenerConfig(")
    if ($usesPolicyListenerGuard) {
        if (-not $testContent.Contains("TestValidatePolicyListenerConfigRejectsPublicAddressWithoutTLS")) {
            Add-Violation $violations $file.FullName "policy listener config is missing public-address without TLS rejection test"
        }
        if (-not $testContent.Contains("TestValidatePolicyListenerConfigAllowsPublicAddressWithTLS")) {
            Add-Violation $violations $file.FullName "policy listener config is missing public-address with TLS allow test"
        }
    }
}

if ($violations.Count -gt 0) {
    throw ($violations -join [Environment]::NewLine)
}

Write-Host "OK   public listener auth guardrails"
