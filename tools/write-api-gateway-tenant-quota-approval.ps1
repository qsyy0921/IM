param(
    [Parameter(Mandatory = $true)]
    [string]$OutputPath,

    [Parameter(Mandatory = $true)]
    [string]$TenantID,

    [Parameter(Mandatory = $true)]
    [double]$RequestsPerSecond,

    [Parameter(Mandatory = $true)]
    [int]$Burst,

    [string]$Enabled = "true",

    [string]$PlanSource = "operator",

    [Parameter(Mandatory = $true)]
    [string]$Operator,

    [Parameter(Mandatory = $true)]
    [string]$Approver,

    [Parameter(Mandatory = $true)]
    [string]$ChangeID,

    [Parameter(Mandatory = $true)]
    [string]$TargetEnvironment,

    [timespan]$ExpiresIn = ([timespan]::FromHours(4)),

    [switch]$Force
)

$ErrorActionPreference = "Stop"

. "$PSScriptRoot\repair-operator-safety.ps1"

Assert-ExternalRepairOutputPath -Value $OutputPath -FieldName "OutputPath"

if ($RequestsPerSecond -le 0) {
    throw "RequestsPerSecond must be positive."
}
if ($Burst -le 0) {
    throw "Burst must be positive."
}
if ($ExpiresIn.TotalMilliseconds -le 0) {
    throw "ExpiresIn must be positive."
}
$enabledValue = $false
if (-not [bool]::TryParse($Enabled.Trim(), [ref]$enabledValue)) {
    throw "Enabled must be true or false."
}
$enabledText = $enabledValue.ToString().ToLowerInvariant()
if ((Test-Path -LiteralPath $OutputPath -PathType Leaf) -and -not $Force) {
    throw "OutputPath already exists. Use -Force to overwrite: $OutputPath"
}

$now = [DateTimeOffset]::UtcNow
$approval = [ordered]@{
    schema_version = "nexusim.api_gateway.tenant_quota_approval.v1"
    service = "api-gateway"
    approval_type = "tenant_quota_change"
    status = "APPROVED"
    change_id = $ChangeID.Trim()
    target_environment = $TargetEnvironment.Trim()
    operator = $Operator.Trim()
    approver = $Approver.Trim()
    generated_at_unix_ms = $now.ToUnixTimeMilliseconds()
    approved_at_unix_ms = $now.ToUnixTimeMilliseconds()
    expires_at_unix_ms = $now.Add($ExpiresIn).ToUnixTimeMilliseconds()
    desired_plan = [ordered]@{
        tenant_id = $TenantID.Trim()
        requests_per_second = $RequestsPerSecond
        burst = $Burst
        enabled = $enabledValue
        source = $PlanSource.Trim()
    }
}

$directory = Split-Path -Parent ([System.IO.Path]::GetFullPath($OutputPath))
New-Item -ItemType Directory -Force -Path $directory | Out-Null
($approval | ConvertTo-Json -Depth 6) | Set-Content -LiteralPath $OutputPath -Encoding UTF8

$validator = Join-Path $PSScriptRoot "validate-api-gateway-tenant-quota-approval.ps1"
& powershell -NoProfile -ExecutionPolicy Bypass -File $validator `
    -ApprovalPath $OutputPath `
    -ExpectedTenantID $TenantID `
    -ExpectedRequestsPerSecond $RequestsPerSecond `
    -ExpectedBurst $Burst `
    -ExpectedEnabled $enabledText `
    -ExpectedSource $PlanSource | Out-Null
if ($LASTEXITCODE -ne 0) {
    Remove-Item -LiteralPath $OutputPath -Force -ErrorAction SilentlyContinue
    throw "Generated api-gateway tenant quota approval failed validation."
}

Write-Host "OK   api-gateway tenant quota approval written: $OutputPath"
