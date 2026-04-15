param(
    [string]$Platform = "all",  # all, windows, linux, darwin
    [string]$Architecture = "all"  # all, x86, x86-64, arm64
)

# PowerShell build script for db-backup-restore

# Stop execution on error
$ErrorActionPreference = "Stop"

# Project root directory
$ProjectRoot = Split-Path -Parent $PSScriptRoot

# Build output base directory
$OutputBaseDir = Join-Path $ProjectRoot "bin"

# All available platform-architecture combinations
$AllPlatforms = @(
    # Windows
    @{ Platform = "windows"; Architecture = "x86"; GOOS = "windows"; GOARCH = "386"; Extension = ".exe" },
    @{ Platform = "windows"; Architecture = "x86-64"; GOOS = "windows"; GOARCH = "amd64"; Extension = ".exe" },
    @{ Platform = "windows"; Architecture = "arm64"; GOOS = "windows"; GOARCH = "arm64"; Extension = ".exe" },
    # Linux
    @{ Platform = "linux"; Architecture = "x86-64"; GOOS = "linux"; GOARCH = "amd64"; Extension = "" },
    @{ Platform = "linux"; Architecture = "arm64"; GOOS = "linux"; GOARCH = "arm64"; Extension = "" },
    # macOS
    @{ Platform = "darwin"; Architecture = "x86-64"; GOOS = "darwin"; GOARCH = "amd64"; Extension = "" },
    @{ Platform = "darwin"; Architecture = "arm64"; GOOS = "darwin"; GOARCH = "arm64"; Extension = "" },
    # macOS Universal (special case)
    @{ Platform = "darwin"; Architecture = "universal"; GOOS = "darwin"; GOARCH = "amd64,arm64"; Extension = "" }
)

# Determine which platform-architecture combinations to build
if ($Platform -eq "all") {
    $SelectedPlatforms = $AllPlatforms
}
else {
    $SelectedPlatforms = $AllPlatforms | Where-Object { $_.Platform -eq $Platform }
    if ($SelectedPlatforms.Count -eq 0) {
        Write-Error "Invalid platform: $Platform. Valid options: all, windows, linux, darwin"
        exit 1
    }
}

if ($Architecture -ne "all") {
    $SelectedPlatforms = $SelectedPlatforms | Where-Object { $_.Architecture -eq $Architecture }
    if ($SelectedPlatforms.Count -eq 0) {
        Write-Error "Invalid architecture: $Architecture for platform $Platform"
        exit 1
    }
}

# Clean previous build artifacts for selected platform-architecture combinations
Write-Host "Cleaning previous build artifacts..."
foreach ($Combo in $SelectedPlatforms) {
    $OutputDir = Join-Path $OutputBaseDir $Combo.Platform | Join-Path -ChildPath $Combo.Architecture
    if (Test-Path $OutputDir) {
        Remove-Item -Recurse -Force $OutputDir
    }
    New-Item -ItemType Directory -Path $OutputDir -Force | Out-Null
}

# Change to project root directory
Set-Location $ProjectRoot

# Check Go version
Write-Host "Checking Go version..."
go version

# Download dependencies
Write-Host "Downloading dependencies..."
go mod tidy

# Build for each platform-architecture combination
foreach ($Combo in $SelectedPlatforms) {
    Write-Host "Building for $($Combo.Platform) $($Combo.Architecture)..."
    
    # Build output directory for this platform-architecture combination
    $OutputDir = Join-Path $OutputBaseDir $Combo.Platform | Join-Path -ChildPath $Combo.Architecture
    
    # Build artifact path
    $BuildArtifactPath = Join-Path $OutputDir "db-backup-restore$($Combo.Extension)"
    
    # Set environment variables for cross-compilation
    $env:GOOS = $Combo.GOOS
    
    if ($Combo.Architecture -eq "universal") {
        # Build macOS universal binary
        # First build x86-64
        $env:GOARCH = "amd64"
        $x86_64ArtifactPath = Join-Path $OutputDir "db-backup-restore_amd64"
        $MainPackagePath = Join-Path $ProjectRoot "cmd" | Join-Path -ChildPath "db-backup-restore"
        go build -ldflags="-s -w" -o "$x86_64ArtifactPath" "$MainPackagePath"
        
        if ($LASTEXITCODE -ne 0) {
            Write-Error "Build failed for $($Combo.Platform) $($Combo.Architecture) (x86-64)!"
            exit $LASTEXITCODE
        }
        
        # Then build arm64
        $env:GOARCH = "arm64"
        $arm64ArtifactPath = Join-Path $OutputDir "db-backup-restore_arm64"
        go build -ldflags="-s -w" -o "$arm64ArtifactPath" "$MainPackagePath"
        
        if ($LASTEXITCODE -ne 0) {
            Write-Error "Build failed for $($Combo.Platform) $($Combo.Architecture) (arm64)!"
            exit $LASTEXITCODE
        }
        
        # Combine into universal binary using lipo (requires macOS)
        Write-Host "Creating universal binary..."
        try {
            # Check if lipo is available
            $lipoPath = Get-Command "lipo" -ErrorAction SilentlyContinue
            if ($lipoPath) {
                & lipo -create -output "$BuildArtifactPath" "$x86_64ArtifactPath" "$arm64ArtifactPath"
                if ($LASTEXITCODE -ne 0) {
                    Write-Error "Failed to create universal binary!"
                    exit $LASTEXITCODE
                }
                # Clean up intermediate files
                Remove-Item "$x86_64ArtifactPath" -ErrorAction SilentlyContinue
                Remove-Item "$arm64ArtifactPath" -ErrorAction SilentlyContinue
            }
            else {
                Write-Warning "lipo not found, skipping universal binary creation."
                # Clean up intermediate files
                Remove-Item "$x86_64ArtifactPath" -ErrorAction SilentlyContinue
                Remove-Item "$arm64ArtifactPath" -ErrorAction SilentlyContinue
            }
        }
        catch {
            Write-Warning "Failed to create universal binary: $($_.Exception.Message)"
            # Clean up intermediate files
            Remove-Item "$x86_64ArtifactPath" -ErrorAction SilentlyContinue
            Remove-Item "$arm64ArtifactPath" -ErrorAction SilentlyContinue
        }
    }
    else {
        # Normal build for other architectures
        $env:GOARCH = $Combo.GOARCH
        
        # Build project (release version)
        $MainPackagePath = Join-Path $ProjectRoot "cmd" | Join-Path -ChildPath "db-backup-restore"
        go build -ldflags="-s -w" -o "$BuildArtifactPath" "$MainPackagePath"
        
        # Check if build was successful
        if ($LASTEXITCODE -ne 0) {
            Write-Error "Build failed for $($Combo.Platform) $($Combo.Architecture)!"
            exit $LASTEXITCODE
        }
    }
    
    # Show build results
    Write-Host "Build successful for $($Combo.Platform) $($Combo.Architecture)!"
    if (Test-Path $BuildArtifactPath) {
        $BuildArtifact = Get-Item $BuildArtifactPath
        Write-Host "Build artifact: $BuildArtifactPath"
        Write-Host "Build artifact size: $([math]::Round($BuildArtifact.Length / 1MB, 2)) MB"
    }
    else {
        Write-Warning "Build artifact not found: $BuildArtifactPath"
    }
    Write-Host ""
}

# Clean up environment variables
Remove-Item env:GOOS -ErrorAction SilentlyContinue
Remove-Item env:GOARCH -ErrorAction SilentlyContinue

# Restore current directory
Set-Location $PSScriptRoot

if ($Platform -eq "all" -and $Architecture -eq "all") {
    Write-Host "Build completed for all platforms and architectures!"
}
elseif ($Platform -eq "all") {
    Write-Host "Build completed for all platforms with architecture $Architecture!"
}
elseif ($Architecture -eq "all") {
    Write-Host "Build completed for platform $Platform with all architectures!"
}
else {
    Write-Host "Build completed for platform $Platform with architecture $Architecture!"
}
