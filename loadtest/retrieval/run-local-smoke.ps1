param(
    [string]$PgDsn = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable",
    [string]$ResultRoot = "H:\NexusIM\loadtest-results",
    [string]$RunName = "",
    [string]$SearchGrpcAddr = "127.0.0.1:10570",
    [string]$MemoryGrpcAddr = "127.0.0.1:10580",
    [string]$RetrievalGrpcAddr = "127.0.0.1:10590",
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot "..\.."))
. (Join-Path $repoRoot "tools\output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $ResultRoot -RepositoryRoot $repoRoot -Name "ResultRoot"

if (-not $RunName) {
    $RunName = "retrieval-gateway-evidence-smoke-" + (Get-Date -Format "yyyyMMdd-HHmmss")
}

$resultDir = Join-Path $ResultRoot $RunName
$logDir = Join-Path $resultDir "logs"
New-Item -ItemType Directory -Force $logDir | Out-Null

. (Join-Path $repoRoot "tools\go-env.ps1")

if (-not $SkipBuild) {
    go build -o (Join-Path $repoRoot "bin\search-service.exe") ./services/search-service/cmd/search-service
    go build -o (Join-Path $repoRoot "bin\memory-service.exe") ./services/memory-service/cmd/memory-service
    go build -o (Join-Path $repoRoot "bin\retrieval-gateway.exe") ./services/retrieval-gateway/cmd/retrieval-gateway
    go build -o (Join-Path $repoRoot "bin\retrieval-smoke.exe") ./loadtest/retrieval
}

function Start-ProcessRole {
    param(
        [string]$Name,
        [string]$Exe,
        [hashtable]$Env
    )

    $stdout = Join-Path $logDir "$Name.out.log"
    $stderr = Join-Path $logDir "$Name.err.log"
    $psi = [System.Diagnostics.ProcessStartInfo]::new()
    $psi.FileName = $Exe
    $psi.WorkingDirectory = $repoRoot
    $psi.UseShellExecute = $false
    $psi.CreateNoWindow = $true
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    foreach ($key in $Env.Keys) {
        $psi.Environment[$key] = [string]$Env[$key]
    }

    $process = [System.Diagnostics.Process]::new()
    $process.StartInfo = $psi
    $null = $process.Start()
    Register-ObjectEvent `
        -InputObject $process `
        -EventName OutputDataReceived `
        -Action {
            if ($EventArgs.Data) {
                Add-Content -LiteralPath $Event.MessageData -Value $EventArgs.Data
            }
        } `
        -MessageData $stdout | Out-Null
    Register-ObjectEvent `
        -InputObject $process `
        -EventName ErrorDataReceived `
        -Action {
            if ($EventArgs.Data) {
                Add-Content -LiteralPath $Event.MessageData -Value $EventArgs.Data
            }
        } `
        -MessageData $stderr | Out-Null
    $process.BeginOutputReadLine()
    $process.BeginErrorReadLine()
    return $process
}

$processes = @()
try {
    $processes += Start-ProcessRole -Name "search-grpc" -Exe (Join-Path $repoRoot "bin\search-service.exe") -Env @{
        NEXUSIM_SEARCH_SERVICE_MODE = "grpc"
        NEXUSIM_SEARCH_GRPC_ADDR = $SearchGrpcAddr
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_SEARCH_DEBUG_ADDR = ""
    }
    $processes += Start-ProcessRole -Name "memory-grpc" -Exe (Join-Path $repoRoot "bin\memory-service.exe") -Env @{
        NEXUSIM_MEMORY_SERVICE_MODE = "grpc"
        NEXUSIM_MEMORY_GRPC_ADDR = $MemoryGrpcAddr
        NEXUSIM_PG_DSN = $PgDsn
        NEXUSIM_MEMORY_DEBUG_ADDR = ""
    }
    Start-Sleep -Milliseconds 700

    $processes += Start-ProcessRole -Name "retrieval-grpc" -Exe (Join-Path $repoRoot "bin\retrieval-gateway.exe") -Env @{
        NEXUSIM_RETRIEVAL_GATEWAY_MODE = "grpc"
        NEXUSIM_RETRIEVAL_GRPC_ADDR = $RetrievalGrpcAddr
        NEXUSIM_SEARCH_GRPC_ADDR = $SearchGrpcAddr
        NEXUSIM_MEMORY_GRPC_ADDR = $MemoryGrpcAddr
        NEXUSIM_RETRIEVAL_DEPENDENCY_TIMEOUT = "3s"
        NEXUSIM_RETRIEVAL_DEBUG_ADDR = ""
    }
    Start-Sleep -Seconds 1

    $runner = Join-Path $repoRoot "bin\retrieval-smoke.exe"
    & $runner `
        --pg-dsn $PgDsn `
        --retrieval-target $RetrievalGrpcAddr `
        --result-root $ResultRoot `
        --run-name $RunName
    if ($LASTEXITCODE -ne 0) {
        throw "retrieval smoke failed with exit code $LASTEXITCODE"
    }

    Write-Host "run_name=$RunName"
    Write-Host "result_dir=$resultDir"
}
finally {
    foreach ($process in $processes) {
        if ($process -and -not $process.HasExited) {
            $process.Kill()
            $process.WaitForExit(5000) | Out-Null
        }
        if ($process) {
            $process.Dispose()
        }
    }
    Get-EventSubscriber |
        Where-Object { $_.SourceObject -is [System.Diagnostics.Process] } |
        Unregister-Event -ErrorAction SilentlyContinue
}
