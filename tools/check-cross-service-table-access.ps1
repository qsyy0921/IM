$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$servicesRoot = Join-Path $repoRoot "services"
$migrationsRoot = Join-Path $repoRoot "migrations\postgres"

function Convert-ToRepoRelativePath {
    param([string]$Path)

    $root = [System.IO.Path]::GetFullPath($repoRoot).TrimEnd("\", "/")
    $fullPath = [System.IO.Path]::GetFullPath($Path)
    $prefix = $root + [System.IO.Path]::DirectorySeparatorChar
    if ($fullPath.StartsWith($prefix, [System.StringComparison]::OrdinalIgnoreCase)) {
        return $fullPath.Substring($prefix.Length)
    }
    return $fullPath
}

function Convert-MigrationOwnerToServiceName {
    param([string]$Owner)

    $directServicePath = Join-Path $servicesRoot $Owner
    if (Test-Path -LiteralPath $directServicePath -PathType Container) {
        return $Owner
    }
    return "$Owner-service"
}

$tableOwners = @{}
$tablePattern = "(?im)\b(?:CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?|ALTER\s+TABLE\s+)([a-zA-Z_][a-zA-Z0-9_]*)"

foreach ($ownerDir in (Get-ChildItem -LiteralPath $migrationsRoot -Directory | Sort-Object Name)) {
    $serviceName = Convert-MigrationOwnerToServiceName -Owner $ownerDir.Name
    foreach ($migration in (Get-ChildItem -LiteralPath $ownerDir.FullName -Filter "*.sql" -File)) {
        $content = Get-Content -LiteralPath $migration.FullName -Raw
        foreach ($match in [regex]::Matches($content, $tablePattern)) {
            $table = $match.Groups[1].Value
            if (-not $tableOwners.ContainsKey($table)) {
                $tableOwners[$table] = $serviceName
                continue
            }
            if ($tableOwners[$table] -ne $serviceName) {
                throw "table $table appears in multiple migration owners: $($tableOwners[$table]) and $serviceName"
            }
        }
    }
}

$allowedCrossServiceTables = @{
    "conversation-service" = @(
        "conversation_seq",
        "conversation_timeline_events",
        "message_outbox"
    )
}

$sqlReferencePattern = "(?im)\b(?:FROM|JOIN|UPDATE|INTO)\s+([a-zA-Z_][a-zA-Z0-9_]*)"
$violations = [System.Collections.Generic.List[string]]::new()

foreach ($service in (Get-ChildItem -LiteralPath $servicesRoot -Directory | Sort-Object Name)) {
    $allowedTables = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
    if ($allowedCrossServiceTables.ContainsKey($service.Name)) {
        foreach ($table in $allowedCrossServiceTables[$service.Name]) {
            [void]$allowedTables.Add($table)
        }
    }

    $goFiles = Get-ChildItem -LiteralPath $service.FullName -Recurse -Filter "*.go" -File |
        Where-Object { $_.Name -notlike "*_test.go" }
    foreach ($file in $goFiles) {
        $content = Get-Content -LiteralPath $file.FullName -Raw
        foreach ($match in [regex]::Matches($content, $sqlReferencePattern)) {
            $table = $match.Groups[1].Value
            if (-not $tableOwners.ContainsKey($table)) {
                continue
            }
            $owner = $tableOwners[$table]
            if ($owner -eq $service.Name) {
                continue
            }
            if ($allowedTables.Contains($table)) {
                continue
            }
            $violations.Add("$(Convert-ToRepoRelativePath -Path $file.FullName): $($service.Name) references table $table owned by $owner")
        }
    }
}

if ($violations.Count -gt 0) {
    throw ($violations -join [Environment]::NewLine)
}

Write-Host "OK   cross-service table access guardrails"
