$ErrorActionPreference = "Stop"

$helperPath = Join-Path $PSScriptRoot "repair-operator-safety.ps1"
if (-not (Test-Path -LiteralPath $helperPath -PathType Leaf)) {
    throw "Missing repair operator safety helper: $helperPath"
}

$helperText = Get-Content -LiteralPath $helperPath -Raw
foreach ($functionName in @(
        "Get-RepairSha256Hex",
        "Assert-LowSensitiveRepairActor",
        "Assert-LowSensitiveRepairIdentifier",
        "Assert-LowSensitiveRepairAdHocEnv",
        "Read-RepairReasonFileSummary"
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

Assert-LowSensitiveRepairIdentifier -Value "approval-1" -FieldName "ApprovalID"
Assert-LowSensitiveRepairIdentifier -Value "repair-batch:local-1" -FieldName "BatchID"
Assert-FailsWith -Label "blank repair identifier" -Pattern "required" -Script {
    Assert-LowSensitiveRepairIdentifier -Value " " -FieldName "ApprovalID"
}
Assert-FailsWith -Label "email repair identifier" -Pattern "repair identifier" -Script {
    Assert-LowSensitiveRepairIdentifier -Value "operator@example.com" -FieldName "ApprovalID"
}
Assert-FailsWith -Label "credential repair identifier" -Pattern "credential-like" -Script {
    Assert-LowSensitiveRepairIdentifier -Value "approval-token-secret" -FieldName "ApprovalID"
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

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-repair-safety-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
try {
    $reasonPath = Join-Path $tempRoot "reason.txt"
    "short operator reason" | Set-Content -LiteralPath $reasonPath -Encoding UTF8
    $reasonSummary = Read-RepairReasonFileSummary -Path $reasonPath -MissingMessage "Missing repair reason"
    if (-not $reasonSummary.Present -or [string]::IsNullOrWhiteSpace([string]$reasonSummary.Sha256) -or $reasonSummary.ByteLength -le 0) {
        throw "Read-RepairReasonFileSummary should hash non-empty reason files."
    }

    $largeReasonPath = Join-Path $tempRoot "large-reason.txt"
    $largeBytes = New-Object byte[] (64 * 1024 + 1)
    [System.IO.File]::WriteAllBytes($largeReasonPath, $largeBytes)
    Assert-FailsWith -Label "large reason file" -Pattern "too large" -Script {
        Read-RepairReasonFileSummary -Path $largeReasonPath -MissingMessage "Missing repair reason" | Out-Null
    }
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
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
            "Assert-LowSensitiveRepairIdentifier",
            "Assert-LowSensitiveAdHocEnv"
        )) {
        if ($writerText -match "function\s+$duplicateFunction\b") {
            throw "$writerScript must not duplicate safety helper function: $duplicateFunction"
        }
    }
}

Write-Host "OK   repair operator safety helper guardrails"
