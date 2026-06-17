$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "evidence-metadata-safety.ps1")

function Assert-Passes {
    param(
        [string]$Value,
        [string]$Message
    )

    try {
        Assert-LowSensitiveEvidenceText -Value $Value -FieldName "Field"
    }
    catch {
        Write-Host "FAIL $Message" -ForegroundColor Red
        Write-Host $_.Exception.Message -ForegroundColor Red
        exit 1
    }
}

function Assert-Fails {
    param(
        [string]$Value,
        [string]$Message
    )

    try {
        Assert-LowSensitiveEvidenceText -Value $Value -FieldName "Field"
        Write-Host "FAIL $Message" -ForegroundColor Red
        exit 1
    }
    catch {
        if ($_.Exception.Message -notmatch "low-sensitive evidence metadata|too long|required") {
            Write-Host "FAIL $Message returned unexpected error." -ForegroundColor Red
            Write-Host $_.Exception.Message -ForegroundColor Red
            exit 1
        }
    }
}

function Assert-EvidenceAppenderSafety {
    param(
        [string]$Name,
        [string]$Content
    )

    if (-not $Content.Contains("evidence-metadata-safety.ps1")) {
        throw "$Name must dot-source evidence-metadata-safety.ps1."
    }
    if (-not $Content.Contains("Assert-LowSensitiveEvidenceText")) {
        throw "$Name must validate caller-supplied evidence metadata."
    }
    if (-not ($Content -match '\$originalJson\s*=\s*Get-Content\s+-LiteralPath\s+\$resolvedManifestPath\s+-Raw')) {
        throw "$Name must capture original manifest JSON before writing."
    }
    if (-not ($Content -match '(?s)catch\s*\{.*\$originalJson')) {
        throw "$Name must restore original manifest JSON if validation fails."
    }
    if (-not $Content.Contains("[System.IO.File]::WriteAllText(`$resolvedManifestPath, `$originalJson")) {
        throw "$Name must restore the original manifest text exactly on validation failure."
    }
}

Assert-Passes -Value "local redis cluster failover fixture" -Message "Plain low-sensitive evidence names should pass."
Assert-Passes -Value "H:\NexusIM\loadtest-results\run-1\summary.json" -Message "Plain local evidence paths should pass."
Assert-Passes -Value "gateway token mode verified without storing token value" -Message "Non-secret technical wording should pass."

Assert-Fails -Value "operator@example.com" -Message "Email values should be rejected."
Assert-Fails -Value "Bearer abcdefghijklmnop" -Message "Bearer token values should be rejected."
Assert-Fails -Value "https://user:password@example.test/metrics" -Message "URL userinfo should be rejected."
Assert-Fails -Value "summary.json?token=secret" -Message "Token query values should be rejected."
Assert-Fails -Value "sk-abcdefghijklmnop" -Message "OpenAI-style secret keys should be rejected."
Assert-Fails -Value "eyJaaaaaaaaaaa.payload.signature" -Message "JWT-like values should be rejected."
Assert-Fails -Value "" -Message "Required evidence metadata should reject blanks."

try {
    Assert-EvidenceAppenderSafety -Name "bad-appender.ps1" -Content @'
. (Join-Path $PSScriptRoot "evidence-metadata-safety.ps1")
Assert-LowSensitiveEvidenceText -Value $Name -FieldName "Name"
$manifest = Get-Content -LiteralPath $resolvedManifestPath -Raw | ConvertFrom-Json
$updated | ConvertTo-Json -Depth 10 | Set-Content -LiteralPath $resolvedManifestPath -Encoding UTF8
& $validator -ManifestPath $resolvedManifestPath | Out-Null
'@
    Write-Host "FAIL evidence append safety should reject missing rollback." -ForegroundColor Red
    exit 1
}
catch {
    if ($_.Exception.Message -notmatch "original manifest|restore") {
        Write-Host "FAIL evidence append safety rejected missing rollback with unexpected error." -ForegroundColor Red
        Write-Host $_.Exception.Message -ForegroundColor Red
        exit 1
    }
}

try {
    Assert-EvidenceAppenderSafety -Name "good-appender.ps1" -Content @'
. (Join-Path $PSScriptRoot "evidence-metadata-safety.ps1")
Assert-LowSensitiveEvidenceText -Value $Name -FieldName "Name"
$originalJson = Get-Content -LiteralPath $resolvedManifestPath -Raw
$manifest = $originalJson | ConvertFrom-Json
try {
    $updated | ConvertTo-Json -Depth 10 | Set-Content -LiteralPath $resolvedManifestPath -Encoding UTF8
    & $validator -ManifestPath $resolvedManifestPath | Out-Null
}
catch {
    $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText($resolvedManifestPath, $originalJson, $utf8NoBom)
    throw
}
'@
}
catch {
    Write-Host "FAIL evidence append safety should accept rollback-protected appender." -ForegroundColor Red
    Write-Host $_.Exception.Message -ForegroundColor Red
    exit 1
}

$appendScripts = Get-ChildItem -LiteralPath $PSScriptRoot -Filter "add-*evidence*.ps1" -File
if (@($appendScripts).Count -lt 1) {
    Write-Host "FAIL expected at least one evidence append script." -ForegroundColor Red
    exit 1
}

foreach ($script in $appendScripts) {
    $content = Get-Content -LiteralPath $script.FullName -Raw
    try {
        Assert-EvidenceAppenderSafety -Name $script.Name -Content $content
    }
    catch {
        Write-Host "FAIL $($_.Exception.Message)" -ForegroundColor Red
        exit 1
    }
}

Write-Host "OK   evidence metadata safety helper self-test"
