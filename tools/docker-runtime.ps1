$ErrorActionPreference = "Stop"

function Invoke-NexusIMDocker {
    param(
        [Parameter(Mandatory = $true)]
        [string[]]$Arguments,
        [int]$TimeoutSeconds = 30,
        [switch]$AllowNonZero
    )

    if ($TimeoutSeconds -le 0) {
        throw "TimeoutSeconds must be greater than zero."
    }

    $stdoutPath = [System.IO.Path]::GetTempFileName()
    $stderrPath = [System.IO.Path]::GetTempFileName()
    try {
        $process = Start-Process `
            -FilePath "docker" `
            -ArgumentList $Arguments `
            -NoNewWindow `
            -PassThru `
            -RedirectStandardOutput $stdoutPath `
            -RedirectStandardError $stderrPath

        if (-not $process.WaitForExit($TimeoutSeconds * 1000)) {
            try {
                $process.Kill()
            } catch {
            }
            throw "docker $($Arguments -join ' ') timed out after ${TimeoutSeconds}s"
        }

        $stdout = ""
        $stderr = ""
        if (Test-Path -LiteralPath $stdoutPath) {
            $stdout = Get-Content -LiteralPath $stdoutPath -Raw
        }
        if (Test-Path -LiteralPath $stderrPath) {
            $stderr = Get-Content -LiteralPath $stderrPath -Raw
        }

        if ($process.ExitCode -ne 0 -and -not $AllowNonZero) {
            $message = "docker $($Arguments -join ' ') failed with exit code $($process.ExitCode)"
            if ($stderr) {
                $message += ": $($stderr.Trim())"
            }
            throw $message
        }

        return [pscustomobject]@{
            ExitCode = $process.ExitCode
            Stdout = $stdout
            Stderr = $stderr
        }
    }
    finally {
        Remove-Item -LiteralPath $stdoutPath -Force -ErrorAction SilentlyContinue
        Remove-Item -LiteralPath $stderrPath -Force -ErrorAction SilentlyContinue
    }
}

function Assert-NexusIMDockerEngine {
    param([int]$TimeoutSeconds = 15)

    $result = Invoke-NexusIMDocker `
        -Arguments @("version", "--format", "{{.Server.Version}}") `
        -TimeoutSeconds $TimeoutSeconds
    $version = $result.Stdout.Trim()
    if (-not $version) {
        throw "Docker engine version probe returned an empty response."
    }
    Write-Host "docker_server_version=$version"
}

function Test-NexusIMDockerImage {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Image,
        [int]$TimeoutSeconds = 15
    )

    $result = Invoke-NexusIMDocker `
        -Arguments @("image", "inspect", $Image) `
        -TimeoutSeconds $TimeoutSeconds `
        -AllowNonZero
    return $result.ExitCode -eq 0
}

function Ensure-NexusIMDockerImage {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Image,
        [int]$TimeoutSeconds = 60,
        [switch]$AllowPull
    )

    if (Test-NexusIMDockerImage -Image $Image -TimeoutSeconds $TimeoutSeconds) {
        Write-Host "docker_image_ready=$Image"
        return
    }
    if (-not $AllowPull) {
        throw "Missing Docker image $Image. Pull manually or rerun with -AllowPull; this script does not pull by default."
    }
    Invoke-NexusIMDocker -Arguments @("pull", $Image) -TimeoutSeconds $TimeoutSeconds | Out-Null
    Write-Host "docker_image_pulled=$Image"
}

function Invoke-NexusIMDockerComposeUp {
    param(
        [Parameter(Mandatory = $true)]
        [string[]]$ComposeFiles,
        [string[]]$Profiles = @(),
        [Parameter(Mandatory = $true)]
        [string[]]$Services,
        [int]$TimeoutSeconds = 60
    )

    $arguments = @("compose")
    foreach ($file in $ComposeFiles) {
        $arguments += @("-f", $file)
    }
    foreach ($profile in $Profiles) {
        $arguments += @("--profile", $profile)
    }
    $arguments += @("up", "-d")
    $arguments += $Services
    Invoke-NexusIMDocker -Arguments $arguments -TimeoutSeconds $TimeoutSeconds | Out-Null
}

function Wait-NexusIMTcp {
    param(
        [string]$HostName,
        [int]$Port,
        [int]$TimeoutSeconds = 60
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        $client = [System.Net.Sockets.TcpClient]::new()
        try {
            $connect = $client.BeginConnect($HostName, $Port, $null, $null)
            if ($connect.AsyncWaitHandle.WaitOne(300)) {
                $client.EndConnect($connect)
                Write-Host "tcp_ready=${HostName}:$Port"
                return
            }
        } catch {
        } finally {
            $client.Close()
        }
        Start-Sleep -Milliseconds 250
    }
    throw "Timed out waiting for ${HostName}:$Port"
}
