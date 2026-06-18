$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot

function Read-RepoFile {
    param(
        [Parameter(Mandatory = $true)]
        [string]$RelativePath
    )

    $path = Join-Path $repoRoot $RelativePath
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Missing required file: $RelativePath"
    }
    return Get-Content -LiteralPath $path -Raw
}

function Assert-Contains {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Content,
        [Parameter(Mandatory = $true)]
        [string]$Needle,
        [Parameter(Mandatory = $true)]
        [string]$Message
    )

    if (-not $Content.Contains($Needle)) {
        throw $Message
    }
}

function Assert-NotContains {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Content,
        [Parameter(Mandatory = $true)]
        [string]$Needle,
        [Parameter(Mandatory = $true)]
        [string]$Message
    )

    if ($Content.Contains($Needle)) {
        throw $Message
    }
}

$config = Read-RepoFile "services/identity-service/cmd/identity-service/key_guard_config.go"
$main = Read-RepoFile "services/identity-service/cmd/identity-service/main.go"
$tests = Read-RepoFile "services/identity-service/cmd/identity-service/main_test.go"
$sdd = Read-RepoFile "docs/sdd/identity-service.md"
$brief = Read-RepoFile "docs/runbook/service-briefs/identity-service.md"

Assert-Contains $config "func validateIdentityProductionKeyGuardFromEnv" "identity production key guard validator is missing."
Assert-Contains $config "NEXUSIM_IDENTITY_PRODUCTION_KEY_GUARD" "identity production key guard env is missing."
Assert-Contains $config "NEXUSIM_IDENTITY_GATEWAY_TOKEN_FORMAT" "gateway token format must be checked by identity production key guard."
Assert-Contains $config 'format != "rs256" && format != "jwt-rs256"' "identity production key guard must require RS256 gateway token formats."
Assert-Contains $config "NEXUSIM_IDENTITY_MFA_SECRET_KEYRING_JSON" "identity production key guard must accept MFA keyring JSON."
Assert-Contains $config "NEXUSIM_IDENTITY_MFA_SECRET_KEYRING_FILE" "identity production key guard must accept MFA keyring file."
Assert-Contains $config "NEXUSIM_IDENTITY_MFA_SECRET_KEY" "identity production key guard must require explicit MFA key input."
Assert-Contains $config "NEXUSIM_IDENTITY_MFA_RECOVERY_CODE_SECRET" "identity production key guard must require explicit recovery-code secret."
Assert-Contains $config "NEXUSIM_IDENTITY_CHALLENGE_REQUEST_LIMIT_SECRET" "identity production key guard must require explicit challenge request-limit secret."
Assert-Contains $config "NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_TOKEN_KEYRING_JSON" "identity production key guard must accept challenge delivery keyring JSON."
Assert-Contains $config "NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_TOKEN_KEYRING_FILE" "identity production key guard must accept challenge delivery keyring file."
Assert-Contains $config "NEXUSIM_IDENTITY_CHALLENGE_DELIVERY_TOKEN_KEY" "identity production key guard must require explicit challenge delivery token key."
Assert-NotContains $config "NEXUSIM_IDENTITY_GATEWAY_TOKEN_SECRET" "identity production key guard must not accept gateway-token secret fallback as MFA/recovery key material."
Assert-NotContains $config "NEXUSIM_PUSH_AUTH_HMAC_SECRET" "identity production key guard must not accept push auth secret fallback as identity key material."

Assert-Contains $main "validateIdentityProductionKeyGuardFromEnv(identityRuntimeKeyGuardScope{" "identity production key guard must be wired into identity-service startup."
Assert-Contains $main "GatewayToken:" "identity grpc mode must validate gateway-token key mode."
Assert-Contains $main "MFA:" "identity grpc mode must validate MFA key material."
Assert-Contains $main "MFARecovery:" "identity grpc mode must validate recovery-code key material."
Assert-Contains $main "ChallengeRequestLimit:" "identity grpc mode must validate challenge request-limit secret."
Assert-Contains $main "ChallengeDeliveryToken: challengeDeliveryMode() == `"outbox`"" "identity grpc mode must validate challenge delivery token keys when durable outbox mode is enabled."
Assert-Contains $main "ChallengeDeliveryToken: true" "challenge-delivery-worker mode must validate challenge delivery token key material."

foreach ($testName in @(
    "TestIdentityProductionKeyGuardDefaultsToDisabled",
    "TestIdentityProductionKeyGuardRejectsLocalCompatibilityKeys",
    "TestIdentityProductionKeyGuardAcceptsExplicitDedicatedKeys",
    "TestIdentityProductionKeyGuardWorkerScopeDoesNotRequireGatewayKeys"
)) {
    Assert-Contains $tests $testName "missing identity production key guard test: $testName"
}

Assert-Contains $sdd "NEXUSIM_IDENTITY_PRODUCTION_KEY_GUARD=true" "identity SDD must document production key guard."
Assert-Contains $sdd "not KMS/HSM-backed key management" "identity SDD must avoid over-claiming production key guard as KMS/HSM."
Assert-Contains $brief "NEXUSIM_IDENTITY_PRODUCTION_KEY_GUARD=true" "identity service brief must document production key guard."
Assert-Contains $brief "KMS/HSM" "identity service brief must avoid over-claiming production key guard as KMS/HSM."

Write-Host "OK   identity production key guardrails"
