$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$serviceCmdRoot = Join-Path $repoRoot "services"

$cmdMainFiles = Get-ChildItem -LiteralPath $serviceCmdRoot -Recurse -Filter "main.go" -File |
    Where-Object { $_.FullName -match "\\cmd\\" }

function Convert-ToRepoRelativePath {
    param([string]$Path)

    $root = [System.IO.Path]::GetFullPath($repoRoot).TrimEnd("\", "/")
    $fullPath = [System.IO.Path]::GetFullPath($Path)
    $prefix = $root + [System.IO.Path]::DirectorySeparatorChar
    if ($fullPath.StartsWith($prefix, [System.StringComparison]::OrdinalIgnoreCase)) {
        return $fullPath.Substring($prefix.Length)
    }
    return $fullPath
}

function Read-CmdPackageProductionSource {
    param([string]$MainPath)

    $cmdDir = Split-Path -Parent $MainPath
    $source = Get-Content -LiteralPath $MainPath -Raw
    $mainFileName = Split-Path -Leaf $MainPath
    $packageFiles = Get-ChildItem -LiteralPath $cmdDir -Filter "*.go" -File |
        Where-Object { $_.Name -ne $mainFileName -and $_.Name -notlike "*_test.go" } |
        Sort-Object Name
    foreach ($packageFile in $packageFiles) {
        $source += "`n" + (Get-Content -LiteralPath $packageFile.FullName -Raw)
    }
    return $source
}

$violations = @()
foreach ($file in $cmdMainFiles) {
    $content = Read-CmdPackageProductionSource -MainPath $file.FullName
    if (-not ($content.Contains("_TLS_REQUIRE_CLIENT_CERT") -and $content.Contains("ConfigFromEnv()"))) {
        continue
    }

    $testFile = Join-Path $file.DirectoryName "main_test.go"
    $testContent = ""
    if (Test-Path -LiteralPath $testFile) {
        $testContent = Get-Content -LiteralPath $testFile -Raw
    }

    $relative = Convert-ToRepoRelativePath -Path $file.FullName
    if ($testContent.Length -eq 0) {
        $violations += "${relative}: server TLS config is missing main_test.go coverage"
        continue
    }

    $requiredFragments = @(
        "RequiresCertKeyPair",
        "RejectsInvalidRequireClientCert",
        "RequiresClientCA",
        "VersionTLS12"
    )
    foreach ($fragment in $requiredFragments) {
        if (-not $testContent.Contains($fragment)) {
            $violations += "${relative}: server TLS config is missing test coverage fragment '$fragment'"
        }
    }

    $supportsClientIdentityAllowlist = $content.Contains("_TLS_CLIENT_ALLOWED_DNS_NAMES") -or
        $content.Contains("_TLS_CLIENT_ALLOWED_URIS") -or
        $content.Contains("VerifyConnection")
    if ($supportsClientIdentityAllowlist) {
        if ($testContent -notmatch "AllowsClient(Identity|DNSName|URI)") {
            $violations += "${relative}: server TLS client identity allowlist is missing positive allow test"
        }
        if (-not $testContent.Contains("RejectsUnlistedClientIdentity")) {
            $violations += "${relative}: server TLS client identity allowlist is missing unlisted-client rejection test"
        }
    }
}

if ($violations.Count -gt 0) {
    throw ($violations -join [Environment]::NewLine)
}

Write-Host "OK   gRPC/WSS TLS config guardrails"
