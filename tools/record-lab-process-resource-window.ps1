param(
    [Parameter(Mandatory = $true)]
    [string]$OutputDir,
    [int]$IntervalSeconds = 1,
    [string]$StopFile = "",
    [string]$WindowsName = "windows",
    [string[]]$WindowsCommandPatterns = @("hotgroup-loadtest", "loadtest\\hotgroup", "loadtest/hotgroup"),
    [string[]]$WindowsExcludeProcessNames = @("powershell.exe", "pwsh.exe", "ssh.exe", "cmd.exe"),
    [string]$UbuntuHost = "qsyy0921@172.31.50.2",
    [string]$UbuntuName = "ubuntu",
    [string[]]$UbuntuContainerPatterns = @(
        "nexusim-postgres",
        "nexusim-kafka",
        "nexusim-redis",
        "nexusim-message-service",
        "nexusim-conversation-service",
        "nexusim-delivery-service",
        "nexusim-policy-service",
        "nexusim-timeline-service",
        "nexusim-push-gateway"
    ),
    [string[]]$UbuntuContainerExcludePatterns = @("nexusim-kafka-ui"),
    [string]$MacHost = "",
    [string]$MacName = "mac",
    [switch]$IncludeMac,
    [string[]]$MacCommandPatterns = @("loadtest/hotgroup", "loadtest\\hotgroup"),
    [string]$CsvPath = "",
    [string]$MarkdownPath = "",
    [int]$MaxSeconds = 3600,
    [switch]$AllowSampleErrors
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $OutputDir -RepositoryRoot (Split-Path -Parent $PSScriptRoot) -Name "OutputDir"

if ($IntervalSeconds -le 0) {
    throw "IntervalSeconds must be greater than zero."
}
if ($MaxSeconds -le 0) {
    throw "MaxSeconds must be greater than zero."
}

$resolvedOutputDir = [System.IO.Path]::GetFullPath($OutputDir)
New-Item -ItemType Directory -Force -Path $resolvedOutputDir | Out-Null

if ($CsvPath.Trim().Length -eq 0) {
    $CsvPath = Join-Path $resolvedOutputDir "lab-process-resource-samples.csv"
}
if ($MarkdownPath.Trim().Length -eq 0) {
    $MarkdownPath = Join-Path $resolvedOutputDir "lab-process-resource-summary.md"
}
if ($StopFile.Trim().Length -eq 0) {
    $StopFile = Join-Path $resolvedOutputDir "STOP"
}

function Format-Number {
    param([object]$Value)
    if ($null -eq $Value -or [string]$Value -eq "") {
        return ""
    }
    return ([double]$Value).ToString("0.###", [System.Globalization.CultureInfo]::InvariantCulture)
}

function Convert-ToDoubleOrNull {
    param([object]$Value)
    if ($null -eq $Value) {
        return $null
    }
    $text = ([string]$Value).Trim().TrimEnd("%")
    if ($text.Length -eq 0) {
        return $null
    }
    $number = 0.0
    if ([double]::TryParse($text, [System.Globalization.NumberStyles]::Float, [System.Globalization.CultureInfo]::InvariantCulture, [ref]$number)) {
        return $number
    }
    return $null
}

function New-ProcessSample {
    param(
        [datetime]$Timestamp,
        [string]$Machine,
        [string]$OS,
        [string]$Kind,
        [string]$Name,
        [string]$ID,
        [nullable[double]]$CPUPercent,
        [nullable[double]]$MemoryMB,
        [string]$ErrorMessage = ""
    )

    return [pscustomobject]@{
        timestamp_utc = $Timestamp.ToUniversalTime().ToString("o")
        machine = $Machine
        os = $OS
        kind = $Kind
        name = $Name
        id = $ID
        cpu_percent = if ($null -eq $CPUPercent) { "" } else { Format-Number $CPUPercent }
        memory_mb = if ($null -eq $MemoryMB) { "" } else { Format-Number $MemoryMB }
        error = $ErrorMessage
    }
}

function Test-AnyPattern {
    param(
        [string]$Text,
        [string[]]$Patterns
    )
    foreach ($pattern in $Patterns) {
        if ($Text -like "*$pattern*") {
            return $true
        }
    }
    return $false
}

function Get-WindowsProcessSamples {
    param([datetime]$Timestamp)

    try {
        $processes = Get-CimInstance Win32_Process | Where-Object {
            $_.CommandLine -and
                (Test-AnyPattern -Text $_.CommandLine -Patterns $WindowsCommandPatterns) -and
                -not (Test-AnyPattern -Text ([string]$_.Name) -Patterns $WindowsExcludeProcessNames)
        }
        $rows = @()
        foreach ($process in $processes) {
            $counter = $null
            try {
                $counter = Get-CimInstance Win32_PerfFormattedData_PerfProc_Process -Filter "IDProcess=$($process.ProcessId)"
            }
            catch {
                $counter = $null
            }
            $memoryMB = if ($process.WorkingSetSize) { [double]$process.WorkingSetSize / 1024.0 / 1024.0 } else { $null }
            $cpu = if ($counter) { [double]$counter.PercentProcessorTime } else { $null }
            $name = if ($process.Name) { [string]$process.Name } else { "process" }
            $rows += New-ProcessSample -Timestamp $Timestamp -Machine $WindowsName -OS "windows" -Kind "process" `
                -Name $name -ID ([string]$process.ProcessId) -CPUPercent $cpu -MemoryMB $memoryMB
        }
        return $rows
    }
    catch {
        return @(New-ProcessSample -Timestamp $Timestamp -Machine $WindowsName -OS "windows" -Kind "process" `
            -Name "windows-process-scan" -ID "" -CPUPercent $null -MemoryMB $null -ErrorMessage $_.Exception.Message)
    }
}

function Invoke-SSHText {
    param(
        [string]$SSHHost,
        [string]$Command,
        [int]$TimeoutSeconds = 8,
        [ValidateSet("linux", "macos")]
        [string]$RemoteOS = "linux"
    )

    if ($SSHHost.Trim().Length -eq 0) {
        throw "SSH host is empty."
    }
    $encoded = [Convert]::ToBase64String([System.Text.Encoding]::UTF8.GetBytes($Command))
    $decodeCommand = if ($RemoteOS -eq "macos") { "base64 -D" } else { "base64 -d" }
    $remoteCommand = "printf '%s' '$encoded' | $decodeCommand | bash"
    $output = & ssh -o BatchMode=yes -o ConnectTimeout=$TimeoutSeconds $SSHHost $remoteCommand 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw (($output | Out-String).Trim())
    }
    return (($output | Out-String).Trim())
}

function Convert-DockerMemoryToMB {
    param([string]$Value)

    $text = $Value.Trim()
    if ($text -match '^([0-9.]+)\s*([KMGT]?i?B)$') {
        $number = [double]::Parse($Matches[1], [System.Globalization.CultureInfo]::InvariantCulture)
        $unit = $Matches[2]
        switch ($unit) {
            "B" { return $number / 1024.0 / 1024.0 }
            "kB" { return $number / 1024.0 }
            "KiB" { return $number / 1024.0 }
            "MB" { return $number }
            "MiB" { return $number }
            "GB" { return $number * 1024.0 }
            "GiB" { return $number * 1024.0 }
            "TB" { return $number * 1024.0 * 1024.0 }
            "TiB" { return $number * 1024.0 * 1024.0 }
        }
    }
    return $null
}

function Get-UbuntuContainerSamples {
    param([datetime]$Timestamp)

    if ($UbuntuHost.Trim().Length -eq 0) {
        return @()
    }
    $patternText = ($UbuntuContainerPatterns -join "|")
    $command = @"
docker stats --no-stream --format '{{json .}}' 2>/dev/null | grep -E '$patternText' || true
"@
    try {
        $text = Invoke-SSHText -SSHHost $UbuntuHost -Command $command -RemoteOS "linux"
        if ($text.Trim().Length -eq 0) {
            return @()
        }
        $rows = @()
        foreach ($line in ($text -split "`n")) {
            if ($line.Trim().Length -eq 0) {
                continue
            }
            $entry = $line | ConvertFrom-Json
            $name = [string]$entry.Name
            if (Test-AnyPattern -Text $name -Patterns $UbuntuContainerExcludePatterns) {
                continue
            }
            $memUsed = ([string]$entry.MemUsage -split "/")[0].Trim()
            $rows += New-ProcessSample -Timestamp $Timestamp -Machine $UbuntuName -OS "linux" -Kind "container" `
                -Name $name -ID ([string]$entry.Container) `
                -CPUPercent (Convert-ToDoubleOrNull $entry.CPUPerc) -MemoryMB (Convert-DockerMemoryToMB $memUsed)
        }
        return $rows
    }
    catch {
        return @(New-ProcessSample -Timestamp $Timestamp -Machine $UbuntuName -OS "linux" -Kind "container" `
            -Name "ubuntu-docker-stats" -ID "" -CPUPercent $null -MemoryMB $null -ErrorMessage $_.Exception.Message)
    }
}

function Get-MacProcessSamples {
    param([datetime]$Timestamp)

    if (-not $IncludeMac -or $MacHost.Trim().Length -eq 0) {
        return @()
    }
    $patternText = ($MacCommandPatterns -join "|")
    $command = @"
ps -axo pid=,pcpu=,rss=,command= | grep -E '$patternText' | grep -v grep || true
"@
    try {
        $text = Invoke-SSHText -SSHHost $MacHost -Command $command -RemoteOS "macos"
        if ($text.Trim().Length -eq 0) {
            return @()
        }
        $rows = @()
        foreach ($line in ($text -split "`n")) {
            if ($line.Trim().Length -eq 0) {
                continue
            }
            $parts = @($line.Trim() -split "\s+", 4)
            if ($parts.Count -lt 4) {
                continue
            }
            $name = (($parts[3] -split "\s+")[0])
            $rows += New-ProcessSample -Timestamp $Timestamp -Machine $MacName -OS "macos" -Kind "process" `
                -Name $name -ID $parts[0] -CPUPercent (Convert-ToDoubleOrNull $parts[1]) `
                -MemoryMB ([double]::Parse($parts[2], [System.Globalization.CultureInfo]::InvariantCulture) / 1024.0)
        }
        return $rows
    }
    catch {
        return @(New-ProcessSample -Timestamp $Timestamp -Machine $MacName -OS "macos" -Kind "process" `
            -Name "mac-process-scan" -ID "" -CPUPercent $null -MemoryMB $null -ErrorMessage $_.Exception.Message)
    }
}

function Get-Numeric {
    param(
        [object]$Row,
        [string]$PropertyName
    )
    $value = $Row.$PropertyName
    if ($null -eq $value -or [string]$value -eq "") {
        return $null
    }
    return [double]::Parse([string]$value, [System.Globalization.CultureInfo]::InvariantCulture)
}

$rows = New-Object System.Collections.Generic.List[object]
$started = Get-Date
while ($true) {
    $timestamp = Get-Date
    foreach ($row in (Get-WindowsProcessSamples -Timestamp $timestamp)) {
        $rows.Add($row)
    }
    foreach ($row in (Get-UbuntuContainerSamples -Timestamp $timestamp)) {
        $rows.Add($row)
    }
    foreach ($row in (Get-MacProcessSamples -Timestamp $timestamp)) {
        $rows.Add($row)
    }

    if (Test-Path -LiteralPath $StopFile) {
        break
    }
    if (((Get-Date) - $started).TotalSeconds -ge $MaxSeconds) {
        break
    }
    Start-Sleep -Seconds $IntervalSeconds
}

$rowArray = @($rows.ToArray())
$rowArray | Export-Csv -LiteralPath $CsvPath -NoTypeInformation -Encoding UTF8

$validRows = @($rowArray | Where-Object { $_.error -eq "" })
$errorRows = @($rowArray | Where-Object { $_.error -ne "" })
$groups = @($validRows | Group-Object machine, kind, name)

$markdown = New-Object System.Collections.Generic.List[string]
$markdown.Add("# Lab Process Resource Window")
$markdown.Add("")
$markdown.Add("- Scope: pressure-related local runner processes and Ubuntu service containers only; not whole-machine utilization.")
$markdown.Add("- Started: $($started.ToUniversalTime().ToString("o"))")
$markdown.Add("- Stopped: $((Get-Date).ToUniversalTime().ToString("o"))")
$markdown.Add("- Interval seconds: $IntervalSeconds")
$markdown.Add("- Stop file: $StopFile")
$markdown.Add("- CSV: $CsvPath")
$markdown.Add("")
$markdown.Add("| Machine | Kind | Name | Samples | CPU avg % | CPU max % | Memory avg MB | Memory max MB |")
$markdown.Add("| --- | --- | --- | ---: | ---: | ---: | ---: | ---: |")
foreach ($group in $groups) {
    $items = @($group.Group)
    $cpuValues = @($items | ForEach-Object { Get-Numeric -Row $_ -PropertyName "cpu_percent" } | Where-Object { $null -ne $_ })
    $memValues = @($items | ForEach-Object { Get-Numeric -Row $_ -PropertyName "memory_mb" } | Where-Object { $null -ne $_ })
    $first = $items[0]
    $markdown.Add("| $($first.machine) | $($first.kind) | $($first.name) | $($items.Count) | $(Format-Number (($cpuValues | Measure-Object -Average).Average)) | $(Format-Number (($cpuValues | Measure-Object -Maximum).Maximum)) | $(Format-Number (($memValues | Measure-Object -Average).Average)) | $(Format-Number (($memValues | Measure-Object -Maximum).Maximum)) |")
}
if ($groups.Count -eq 0) {
    $markdown.Add("| none | none | none | 0 |  |  |  |  |")
}
$markdown.Add("")
$markdown.Add("## Sample Errors")
$markdown.Add("")
if ($errorRows.Count -eq 0) {
    $markdown.Add("- none")
}
else {
    foreach ($row in ($errorRows | Select-Object -First 20)) {
        $markdown.Add("- $($row.machine) $($row.name): $($row.error)")
    }
}
$markdown.Add("")
$markdown.Add("Use this report with the matching hotgroup summary and Prometheus/PostgreSQL window. Whole-machine resource charts are useful background, but capacity claims should use this pressure-related process/container view.")
$markdown | Set-Content -LiteralPath $MarkdownPath -Encoding UTF8

if (-not $AllowSampleErrors -and $validRows.Count -eq 0) {
    throw "process resource sampling produced no valid rows. See $CsvPath"
}

Write-Host "OK   process resource samples written: $CsvPath"
Write-Host "OK   process resource summary written: $MarkdownPath"
