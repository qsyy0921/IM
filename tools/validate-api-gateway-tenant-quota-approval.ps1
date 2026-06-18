param(
    [Parameter(Mandatory = $true)]
    [string]$ApprovalPath,

    [string]$ExpectedTenantID = "",
    [double]$ExpectedRequestsPerSecond = 0,
    [int]$ExpectedBurst = 0,
    [string]$ExpectedEnabled = "",
    [string]$ExpectedSource = "",
    [int64]$NowUnixMS = 0
)

$ErrorActionPreference = "Stop"

. "$PSScriptRoot\repair-operator-safety.ps1"

function Assert-Condition {
    param(
        [bool]$Condition,
        [string]$Message
    )
    if (-not $Condition) {
        throw $Message
    }
}

function Get-RequiredString {
    param(
        [object]$Object,
        [string]$Name
    )
    if ($null -eq $Object.PSObject.Properties[$Name]) {
        throw "Tenant quota approval missing required field: $Name"
    }
    $value = ([string]$Object.$Name).Trim()
    if ($value.Length -eq 0) {
        throw "Tenant quota approval field is required: $Name"
    }
    return $value
}

function Assert-TenantQuotaLabel {
    param(
        [string]$Value,
        [string]$FieldName
    )

    $text = ([string]$Value).Trim()
    Assert-Condition ($text.Length -gt 0) "$FieldName is required."
    Assert-Condition ($text.Length -le 128) "$FieldName is too long."
    Assert-Condition ($text -notmatch "\s") "$FieldName must not contain whitespace."
    Assert-LowSensitiveRepairIdentifier -Value $text -FieldName $FieldName
}

if (-not (Test-Path -LiteralPath $ApprovalPath -PathType Leaf)) {
    throw "Missing api-gateway tenant quota approval: $ApprovalPath"
}

try {
    $raw = Get-Content -LiteralPath $ApprovalPath -Raw
    $approval = $raw | ConvertFrom-Json
} catch {
    throw "Invalid api-gateway tenant quota approval JSON: $($_.Exception.Message)"
}

Assert-Condition ((Get-RequiredString $approval "schema_version") -eq "nexusim.api_gateway.tenant_quota_approval.v1") "Unsupported api-gateway tenant quota approval schema_version."
Assert-Condition ((Get-RequiredString $approval "service") -eq "api-gateway") "Tenant quota approval service must be api-gateway."
Assert-Condition ((Get-RequiredString $approval "approval_type") -eq "tenant_quota_change") "Tenant quota approval approval_type must be tenant_quota_change."
Assert-Condition ((Get-RequiredString $approval "status") -eq "APPROVED") "Tenant quota approval status must be APPROVED."

$changeID = Get-RequiredString $approval "change_id"
$targetEnvironment = Get-RequiredString $approval "target_environment"
$operator = Get-RequiredString $approval "operator"
$approver = Get-RequiredString $approval "approver"
Assert-TenantQuotaLabel -Value $changeID -FieldName "change_id"
Assert-TenantQuotaLabel -Value $targetEnvironment -FieldName "target_environment"
Assert-LowSensitiveRepairActor -Value $operator -FieldName "operator"
Assert-LowSensitiveRepairActor -Value $approver -FieldName "approver"

foreach ($field in @("generated_at_unix_ms", "approved_at_unix_ms", "expires_at_unix_ms")) {
    Assert-Condition ($null -ne $approval.PSObject.Properties[$field]) "Tenant quota approval missing required field: $field"
    Assert-Condition ([int64]$approval.$field -gt 0) "Tenant quota approval field must be positive: $field"
}

if ($NowUnixMS -le 0) {
    $NowUnixMS = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
}
Assert-Condition ([int64]$approval.approved_at_unix_ms -le ($NowUnixMS + 60000)) "Tenant quota approval approved_at_unix_ms is in the future."
Assert-Condition ([int64]$approval.expires_at_unix_ms -gt $NowUnixMS) "Tenant quota approval is expired."

Assert-Condition ($null -ne $approval.desired_plan) "Tenant quota approval missing desired_plan."
$plan = $approval.desired_plan
$tenantID = Get-RequiredString $plan "tenant_id"
$source = Get-RequiredString $plan "source"
Assert-TenantQuotaLabel -Value $tenantID -FieldName "desired_plan.tenant_id"
Assert-TenantQuotaLabel -Value $source -FieldName "desired_plan.source"
Assert-Condition ([double]$plan.requests_per_second -gt 0) "desired_plan.requests_per_second must be positive."
Assert-Condition ([int]$plan.burst -gt 0) "desired_plan.burst must be positive."
Assert-Condition ($null -ne $plan.PSObject.Properties["enabled"]) "desired_plan.enabled is required."

if ($ExpectedTenantID.Trim().Length -gt 0) {
    Assert-Condition ($tenantID -eq $ExpectedTenantID.Trim()) "Tenant quota approval tenant_id does not match expected value."
}
if ($ExpectedRequestsPerSecond -gt 0) {
    Assert-Condition ([math]::Abs(([double]$plan.requests_per_second) - $ExpectedRequestsPerSecond) -lt 0.000000001) "Tenant quota approval requests_per_second does not match expected value."
}
if ($ExpectedBurst -gt 0) {
    Assert-Condition ([int]$plan.burst -eq $ExpectedBurst) "Tenant quota approval burst does not match expected value."
}
if ($ExpectedEnabled.Trim().Length -gt 0) {
    $parsedEnabled = $false
    if (-not [bool]::TryParse($ExpectedEnabled.Trim(), [ref]$parsedEnabled)) {
        throw "ExpectedEnabled must be true or false."
    }
    Assert-Condition ([bool]$plan.enabled -eq $parsedEnabled) "Tenant quota approval enabled does not match expected value."
}
if ($ExpectedSource.Trim().Length -gt 0) {
    Assert-Condition ($source -eq $ExpectedSource.Trim()) "Tenant quota approval source does not match expected value."
}

$summary = [ordered]@{
    approval_path = $ApprovalPath
    service = "api-gateway"
    approval_type = "tenant_quota_change"
    status = "APPROVED"
    change_id = $changeID
    target_environment = $targetEnvironment
    tenant_id = $tenantID
    expires_at_unix_ms = [int64]$approval.expires_at_unix_ms
    approval_sha256 = Get-RepairSha256Hex -Bytes ([System.Text.Encoding]::UTF8.GetBytes($raw))
}

$summary | ConvertTo-Json -Depth 4
