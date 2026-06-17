$ErrorActionPreference = "Stop"

$toolsRoot = $PSScriptRoot
$checkLocalPath = Join-Path $toolsRoot "check-local.ps1"

if (-not (Test-Path -LiteralPath $checkLocalPath -PathType Leaf)) {
    throw "Missing check-local.ps1"
}

$checkLocal = [System.IO.File]::ReadAllText($checkLocalPath, [System.Text.Encoding]::UTF8)

$indirectChecks = @{
    "check-api-gateway-legacy-descriptor-migration.ps1" = "check-api-gateway-gates.ps1"
    "check-api-gateway-legacy-observation-window.ps1" = "check-api-gateway-gates.ps1"
    "check-api-gateway-legacy-removal-plan.ps1" = "check-api-gateway-gates.ps1"
    "check-api-gateway-quota-snapshot.ps1" = "check-api-gateway-gates.ps1"
    "validate-api-gateway-legacy-removal-plan.ps1" = "check-api-gateway-gates.ps1"
}

$manualChecks = @(
    "check-mac-docker-desktop.ps1"
)

function Test-ScriptMentions {
    param(
        [string]$ScriptName,
        [string]$Text
    )

    return $Text.Contains($ScriptName)
}

$violations = [System.Collections.Generic.List[string]]::new()
$checkScripts = Get-ChildItem -LiteralPath $toolsRoot -File -Filter "check-*.ps1" | Sort-Object Name

foreach ($script in $checkScripts) {
    $scriptName = $script.Name

    if ($scriptName -eq "check-local.ps1") {
        continue
    }

    if (Test-ScriptMentions -ScriptName $scriptName -Text $checkLocal) {
        continue
    }

    if ($indirectChecks.ContainsKey($scriptName)) {
        $parentName = [string]$indirectChecks[$scriptName]
        if (-not (Test-ScriptMentions -ScriptName $parentName -Text $checkLocal)) {
            $violations.Add("$scriptName is indirect through $parentName, but $parentName is not invoked by check-local.ps1")
            continue
        }

        $parentPath = Join-Path $toolsRoot $parentName
        if (-not (Test-Path -LiteralPath $parentPath -PathType Leaf)) {
            $violations.Add("$scriptName declares missing indirect parent: $parentName")
            continue
        }

        $parentText = [System.IO.File]::ReadAllText($parentPath, [System.Text.Encoding]::UTF8)
        if (-not (Test-ScriptMentions -ScriptName $scriptName -Text $parentText)) {
            $violations.Add("$scriptName is not invoked by check-local.ps1 and is not mentioned by indirect parent $parentName")
        }
        continue
    }

    if ($manualChecks -contains $scriptName) {
        continue
    }

    $violations.Add("$scriptName is not invoked by check-local.ps1. Add it to check-local.ps1 or list an explicit indirect/manual exception in check-local-coverage.ps1.")
}

foreach ($manualCheck in $manualChecks) {
    if (-not (Test-Path -LiteralPath (Join-Path $toolsRoot $manualCheck) -PathType Leaf)) {
        $violations.Add("manual exception references missing script: $manualCheck")
    }
}

foreach ($entry in $indirectChecks.GetEnumerator()) {
    if (-not (Test-Path -LiteralPath (Join-Path $toolsRoot $entry.Key) -PathType Leaf)) {
        $violations.Add("indirect exception references missing script: $($entry.Key)")
    }
    if (-not (Test-Path -LiteralPath (Join-Path $toolsRoot $entry.Value) -PathType Leaf)) {
        $violations.Add("indirect exception references missing parent: $($entry.Value)")
    }
}

if ($violations.Count -gt 0) {
    throw ($violations -join [Environment]::NewLine)
}

Write-Host "OK   check-local covers check scripts, with explicit indirect/manual exceptions."
