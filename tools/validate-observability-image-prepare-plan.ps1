param(
    [Parameter(Mandatory = $true)]
    [string]$PlanPath,
    [switch]$RequireReport
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path -LiteralPath $PlanPath -PathType Leaf)) {
    throw "Missing observability image prepare plan: $PlanPath"
}

try {
    $plan = Get-Content -LiteralPath $PlanPath -Raw | ConvertFrom-Json
}
catch {
    throw "Invalid observability image prepare plan JSON: $($_.Exception.Message)"
}

function Assert-Property {
    param(
        [object]$Object,
        [string]$Name
    )

    if (-not ($Object.PSObject.Properties.Name -contains $Name)) {
        throw "Observability image prepare plan missing property: $Name"
    }
}

function Assert-Boolean {
    param(
        [object]$Value,
        [string]$Name
    )

    if ($Value -isnot [bool]) {
        throw "Observability image prepare plan property '$Name' must be boolean."
    }
}

foreach ($property in @(
    "generated_at_utc",
    "docker_available",
    "include_alertmanager",
    "allow_image_pull",
    "platform",
    "missing_count",
    "images",
    "boundary"
)) {
    Assert-Property -Object $plan -Name $property
}

$generatedAt = [datetime]::MinValue
if (-not [datetime]::TryParse([string]$plan.generated_at_utc, [ref]$generatedAt)) {
    throw "Observability image prepare plan generated_at_utc is not a valid timestamp."
}

Assert-Boolean -Value $plan.docker_available -Name "docker_available"
Assert-Boolean -Value $plan.include_alertmanager -Name "include_alertmanager"
Assert-Boolean -Value $plan.allow_image_pull -Name "allow_image_pull"

$images = @($plan.images)
$expectedImageCount = if ($plan.include_alertmanager) { 3 } else { 2 }
if ($images.Count -ne $expectedImageCount) {
    throw "Observability image prepare plan expected $expectedImageCount image entries, found $($images.Count)."
}

$requiredNames = @("prometheus", "grafana")
if ($plan.include_alertmanager) {
    $requiredNames += "alertmanager"
}

foreach ($name in $requiredNames) {
    if (-not @($images | Where-Object { $_.name -eq $name })) {
        throw "Observability image prepare plan missing image role: $name"
    }
}

$allowedStatuses = @("present", "missing", "pulled")
$notInitiallyPresentCount = 0
foreach ($image in $images) {
    foreach ($property in @("name", "image", "status", "pull_command")) {
        Assert-Property -Object $image -Name $property
    }

    if ([string]::IsNullOrWhiteSpace([string]$image.image)) {
        throw "Observability image prepare plan has an empty image value for role $($image.name)."
    }
    if ($allowedStatuses -notcontains [string]$image.status) {
        throw "Observability image prepare plan has invalid status '$($image.status)' for role $($image.name)."
    }
    if ($image.status -ne "present") {
        $notInitiallyPresentCount++
    }

    $pullCommand = [string]$image.pull_command
    if ($pullCommand -notmatch "^docker pull(\s+--platform\s+\S+)?\s+\S+") {
        throw "Observability image prepare plan has invalid pull command for role $($image.name)."
    }
    if (-not $pullCommand.Contains([string]$image.image)) {
        throw "Observability image prepare plan pull command must include image name for role $($image.name)."
    }
    if (-not [string]::IsNullOrWhiteSpace([string]$plan.platform) -and $pullCommand -notmatch "--platform\s+$([regex]::Escape([string]$plan.platform))") {
        throw "Observability image prepare plan pull command must include selected platform for role $($image.name)."
    }
    if (-not $plan.allow_image_pull -and $image.status -eq "pulled") {
        throw "Observability image prepare plan dry-run cannot contain pulled status."
    }
}

if ([int]$plan.missing_count -ne $notInitiallyPresentCount) {
    throw "Observability image prepare plan missing_count mismatch: expected $notInitiallyPresentCount, found $($plan.missing_count)."
}

if ([string]$plan.boundary -notmatch "local observability image preparation") {
    throw "Observability image prepare plan boundary must describe local image preparation."
}

if ($RequireReport) {
    $reportPath = Join-Path (Split-Path -Parent $PlanPath) "observability-image-prepare-plan.md"
    if (-not (Test-Path -LiteralPath $reportPath -PathType Leaf)) {
        throw "Missing observability image prepare plan report: $reportPath"
    }
    $report = Get-Content -LiteralPath $reportPath -Raw
    if ($report -notmatch "NexusIM Observability Image Prepare Plan") {
        throw "Observability image prepare plan report missing title."
    }
    foreach ($image in $images) {
        if ($report -notmatch [regex]::Escape([string]$image.image)) {
            throw "Observability image prepare plan report missing image: $($image.image)"
        }
    }
}

Write-Host "OK   observability image prepare plan validated: $PlanPath"
