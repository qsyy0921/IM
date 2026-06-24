$ErrorActionPreference = "Stop"

function Test-PathInsideDirectory {
    param(
        [string]$Path,
        [string]$Directory
    )

    try {
        $fullPath = [System.IO.Path]::GetFullPath($Path).TrimEnd(
            [System.IO.Path]::DirectorySeparatorChar,
            [System.IO.Path]::AltDirectorySeparatorChar
        )
        $fullDirectory = [System.IO.Path]::GetFullPath($Directory).TrimEnd(
            [System.IO.Path]::DirectorySeparatorChar,
            [System.IO.Path]::AltDirectorySeparatorChar
        )
    }
    catch {
        throw "Invalid path while checking output root. Path=`"$Path`"; Directory=`"$Directory`"; error=$($_.Exception.Message)"
    }

    if ($fullPath.Equals($fullDirectory, [System.StringComparison]::OrdinalIgnoreCase)) {
        return $true
    }

    $prefix = $fullDirectory + [System.IO.Path]::DirectorySeparatorChar
    return $fullPath.StartsWith($prefix, [System.StringComparison]::OrdinalIgnoreCase)
}

function Assert-ExternalOutputRoot {
    param(
        [string]$Value,
        [string]$RepositoryRoot,
        [string]$Name = "ResultRoot",
        [string]$SuggestedRoot = "H:\NexusIM\loadtest-results"
    )

    if ([string]::IsNullOrWhiteSpace($Value)) {
        throw "$Name must not be empty. Store raw smoke/loadtest output under $SuggestedRoot or another external scratch directory."
    }

    if ([string]::IsNullOrWhiteSpace($RepositoryRoot)) {
        throw "RepositoryRoot must not be empty while validating $Name."
    }

    if (Test-PathInsideDirectory -Path $Value -Directory $RepositoryRoot) {
        throw "$Name must not be inside the repository. Store raw smoke/loadtest output under $SuggestedRoot or another external scratch directory."
    }
}
