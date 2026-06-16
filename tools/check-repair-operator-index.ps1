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
        Modes = @("outbox-audit", "outbox-repair", "outbox-repair-audit", "outbox-repair-cleanup", "change-history-audit")
        OutputEnvs = @(
            "NEXUSIM_MESSAGE_OUTBOX_AUDIT_OUTPUT",
            "NEXUSIM_MESSAGE_OUTBOX_REPAIR_OUTPUT",
            "NEXUSIM_MESSAGE_OUTBOX_REPAIR_AUDIT_OUTPUT",
            "NEXUSIM_MESSAGE_OUTBOX_REPAIR_CLEANUP_OUTPUT",
            "NEXUSIM_MESSAGE_CHANGE_HISTORY_AUDIT_OUTPUT"
        )
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
        OutputEnvs = @(
            "NEXUSIM_DELIVERY_OUTBOX_AUDIT_OUTPUT",
            "NEXUSIM_DELIVERY_OUTBOX_REPAIR_OUTPUT",
            "NEXUSIM_DELIVERY_OUTBOX_REPAIR_AUDIT_OUTPUT",
            "NEXUSIM_DELIVERY_OUTBOX_REPAIR_CLEANUP_OUTPUT",
            "NEXUSIM_DELIVERY_PROJECTION_FAILURE_AUDIT_OUTPUT",
            "NEXUSIM_DELIVERY_PROJECTION_REPAIR_OUTPUT",
            "NEXUSIM_DELIVERY_PROJECTION_REPAIR_AUDIT_OUTPUT",
            "NEXUSIM_DELIVERY_PROJECTION_REPAIR_CLEANUP_OUTPUT",
            "NEXUSIM_DELIVERY_PROJECTION_FAILURE_CLEANUP_OUTPUT"
        )
    },
    @{
        Service = "receipt-service"
        Cmd = "services\receipt-service\cmd\receipt-service\main.go"
        Env = "NEXUSIM_RECEIPT_SERVICE_MODE"
        Modes = @("outbox-audit", "outbox-repair", "outbox-repair-audit", "outbox-repair-cleanup")
        OutputEnvs = @(
            "NEXUSIM_RECEIPT_OUTBOX_AUDIT_OUTPUT",
            "NEXUSIM_RECEIPT_OUTBOX_REPAIR_OUTPUT",
            "NEXUSIM_RECEIPT_OUTBOX_REPAIR_AUDIT_OUTPUT",
            "NEXUSIM_RECEIPT_OUTBOX_REPAIR_CLEANUP_OUTPUT"
        )
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
        OutputEnvs = @(
            "NEXUSIM_CONTACTS_OUTBOX_AUDIT_OUTPUT",
            "NEXUSIM_CONTACTS_OUTBOX_REPAIR_OUTPUT",
            "NEXUSIM_CONTACTS_OUTBOX_REPAIR_AUDIT_OUTPUT",
            "NEXUSIM_CONTACTS_OUTBOX_REPAIR_CLEANUP_OUTPUT",
            "NEXUSIM_CONTACTS_TENANT_PRIVACY_AUDIT_OUTPUT",
            "NEXUSIM_CONTACTS_TENANT_PRIVACY_SET_OUTPUT",
            "NEXUSIM_CONTACTS_SOURCE_POLICY_AUDIT_OUTPUT",
            "NEXUSIM_CONTACTS_SOURCE_POLICY_SET_OUTPUT"
        )
    },
    @{
        Service = "policy-service"
        Cmd = "services\policy-service\cmd\policy-service\main.go"
        Env = "NEXUSIM_POLICY_SERVICE_MODE"
        Modes = @("outbox-audit", "outbox-repair", "outbox-repair-audit", "outbox-repair-cleanup")
        OutputEnvs = @(
            "NEXUSIM_POLICY_OUTBOX_AUDIT_OUTPUT",
            "NEXUSIM_POLICY_OUTBOX_REPAIR_OUTPUT",
            "NEXUSIM_POLICY_OUTBOX_REPAIR_AUDIT_OUTPUT",
            "NEXUSIM_POLICY_OUTBOX_REPAIR_CLEANUP_OUTPUT"
        )
    },
    @{
        Service = "conversation-service"
        Cmd = "services\conversation-service\cmd\conversation-service\main.go"
        Env = "NEXUSIM_CONVERSATION_SERVICE_MODE"
        Modes = @("member-change-audit")
        OutputEnvs = @(
            "NEXUSIM_CONVERSATION_MEMBER_CHANGE_AUDIT_OUTPUT"
        )
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
        ExtraCmdFiles = @(
            "services\identity-service\cmd\identity-service\gateway_token_config.go"
        )
        OutputEnvs = @(
            "NEXUSIM_IDENTITY_SESSION_MFA_PROOF_AUDIT_OUTPUT",
            "NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_OUTPUT",
            "NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_AUDIT_OUTPUT",
            "NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_CLEANUP_OUTPUT",
            "NEXUSIM_IDENTITY_CHALLENGE_REQUEST_LIMIT_CLEANUP_OUTPUT",
            "NEXUSIM_IDENTITY_GATEWAY_TOKEN_KEYRING_ROTATE_OUTPUT"
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
    foreach ($extraCmd in @($spec.ExtraCmdFiles)) {
        $extraCmd = [string]$extraCmd
        if ([string]::IsNullOrWhiteSpace($extraCmd)) {
            continue
        }
        $extraCmdPath = Join-Path $repoRoot $extraCmd
        if (-not (Test-Path -LiteralPath $extraCmdPath -PathType Leaf)) {
            throw "Missing extra service cmd file for repair operator check: $extraCmd"
        }
        $cmd += "`n" + (Get-Content -LiteralPath $extraCmdPath -Raw)
    }
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

    foreach ($outputEnv in @($spec.OutputEnvs)) {
        $outputEnv = [string]$outputEnv
        if (-not $repairIndex.Contains($outputEnv)) {
            throw "docs/runbook/repair-operators.md missing documented output env for ${service}: $outputEnv"
        }
        if (-not $cmd.Contains($outputEnv)) {
            throw "$($spec.Cmd) missing documented output env for ${service}: $outputEnv"
        }
    }
}

$operatorModePattern = "(audit|repair|cleanup|keyring-rotate|tenant-privacy-default-set|source-policy-set)"
$serviceCommandFiles = Get-ChildItem -LiteralPath (Join-Path $repoRoot "services") -Recurse -Filter "main.go" -File |
    Where-Object { $_.FullName -like "*\cmd\*" } |
    Sort-Object FullName

foreach ($commandFile in $serviceCommandFiles) {
    $relativeCommandPath = Resolve-Path -LiteralPath $commandFile.FullName -Relative
    $commandText = Get-Content -LiteralPath $commandFile.FullName -Raw
    $modeMatches = [regex]::Matches($commandText, 'case\s+"([^"]+)"')
    foreach ($match in $modeMatches) {
        $mode = [string]$match.Groups[1].Value
        if ($mode -match $operatorModePattern -and -not $repairIndex.Contains($mode)) {
            throw "docs/runbook/repair-operators.md missing discovered operator mode ${mode} from ${relativeCommandPath}."
        }
    }
}

Write-Host "OK   repair operator index guardrails"
