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

$appendScripts = Get-ChildItem -LiteralPath $PSScriptRoot -Filter "add-*evidence*.ps1" -File
if (@($appendScripts).Count -lt 1) {
    Write-Host "FAIL expected at least one evidence append script." -ForegroundColor Red
    exit 1
}

foreach ($script in $appendScripts) {
    $content = Get-Content -LiteralPath $script.FullName -Raw
    if (-not $content.Contains("evidence-metadata-safety.ps1")) {
        Write-Host "FAIL $($script.Name) must dot-source evidence-metadata-safety.ps1." -ForegroundColor Red
        exit 1
    }
    if (-not $content.Contains("Assert-LowSensitiveEvidenceText")) {
        Write-Host "FAIL $($script.Name) must validate caller-supplied evidence metadata." -ForegroundColor Red
        exit 1
    }
}

Write-Host "OK   evidence metadata safety helper self-test"
