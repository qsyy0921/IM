$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$forbiddenKeys = @(
    "nexusim.trace_id",
    "nexusim.request_id",
    "nexusim.tenant_id",
    "nexusim.user_id",
    "nexusim.device_id",
    "nexusim.session_id",
    "nexusim.conversation_id",
    "nexusim.message_id"
)

$productionGoFiles = Get-ChildItem -LiteralPath (Join-Path $repoRoot "services") -Recurse -Filter "*.go" -File |
    Where-Object { $_.Name -notlike "*_test.go" }

$violations = @()
foreach ($file in $productionGoFiles) {
    $content = Get-Content -LiteralPath $file.FullName -Raw
    foreach ($key in $forbiddenKeys) {
        if ($content.Contains("attribute.String(`"$key`"") -or
            $content.Contains("attribute.Stringer(`"$key`"") -or
            $content.Contains("attribute.Int(`"$key`"") -or
            $content.Contains("attribute.Int64(`"$key`"") -or
            $content.Contains("attribute.Bool(`"$key`"")) {
            $relative = [System.IO.Path]::GetRelativePath($repoRoot, $file.FullName)
            $violations += "${relative}: forbidden high-cardinality OTel span attribute '$key'"
        }
    }
}

if ($violations.Count -gt 0) {
    throw ($violations -join [Environment]::NewLine)
}

Write-Host "OK   OTel span attribute guardrails"
