$ErrorActionPreference = "Stop"

$validator = Join-Path $PSScriptRoot "validate-capacity-longrun-campaign-evidence.ps1"
$adder = Join-Path $PSScriptRoot "add-capacity-longrun-campaign-evidence.ps1"
$writer = Join-Path $PSScriptRoot "write-capacity-longrun-campaign-plan.ps1"
$powerShellExe = (Get-Command powershell -ErrorAction Stop).Source

foreach ($path in @($validator, $adder, $writer)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Missing capacity long-run campaign evidence dependency: $path"
    }
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
        [string]$ExpectedResultRoot,
        [switch]$RequireFiles,
        [string]$MarkdownPath = ""
    )

    try {
        $args = @{
            ManifestPath = $ManifestPath
            ExpectedResultRoot = $ExpectedResultRoot
        }
        if ($RequireFiles) {
            $args.RequireFiles = $true
        }
        if ($MarkdownPath.Trim().Length -gt 0) {
            $args.MarkdownPath = $MarkdownPath
        }
        $output = & $validator @args 2>&1
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

function Invoke-Adder {
    param([string[]]$Arguments)

    try {
        $output = & $powerShellExe -NoProfile -ExecutionPolicy Bypass -File $adder @Arguments 2>&1
        return [pscustomobject]@{
            ExitCode = $LASTEXITCODE
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

$repoManifest = Join-Path (Split-Path -Parent $PSScriptRoot) "docs\runbook\capacity-longrun-campaign-evidence.json"
$repoResult = Invoke-Validator -ManifestPath $repoManifest -ExpectedResultRoot "H:\NexusIM\loadtest-results"
if ($repoResult.ExitCode -ne 0) {
    Write-Host "FAIL repo capacity long-run campaign evidence manifest should pass." -ForegroundColor Red
    Write-Host $repoResult.Output -ForegroundColor Red
    exit 1
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("nexusim-capacity-longrun-evidence-" + [System.Guid]::NewGuid().ToString("N"))
try {
    New-Item -ItemType Directory -Force -Path $tempRoot | Out-Null
    $campaignRoot = Join-Path $tempRoot "campaigns"
    $planName = "longrun-evidence-selftest"
    $writerOutput = & $powerShellExe -NoProfile -ExecutionPolicy Bypass -File $writer `
        -OutputRoot $campaignRoot `
        -CampaignName $planName `
        -Services "message-service,push-gateway" `
        -Duration "30m" `
        -Warmup "2m" `
        -VUs 2 `
        -MaxVUs 4 `
        -TargetEnvironment "fixture" `
        -Operator "fixture-operator" `
        -Notes "fixture campaign plan" 2>&1
    if ($LASTEXITCODE -ne 0) {
        Write-Host "FAIL capacity long-run campaign plan fixture should be written." -ForegroundColor Red
        Write-Host (($writerOutput | Out-String).Trim()) -ForegroundColor Red
        exit 1
    }

    $planPath = Join-Path $campaignRoot "$planName\capacity-longrun-campaign-plan.json"
    $manifestPath = Join-Path $tempRoot "capacity-longrun-campaign-evidence.json"
    Write-JsonFile -Path $manifestPath -Value ([ordered]@{
        schema_version = 1
        updated_at = "2026-06-18"
        scope = "self-test long-run capacity evidence"
        entries = @(
            [ordered]@{
                name = "planned-fixture"
                status = "planned"
                plan_path = $planPath
                note = "fixture"
            }
        )
    })

    $markdownPath = Join-Path $tempRoot "capacity-longrun-campaign-evidence.md"
    $goodResult = Invoke-Validator -ManifestPath $manifestPath -ExpectedResultRoot $campaignRoot -RequireFiles -MarkdownPath $markdownPath
    if ($goodResult.ExitCode -ne 0) {
        Write-Host "FAIL capacity long-run campaign evidence fixture should pass." -ForegroundColor Red
        Write-Host $goodResult.Output -ForegroundColor Red
        exit 1
    }
    $markdown = Get-Content -LiteralPath $markdownPath -Raw
    if (-not $markdown.Contains("Capacity Long-Run Campaign Evidence") -or -not $markdown.Contains("not a production SLO") -or -not $markdown.Contains("planned-fixture")) {
        Write-Host "FAIL capacity long-run campaign evidence markdown missing boundary text." -ForegroundColor Red
        exit 1
    }

    $repoPlanManifest = Join-Path $tempRoot "repo-plan-evidence.json"
    Write-JsonFile -Path $repoPlanManifest -Value ([ordered]@{
        schema_version = 1
        updated_at = "2026-06-18"
        scope = "self-test"
        entries = @(
            [ordered]@{
                name = "bad-repo-plan"
                status = "planned"
                plan_path = "docs/runbook/capacity-longrun-campaign-evidence.json"
                note = "fixture"
            }
        )
    })
    $repoPlanResult = Invoke-Validator -ManifestPath $repoPlanManifest -ExpectedResultRoot $campaignRoot
    if ($repoPlanResult.ExitCode -eq 0 -or -not $repoPlanResult.Output.Contains("must point under")) {
        Write-Host "FAIL capacity long-run evidence with repo-local plan_path should fail." -ForegroundColor Red
        Write-Host $repoPlanResult.Output -ForegroundColor Red
        exit 1
    }

    $badCompletedManifest = Join-Path $tempRoot "bad-completed-evidence.json"
    Write-JsonFile -Path $badCompletedManifest -Value ([ordered]@{
        schema_version = 1
        updated_at = "2026-06-18"
        scope = "self-test"
        entries = @(
            [ordered]@{
                name = "bad-completed"
                status = "completed"
                plan_path = $planPath
                note = "fixture"
            }
        )
    })
    $badCompletedResult = Invoke-Validator -ManifestPath $badCompletedManifest -ExpectedResultRoot $campaignRoot
    if ($badCompletedResult.ExitCode -eq 0 -or -not $badCompletedResult.Output.Contains("summary_path is required")) {
        Write-Host "FAIL completed capacity long-run evidence without summary/report should fail." -ForegroundColor Red
        Write-Host $badCompletedResult.Output -ForegroundColor Red
        exit 1
    }

    $emptyResult = Invoke-Validator -ManifestPath $repoManifest -ExpectedResultRoot "H:\NexusIM\loadtest-results" -MarkdownPath (Join-Path $tempRoot "repo-empty.md")
    if ($emptyResult.ExitCode -ne 0) {
        Write-Host "FAIL empty repo capacity long-run campaign evidence manifest should pass." -ForegroundColor Red
        Write-Host $emptyResult.Output -ForegroundColor Red
        exit 1
    }

    $adderManifestPath = Join-Path $tempRoot "adder-evidence.json"
    Write-JsonFile -Path $adderManifestPath -Value ([ordered]@{
        schema_version = 1
        updated_at = "2026-06-18"
        scope = "self-test adder"
        entries = @()
    })

    $addResult = Invoke-Adder -Arguments @(
        "-ManifestPath", $adderManifestPath,
        "-ExpectedResultRoot", $campaignRoot,
        "-Name", "adder-fixture",
        "-Status", "planned",
        "-PlanPath", $planPath,
        "-Note", "fixture"
    )
    if ($addResult.ExitCode -ne 0) {
        Write-Host "FAIL capacity long-run campaign evidence adder should pass." -ForegroundColor Red
        Write-Host $addResult.Output -ForegroundColor Red
        exit 1
    }
    $added = Get-Content -LiteralPath $adderManifestPath -Raw | ConvertFrom-Json
    if (@($added.entries).Count -ne 1 -or @($added.entries)[0].name -ne "adder-fixture") {
        Write-Host "FAIL capacity long-run campaign evidence adder did not write expected entry." -ForegroundColor Red
        exit 1
    }

    $duplicateResult = Invoke-Adder -Arguments @(
        "-ManifestPath", $adderManifestPath,
        "-ExpectedResultRoot", $campaignRoot,
        "-Name", "adder-fixture",
        "-Status", "planned",
        "-PlanPath", $planPath,
        "-Note", "fixture"
    )
    if ($duplicateResult.ExitCode -eq 0 -or -not $duplicateResult.Output.Contains("already exists")) {
        Write-Host "FAIL duplicate capacity long-run campaign evidence entry should require -Replace." -ForegroundColor Red
        Write-Host $duplicateResult.Output -ForegroundColor Red
        exit 1
    }

    $sensitiveResult = Invoke-Adder -Arguments @(
        "-ManifestPath", $adderManifestPath,
        "-ExpectedResultRoot", $campaignRoot,
        "-Name", "sensitive-fixture",
        "-Status", "planned",
        "-PlanPath", $planPath,
        "-Note", "token=super-secret"
    )
    if ($sensitiveResult.ExitCode -eq 0 -or -not $sensitiveResult.Output.Contains("must be low-sensitive")) {
        Write-Host "FAIL capacity long-run campaign evidence adder should reject sensitive note." -ForegroundColor Red
        Write-Host $sensitiveResult.Output -ForegroundColor Red
        exit 1
    }
}
finally {
    if (Test-Path -LiteralPath $tempRoot) {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    }
}

Write-Host "OK   capacity long-run campaign evidence self-test"
