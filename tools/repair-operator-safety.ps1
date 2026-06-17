$ErrorActionPreference = "Stop"

function Get-RepairSha256Hex {
    param([byte[]]$Bytes)

    $sha = [System.Security.Cryptography.SHA256]::Create()
    try {
        $hash = $sha.ComputeHash($Bytes)
    } finally {
        $sha.Dispose()
    }
    return -join ($hash | ForEach-Object { $_.ToString("x2") })
}

function Assert-LowSensitiveRepairActor {
    param(
        [string]$Value,
        [string]$FieldName
    )

    if ([string]::IsNullOrWhiteSpace($Value)) {
        throw "$FieldName is required."
    }
    if ($Value.Length -gt 64 -or $Value -notmatch "^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$") {
        throw "$FieldName must be a low-sensitive operator id using letters, digits, dot, underscore, or dash."
    }
    if ($Value -match "(?i)(password|passwd|secret|token|bearer|credential|api[_-]?key|access[_-]?key|refresh|session|cookie|sk-|eyJ)") {
        throw "$FieldName must be a low-sensitive operator id, not a credential-like value."
    }
}

function Assert-LowSensitiveRepairAdHocEnv {
    param(
        [string]$Key,
        [string]$Value
    )

    if ($Key -cnotmatch "^[A-Z][A-Z0-9_]*$") {
        throw "Env key must be an uppercase environment variable name: $Key"
    }

    if ($Key -match "(?i)(PASSWORD|PASSWD|SECRET|TOKEN|BEARER|PRIVATE|CREDENTIAL|API[_-]?KEY|ACCESS[_-]?KEY|REFRESH|SESSION|COOKIE)") {
        throw "Refusing to write potentially sensitive Env key into repair operator plan: $Key"
    }

    if ($Value -match "(?i)(bearer\s+\S+|password\s*=|secret\s*=|token\s*=|sk-[A-Za-z0-9_-]{8,}|eyJ[A-Za-z0-9_-]+\.)") {
        throw "Refusing to write potentially sensitive Env value into repair operator plan: $Key"
    }
}
