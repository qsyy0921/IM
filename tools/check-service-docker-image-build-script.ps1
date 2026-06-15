$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$servicesRoot = Join-Path $repoRoot "services"
$buildScript = Join-Path $PSScriptRoot "build-service-docker-images.ps1"
$powerShellExe = (Get-Command powershell -ErrorAction Stop).Source

if (-not (Test-Path -LiteralPath $buildScript)) {
    throw "Missing local service Docker image build script: $buildScript"
}

$expectedServices = @(Get-ChildItem -LiteralPath $servicesRoot -Directory |
    Sort-Object Name |
    Select-Object -ExpandProperty Name)

$actualServices = @(& $powerShellExe -NoProfile -ExecutionPolicy Bypass -File $buildScript -ListServices)
if ($LASTEXITCODE -ne 0) {
    throw "build-service-docker-images.ps1 -ListServices failed with exit code $LASTEXITCODE"
}
$actualServices = @($actualServices | Where-Object { $_ } | Sort-Object)

$serviceDiff = Compare-Object -ReferenceObject $expectedServices -DifferenceObject $actualServices
if ($serviceDiff) {
    $diffText = ($serviceDiff | ForEach-Object { "$($_.SideIndicator) $($_.InputObject)" }) -join ", "
    throw "Local Docker image build script service coverage mismatch with services directory: $diffText"
}

foreach ($service in $expectedServices) {
    $commandPath = Join-Path $repoRoot "services\$service\cmd\$service"
    $dockerfile = Join-Path $repoRoot "deploy\docker\$service.runtime.Dockerfile"
    if (-not (Test-Path -LiteralPath $commandPath)) {
        throw "Local Docker image build script default includes $service, but services\$service\cmd\$service is missing."
    }
    if (-not (Test-Path -LiteralPath $dockerfile)) {
        throw "Local Docker image build script default includes $service, but deploy\docker\$service.runtime.Dockerfile is missing."
    }
}

Write-Host "OK   local Docker image build script covers $($expectedServices.Count) services."
