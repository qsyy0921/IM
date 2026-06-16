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

$operatorSpecs = @(
    @{
        Service = "message-service"
        Cmd = "services\message-service\cmd\message-service\main.go"
        Env = "NEXUSIM_MESSAGE_SERVICE_MODE"
        Modes = @("outbox-audit", "outbox-repair", "outbox-repair-audit", "outbox-repair-cleanup")
    },
    @{
        Service = "delivery-service"
        Cmd = "services\delivery-service\cmd\delivery-service\main.go"
        Env = "NEXUSIM_DELIVERY_SERVICE_MODE"
        Modes = @(
            "outbox-audit",
            "outbox-repair",
            "outbox-repair-audit",
            "outbox-repair-cleanup",
            "projection-failure-audit",
            "projection-checkpoint-repair",
            "projection-checkpoint-repair-audit",
            "projection-checkpoint-repair-cleanup",
            "projection-failure-cleanup"
        )
    },
    @{
        Service = "receipt-service"
        Cmd = "services\receipt-service\cmd\receipt-service\main.go"
        Env = "NEXUSIM_RECEIPT_SERVICE_MODE"
        Modes = @("outbox-audit", "outbox-repair", "outbox-repair-audit", "outbox-repair-cleanup")
    },
    @{
        Service = "contacts-service"
        Cmd = "services\contacts-service\cmd\contacts-service\main.go"
        Env = "NEXUSIM_CONTACTS_SERVICE_MODE"
        Modes = @(
            "outbox-audit",
            "outbox-repair",
            "outbox-repair-audit",
            "outbox-repair-cleanup",
            "tenant-privacy-default-audit",
            "tenant-privacy-default-set",
            "source-policy-audit",
            "source-policy-set"
        )
    },
    @{
        Service = "policy-service"
        Cmd = "services\policy-service\cmd\policy-service\main.go"
        Env = "NEXUSIM_POLICY_SERVICE_MODE"
        Modes = @("outbox-audit", "outbox-repair", "outbox-repair-audit", "outbox-repair-cleanup")
    },
    @{
        Service = "conversation-service"
        Cmd = "services\conversation-service\cmd\conversation-service\main.go"
        Env = "NEXUSIM_CONVERSATION_SERVICE_MODE"
        Modes = @("member-change-audit")
    },
    @{
        Service = "identity-service"
        Cmd = "services\identity-service\cmd\identity-service\main.go"
        Env = "NEXUSIM_IDENTITY_SERVICE_MODE"
        Modes = @(
            "session-mfa-proof-audit",
            "challenge-delivery-repair",
            "challenge-delivery-repair-audit",
            "challenge-delivery-repair-cleanup",
            "challenge-request-limit-cleanup",
            "gateway-token-keyring-rotate"
        )
    }
)

$requiredSharedTerms = @("operator", "repair", "audit", "SQL")
foreach ($term in $requiredSharedTerms) {
    if (-not $repairIndex.Contains($term)) {
        throw "docs/runbook/repair-operators.md missing required repair operator term: $term"
    }
}

foreach ($spec in $operatorSpecs) {
    $service = [string]$spec.Service
    $cmdPath = Join-Path $repoRoot ([string]$spec.Cmd)
    if (-not (Test-Path -LiteralPath $cmdPath -PathType Leaf)) {
        throw "Missing service cmd file for repair operator check: $($spec.Cmd)"
    }

    $cmd = Get-Content -LiteralPath $cmdPath -Raw
    $envName = [string]$spec.Env
    if (-not $repairIndex.Contains($service)) {
        throw "docs/runbook/repair-operators.md missing service: $service"
    }
    if (-not $repairIndex.Contains($envName)) {
        throw "docs/runbook/repair-operators.md missing env var for ${service}: $envName"
    }
    if (-not $cmd.Contains($envName)) {
        throw "$($spec.Cmd) missing env var referenced by repair index: $envName"
    }

    foreach ($mode in @($spec.Modes)) {
        $mode = [string]$mode
        if (-not $repairIndex.Contains($mode)) {
            throw "docs/runbook/repair-operators.md missing documented mode for ${service}: $mode"
        }
        if (-not $cmd.Contains("`"$mode`"")) {
            throw "$($spec.Cmd) missing documented repair operator mode for ${service}: $mode"
        }
    }
}

Write-Host "OK   repair operator index guardrails"
