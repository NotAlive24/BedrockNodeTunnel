[CmdletBinding()]
param(
    [ValidateSet("install", "update", "uninstall")]
    [string]$Action = "install",

    [string]$InstallDir = "",

    [switch]$PurgeData,

    [switch]$NoPathUpdate
)

$ErrorActionPreference = "Stop"
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$Repository = "NotAlive24/BedrockNodeTunnel"
$ReleaseBase = "https://github.com/$Repository/releases/latest/download"
$ConfigDir = Join-Path $env:APPDATA "bedrocknode-tunnel"

if ([string]::IsNullOrWhiteSpace($InstallDir)) {
    $InstallDir = Join-Path $env:LOCALAPPDATA "Programs\BedrockNodeTunnel"
}
$InstallDir = [IO.Path]::GetFullPath($InstallDir)
$BinaryPath = Join-Path $InstallDir "nethernet-client.exe"

function Test-TrustedGitHubAssetUri {
    param([Uri]$Uri)

    if ($Uri.Scheme -ne "https") { return $false }
    $hostName = $Uri.Host.ToLowerInvariant()
    return (
        $hostName -eq "github.com" -or
        $hostName -eq "objects.githubusercontent.com" -or
        $hostName -eq "release-assets.githubusercontent.com" -or
        $hostName.EndsWith(".githubusercontent.com")
    )
}

function Download-HttpsAsset {
    param(
        [string]$Uri,
        [string]$OutFile,
        [long]$MaxBytes = 104857600
    )

    $current = [Uri]$Uri
    for ($redirect = 0; $redirect -le 5; $redirect++) {
        if (-not (Test-TrustedGitHubAssetUri -Uri $current)) {
            throw "Refusing non-HTTPS or untrusted GitHub asset URL: $current"
        }

        $request = [System.Net.HttpWebRequest]::Create($current)
        $request.AllowAutoRedirect = $false
        $request.Timeout = 120000
        $request.ReadWriteTimeout = 120000
        $request.UserAgent = "BedrockNodeTunnel-Installer/2.0.0"

        $response = $null
        $stream = $null
        $file = $null
        try {
            try {
                $response = $request.GetResponse()
            } catch [System.Net.WebException] {
                $response = $_.Exception.Response
                if ($null -eq $response) { throw }
            }

            $status = [int]$response.StatusCode
            if ($status -in @(301, 302, 303, 307, 308)) {
                $location = $response.Headers["Location"]
                if ([string]::IsNullOrWhiteSpace($location)) {
                    throw "GitHub asset redirect did not contain a Location header."
                }
                $current = [Uri]::new($current, $location)
                continue
            }
            if ($status -ne 200) {
                throw "GitHub asset download returned HTTP $status."
            }
            if ($response.ContentLength -gt $MaxBytes) {
                throw "GitHub asset exceeded the configured download size limit."
            }

            $stream = $response.GetResponseStream()
            $file = [IO.File]::Open($OutFile, [IO.FileMode]::Create, [IO.FileAccess]::Write, [IO.FileShare]::None)
            $buffer = New-Object byte[] 65536
            [long]$total = 0
            while (($read = $stream.Read($buffer, 0, $buffer.Length)) -gt 0) {
                $total += $read
                if ($total -gt $MaxBytes) {
                    throw "GitHub asset exceeded the configured download size limit."
                }
                $file.Write($buffer, 0, $read)
            }
            return
        } finally {
            if ($file) { $file.Dispose() }
            if ($stream) { $stream.Dispose() }
            if ($response) { $response.Close() }
        }
    }

    throw "GitHub asset exceeded the maximum number of HTTPS redirects."
}

function Remove-UserPathEntry {
    param([string]$PathToRemove)

    $current = [Environment]::GetEnvironmentVariable("Path", "User")
    $entries = @($current -split ";" | Where-Object {
        -not [string]::IsNullOrWhiteSpace($_) -and
        [string]::Compare($_.TrimEnd("\"), $PathToRemove.TrimEnd("\"), $true) -ne 0
    })
    [Environment]::SetEnvironmentVariable("Path", ($entries -join ";"), "User")
}

if ($Action -eq "uninstall") {
    if (Test-Path -LiteralPath $BinaryPath) {
        Remove-Item -LiteralPath $BinaryPath -Force
    }

    if (-not $NoPathUpdate) {
        Remove-UserPathEntry -PathToRemove $InstallDir
        $env:Path = (($env:Path -split ";" | Where-Object {
            [string]::Compare($_.TrimEnd("\"), $InstallDir.TrimEnd("\"), $true) -ne 0
        }) -join ";")
    }

    if ($PurgeData -and (Test-Path -LiteralPath $ConfigDir)) {
        Remove-Item -LiteralPath $ConfigDir -Recurse -Force
        Write-Host "Removed saved tunnel token and node configuration."
    } else {
        Write-Host "Saved tunnel data was kept at: $ConfigDir"
    }

    if (Test-Path -LiteralPath $InstallDir) {
        $remaining = Get-ChildItem -LiteralPath $InstallDir -Force
        if (-not $remaining) {
            Remove-Item -LiteralPath $InstallDir -Force
        }
    }

    Write-Host "nethernet-client has been uninstalled."
    return
}

$architecture = [Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()
switch ($architecture) {
    "X64"   { $GoArch = "amd64" }
    "Arm64" { $GoArch = "arm64" }
    default { throw "Unsupported Windows architecture: $architecture. Supported: x64 and ARM64." }
}

$Asset = "nethernet-client-windows-$GoArch.exe"
$TempDir = Join-Path ([IO.Path]::GetTempPath()) ("BedrockNodeTunnel-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $TempDir -Force | Out-Null

try {
    Write-Host "Downloading latest Windows/$GoArch build..."
    $DownloadPath = Join-Path $TempDir $Asset
    $ChecksumsPath = Join-Path $TempDir "SHA256SUMS"

    Download-HttpsAsset -Uri "$ReleaseBase/$Asset" -OutFile $DownloadPath -MaxBytes 104857600
    Download-HttpsAsset -Uri "$ReleaseBase/SHA256SUMS" -OutFile $ChecksumsPath -MaxBytes 1048576

    $checksumLine = Get-Content -LiteralPath $ChecksumsPath | Where-Object {
        $_ -match ("^([a-fA-F0-9]{64})\s+\*?" + [regex]::Escape($Asset) + "$")
    } | Select-Object -First 1

    if (-not $checksumLine) {
        throw "No SHA-256 entry found for $Asset. Refusing to install an unverified build."
    }

    $expectedHash = (($checksumLine -split "\s+")[0]).ToLowerInvariant()
    $actualHash = (Get-FileHash -LiteralPath $DownloadPath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actualHash -ne $expectedHash) {
        throw "SHA-256 verification failed for $Asset."
    }

    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    try {
        Move-Item -LiteralPath $DownloadPath -Destination $BinaryPath -Force
    } catch {
        throw "Could not replace nethernet-client.exe. Stop any running client and retry. $($_.Exception.Message)"
    }

    if (-not $NoPathUpdate) {
        $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
        $entries = @($userPath -split ";" | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
        $alreadyPresent = $false
        foreach ($entry in $entries) {
            if ([string]::Compare($entry.TrimEnd("\"), $InstallDir.TrimEnd("\"), $true) -eq 0) {
                $alreadyPresent = $true
                break
            }
        }
        if (-not $alreadyPresent) {
            $newPath = if ([string]::IsNullOrWhiteSpace($userPath)) { $InstallDir } else { "$userPath;$InstallDir" }
            [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
        }
        if (($env:Path -split ";" | Where-Object {
            [string]::Compare($_.TrimEnd("\"), $InstallDir.TrimEnd("\"), $true) -eq 0
        }).Count -eq 0) {
            $env:Path = "$env:Path;$InstallDir"
        }
    }

    $installedVersion = & $BinaryPath -version
    Write-Host "Installed nethernet-client $installedVersion to $BinaryPath"
    Write-Host "Saved tokens and node settings are preserved in: $ConfigDir"
    if ($Action -eq "update") {
        Write-Host "Update complete."
    }
} finally {
    if (Test-Path -LiteralPath $TempDir) {
        Remove-Item -LiteralPath $TempDir -Recurse -Force
    }
}
