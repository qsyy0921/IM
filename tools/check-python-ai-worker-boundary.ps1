$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$pythonRoot = Join-Path $repoRoot "ai\python"
$environmentPath = Join-Path $pythonRoot "environment.yml"
$pyprojectPath = Join-Path $pythonRoot "pyproject.toml"
$schemaPath = Join-Path $pythonRoot "contracts\worker-candidate.schema.json"
$gitignorePath = Join-Path $repoRoot ".gitignore"

function Fail($message) {
    Write-Error $message
    exit 1
}

foreach ($path in @($pythonRoot, $environmentPath, $pyprojectPath, $schemaPath, $gitignorePath)) {
    if (-not (Test-Path -LiteralPath $path)) {
        Fail "Missing required Python AI worker boundary file: $path"
    }
}

$environmentText = Get-Content -LiteralPath $environmentPath -Raw
if ($environmentText -notmatch "(?m)^name:\s*IM\s*$") {
    Fail "ai/python/environment.yml must define the conda environment name as IM"
}
if ($environmentText -notmatch "-e \.\[dev\]") {
    Fail "ai/python/environment.yml must install the Python worker package in editable dev mode"
}

$pyprojectText = Get-Content -LiteralPath $pyprojectPath -Raw
if ($pyprojectText -notmatch 'boundary\s*=\s*"candidate-only"') {
    Fail "ai/python/pyproject.toml must declare tool.nexusim.boundary = candidate-only"
}
if ($pyprojectText -notmatch 'control_plane\s*=\s*"go-services"') {
    Fail "ai/python/pyproject.toml must declare tool.nexusim.control_plane = go-services"
}

$gitignoreText = Get-Content -LiteralPath $gitignorePath -Raw
foreach ($requiredIgnore in @("__pycache__/", "*.egg-info/", ".pytest_cache/", ".ruff_cache/", ".mypy_cache/")) {
    if (-not $gitignoreText.Contains($requiredIgnore)) {
        Fail ".gitignore must ignore Python generated output: $requiredIgnore"
    }
}

$pythonFiles = Get-ChildItem -LiteralPath $pythonRoot -Recurse -File -Include *.py |
    Where-Object {
        $_.FullName -notmatch "\\(__pycache__|\.pytest_cache|\.ruff_cache|\.mypy_cache|.*\.egg-info)\\"
    }

$forbiddenPatterns = @(
    @{ Pattern = "(?m)^\s*(from|import)\s+(psycopg|psycopg2|asyncpg|sqlalchemy)\b"; Reason = "direct PostgreSQL/ORM client import" },
    @{ Pattern = "\b(create_engine|connect\(\s*['""]postgres|postgresql://|NEXUSIM_PG_DSN)\b"; Reason = "direct PostgreSQL connection" },
    @{ Pattern = "\b(INSERT|UPDATE|DELETE|SELECT)\b[\s\S]{0,160}\b(identity_|message_|conversation_|delivery_|receipt_|contacts_|policy_)[A-Za-z0-9_]*"; Reason = "direct IM business table SQL" }
)

foreach ($file in $pythonFiles) {
    $content = Get-Content -LiteralPath $file.FullName -Raw
    foreach ($rule in $forbiddenPatterns) {
        if ($content -match $rule.Pattern) {
            Fail "Python AI worker boundary violation in $($file.FullName): $($rule.Reason)"
        }
    }
}

Write-Host "OK   Python AI worker boundary guard passed"
