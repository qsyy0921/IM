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

$violations = @()
foreach ($file in $cmdMainFiles) {
    $content = Get-Content -LiteralPath $file.FullName -Raw
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
        "RequiresClientCA"
    )
    foreach ($fragment in $requiredFragments) {
        if (-not $testContent.Contains($fragment)) {
            $violations += "${relative}: server TLS config is missing test coverage fragment '$fragment'"
        }
    }
}

if ($violations.Count -gt 0) {
    throw ($violations -join [Environment]::NewLine)
}

Write-Host "OK   gRPC/WSS TLS config guardrails"
