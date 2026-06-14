$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$searchRoots = @(
    (Join-Path $repoRoot "services"),
    (Join-Path $repoRoot "internal")
) | Where-Object { Test-Path -LiteralPath $_ }

if ($searchRoots.Count -eq 0) {
    Write-Host "OK   gRPC correlation log guardrails (no search roots)"
    exit 0
}

function Convert-ToRepoRelativePath {
    param([string]$Path)

    $root = [System.IO.Path]::GetFullPath($repoRoot).TrimEnd("\") + "\"
    $fullPath = [System.IO.Path]::GetFullPath($Path)
    if ($fullPath.StartsWith($root, [System.StringComparison]::OrdinalIgnoreCase)) {
        return $fullPath.Substring($root.Length) -replace "/", "\"
    }
    return $fullPath -replace "/", "\"
}

$productionGoFiles = Get-ChildItem -LiteralPath $searchRoots -Recurse -Filter "*.go" -File |
    Where-Object { $_.Name -notlike "*_test.go" }

$violations = @()
foreach ($file in $productionGoFiles) {
    $content = Get-Content -LiteralPath $file.FullName -Raw
    $relative = Convert-ToRepoRelativePath -Path $file.FullName

    if ($content -match "\btrimGRPCLogMetadata\s*\(") {
        $violations += "${relative}: trimGRPCLogMetadata only trims metadata; use sanitizeGRPCLogMetadata before logging correlation ids."
    }

    if ($content -match '"grpc_request"' -and
        $content -match "\bmetadataTraceID\b" -and
        $content -notmatch "\bsanitizeGRPCLogMetadata\s*\(") {
        $violations += "${relative}: grpc_request correlation metadata must be sanitized with sanitizeGRPCLogMetadata before logging."
    }
}

if ($violations.Count -gt 0) {
    throw ($violations -join [Environment]::NewLine)
}

Write-Host "OK   gRPC correlation log guardrails"
