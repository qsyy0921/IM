$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$servicesRoot = Join-Path $repoRoot "services"
$syncScript = Join-Path $PSScriptRoot "sync-mac-service-docker-images.ps1"
$powerShellExe = (Get-Command powershell -ErrorAction Stop).Source

if (-not (Test-Path -LiteralPath $syncScript)) {
    throw "Missing Mac service image sync script: $syncScript"
}

$expectedServices = @(Get-ChildItem -LiteralPath $servicesRoot -Directory |
    Sort-Object Name |
    Select-Object -ExpandProperty Name)

$actualServices = @(& $powerShellExe -NoProfile -ExecutionPolicy Bypass -File $syncScript -ListServices)
if ($LASTEXITCODE -ne 0) {
    throw "sync-mac-service-docker-images.ps1 -ListServices failed with exit code $LASTEXITCODE"
}
$actualServices = @($actualServices | Where-Object { $_ } | Sort-Object)

$serviceDiff = Compare-Object -ReferenceObject $expectedServices -DifferenceObject $actualServices
if ($serviceDiff) {
    $diffText = ($serviceDiff | ForEach-Object { "$($_.SideIndicator) $($_.InputObject)" }) -join ", "
    throw "Mac Docker image sync service coverage mismatch with services directory: $diffText"
}

foreach ($service in $expectedServices) {
    $commandPath = Join-Path $repoRoot "services\$service\cmd\$service"
    if (-not (Test-Path -LiteralPath $commandPath)) {
        throw "Mac Docker image sync default includes $service, but services\$service\cmd\$service is missing."
    }
}

Write-Host "OK   Mac Docker image sync covers $($expectedServices.Count) services."
