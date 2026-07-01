param(
    [Parameter(Mandatory = $true)]
    [string]$OutputDir,
    [int]$DurationSeconds = 120,
    [int]$IntervalSeconds = 1,
    [string]$WindowsName = "windows",
    [string]$UbuntuHost = "qsyy0921@172.31.50.2",
    [string]$UbuntuName = "ubuntu",
    [string]$MacHost = "",
    [string]$MacName = "mac",
    [switch]$IncludeMac,
    [string]$CsvPath = "",
    [string]$MarkdownPath = "",
    [string]$SvgPath = ""
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "output-root-safety.ps1")
Assert-ExternalOutputRoot -Value $OutputDir -RepositoryRoot (Split-Path -Parent $PSScriptRoot) -Name "OutputDir"

if ($DurationSeconds -le 0) {
    throw "DurationSeconds must be greater than zero."
}
if ($IntervalSeconds -le 0) {
    throw "IntervalSeconds must be greater than zero."
}

$resolvedOutputDir = [System.IO.Path]::GetFullPath($OutputDir)
New-Item -ItemType Directory -Force -Path $resolvedOutputDir | Out-Null

if ($CsvPath.Trim().Length -eq 0) {
    $CsvPath = Join-Path $resolvedOutputDir "lab-resource-samples.csv"
}
if ($MarkdownPath.Trim().Length -eq 0) {
    $MarkdownPath = Join-Path $resolvedOutputDir "lab-resource-summary.md"
}
if ($SvgPath.Trim().Length -eq 0) {
    $SvgPath = Join-Path $resolvedOutputDir "lab-resource-usage.svg"
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
    if ([double]::TryParse(
            $text,
            [System.Globalization.NumberStyles]::Float,
            [System.Globalization.CultureInfo]::InvariantCulture,
            [ref]$number
        )) {
        return $number
    }
    return $null
}

function New-ResourceSample {
    param(
        [datetime]$Timestamp,
        [string]$Machine,
        [string]$OS,
        [nullable[double]]$CPUPercent,
        [nullable[double]]$MemoryUsedPercent,
        [nullable[double]]$MemoryUsedMB,
        [nullable[double]]$MemoryTotalMB,
        [string]$ErrorMessage = ""
    )

    return [pscustomobject]@{
        timestamp_utc = $Timestamp.ToUniversalTime().ToString("o")
        machine = $Machine
        os = $OS
        cpu_percent = if ($null -eq $CPUPercent) { "" } else { Format-Number $CPUPercent }
        memory_used_percent = if ($null -eq $MemoryUsedPercent) { "" } else { Format-Number $MemoryUsedPercent }
        memory_used_mb = if ($null -eq $MemoryUsedMB) { "" } else { Format-Number $MemoryUsedMB }
        memory_total_mb = if ($null -eq $MemoryTotalMB) { "" } else { Format-Number $MemoryTotalMB }
        error = $ErrorMessage
    }
}

function Get-WindowsResourceSample {
    param([datetime]$Timestamp)

    try {
        $cpu = (Get-CimInstance -ClassName Win32_PerfFormattedData_PerfOS_Processor -Filter "Name='_Total'").PercentProcessorTime
        $os = Get-CimInstance -ClassName Win32_OperatingSystem
        $totalMB = [double]$os.TotalVisibleMemorySize / 1024.0
        $freeMB = [double]$os.FreePhysicalMemory / 1024.0
        $usedMB = [Math]::Max(0, $totalMB - $freeMB)
        $usedPercent = if ($totalMB -gt 0) { ($usedMB / $totalMB) * 100.0 } else { 0.0 }
        return New-ResourceSample -Timestamp $Timestamp -Machine $WindowsName -OS "windows" `
            -CPUPercent ([double]$cpu) -MemoryUsedPercent $usedPercent -MemoryUsedMB $usedMB -MemoryTotalMB $totalMB
    }
    catch {
        return New-ResourceSample -Timestamp $Timestamp -Machine $WindowsName -OS "windows" `
            -CPUPercent $null -MemoryUsedPercent $null -MemoryUsedMB $null -MemoryTotalMB $null -ErrorMessage $_.Exception.Message
    }
}

function Invoke-SSHText {
    param(
        [string]$SSHHost,
        [string]$Command,
        [int]$TimeoutSeconds = 8
    )

    if ($SSHHost.Trim().Length -eq 0) {
        throw "SSH host is empty."
    }
    $output = & ssh -o BatchMode=yes -o ConnectTimeout=$TimeoutSeconds $SSHHost $Command 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw (($output | Out-String).Trim())
    }
    return (($output | Out-String).Trim())
}

function Get-LinuxResourceSample {
    param(
        [datetime]$Timestamp,
        [string]$SSHHost,
        [string]$Name
    )

    $command = @'
read _ u1 n1 s1 i1 w1 irq1 sirq1 st1 rest < /proc/stat; t1=$((u1+n1+s1+i1+w1+irq1+sirq1+st1)); idle1=$((i1+w1)); sleep 0.2; read _ u2 n2 s2 i2 w2 irq2 sirq2 st2 rest < /proc/stat; t2=$((u2+n2+s2+i2+w2+irq2+sirq2+st2)); idle2=$((i2+w2)); dt=$((t2-t1)); di=$((idle2-idle1)); if [ "$dt" -le 0 ]; then cpu=0; else cpu=$(awk -v dt="$dt" -v di="$di" 'BEGIN { printf "%.3f", 100*(dt-di)/dt }'); fi; free -m | awk -v cpu="$cpu" '/^Mem:/ { printf "%s %.3f %.3f %.3f", cpu, 100*$3/$2, $3, $2 }'
'@
    try {
        $text = Invoke-SSHText -SSHHost $SSHHost -Command $command
        $parts = @($text -split "\s+" | Where-Object { $_.Trim().Length -gt 0 })
        if ($parts.Count -lt 4) {
            throw "unexpected linux sample: $text"
        }
        return New-ResourceSample -Timestamp $Timestamp -Machine $Name -OS "linux" `
            -CPUPercent (Convert-ToDoubleOrNull $parts[0]) `
            -MemoryUsedPercent (Convert-ToDoubleOrNull $parts[1]) `
            -MemoryUsedMB (Convert-ToDoubleOrNull $parts[2]) `
            -MemoryTotalMB (Convert-ToDoubleOrNull $parts[3])
    }
    catch {
        return New-ResourceSample -Timestamp $Timestamp -Machine $Name -OS "linux" `
            -CPUPercent $null -MemoryUsedPercent $null -MemoryUsedMB $null -MemoryTotalMB $null -ErrorMessage $_.Exception.Message
    }
}

function Get-MacResourceSample {
    param(
        [datetime]$Timestamp,
        [string]$SSHHost,
        [string]$Name
    )

    $command = @'
idle=$(top -l 1 -n 0 | awk '/CPU usage/ { for (i=1;i<=NF;i++) if ($i=="idle") { gsub("%", "", $(i-1)); print $(i-1); exit } }'); if [ -z "$idle" ]; then idle=100; fi; cpu=$(awk -v idle="$idle" 'BEGIN { printf "%.3f", 100-idle }'); total=$(sysctl -n hw.memsize); pagesize=$(sysctl -n hw.pagesize); used_pages=$(vm_stat | awk '/Pages active|Pages wired down|Pages occupied by compressor/ { gsub("\\.", "", $NF); total += $NF } END { print total+0 }'); used_mb=$(awk -v p="$used_pages" -v ps="$pagesize" 'BEGIN { printf "%.3f", p*ps/1024/1024 }'); total_mb=$(awk -v t="$total" 'BEGIN { printf "%.3f", t/1024/1024 }'); mem_pct=$(awk -v u="$used_mb" -v t="$total_mb" 'BEGIN { if (t <= 0) print "0.000"; else printf "%.3f", 100*u/t }'); printf "%s %s %s %s" "$cpu" "$mem_pct" "$used_mb" "$total_mb"
'@
    try {
        $text = Invoke-SSHText -SSHHost $SSHHost -Command $command
        $parts = @($text -split "\s+" | Where-Object { $_.Trim().Length -gt 0 })
        if ($parts.Count -lt 4) {
            throw "unexpected mac sample: $text"
        }
        return New-ResourceSample -Timestamp $Timestamp -Machine $Name -OS "macos" `
            -CPUPercent (Convert-ToDoubleOrNull $parts[0]) `
            -MemoryUsedPercent (Convert-ToDoubleOrNull $parts[1]) `
            -MemoryUsedMB (Convert-ToDoubleOrNull $parts[2]) `
            -MemoryTotalMB (Convert-ToDoubleOrNull $parts[3])
    }
    catch {
        return New-ResourceSample -Timestamp $Timestamp -Machine $Name -OS "macos" `
            -CPUPercent $null -MemoryUsedPercent $null -MemoryUsedMB $null -MemoryTotalMB $null -ErrorMessage $_.Exception.Message
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

function Write-ResourceSvg {
    param(
        [object[]]$Rows,
        [string]$Path
    )

    $validRows = @($Rows | Where-Object { $_.error -eq "" })
    if ($validRows.Count -eq 0) {
        return
    }
    $machines = @($validRows | Select-Object -ExpandProperty machine -Unique)
    $colors = @("#2563eb", "#16a34a", "#dc2626", "#7c3aed", "#f97316")
    $width = 1100
    $height = 520
    $left = 70
    $right = 25
    $top = 40
    $panelHeight = 190
    $gap = 70
    $plotWidth = $width - $left - $right
    $firstRow = $validRows[0]
    $lastRow = $validRows[$validRows.Count - 1]
    $first = [datetime]::Parse([string]$firstRow.timestamp_utc)
    $last = [datetime]::Parse([string]$lastRow.timestamp_utc)
    $span = [Math]::Max(1.0, ($last - $first).TotalSeconds)

    function New-Polyline {
        param(
            [object[]]$Series,
            [string]$Metric,
            [int]$PanelTop
        )
        $points = New-Object System.Collections.Generic.List[string]
        foreach ($row in $Series) {
            $value = Get-Numeric -Row $row -PropertyName $Metric
            if ($null -eq $value) {
                continue
            }
            $timestamp = [datetime]::Parse([string]$row.timestamp_utc)
            $x = $left + ((($timestamp - $first).TotalSeconds / $span) * $plotWidth)
            $clamped = [Math]::Max(0.0, [Math]::Min(100.0, $value))
            $y = $PanelTop + $panelHeight - (($clamped / 100.0) * $panelHeight)
            $points.Add(("{0:0.###},{1:0.###}" -f $x, $y))
        }
        return ($points -join " ")
    }

    $svg = New-Object System.Collections.Generic.List[string]
    $svg.Add("<svg xmlns=`"http://www.w3.org/2000/svg`" width=`"$width`" height=`"$height`" viewBox=`"0 0 $width $height`">")
    $svg.Add("<rect width=`"100%`" height=`"100%`" fill=`"#ffffff`"/>")
    $svg.Add("<text x=`"30`" y=`"25`" font-family=`"Segoe UI, Arial`" font-size=`"18`" font-weight=`"600`">NexusIM lab resource usage</text>")
    $panelNames = @(
        @{ title = "CPU used (%)"; metric = "cpu_percent"; y = $top },
        @{ title = "Memory used (%)"; metric = "memory_used_percent"; y = ($top + $panelHeight + $gap) }
    )
    foreach ($panel in $panelNames) {
        $panelTop = [int]$panel.y
        $svg.Add("<text x=`"$left`" y=`"$($panelTop - 12)`" font-family=`"Segoe UI, Arial`" font-size=`"14`" font-weight=`"600`">$($panel.title)</text>")
        $svg.Add("<rect x=`"$left`" y=`"$panelTop`" width=`"$plotWidth`" height=`"$panelHeight`" fill=`"#f8fafc`" stroke=`"#cbd5e1`"/>")
        foreach ($tick in 0, 25, 50, 75, 100) {
            $y = $panelTop + $panelHeight - (($tick / 100.0) * $panelHeight)
            $svg.Add("<line x1=`"$left`" x2=`"$($left + $plotWidth)`" y1=`"$y`" y2=`"$y`" stroke=`"#e2e8f0`"/>")
            $svg.Add("<text x=`"25`" y=`"$($y + 4)`" font-family=`"Segoe UI, Arial`" font-size=`"11`" fill=`"#475569`">$tick</text>")
        }
        for ($i = 0; $i -lt $machines.Count; $i++) {
            $machine = [string]$machines[$i]
            $series = @($validRows | Where-Object { $_.machine -eq $machine })
            $polyline = New-Polyline -Series $series -Metric ([string]$panel.metric) -PanelTop $panelTop
            if ($polyline.Trim().Length -gt 0) {
                $color = $colors[$i % $colors.Count]
                $svg.Add("<polyline points=`"$polyline`" fill=`"none`" stroke=`"$color`" stroke-width=`"2`"/>")
            }
        }
    }
    for ($i = 0; $i -lt $machines.Count; $i++) {
        $x = $left + ($i * 150)
        $y = $height - 28
        $color = $colors[$i % $colors.Count]
        $machine = [string]$machines[$i]
        $svg.Add("<rect x=`"$x`" y=`"$($y - 10)`" width=`"20`" height=`"3`" fill=`"$color`"/>")
        $svg.Add("<text x=`"$($x + 28)`" y=`"$($y - 4)`" font-family=`"Segoe UI, Arial`" font-size=`"12`">$machine</text>")
    }
    $svg.Add("</svg>")
    $svg | Set-Content -LiteralPath $Path -Encoding UTF8
}

$rows = New-Object System.Collections.Generic.List[object]
$sampleCount = [Math]::Ceiling($DurationSeconds / $IntervalSeconds)
$started = Get-Date

for ($i = 0; $i -lt $sampleCount; $i++) {
    $timestamp = Get-Date
    $rows.Add((Get-WindowsResourceSample -Timestamp $timestamp))
    if ($UbuntuHost.Trim().Length -gt 0) {
        $rows.Add((Get-LinuxResourceSample -Timestamp $timestamp -SSHHost $UbuntuHost -Name $UbuntuName))
    }
    if ($IncludeMac -and $MacHost.Trim().Length -gt 0) {
        $rows.Add((Get-MacResourceSample -Timestamp $timestamp -SSHHost $MacHost -Name $MacName))
    }
    $elapsed = ((Get-Date) - $started).TotalSeconds
    $targetElapsed = [Math]::Min($DurationSeconds, ($i + 1) * $IntervalSeconds)
    $sleepSeconds = [Math]::Max(0.0, $targetElapsed - $elapsed)
    if ($sleepSeconds -gt 0 -and $i -lt ($sampleCount - 1)) {
        Start-Sleep -Milliseconds ([int]($sleepSeconds * 1000))
    }
}

$rowArray = @($rows.ToArray())
$rowArray | Export-Csv -LiteralPath $CsvPath -NoTypeInformation -Encoding UTF8
Write-ResourceSvg -Rows $rowArray -Path $SvgPath

$validRows = @($rowArray | Where-Object { $_.error -eq "" })
$machineSummaries = @()
foreach ($machine in @($rowArray | Select-Object -ExpandProperty machine -Unique)) {
	$machineRows = @($validRows | Where-Object { $_.machine -eq $machine })
	$errorRows = @($rowArray | Where-Object { $_.machine -eq $machine -and $_.error -ne "" })
    $cpuValues = @($machineRows | ForEach-Object { Get-Numeric -Row $_ -PropertyName "cpu_percent" } | Where-Object { $null -ne $_ })
    $memValues = @($machineRows | ForEach-Object { Get-Numeric -Row $_ -PropertyName "memory_used_percent" } | Where-Object { $null -ne $_ })
    $machineSummaries += [pscustomobject]@{
        machine = $machine
        samples = $machineRows.Count
        errors = $errorRows.Count
        cpu_avg = if ($cpuValues.Count -gt 0) { ($cpuValues | Measure-Object -Average).Average } else { $null }
        cpu_max = if ($cpuValues.Count -gt 0) { ($cpuValues | Measure-Object -Maximum).Maximum } else { $null }
        mem_avg = if ($memValues.Count -gt 0) { ($memValues | Measure-Object -Average).Average } else { $null }
        mem_max = if ($memValues.Count -gt 0) { ($memValues | Measure-Object -Maximum).Maximum } else { $null }
    }
}

$markdown = New-Object System.Collections.Generic.List[string]
$startedText = $started.ToUniversalTime().ToString("o")
$markdown.Add("# Lab Resource Window")
$markdown.Add("")
$markdown.Add("- Scope: CPU and memory time-series for local lab hosts during a loadtest window; not a production SLO.")
$markdown.Add("- Started: $startedText")
$markdown.Add("- Duration seconds: $DurationSeconds")
$markdown.Add("- Interval seconds: $IntervalSeconds")
$markdown.Add("- CSV: $CsvPath")
$markdown.Add("- SVG: $SvgPath")
$markdown.Add("")
$markdown.Add("![Lab resource usage]($(Split-Path -Leaf $SvgPath))")
$markdown.Add("")
$markdown.Add("| Machine | Samples | Errors | CPU avg % | CPU max % | Memory avg % | Memory max % |")
$markdown.Add("| --- | ---: | ---: | ---: | ---: | ---: | ---: |")
foreach ($summary in $machineSummaries) {
    $markdown.Add("| $($summary.machine) | $($summary.samples) | $($summary.errors) | $(Format-Number $summary.cpu_avg) | $(Format-Number $summary.cpu_max) | $(Format-Number $summary.mem_avg) | $(Format-Number $summary.mem_max) |")
}
$markdown.Add("")
$markdown.Add("Use this report together with hotgroup summary, Prometheus window, Kafka lag, and PostgreSQL metrics before claiming a bottleneck.")
$markdown | Set-Content -LiteralPath $MarkdownPath -Encoding UTF8

Write-Host "OK   resource samples written: $CsvPath"
Write-Host "OK   resource summary written: $MarkdownPath"
Write-Host "OK   resource chart written: $SvgPath"
