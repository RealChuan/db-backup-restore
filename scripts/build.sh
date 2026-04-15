#!/bin/bash

# Bash build script for db-backup-restore

# Exit on error
set -e

# Default parameters
PLATFORM="all"  # all, windows, linux, darwin
ARCHITECTURE="all"  # all, x86, x86-64, arm64

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -p|--platform)
            PLATFORM="$2"
            shift 2
            ;;
        -a|--architecture)
            ARCHITECTURE="$2"
            shift 2
            ;;
        *)
            echo "Unknown argument: $1"
            exit 1
            ;;
    esac
done

# Project root directory
PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# Build output base directory
OUTPUT_BASE_DIR="$PROJECT_ROOT/bin"

# Define platform-architecture combinations
declare -A PLATFORMS
PLATFORMS["windows_x86"]="windows 386 .exe"
PLATFORMS["windows_x86-64"]="windows amd64 .exe"
PLATFORMS["windows_arm64"]="windows arm64 .exe"
PLATFORMS["linux_x86-64"]="linux amd64 "
PLATFORMS["linux_arm64"]="linux arm64 "
PLATFORMS["darwin_x86-64"]="darwin amd64 "
PLATFORMS["darwin_arm64"]="darwin arm64 "
PLATFORMS["darwin_universal"]="darwin universal "

# Determine which platform-architecture combinations to build
SELECTED_PLATFORMS=()
for combo in "${!PLATFORMS[@]}"; do
    IFS=' ' read -r platform arch extension <<< "${PLATFORMS[$combo]}"
    
    if [[ "$PLATFORM" == "all" || "$PLATFORM" == "$platform" ]]; then
        if [[ "$ARCHITECTURE" == "all" || "$ARCHITECTURE" == "$arch" || ("$combo" == "darwin_universal" && "$ARCHITECTURE" == "universal") ]]; then
            SELECTED_PLATFORMS+=($combo)
        fi
    fi
done

if [ ${#SELECTED_PLATFORMS[@]} -eq 0 ]; then
    echo "No valid platform-architecture combinations found"
    exit 1
fi

# Clean previous build artifacts for selected platform-architecture combinations
echo "Cleaning previous build artifacts..."
for combo in "${SELECTED_PLATFORMS[@]}"; do
    IFS='_' read -r platform arch <<< "$combo"
    OUTPUT_DIR="$OUTPUT_BASE_DIR/$platform/$arch"
    if [ -d "$OUTPUT_DIR" ]; then
        rm -rf "$OUTPUT_DIR"
    fi
    mkdir -p "$OUTPUT_DIR"
done

# Change to project root directory
cd "$PROJECT_ROOT"

# Check Go version
echo "Checking Go version..."
go version

# Download dependencies
echo "Downloading dependencies..."
go mod tidy

# Build for each platform-architecture combination
for combo in "${SELECTED_PLATFORMS[@]}"; do
    IFS='_' read -r platform arch <<< "$combo"
    IFS=' ' read -r goos goarch extension <<< "${PLATFORMS[$combo]}"
    
    echo "Building for $platform $arch..."
    
    # Build output directory
    OUTPUT_DIR="$OUTPUT_BASE_DIR/$platform/$arch"
    
    # Build artifact path
    BUILD_ARTIFACT_PATH="$OUTPUT_DIR/db-backup-restore$extension"
    
    if [ "$arch" == "universal" ]; then
        # Build macOS universal binary
        # First build x86-64
        export GOOS="darwin"
        export GOARCH="amd64"
        X86_64_ARTIFACT_PATH="$OUTPUT_DIR/db-backup-restore_amd64"
        go build -ldflags="-s -w" -o "$X86_64_ARTIFACT_PATH" "./cmd/db-backup-restore"
        
        # Then build arm64
        export GOOS="darwin"
        export GOARCH="arm64"
        ARM64_ARTIFACT_PATH="$OUTPUT_DIR/db-backup-restore_arm64"
        go build -ldflags="-s -w" -o "$ARM64_ARTIFACT_PATH" "./cmd/db-backup-restore"
        
        # Combine into universal binary using lipo
        echo "Creating universal binary..."
        if command -v lipo > /dev/null; then
            lipo -create -output "$BUILD_ARTIFACT_PATH" "$X86_64_ARTIFACT_PATH" "$ARM64_ARTIFACT_PATH"
            # Clean up intermediate files
            rm -f "$X86_64_ARTIFACT_PATH" "$ARM64_ARTIFACT_PATH"
        else
            echo "Warning: lipo not found, skipping universal binary creation."
            # Clean up intermediate files
            rm -f "$X86_64_ARTIFACT_PATH" "$ARM64_ARTIFACT_PATH"
        fi
    else
        # Normal build for other architectures
        export GOOS="$goos"
        export GOARCH="$goarch"
        
        # Build project (release version)
        go build -ldflags="-s -w" -o "$BUILD_ARTIFACT_PATH" "./cmd/db-backup-restore"
    fi
    
    # Show build results
    echo "Build successful for $platform $arch!"
    if [ -f "$BUILD_ARTIFACT_PATH" ]; then
        echo "Build artifact: $BUILD_ARTIFACT_PATH"
        echo "Build artifact size: $(du -h "$BUILD_ARTIFACT_PATH" | cut -f1)"
    else
        echo "Warning: Build artifact not found: $BUILD_ARTIFACT_PATH"
    fi
    echo ""
done

# Clean up environment variables
unset GOOS
unset GOARCH

# Restore current directory
cd "$(dirname "$0")"

# Show completion message
if [ "$PLATFORM" == "all" ] && [ "$ARCHITECTURE" == "all" ]; then
    echo "Build completed for all platforms and architectures!"
elif [ "$PLATFORM" == "all" ]; then
    echo "Build completed for all platforms with architecture $ARCHITECTURE!"
elif [ "$ARCHITECTURE" == "all" ]; then
    echo "Build completed for platform $PLATFORM with all architectures!"
else
    echo "Build completed for platform $PLATFORM with architecture $ARCHITECTURE!"
fi