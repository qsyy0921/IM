$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$runbookIndexPath = Join-Path $repoRoot "docs\runbook\README.md"
$repairIndexPath = Join-Path $repoRoot "docs\runbook\repair-operators.md"
$repairCatalogPath = Join-Path $repoRoot "docs\runbook\repair-operators.catalog.json"

foreach ($path in @($runbookIndexPath, $repairIndexPath, $repairCatalogPath)) {
    if (-not (Test-Path -LiteralPath $path)) {
        throw "Missing repair operator documentation file: $path"
    }
}

$runbookIndex = Get-Content -LiteralPath $runbookIndexPath -Raw
$repairIndex = Get-Content -LiteralPath $repairIndexPath -Raw
$repairCatalog = Get-Content -LiteralPath $repairCatalogPath -Raw | ConvertFrom-Json

if ($runbookIndex -notmatch [regex]::Escape("repair-operators.md")) {
    throw "docs/runbook/README.md must link repair-operators.md."
}
if ($runbookIndex -notmatch [regex]::Escape("repair-operators.catalog.json")) {
    throw "docs/runbook/README.md must link repair-operators.catalog.json."
}

$operatorSpecs = @(
    @{
        Service = "message-service"
        Cmd = "services\message-service\cmd\message-service\main.go"
        Env = "NEXUSIM_MESSAGE_SERVICE_MODE"
        Modes = @(
            "outbox-audit",
            "outbox-repair",
            "outbox-repair-audit",
            "outbox-repair-cleanup",
            "change-history-audit",
            "retention-proof-audit",
            "legal-hold-audit",
            "legal-hold-set",
            "legal-hold-release",
            "compliance-proof-audit",
            "compliance-proof-register",
            "compliance-proof-revoke",
            "compliance-approval-audit",
            "compliance-approval-create",
            "compliance-approval-cancel"
        )
        OutputEnvs = @(
            "NEXUSIM_MESSAGE_OUTBOX_AUDIT_OUTPUT",
            "NEXUSIM_MESSAGE_OUTBOX_REPAIR_OUTPUT",
            "NEXUSIM_MESSAGE_OUTBOX_REPAIR_AUDIT_OUTPUT",
            "NEXUSIM_MESSAGE_OUTBOX_REPAIR_CLEANUP_OUTPUT",
            "NEXUSIM_MESSAGE_CHANGE_HISTORY_AUDIT_OUTPUT",
            "NEXUSIM_MESSAGE_RETENTION_PROOF_AUDIT_OUTPUT",
            "NEXUSIM_MESSAGE_LEGAL_HOLD_AUDIT_OUTPUT",
            "NEXUSIM_MESSAGE_LEGAL_HOLD_OUTPUT",
            "NEXUSIM_MESSAGE_COMPLIANCE_PROOF_AUDIT_OUTPUT",
            "NEXUSIM_MESSAGE_COMPLIANCE_PROOF_OUTPUT",
            "NEXUSIM_MESSAGE_COMPLIANCE_APPROVAL_AUDIT_OUTPUT",
            "NEXUSIM_MESSAGE_COMPLIANCE_APPROVAL_OUTPUT"
        )
        DryRunEnvs = @(
            "NEXUSIM_MESSAGE_OUTBOX_REPAIR_CLEANUP_DRY_RUN"
        )
        ReasonFileEnvs = @(
            "NEXUSIM_MESSAGE_OUTBOX_REPAIR_REASON_FILE",
            "NEXUSIM_MESSAGE_LEGAL_HOLD_REASON_FILE",
            "NEXUSIM_MESSAGE_COMPLIANCE_APPROVAL_REASON_FILE"
        )
        ExtraCmdFiles = @(
            "services\message-service\cmd\message-service\message_legal_hold_operator.go",
            "services\message-service\cmd\message-service\message_compliance_proof_operator.go",
            "services\message-service\cmd\message-service\message_compliance_proof_provider.go",
            "services\message-service\cmd\message-service\message_compliance_approval_operator.go"
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
            "projection-failure-resolve",
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
            "NEXUSIM_DELIVERY_PROJECTION_FAILURE_RESOLVE_OUTPUT",
            "NEXUSIM_DELIVERY_PROJECTION_FAILURE_CLEANUP_OUTPUT"
        )
        DryRunEnvs = @(
            "NEXUSIM_DELIVERY_OUTBOX_REPAIR_DRY_RUN",
            "NEXUSIM_DELIVERY_PROJECTION_REPAIR_DRY_RUN",
            "NEXUSIM_DELIVERY_PROJECTION_FAILURE_RESOLVE_DRY_RUN",
            "NEXUSIM_DELIVERY_OUTBOX_REPAIR_CLEANUP_DRY_RUN",
            "NEXUSIM_DELIVERY_PROJECTION_REPAIR_CLEANUP_DRY_RUN",
            "NEXUSIM_DELIVERY_PROJECTION_FAILURE_CLEANUP_DRY_RUN"
        )
        ReasonFileEnvs = @(
            "NEXUSIM_DELIVERY_OUTBOX_REPAIR_REASON_FILE",
            "NEXUSIM_DELIVERY_PROJECTION_REPAIR_REASON_FILE",
            "NEXUSIM_DELIVERY_PROJECTION_FAILURE_RESOLVE_REASON_FILE"
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
        DryRunEnvs = @(
            "NEXUSIM_RECEIPT_OUTBOX_REPAIR_CLEANUP_DRY_RUN"
        )
        ReasonFileEnvs = @(
            "NEXUSIM_RECEIPT_OUTBOX_REPAIR_REASON_FILE"
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
            "source-policy-set",
            "contact-request-review",
            "contact-request-review-audit"
        )
        OutputEnvs = @(
            "NEXUSIM_CONTACTS_OUTBOX_AUDIT_OUTPUT",
            "NEXUSIM_CONTACTS_OUTBOX_REPAIR_OUTPUT",
            "NEXUSIM_CONTACTS_OUTBOX_REPAIR_AUDIT_OUTPUT",
            "NEXUSIM_CONTACTS_OUTBOX_REPAIR_CLEANUP_OUTPUT",
            "NEXUSIM_CONTACTS_TENANT_PRIVACY_AUDIT_OUTPUT",
            "NEXUSIM_CONTACTS_TENANT_PRIVACY_SET_OUTPUT",
            "NEXUSIM_CONTACTS_SOURCE_POLICY_AUDIT_OUTPUT",
            "NEXUSIM_CONTACTS_SOURCE_POLICY_SET_OUTPUT",
            "NEXUSIM_CONTACTS_REQUEST_REVIEW_OUTPUT",
            "NEXUSIM_CONTACTS_REQUEST_REVIEW_AUDIT_OUTPUT"
        )
        DryRunEnvs = @(
            "NEXUSIM_CONTACTS_OUTBOX_REPAIR_CLEANUP_DRY_RUN"
        )
        ReasonFileEnvs = @(
            "NEXUSIM_CONTACTS_OUTBOX_REPAIR_REASON_FILE",
            "NEXUSIM_CONTACTS_REQUEST_REVIEW_REASON_FILE"
        )
    },
    @{
        Service = "policy-service"
        Cmd = "services\policy-service\cmd\policy-service\main.go"
        Env = "NEXUSIM_POLICY_SERVICE_MODE"
        Modes = @(
            "outbox-audit",
            "outbox-repair",
            "outbox-repair-audit",
            "outbox-repair-cleanup",
            "decision-audit-export",
            "tenant-quota-audit",
            "tenant-quota-set"
        )
        OutputEnvs = @(
            "NEXUSIM_POLICY_OUTBOX_AUDIT_OUTPUT",
            "NEXUSIM_POLICY_OUTBOX_REPAIR_OUTPUT",
            "NEXUSIM_POLICY_OUTBOX_REPAIR_AUDIT_OUTPUT",
            "NEXUSIM_POLICY_OUTBOX_REPAIR_CLEANUP_OUTPUT",
            "NEXUSIM_POLICY_DECISION_AUDIT_EXPORT_OUTPUT",
            "NEXUSIM_POLICY_TENANT_QUOTA_AUDIT_OUTPUT",
            "NEXUSIM_POLICY_TENANT_QUOTA_SET_OUTPUT"
        )
        DryRunEnvs = @(
            "NEXUSIM_POLICY_OUTBOX_REPAIR_CLEANUP_DRY_RUN"
        )
        ReasonFileEnvs = @(
            "NEXUSIM_POLICY_OUTBOX_REPAIR_REASON_FILE",
            "NEXUSIM_POLICY_TENANT_QUOTA_SET_REASON_FILE"
        )
    },
    @{
        Service = "conversation-service"
        Cmd = "services\conversation-service\cmd\conversation-service\main.go"
        Env = "NEXUSIM_CONVERSATION_SERVICE_MODE"
        Modes = @("member-change-audit", "member-window-audit", "member-window-repair", "member-window-repair-audit")
        OutputEnvs = @(
            "NEXUSIM_CONVERSATION_MEMBER_CHANGE_AUDIT_OUTPUT",
            "NEXUSIM_CONVERSATION_MEMBER_WINDOW_AUDIT_OUTPUT",
            "NEXUSIM_CONVERSATION_MEMBER_WINDOW_REPAIR_OUTPUT",
            "NEXUSIM_CONVERSATION_MEMBER_WINDOW_REPAIR_AUDIT_OUTPUT"
        )
        DryRunEnvs = @(
            "NEXUSIM_CONVERSATION_MEMBER_WINDOW_REPAIR_DRY_RUN"
        )
        ReasonFileEnvs = @(
            "NEXUSIM_CONVERSATION_MEMBER_WINDOW_REPAIR_REASON_FILE"
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
        DryRunEnvs = @(
            "NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_DRY_RUN",
            "NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_CLEANUP_DRY_RUN",
            "NEXUSIM_IDENTITY_CHALLENGE_REQUEST_LIMIT_CLEANUP_DRY_RUN"
        )
        ReasonFileEnvs = @(
            "NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_REPAIR_REASON_FILE"
        )
    },
    @{
        Service = "api-gateway"
        Cmd = "services\api-gateway\cmd\api-gateway\main.go"
        Env = "NEXUSIM_API_GATEWAY_MODE"
        Modes = @(
            "tenant-quota-audit",
            "tenant-quota-set"
        )
        OutputEnvs = @(
            "NEXUSIM_API_GATEWAY_TENANT_QUOTA_AUDIT_OUTPUT",
            "NEXUSIM_API_GATEWAY_TENANT_QUOTA_SET_OUTPUT"
        )
        DryRunEnvs = @(
            "NEXUSIM_API_GATEWAY_TENANT_QUOTA_SET_DRY_RUN"
        )
        ReasonFileEnvs = @()
    },
    @{
        Service = "admin-service"
        Cmd = "services\admin-service\cmd\admin-service\main.go"
        Env = "NEXUSIM_ADMIN_SERVICE_MODE"
        Modes = @(
            "compensation-request"
        )
        OutputEnvs = @(
            "NEXUSIM_ADMIN_COMPENSATION_OUTPUT"
        )
        DryRunEnvs = @(
            "NEXUSIM_ADMIN_COMPENSATION_DRY_RUN"
        )
        ReasonFileEnvs = @(
            "NEXUSIM_ADMIN_COMPENSATION_REASON_FILE"
        )
    },
    @{
        Service = "workflow-service"
        Cmd = "services\workflow-service\cmd\workflow-service\main.go"
        Env = "NEXUSIM_WORKFLOW_SERVICE_MODE"
        Modes = @(
            "compensation-instruction-import"
        )
        OutputEnvs = @()
        DryRunEnvs = @()
        ReasonFileEnvs = @()
    }
)

$requiredSharedTerms = @("operator", "repair", "audit", "SQL")
foreach ($term in $requiredSharedTerms) {
    if (-not $repairIndex.Contains($term)) {
        throw "docs/runbook/repair-operators.md missing required repair operator term: $term"
    }
}

if ($repairCatalog.schema_version -ne 1) {
    throw "docs/runbook/repair-operators.catalog.json must have schema_version=1."
}
if (-not $repairCatalog.services) {
    throw "docs/runbook/repair-operators.catalog.json must define services."
}

$catalogByService = @{}
foreach ($catalogService in @($repairCatalog.services)) {
    $catalogServiceName = [string]$catalogService.service
    if ([string]::IsNullOrWhiteSpace($catalogServiceName)) {
        throw "docs/runbook/repair-operators.catalog.json contains a service without name."
    }
    if ($catalogByService.ContainsKey($catalogServiceName)) {
        throw "docs/runbook/repair-operators.catalog.json contains duplicate service: $catalogServiceName"
    }
    $catalogByService[$catalogServiceName] = $catalogService
}

function Assert-CatalogArrayContainsAll {
    param(
        [string]$Service,
        [string]$FieldName,
        [object[]]$ExpectedValues,
        [object[]]$ActualValues
    )

    $actualSet = @{}
    foreach ($actual in @($ActualValues)) {
        $actualValue = [string]$actual
        if (-not [string]::IsNullOrWhiteSpace($actualValue)) {
            $actualSet[$actualValue] = $true
        }
    }

    foreach ($expected in @($ExpectedValues)) {
        $expectedValue = [string]$expected
        if ([string]::IsNullOrWhiteSpace($expectedValue)) {
            continue
        }
        if (-not $actualSet.ContainsKey($expectedValue)) {
            throw "docs/runbook/repair-operators.catalog.json service ${Service} missing ${FieldName}: $expectedValue"
        }
    }
}

foreach ($spec in $operatorSpecs) {
    $service = [string]$spec.Service
    if (-not $catalogByService.ContainsKey($service)) {
        throw "docs/runbook/repair-operators.catalog.json missing service: $service"
    }
    $catalogService = $catalogByService[$service]
    $catalogEnv = [string]$catalogService.mode_env
    if ($catalogEnv -ne [string]$spec.Env) {
        throw "docs/runbook/repair-operators.catalog.json service ${service} has mode_env=${catalogEnv}, expected $($spec.Env)"
    }
    Assert-CatalogArrayContainsAll -Service $service -FieldName "mode" -ExpectedValues @($spec.Modes) -ActualValues @($catalogService.modes)
    Assert-CatalogArrayContainsAll -Service $service -FieldName "output_env" -ExpectedValues @($spec.OutputEnvs) -ActualValues @($catalogService.output_envs)
    Assert-CatalogArrayContainsAll -Service $service -FieldName "dry_run_env" -ExpectedValues @($spec.DryRunEnvs) -ActualValues @($catalogService.dry_run_envs)
    Assert-CatalogArrayContainsAll -Service $service -FieldName "reason_file_env" -ExpectedValues @($spec.ReasonFileEnvs) -ActualValues @($catalogService.reason_file_envs)

    $cmdPath = Join-Path $repoRoot ([string]$spec.Cmd)
    if (-not (Test-Path -LiteralPath $cmdPath -PathType Leaf)) {
        throw "Missing service cmd file for repair operator check: $($spec.Cmd)"
    }

    $cmd = Get-Content -LiteralPath $cmdPath -Raw
    $cmdDir = Split-Path -Parent $cmdPath
    $cmdFileName = Split-Path -Leaf $cmdPath
    $cmdPackageFiles = Get-ChildItem -LiteralPath $cmdDir -Filter "*.go" -File |
        Where-Object { $_.Name -ne $cmdFileName -and $_.Name -notlike "*_test.go" } |
        Sort-Object Name
    foreach ($cmdPackageFile in $cmdPackageFiles) {
        $cmd += "`n" + (Get-Content -LiteralPath $cmdPackageFile.FullName -Raw)
    }
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

    foreach ($dryRunEnv in @($spec.DryRunEnvs)) {
        $dryRunEnv = [string]$dryRunEnv
        if ([string]::IsNullOrWhiteSpace($dryRunEnv)) {
            continue
        }
        if (-not $repairIndex.Contains($dryRunEnv)) {
            throw "docs/runbook/repair-operators.md missing documented dry-run env for ${service}: $dryRunEnv"
        }
        if (-not $cmd.Contains($dryRunEnv)) {
            throw "$($spec.Cmd) missing documented dry-run env for ${service}: $dryRunEnv"
        }
    }

    $catalogReasonFileSet = @{}
    foreach ($catalogReasonFileEnv in @($catalogService.reason_file_envs)) {
        $catalogReasonFileEnv = [string]$catalogReasonFileEnv
        if (-not [string]::IsNullOrWhiteSpace($catalogReasonFileEnv)) {
            $catalogReasonFileSet[$catalogReasonFileEnv] = $true
        }
    }

    foreach ($reasonFileEnv in @($spec.ReasonFileEnvs)) {
        $reasonFileEnv = [string]$reasonFileEnv
        if ([string]::IsNullOrWhiteSpace($reasonFileEnv)) {
            continue
        }
        if (-not $repairIndex.Contains($reasonFileEnv)) {
            throw "docs/runbook/repair-operators.md missing documented reason file env for ${service}: $reasonFileEnv"
        }
        if (-not $cmd.Contains($reasonFileEnv)) {
            throw "$($spec.Cmd) missing documented reason file env for ${service}: $reasonFileEnv"
        }
    }

    $reasonFileMatches = [regex]::Matches($cmd, 'NEXUSIM_[A-Z0-9_]+_REASON_FILE')
    foreach ($match in $reasonFileMatches) {
        $reasonFileEnv = [string]$match.Value
        if ($reasonFileEnv -match "_TEST_REASON_FILE$") {
            continue
        }
        if (-not $catalogReasonFileSet.ContainsKey($reasonFileEnv)) {
            throw "docs/runbook/repair-operators.catalog.json service ${service} missing discovered reason_file_env from cmd package: $reasonFileEnv"
        }
    }
}

$operatorModePattern = "(audit|repair|cleanup|keyring-rotate|tenant-privacy-default-set|source-policy-set|tenant-quota-set|compensation-request|compensation-instruction-import)"
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
