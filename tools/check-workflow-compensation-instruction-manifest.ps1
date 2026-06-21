$ErrorActionPreference = "Stop"

$writerPath = Join-Path $PSScriptRoot "write-workflow-compensation-instruction-manifest.ps1"
$validatorPath = Join-Path $PSScriptRoot "validate-workflow-compensation-instruction-manifest.ps1"
$safetyHelperPath = Join-Path $PSScriptRoot "repair-operator-safety.ps1"
if (-not (Test-Path -LiteralPath $safetyHelperPath -PathType Leaf)) {
    throw "Missing repair operator safety helper: $safetyHelperPath"
}
. $safetyHelperPath
foreach ($path in @($writerPath, $validatorPath)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Missing workflow compensation instruction manifest tool dependency: $path"
    }
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-workflow-compensation-instruction-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
try {
    $reasonPath = Join-Path $tempRoot "reason.txt"
    $payloadRefPath = Join-Path $tempRoot "payload-ref.txt"
    $manifestPath = Join-Path $tempRoot "workflow-compensation-instruction.json"
    "rollback approved after reviewing external ticket" | Set-Content -LiteralPath $reasonPath -Encoding UTF8
    "external payload lives in protected approval system" | Set-Content -LiteralPath $payloadRefPath -Encoding UTF8
    $payloadHash = "sha256:" + (Get-RepairSha256Hex -Bytes ([System.IO.File]::ReadAllBytes($payloadRefPath)))

    & powershell -NoProfile -ExecutionPolicy Bypass -File $writerPath `
        -OutputPath $manifestPath `
        -WorkflowID "wf_comp_1" `
        -PayloadRefFile $payloadRefPath `
        -Environment "local" `
        -ConfigKind "API_GATEWAY_TENANT_QUOTA" `
        -BundleKey "tenant-a" `
        -TargetVersion "quota-v1" `
        -OperatorRef "operator:rollback" `
        -ReasonFile $reasonPath | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "write-workflow-compensation-instruction-manifest.ps1 failed"
    }

    $summaryRaw = & powershell -NoProfile -ExecutionPolicy Bypass -File $validatorPath `
        -ManifestPath $manifestPath `
        -ExpectedWorkflowID "wf_comp_1" `
        -ExpectedPayloadRefHash $payloadHash `
        -ExpectedTargetVersion "quota-v1"
    if ($LASTEXITCODE -ne 0) {
        throw "validate-workflow-compensation-instruction-manifest.ps1 failed"
    }
    $summary = ($summaryRaw -join "`n") | ConvertFrom-Json
    if ($summary.instruction_count -ne 1 -or @($summary.instructions).Count -ne 1) {
        throw "Workflow compensation instruction manifest summary has unexpected instruction count."
    }

    $manifestRaw = Get-Content -LiteralPath $manifestPath -Raw
    $manifest = $manifestRaw | ConvertFrom-Json
    $instruction = @($manifest.instructions)[0]
    if ($instruction.workflow_id -ne "wf_comp_1" -or
        $instruction.payload_ref_hash -ne $payloadHash -or
        $instruction.reason_ref -notmatch "^reason-sha256:[a-f0-9]{64}$" -or
        $instruction.target_version -ne "quota-v1") {
        throw "Workflow compensation instruction manifest has unexpected normalized fields."
    }
    if ($manifestRaw.Contains("rollback approved after reviewing external ticket") -or
        $manifestRaw.Contains("external payload lives in protected approval system") -or
        $manifestRaw.Contains("reason.txt") -or
        $manifestRaw.Contains("payload-ref.txt")) {
        throw "Workflow compensation instruction manifest leaked reason text, payload ref body, or local paths."
    }

    $unknownPath = Join-Path $tempRoot "unknown-field.json"
    @'
{
  "instructions": [
    {
      "workflow_id": "wf_comp_1",
      "payload_ref_hash": "sha256:payload",
      "environment": "local",
      "config_kind": "API_GATEWAY_TENANT_QUOTA",
      "bundle_key": "tenant-a",
      "target_version": "quota-v1",
      "operator_ref": "operator:rollback",
      "reason_ref": "reason-sha256:rollback",
      "raw_payload": "must not be embedded"
    }
  ]
}
'@ | Set-Content -LiteralPath $unknownPath -Encoding UTF8
    $badExitCode = 0
    try {
        & powershell -NoProfile -ExecutionPolicy Bypass -File $validatorPath -ManifestPath $unknownPath *> $null
        $badExitCode = $LASTEXITCODE
    } catch {
        $badExitCode = 1
    }
    if ($badExitCode -eq 0) {
        throw "Workflow compensation instruction manifest validator should reject unknown/raw payload fields."
    }

    $badOutput = ""
    $badExitCode = 0
    try {
        $badOutput = & powershell -NoProfile -ExecutionPolicy Bypass -File $writerPath `
            -OutputPath (Join-Path $tempRoot "bad.json") `
            -WorkflowID "wf_comp_1" `
            -PayloadRefHash "sha256:payload" `
            -Environment "local" `
            -ConfigKind "API_GATEWAY_TENANT_QUOTA" `
            -BundleKey "tenant-a" `
            -TargetVersion "quota-v1" `
            -OperatorRef "operator-token-secret" `
            -ReasonRef "reason-sha256:rollback" 2>&1
        $badExitCode = $LASTEXITCODE
    } catch {
        $badOutput = $_.Exception.Message
        $badExitCode = 1
    }
    if ($badExitCode -eq 0) {
        throw "Workflow compensation instruction manifest writer should reject credential-like operator refs."
    }
    if (($badOutput -join "`n") -notmatch "low-sensitive") {
        throw "Workflow compensation instruction manifest writer rejection did not mention low-sensitive refs."
    }
} finally {
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host "OK   workflow compensation instruction manifest self-test"
