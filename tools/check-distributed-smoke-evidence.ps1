$ErrorActionPreference = "Stop"

$validator = Join-Path $PSScriptRoot "validate-distributed-smoke-evidence.ps1"

if (-not (Test-Path -LiteralPath $validator -PathType Leaf)) {
    throw "Missing distributed smoke evidence validator: $validator"
}

function Write-JsonFile {
    param(
        [string]$Path,
        $Value
    )

    $dir = Split-Path -Parent $Path
    if ($dir -and -not (Test-Path -LiteralPath $dir)) {
        New-Item -ItemType Directory -Force -Path $dir | Out-Null
    }
    $Value | ConvertTo-Json -Depth 10 | Set-Content -LiteralPath $Path -Encoding UTF8
}

function Invoke-Validator {
    param(
        [string]$ManifestPath,
        [switch]$RequireFiles,
        [string]$MarkdownPath = ""
    )

    try {
        $invocationArgs = @{
            ManifestPath = $ManifestPath
        }
        if ($RequireFiles) {
            $invocationArgs.RequireFiles = $true
        }
        if ($MarkdownPath.Trim().Length -gt 0) {
            $invocationArgs.MarkdownPath = $MarkdownPath
        }
        $output = & $validator @invocationArgs 2>&1
        return [pscustomobject]@{
            ExitCode = 0
            Output = (($output | Out-String).Trim())
        }
    }
    catch {
        return [pscustomobject]@{
            ExitCode = 1
            Output = [string]$_.Exception.Message
        }
    }
}

$repoManifest = Join-Path (Split-Path -Parent $PSScriptRoot) "docs\runbook\distributed-smoke-evidence.json"
$schemaOnlyResult = Invoke-Validator -ManifestPath $repoManifest
if ($schemaOnlyResult.ExitCode -ne 0) {
    Write-Host "FAIL repo distributed smoke evidence manifest schema should pass." -ForegroundColor Red
    Write-Host $schemaOnlyResult.Output -ForegroundColor Red
    exit 1
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-distributed-evidence-" + [System.Guid]::NewGuid().ToString("N"))
try {
    New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
    $pushPath = Join-Path $tempRoot "pushgateway-summary.json"
    Write-JsonFile -Path $pushPath -Value ([ordered]@{
        git_dirty = $false
        scenario = "full"
        success = $true
        send_message = [ordered]@{ conversation_seq = 2 }
        pull_inbox = [ordered]@{ item_count = 1; max_seq = 2 }
        delivery_ack_ok = [ordered]@{ op = "delivery.ack.ok" }
        delivery_outbox_published = 2
        delivery_outbox_pending = 0
        delivery_outbox_dlq = 0
    })
    $manifestPath = Join-Path $tempRoot "distributed-smoke-evidence.json"
    Write-JsonFile -Path $manifestPath -Value ([ordered]@{
        schema_version = 1
        scope = "self-test"
        entries = @(
            [ordered]@{
                name = "self-test pushgateway"
                kind = "pushgateway-full"
                summary_path = $pushPath
                expected_scenario = "full"
                require_clean_git = $true
            }
        )
    })
    $markdownPath = Join-Path $tempRoot "distributed-smoke-evidence.md"
    $goodResult = Invoke-Validator -ManifestPath $manifestPath -RequireFiles -MarkdownPath $markdownPath
    if ($goodResult.ExitCode -ne 0) {
        Write-Host "FAIL distributed smoke evidence fixture should pass." -ForegroundColor Red
        Write-Host $goodResult.Output -ForegroundColor Red
        exit 1
    }
    $markdown = Get-Content -LiteralPath $markdownPath -Raw
    if (-not $markdown.Contains("NexusIM Distributed Smoke Evidence") -or -not $markdown.Contains("self-test pushgateway") -or -not $markdown.Contains("not a production HA or SLO claim")) {
        Write-Host "FAIL distributed smoke evidence markdown missing expected boundary text." -ForegroundColor Red
        exit 1
    }

    $badManifestPath = Join-Path $tempRoot "bad-distributed-smoke-evidence.json"
    Write-JsonFile -Path $badManifestPath -Value ([ordered]@{
        schema_version = 1
        scope = "self-test"
        entries = @(
            [ordered]@{
                name = "bad duplicate"
                kind = "pushgateway-full"
                summary_path = $pushPath
            },
            [ordered]@{
                name = "bad duplicate"
                kind = "pushgateway-full"
                summary_path = $pushPath
            }
        )
    })
    $badResult = Invoke-Validator -ManifestPath $badManifestPath
    if ($badResult.ExitCode -eq 0) {
        Write-Host "FAIL duplicate distributed smoke evidence entries should fail." -ForegroundColor Red
        exit 1
    }
    if (-not $badResult.Output.Contains("duplicate")) {
        Write-Host "FAIL bad distributed smoke evidence fixture returned unexpected error." -ForegroundColor Red
        Write-Host $badResult.Output -ForegroundColor Red
        exit 1
    }
}
finally {
    if (Test-Path -LiteralPath $tempRoot) {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    }
}

Write-Host "OK   distributed smoke evidence self-test"
