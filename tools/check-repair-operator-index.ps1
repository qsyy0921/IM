$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$runbookIndexPath = Join-Path $repoRoot "docs\runbook\README.md"
$repairIndexPath = Join-Path $repoRoot "docs\runbook\repair-operators.md"

foreach ($path in @($runbookIndexPath, $repairIndexPath)) {
    if (-not (Test-Path -LiteralPath $path)) {
        throw "Missing repair operator documentation file: $path"
    }
}

$runbookIndex = Get-Content -LiteralPath $runbookIndexPath -Raw
$repairIndex = Get-Content -LiteralPath $repairIndexPath -Raw

if ($runbookIndex -notmatch [regex]::Escape("repair-operators.md")) {
    throw "docs/runbook/README.md must link repair-operators.md."
}

$requiredTerms = @(
    "message-service",
    "delivery-service",
    "receipt-service",
    "contacts-service",
    "policy-service",
    "conversation-service",
    "identity-service",
    "outbox-audit",
    "outbox-repair",
    "outbox-repair-audit",
    "outbox-repair-cleanup",
    "projection-failure-audit",
    "projection-checkpoint-repair",
    "member-change-audit",
    "session-mfa-proof-audit",
    "challenge-delivery-repair",
    "tenant-privacy-default-audit",
    "source-policy-audit",
    "operator",
    "repair",
    "audit",
    "SQL"
)

foreach ($term in $requiredTerms) {
    if ($repairIndex -notmatch [regex]::Escape($term)) {
        throw "docs/runbook/repair-operators.md missing required repair operator term: $term"
    }
}

Write-Host "OK   repair operator index guardrails"
