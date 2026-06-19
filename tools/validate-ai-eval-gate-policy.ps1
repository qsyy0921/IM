param(
    [string]$GatePolicyPath = "docs/runbook/ai-eval/gate-policy.local.json"
)

$ErrorActionPreference = "Stop"

function Assert-Condition {
    param(
        [bool]$Condition,
        [string]$Message
    )

    if (-not $Condition) {
        throw $Message
    }
}

function Get-JsonPropertyBool {
    param(
        $Object,
        [string]$Name,
        [bool]$DefaultValue = $false
    )

    if ($null -eq $Object -or $null -eq $Object.PSObject.Properties[$Name]) {
        return $DefaultValue
    }
    return [System.Convert]::ToBoolean($Object.$Name)
}

function Get-JsonPropertyString {
    param(
        $Object,
        [string]$Name
    )

    if ($null -eq $Object -or $null -eq $Object.PSObject.Properties[$Name]) {
        return ""
    }
    return ([string]$Object.$Name).Trim()
}

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
if ([System.IO.Path]::IsPathRooted($GatePolicyPath)) {
    $resolvedPath = [System.IO.Path]::GetFullPath($GatePolicyPath)
} else {
    $resolvedPath = [System.IO.Path]::GetFullPath((Join-Path $repoRoot $GatePolicyPath))
}
Assert-Condition (Test-Path -LiteralPath $resolvedPath -PathType Leaf) "Gate policy does not exist: $resolvedPath"

$policy = Get-Content -LiteralPath $resolvedPath -Raw | ConvertFrom-Json
Assert-Condition ([int]$policy.schema_version -eq 1) "unsupported schema_version"
Assert-Condition ((Get-JsonPropertyString -Object $policy -Name "gate_id").Length -gt 0) "gate_id is required"
Assert-Condition ($null -ne $policy.policy) "policy block is required"
Assert-Condition ([int]$policy.policy.min_case_count -gt 0) "policy.min_case_count must be positive"
Assert-Condition ([int]$policy.policy.max_failed_count -ge 0) "policy.max_failed_count must be non-negative"

$adapters = @($policy.adapters)
Assert-Condition ($adapters.Count -gt 0) "at least one adapter is required"
$adapterNames = New-Object System.Collections.Generic.HashSet[string]
$requiredNames = New-Object System.Collections.Generic.HashSet[string]
foreach ($adapter in $adapters) {
    $name = Get-JsonPropertyString -Object $adapter -Name "name"
    $script = Get-JsonPropertyString -Object $adapter -Name "script"
    Assert-Condition ($name.Length -gt 0) "adapter.name is required"
    Assert-Condition ($adapterNames.Add($name)) "duplicate adapter name: $name"
    Assert-Condition ($script.Length -gt 0) "adapter.script is required for $name"
    $scriptPath = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot $script))
    Assert-Condition (Test-Path -LiteralPath $scriptPath -PathType Leaf) "adapter script does not exist for $name`: $scriptPath"
    Assert-Condition ((Get-JsonPropertyString -Object $adapter -Name "summary_file").Length -gt 0) "summary_file is required for $name"
    if (Get-JsonPropertyBool -Object $adapter -Name "required") {
        [void]$requiredNames.Add($name)
    }
}

foreach ($requiredName in @($policy.policy.required_adapters)) {
    $required = ([string]$requiredName).Trim()
    Assert-Condition ($required.Length -gt 0) "empty required adapter name"
    Assert-Condition ($adapterNames.Contains($required)) "required adapter is not declared: $required"
    Assert-Condition ($requiredNames.Contains($required)) "required adapter is not marked required: $required"
}

foreach ($forbidden in @($policy.forbidden_persisted_fields)) {
    Assert-Condition (([string]$forbidden).Trim().Length -gt 0) "forbidden persisted field list contains empty value"
}

Write-Host "OK   ai-eval gate policy validated: $resolvedPath"
