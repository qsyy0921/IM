$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$serviceCmdRoot = Join-Path $repoRoot "services"

$cmdMainFiles = Get-ChildItem -LiteralPath $serviceCmdRoot -Recurse -Filter "main.go" -File |
    Where-Object { $_.FullName -match "\\cmd\\" }

function Convert-ToRepoRelativePath {
    param([string]$Path)

    $root = [System.IO.Path]::GetFullPath($repoRoot).TrimEnd("\", "/")
    $fullPath = [System.IO.Path]::GetFullPath($Path)
    $prefix = $root + [System.IO.Path]::DirectorySeparatorChar
    if ($fullPath.StartsWith($prefix, [System.StringComparison]::OrdinalIgnoreCase)) {
        return $fullPath.Substring($prefix.Length)
    }
    return $fullPath
}

$violations = @()
foreach ($file in $cmdMainFiles) {
    $content = Get-Content -LiteralPath $file.FullName -Raw
    $testFile = Join-Path $file.DirectoryName "main_test.go"
    $testContent = ""
    if (Test-Path -LiteralPath $testFile) {
        $testContent = Get-Content -LiteralPath $testFile -Raw
    }

    $addrMatches = [regex]::Matches($content, '"NEXUSIM_([A-Z0-9_]+)_DEBUG_ADDR"')
    $seenPrefixes = @{}
    foreach ($match in $addrMatches) {
        $prefix = $match.Groups[1].Value
        if ($seenPrefixes.ContainsKey($prefix)) {
            continue
        }
        $seenPrefixes[$prefix] = $true
        $allowPublicEnv = "NEXUSIM_${prefix}_DEBUG_ALLOW_PUBLIC"
        if (-not $content.Contains($allowPublicEnv)) {
            $relative = Convert-ToRepoRelativePath -Path $file.FullName
            $violations += "${relative}: $($match.Value.Trim('"')) is missing $allowPublicEnv public exposure opt-in guard"
        }
    }

    $validatorMatches = [regex]::Matches($content, "func (validate[A-Za-z0-9]+DebugListenerConfig)\(")
    $seenValidators = @{}
    foreach ($match in $validatorMatches) {
        $validatorName = $match.Groups[1].Value
        if ($seenValidators.ContainsKey($validatorName)) {
            continue
        }
        $seenValidators[$validatorName] = $true

        $relative = Convert-ToRepoRelativePath -Path $file.FullName
        if ($testContent.Length -eq 0) {
            $violations += "${relative}: $validatorName is missing main_test.go coverage"
            continue
        }

        $exportedValidatorName = $validatorName.Substring(0, 1).ToUpperInvariant() + $validatorName.Substring(1)
        $requiredTests = @(
            "Test${exportedValidatorName}AllowsEmptyOrPrivateAddress",
            "Test${exportedValidatorName}RejectsPublicAddressByDefault",
            "Test${exportedValidatorName}AllowsExplicitPublicOptIn"
        )
        foreach ($requiredTest in $requiredTests) {
            if (-not $testContent.Contains($requiredTest)) {
                $violations += "${relative}: $validatorName is missing $requiredTest"
            }
        }
    }
}

if ($violations.Count -gt 0) {
    throw ($violations -join [Environment]::NewLine)
}

Write-Host "OK   debug listener exposure guardrails"
