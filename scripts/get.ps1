param(
    [string]$Version = $(if ($env:DA_VERSION) { $env:DA_VERSION } else { "edge" }),
    [string]$Destination = $(if ($env:DA_DEST) { $env:DA_DEST } else { ".\deploy-agent.exe" }),
    [string]$Repository = $(if ($env:DA_REPO) { $env:DA_REPO } else { "ngtrthanh/deploy-agent" })
)

$ErrorActionPreference = "Stop"

$arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString().ToLowerInvariant()
switch ($arch) {
    "x64"   { $arch = "amd64" }
    "x86"   { $arch = "386" }
    "arm64" { $arch = "arm64" }
    default  { throw "Unsupported Windows architecture: $arch" }
}

$asset = "deploy-agent-windows-$arch.exe"
$base = "https://github.com/$Repository/releases/download/$Version"
$tmp = "$Destination.tmp"
$checksums = "$Destination.checksums"

try {
    Invoke-WebRequest -UseBasicParsing "$base/$asset" -OutFile $tmp
    Invoke-WebRequest -UseBasicParsing "$base/checksums.txt" -OutFile $checksums

    $line = Get-Content $checksums | Where-Object { $_ -match "\s+$([regex]::Escape($asset))$" } | Select-Object -First 1
    if (-not $line) { throw "Checksum not found for $asset" }
    $expected = ($line -split '\s+')[0].ToLowerInvariant()
    $actual = (Get-FileHash -Algorithm SHA256 $tmp).Hash.ToLowerInvariant()
    if ($actual -ne $expected) { throw "Checksum mismatch" }

    Move-Item -Force $tmp $Destination
    Write-Host "Installed $asset -> $Destination"
    & $Destination -version
}
finally {
    Remove-Item -Force -ErrorAction SilentlyContinue $tmp, $checksums
}
