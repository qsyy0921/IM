$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$policyPath = Join-Path $repoRoot "deploy\local\otel-sampling-policy.json"

if (-not (Test-Path -LiteralPath $policyPath)) {
    throw "Missing OTel sampling policy file: $policyPath"
}

try {
    $policy = Get-Content -LiteralPath $policyPath -Raw | ConvertFrom-Json
} catch {
    throw "OTel sampling policy is not valid JSON: $($_.Exception.Message)"
}

if ($policy.version -lt 1) {
    throw "OTel sampling policy must include a positive version."
}

$profiles = @{}
$policy.profiles.PSObject.Properties | ForEach-Object {
    $name = $_.Name
    $profile = $_.Value
    $ratio = [double]$profile.sampling_ratio
    if ($ratio -le 0 -or $ratio -gt 1) {
        throw "OTel sampling profile '$name' has invalid sampling_ratio $ratio; expected > 0 and <= 1."
    }
    $profiles[$name] = $ratio
}

foreach ($requiredProfile in @("local_smoke", "dev_interactive", "production_starting_point", "high_volume_starting_point")) {
    if (-not $profiles.ContainsKey($requiredProfile)) {
        throw "OTel sampling policy missing required profile '$requiredProfile'."
    }
}
if ($profiles["local_smoke"] -ne 1.0) {
    throw "OTel local_smoke profile must keep sampling_ratio=1.0 for deterministic smoke debugging."
}
if ($profiles["production_starting_point"] -gt 0.1) {
    throw "OTel production_starting_point profile should not exceed 0.1 without an explicit policy update."
}
if ($profiles["high_volume_starting_point"] -gt $profiles["production_starting_point"]) {
    throw "OTel high_volume_starting_point should be less than or equal to production_starting_point."
}

$requiredServices = @(
    "api-gateway",
    "contacts-service",
    "identity-service",
    "message-service",
    "conversation-service",
    "delivery-service",
    "receipt-service",
    "policy-service",
    "push-gateway"
)

$services = @{}
foreach ($service in $policy.services) {
    $name = [string]$service.service
    if ([string]::IsNullOrWhiteSpace($name)) {
        throw "OTel sampling policy contains service without name."
    }
    if ($services.ContainsKey($name)) {
        throw "OTel sampling policy contains duplicate service '$name'."
    }
    $prefix = [string]$service.env_prefix
    if ($prefix -notmatch "^NEXUSIM_[A-Z0-9_]+_OTEL$") {
        throw "OTel sampling policy service '$name' has invalid env_prefix '$prefix'."
    }
    $profileName = [string]$service.default_profile
    if (-not $profiles.ContainsKey($profileName)) {
        throw "OTel sampling policy service '$name' references unknown default_profile '$profileName'."
    }
    if ($service.high_volume -and $profileName -ne "high_volume_starting_point") {
        throw "High-volume service '$name' must use high_volume_starting_point by default."
    }
    $services[$name] = $true
}

foreach ($requiredService in $requiredServices) {
    if (-not $services.ContainsKey($requiredService)) {
        throw "OTel sampling policy missing service '$requiredService'."
    }
}

if (-not $policy.guardrails.production_full_sampling_requires_time_box) {
    throw "OTel sampling policy must require time-boxed production full sampling."
}
if (-not $policy.guardrails.forbid_sensitive_span_attributes) {
    throw "OTel sampling policy must keep sensitive span attributes forbidden."
}
if (-not $policy.guardrails.forbid_high_cardinality_metric_labels) {
    throw "OTel sampling policy must keep high-cardinality metric labels forbidden."
}

Write-Host "OK   OTel sampling policy"
