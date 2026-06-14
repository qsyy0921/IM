$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$serviceCmdRoot = Join-Path $repoRoot "services"

$cmdMainFiles = Get-ChildItem -LiteralPath $serviceCmdRoot -Recurse -Filter "main.go" -File |
    Where-Object { $_.FullName -match "\\cmd\\" }

$violations = @()
foreach ($file in $cmdMainFiles) {
    $content = Get-Content -LiteralPath $file.FullName -Raw
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
            $relative = [System.IO.Path]::GetRelativePath($repoRoot, $file.FullName)
            $violations += "${relative}: $($match.Value.Trim('"')) is missing $allowPublicEnv public exposure opt-in guard"
        }
    }
}

if ($violations.Count -gt 0) {
    throw ($violations -join [Environment]::NewLine)
}

Write-Host "OK   debug listener exposure guardrails"
