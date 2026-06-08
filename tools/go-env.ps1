$GoRoot = Join-Path $env:USERPROFILE ".local\go\go1.26.4\bin"
$GoPathBin = Join-Path $env:USERPROFILE "go\bin"

$paths = @($GoRoot, $GoPathBin)
foreach ($path in $paths) {
    if ((Test-Path $path) -and (($env:PATH -split ';') -notcontains $path)) {
        $env:PATH = "$path;$env:PATH"
    }
}

$env:GOPROXY = "https://goproxy.cn,direct"
$env:GOSUMDB = "sum.golang.google.cn"
