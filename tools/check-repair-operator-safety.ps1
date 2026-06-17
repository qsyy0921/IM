$ErrorActionPreference = "Stop"

$helperPath = Join-Path $PSScriptRoot "repair-operator-safety.ps1"
if (-not (Test-Path -LiteralPath $helperPath -PathType Leaf)) {
    throw "Missing repair operator safety helper: $helperPath"
}

$helperText = Get-Content -LiteralPath $helperPath -Raw
foreach ($functionName in @(
        "Get-RepairSha256Hex",
        "Assert-LowSensitiveRepairActor",
        "Assert-LowSensitiveRepairAdHocEnv"
    )) {
    if ($helperText -notmatch "function\s+$functionName\b") {
        throw "repair-operator-safety.ps1 missing function: $functionName"
    }
}

. $helperPath

function Assert-FailsWith {
    param(
        [scriptblock]$Script,
        [string]$Pattern,
        [string]$Label
    )

    try {
        & $Script
    } catch {
        $message = [string]$_.Exception.Message
        if ($message -match $Pattern) {
            return
        }
        throw "$Label failed with unexpected error: $message"
    }
    throw "$Label should have failed."
}

$abcHash = Get-RepairSha256Hex -Bytes ([System.Text.Encoding]::UTF8.GetBytes("abc"))
if ($abcHash -ne "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad") {
    throw "Get-RepairSha256Hex returned unexpected digest."
}

Assert-LowSensitiveRepairActor -Value "operator-a" -FieldName "RequestedBy"
Assert-FailsWith -Label "blank actor" -Pattern "required" -Script {
    Assert-LowSensitiveRepairActor -Value " " -FieldName "RequestedBy"
}
Assert-FailsWith -Label "email actor" -Pattern "low-sensitive operator id" -Script {
    Assert-LowSensitiveRepairActor -Value "operator@example.com" -FieldName "RequestedBy"
}
Assert-FailsWith -Label "credential actor" -Pattern "credential-like" -Script {
    Assert-LowSensitiveRepairActor -Value "operator-token" -FieldName "RequestedBy"
}

Assert-LowSensitiveRepairAdHocEnv -Key "NEXUSIM_FILTER" -Value "tenant-1"
Assert-FailsWith -Label "lowercase env key" -Pattern "uppercase environment variable" -Script {
    Assert-LowSensitiveRepairAdHocEnv -Key "nexusim_filter" -Value "tenant-1"
}
Assert-FailsWith -Label "sensitive env key" -Pattern "sensitive Env key" -Script {
    Assert-LowSensitiveRepairAdHocEnv -Key "NEXUSIM_TOKEN" -Value "redacted"
}
Assert-FailsWith -Label "sensitive env value" -Pattern "sensitive Env value" -Script {
    Assert-LowSensitiveRepairAdHocEnv -Key "NEXUSIM_FILTER" -Value "Bearer abc.def.ghi"
}

$writerScripts = @(
    "write-repair-operator-plan.ps1",
    "write-repair-approval-request.ps1",
    "write-repair-approval-decision.ps1",
    "write-repair-batch-manifest.ps1",
    "write-repair-audit-bundle.ps1"
)

foreach ($writerScript in $writerScripts) {
    $writerPath = Join-Path $PSScriptRoot $writerScript
    if (-not (Test-Path -LiteralPath $writerPath -PathType Leaf)) {
        throw "Missing repair writer script: $writerScript"
    }
    $writerText = Get-Content -LiteralPath $writerPath -Raw
    if ($writerText -notmatch "repair-operator-safety\.ps1") {
        throw "$writerScript must dot-source repair-operator-safety.ps1"
    }
    foreach ($duplicateFunction in @(
            "Get-Sha256Hex",
            "Assert-LowSensitiveActor",
            "Assert-LowSensitiveAdHocEnv"
        )) {
        if ($writerText -match "function\s+$duplicateFunction\b") {
            throw "$writerScript must not duplicate safety helper function: $duplicateFunction"
        }
    }
}

Write-Host "OK   repair operator safety helper guardrails"
