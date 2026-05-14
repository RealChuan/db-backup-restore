#Requires -Version 5.1

param(
    [switch]$Fix,
    [switch]$Verbose,
    [switch]$SkipTests
)

$ErrorActionPreference = "Continue"
$Script:HasErrors = $false
$Script:ProjectRoot = Split-Path $PSScriptRoot -Parent

Push-Location $Script:ProjectRoot

Write-Host "========================================" -ForegroundColor Cyan
Write-Host " Go Project Code Check" -ForegroundColor Cyan
Write-Host " Project: $Script:ProjectRoot" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan

$goVersion = & go version
Write-Host "Go Version: $goVersion"
Write-Host ""

if (-not (Test-Path "go.mod")) {
    Write-Host "[FAIL] go.mod not found" -ForegroundColor Red
    $Script:HasErrors = $true
}

Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host " 1. Dependency Verify" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan

$modOutput = & go mod verify 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Host "[WARN] Dependency verify failed" -ForegroundColor Yellow
    $Script:HasErrors = $true
    if ($Verbose) { Write-Host $modOutput }
} else {
    Write-Host "[PASS] Dependency verify passed" -ForegroundColor Green
}

Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host " 2. Format Check" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan

$fmtOutput = & go fmt ./... 2>&1
$files = @($fmtOutput) | Where-Object { $_ -match '\S' }

if ($files.Count -gt 0) {
    Write-Host "[WARN] Found $($files.Count) files with formatting issues" -ForegroundColor Yellow
    $Script:HasErrors = $true
    if ($Verbose) {
        foreach ($f in $files) {
            Write-Host "  - $f" -ForegroundColor Yellow
        }
    }
    if ($Fix) {
        Write-Host "Fixing format..." -ForegroundColor Cyan
        & go fmt ./...
        Write-Host "[PASS] Format fixed" -ForegroundColor Green
    }
} else {
    Write-Host "[PASS] All files properly formatted" -ForegroundColor Green
}

Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host " 3. Static Analysis (go vet)" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan

$vetOutput = & go vet ./... 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Host "[WARN] go vet found issues" -ForegroundColor Yellow
    $Script:HasErrors = $true
    if ($Verbose) { Write-Host $vetOutput }
} else {
    Write-Host "[PASS] Static analysis passed" -ForegroundColor Green
}

if (-not $SkipTests) {
    Write-Host ""
    Write-Host "========================================" -ForegroundColor Cyan
    Write-Host " 4. Test Check" -ForegroundColor Cyan
    Write-Host "========================================" -ForegroundColor Cyan

    $testFiles = Get-ChildItem -Path . -Recurse -Filter "*_test.go" -ErrorAction SilentlyContinue
    if ($testFiles) {
        Write-Host "Found $($testFiles.Count) test files"
        $testOutput = & go test -v ./... 2>&1
        if ($LASTEXITCODE -ne 0) {
            Write-Host "[WARN] Some tests failed" -ForegroundColor Yellow
            $Script:HasErrors = $true
            if ($Verbose) { Write-Host $testOutput }
        } else {
            Write-Host "[PASS] All tests passed" -ForegroundColor Green
        }
    } else {
        Write-Host "No test files found"
    }
}

Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host " 5. Module Dependency Check" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan

$graphOutput = & go mod graph 2>&1
if ($LASTEXITCODE -eq 0) {
    Write-Host "[PASS] Module dependency graph OK" -ForegroundColor Green
} else {
    Write-Host "[WARN] Module dependency check failed" -ForegroundColor Yellow
    $Script:HasErrors = $true
}

Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host " 6. Security Check" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan

$secretsPatterns = @(
    "password\s*=\s*[`"'][^`"']+[`"']",
    "api[_-]?key\s*=\s*[`"'][^`"']+[`"']",
    "secret\s*=\s*[`"'][^`"']+[`"']"
)

$suspiciousFiles = @()
$goFiles = Get-ChildItem -Path . -Recurse -Filter "*.go" -ErrorAction SilentlyContinue | Where-Object { $_.Name -notmatch '_test\.go$' }

foreach ($file in $goFiles) {
    $content = Get-Content $file.FullName -Raw -ErrorAction SilentlyContinue
    foreach ($pattern in $secretsPatterns) {
        if ($content -imatch $pattern) {
            $suspiciousFiles += $file.FullName
            break
        }
    }
}

if ($suspiciousFiles.Count -gt 0) {
    Write-Host "[WARN] Found $($suspiciousFiles.Count) files may contain secrets" -ForegroundColor Yellow
    $Script:HasErrors = $true
    if ($Verbose) {
        foreach ($f in $suspiciousFiles) {
            Write-Host "  - $f" -ForegroundColor Yellow
        }
    }
} else {
    Write-Host "[PASS] No secret leaks found" -ForegroundColor Green
}

Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host " 7. Panic Check" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan

$panicFiles = @()
foreach ($file in $goFiles) {
    $content = Get-Content $file.FullName -Raw
    if ($content -imatch '\bpanic\s*\(') {
        $panicFiles += $file.FullName
    }
}

if ($panicFiles.Count -gt 0) {
    Write-Host "[WARN] Found $($panicFiles.Count) files using panic" -ForegroundColor Yellow
    $Script:HasErrors = $true
    if ($Verbose) {
        foreach ($f in $panicFiles) {
            Write-Host "  - $f" -ForegroundColor Yellow
        }
    }
} else {
    Write-Host "[PASS] No panic usage found" -ForegroundColor Green
}

Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host " 8. Context Check" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan

$ctxIssueFiles = @()
foreach ($file in $goFiles) {
    $content = Get-Content $file.FullName -Raw
    $hasExecCommand = $content -imatch 'exec\.Command'
    $hasContext = $content -imatch 'Context'
    if ($hasExecCommand -and -not $hasContext) {
        $ctxIssueFiles += $file.FullName
    }
}

if ($ctxIssueFiles.Count -gt 0) {
    Write-Host "[WARN] Found $($ctxIssueFiles.Count) files may lack context" -ForegroundColor Yellow
    $Script:HasErrors = $true
    if ($Verbose) {
        foreach ($f in $ctxIssueFiles) {
            Write-Host "  - $f" -ForegroundColor Yellow
        }
    }
} else {
    Write-Host "[PASS] Context usage OK" -ForegroundColor Green
}

Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host " Summary" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

if ($Script:HasErrors) {
    Write-Host "========================================" -ForegroundColor Red
    Write-Host " Check Complete - Issues Found" -ForegroundColor Red
    Write-Host "========================================" -ForegroundColor Red
    Write-Host ""
    Write-Host "Tip: Use -Fix to fix formatting issues" -ForegroundColor Yellow
    Write-Host "     Use -Verbose for detailed output" -ForegroundColor Yellow
    Pop-Location
    exit 1
} else {
    Write-Host "========================================" -ForegroundColor Green
    Write-Host " Check Complete - All Passed!" -ForegroundColor Green
    Write-Host "========================================" -ForegroundColor Green
    Pop-Location
    exit 0
}
