$ErrorActionPreference = "Stop"

function Assert-LowSensitiveEvidenceText {
    param(
        [string]$Value,
        [string]$FieldName,
        [int]$MaxLength = 512,
        [switch]$AllowEmpty
    )

    $text = ([string]$Value).Trim()
    if ($text.Length -eq 0) {
        if ($AllowEmpty) {
            return
        }
        throw "$FieldName is required."
    }

    if ($text.Length -gt $MaxLength) {
        throw "$FieldName is too long for a low-sensitive evidence manifest field."
    }

    $emailPattern = "[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}"
    $credentialPattern = "(?i)(bearer\s+[A-Za-z0-9._~+/=-]{8,}|password\s*=|passwd\s*=|secret\s*=|token\s*=|api[_-]?key\s*=|access[_-]?key\s*=|sk-[A-Za-z0-9_-]{8,}|eyJ[A-Za-z0-9_-]{10,}\.|https?://[^/\s:@]+:[^/\s@]+@)"

    if ($text -match $emailPattern -or $text -match $credentialPattern) {
        throw "$FieldName must be low-sensitive evidence metadata and must not contain email, token, password, key, bearer, JWT, or URL userinfo values."
    }
}
