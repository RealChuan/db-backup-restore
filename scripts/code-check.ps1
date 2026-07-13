#Requires -Version 5.1

param(
    [switch]$Fix,
    [switch]$Verbose,
    [switch]$SkipTests,
    [double]$CoverageThreshold = 0
)

$ErrorActionPreference = "Continue"
$Script:HasErrors = $false
$Script:HasWarnings = $false
$Script:ProjectRoot = Split-Path $PSScriptRoot -Parent

Push-Location $Script:ProjectRoot

Write-Host "========================================" -ForegroundColor Cyan
Write-Host " Go Project Code Check (Enhanced)" -ForegroundColor Cyan
Write-Host " Project: $Script:ProjectRoot" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan

$goVersion = & go version
Write-Host "Go Version: $goVersion"
Write-Host ""

if (-not (Test-Path "go.mod")) {
    Write-Host "[FAIL] go.mod not found" -ForegroundColor Red
    $Script:HasErrors = $true
}

# ============================================================
# 1. Dependency Verify
# ============================================================
Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host " 1. Dependency Verify" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan

$modOutput = & go mod verify 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Host "[FAIL] Dependency verify failed" -ForegroundColor Red
    $Script:HasErrors = $true
    if ($Verbose) { Write-Host $modOutput }
}
else {
    Write-Host "[PASS] Dependency verify passed" -ForegroundColor Green
}

# ============================================================
# 2. go.mod Tidy Check
# ============================================================
Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host " 2. go.mod Tidy Check" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan

$goModBefore = Get-Content "go.mod" -Raw
$goSumBefore = if (Test-Path "go.sum") { Get-Content "go.sum" -Raw } else { "" }

& go mod tidy 2>&1 | Out-Null

$goModAfter = Get-Content "go.mod" -Raw
$goSumAfter = if (Test-Path "go.sum") { Get-Content "go.sum" -Raw } else { "" }

if ($goModBefore -ne $goModAfter -or $goSumBefore -ne $goSumAfter) {
    Write-Host "[FAIL] go.mod or go.sum is out of date (run 'go mod tidy')" -ForegroundColor Red
    $Script:HasErrors = $true
    if ($Fix) {
        Write-Host "  Fixed by go mod tidy" -ForegroundColor Cyan
    }
    else {
        # 恢复原始文件
        Set-Content "go.mod" -Value $goModBefore -NoNewline
        if ($goSumBefore -ne "") { Set-Content "go.sum" -Value $goSumBefore -NoNewline }
    }
}
else {
    Write-Host "[PASS] go.mod and go.sum are tidy" -ForegroundColor Green
}

# ============================================================
# 3. Format Check (gofumpt)
# ============================================================
Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host " 3. Format Check (gofumpt)" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan

$gofumptPath = Get-Command "gofumpt" -ErrorAction SilentlyContinue
if ($gofumptPath) {
    $fmtOutput = & gofumpt -l . 2>&1
    $files = @($fmtOutput) | Where-Object { $_ -match '\.go$' }

    if ($files.Count -gt 0) {
        Write-Host "[FAIL] Found $($files.Count) files with formatting issues (gofumpt)" -ForegroundColor Red
        $Script:HasErrors = $true
        foreach ($f in $files) {
            Write-Host "  - $f" -ForegroundColor Yellow
        }
        if ($Fix) {
            Write-Host "  Fixing format..." -ForegroundColor Cyan
            & gofumpt -w .
            Write-Host "  [PASS] Format fixed" -ForegroundColor Green
        }
    }
    else {
        Write-Host "[PASS] All files properly formatted (gofumpt)" -ForegroundColor Green
    }
}
else {
    Write-Host "[WARN] gofumpt not installed(install: go install mvdan.cc/gofumpt@latest), falling back to go fmt" -ForegroundColor Yellow
    $Script:HasWarnings = $true
    $fmtOutput = & go fmt ./... 2>&1
    $files = @($fmtOutput) | Where-Object { $_ -match '\S' }

    if ($files.Count -gt 0) {
        Write-Host "[FAIL] Found $($files.Count) files with formatting issues" -ForegroundColor Red
        $Script:HasErrors = $true
        foreach ($f in $files) {
            Write-Host "  - $f" -ForegroundColor Yellow
        }
        if ($Fix) {
            Write-Host "  Fixing format..." -ForegroundColor Cyan
            & go fmt ./...
            Write-Host "  [PASS] Format fixed" -ForegroundColor Green
        }
    }
    else {
        Write-Host "[PASS] All files properly formatted (go fmt)" -ForegroundColor Green
    }
}

# ============================================================
# 4. goimports Check
# ============================================================
Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host " 4. Import Grouping Check (goimports)" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan

$goimportsPath = Get-Command "goimports" -ErrorAction SilentlyContinue
if ($goimportsPath) {
    $goFiles = Get-ChildItem -Path . -Recurse -Filter "*.go" -ErrorAction SilentlyContinue |
    Where-Object { $_.FullName -notmatch '\\vendor\\' -and $_.FullName -notmatch '\\\.git\\' }

    $importIssues = @()
    foreach ($file in $goFiles) {
        $diffOutput = & goimports -l $file.FullName 2>&1
        if ($diffOutput) {
            $importIssues += $file.FullName
        }
    }

    if ($importIssues.Count -gt 0) {
        Write-Host "[FAIL] Found $($importIssues.Count) files with import grouping issues" -ForegroundColor Red
        $Script:HasErrors = $true
        foreach ($f in $importIssues) {
            $relPath = $f.Replace($Script:ProjectRoot, "").TrimStart("\")
            Write-Host "  - $relPath" -ForegroundColor Yellow
        }
        if ($Fix) {
            Write-Host "  Fixing imports..." -ForegroundColor Cyan
            & goimports -w .
            Write-Host "  [PASS] Imports fixed" -ForegroundColor Green
        }
    }
    else {
        Write-Host "[PASS] All imports properly grouped" -ForegroundColor Green
    }
}
else {
    Write-Host "[SKIP] goimports not installed (install: go install golang.org/x/tools/cmd/goimports@latest)" -ForegroundColor DarkGray
}

# ============================================================
# 5. Static Analysis (golangci-lint)
# ============================================================
Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host " 5. Static Analysis (golangci-lint)" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan

$lintPath = Get-Command "golangci-lint" -ErrorAction SilentlyContinue
if ($lintPath) {
    $lintArgs = @("run", "--timeout=5m")
    $lintOutput = & golangci-lint @lintArgs 2>&1
    if ($LASTEXITCODE -ne 0) {
        Write-Host "[FAIL] golangci-lint found issues" -ForegroundColor Red
        $Script:HasErrors = $true
        Write-Host $lintOutput
    }
    else {
        Write-Host "[PASS] golangci-lint passed" -ForegroundColor Green
    }
}
else {
    Write-Host "[WARN] golangci-lint not installed(install: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest), falling back to go vet" -ForegroundColor Yellow
    $Script:HasWarnings = $true
    $vetOutput = & go vet ./... 2>&1
    if ($LASTEXITCODE -ne 0) {
        Write-Host "[FAIL] go vet found issues" -ForegroundColor Red
        $Script:HasErrors = $true
        if ($Verbose) { Write-Host $vetOutput }
    }
    else {
        Write-Host "[PASS] go vet passed" -ForegroundColor Green
    }
}

# ============================================================
# 6. Vulnerability Check (govulncheck)
# ============================================================
Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host " 6. Vulnerability Check (govulncheck)" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan

$vulnPath = Get-Command "govulncheck" -ErrorAction SilentlyContinue
if ($vulnPath) {
    $vulnOutput = & govulncheck ./... 2>&1
    if ($LASTEXITCODE -ne 0) {
        Write-Host "[FAIL] govulncheck found vulnerabilities" -ForegroundColor Red
        $Script:HasErrors = $true
        Write-Host $vulnOutput
    }
    else {
        Write-Host "[PASS] No known vulnerabilities" -ForegroundColor Green
    }
}
else {
    Write-Host "[SKIP] govulncheck not installed (install: go install golang.org/x/vuln/cmd/govulncheck@latest)" -ForegroundColor DarkGray
}

# ============================================================
# 7. Test Check
# ============================================================
if (-not $SkipTests) {
    Write-Host ""
    Write-Host "========================================" -ForegroundColor Cyan
    Write-Host " 7. Test Check" -ForegroundColor Cyan
    Write-Host "========================================" -ForegroundColor Cyan

    $testFiles = Get-ChildItem -Path . -Recurse -Filter "*_test.go" -ErrorAction SilentlyContinue
    if ($testFiles) {
        Write-Host "Found $($testFiles.Count) test files"

        if ($CoverageThreshold -gt 0) {
            $testOutput = & go test -coverprofile=coverage.out ./... 2>&1
        }
        else {
            $testOutput = & go test ./... 2>&1
        }

        if ($LASTEXITCODE -ne 0) {
            Write-Host "[FAIL] Some tests failed" -ForegroundColor Red
            $Script:HasErrors = $true
            Write-Host $testOutput
        }
        else {
            Write-Host "[PASS] All tests passed" -ForegroundColor Green
        }

        # 覆盖率检查
        if ($CoverageThreshold -gt 0 -and (Test-Path "coverage.out")) {
            $coverFuncOutput = & go tool cover -func=coverage.out 2>&1
            $totalLine = $coverFuncOutput | Select-String "total:"
            if ($totalLine) {
                $coveragePercent = [double]($totalLine -replace '.*\s+(\d+\.\d+)%.*', '$1')
                Write-Host "  Total coverage: $coveragePercent%" -ForegroundColor Cyan

                if ($coveragePercent -lt $CoverageThreshold) {
                    Write-Host "[FAIL] Coverage $coveragePercent% is below threshold $CoverageThreshold%" -ForegroundColor Red
                    $Script:HasErrors = $true
                }
                else {
                    Write-Host "[PASS] Coverage meets threshold ($CoverageThreshold%)" -ForegroundColor Green
                }
            }

            # 清理
            Remove-Item "coverage.out" -ErrorAction SilentlyContinue
        }
    }
    else {
        Write-Host "[WARN] No test files found" -ForegroundColor Yellow
        $Script:HasWarnings = $true
    }
}

# ============================================================
# 8. Security Check (enhanced)
# ============================================================
Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host " 8. Security Check" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan

$goFiles = Get-ChildItem -Path . -Recurse -Filter "*.go" -ErrorAction SilentlyContinue |
Where-Object { $_.FullName -notmatch '\\vendor\\' -and $_.FullName -notmatch '\\\.git\\' }

# 检查硬编码密钥（排除结构体标签和注释）
$secretsPatterns = @(
    @{ Pattern = '(?<!//.*)password\s*[:=]\s*"[^"]+"'; Desc = "hardcoded password" },
    @{ Pattern = '(?<!//.*)api[_-]?key\s*[:=]\s*"[^"]+"'; Desc = "hardcoded API key" },
    @{ Pattern = '(?<!//.*)secret[_-]?key\s*[:=]\s*"[^"]+"'; Desc = "hardcoded secret key" }
)

$suspiciousFiles = @()
foreach ($file in $goFiles) {
    # 跳过测试文件和配置结构体定义
    if ($file.Name -match '_test\.go$') { continue }

    $content = Get-Content $file.FullName -Raw -ErrorAction SilentlyContinue
    if (-not $content) { continue }

    # 跳过仅包含结构体标签（json:"password"）的配置定义文件
    $lines = $content -split "`n"
    $hasRealSecret = $false
    foreach ($line in $lines) {
        $trimmed = $line.Trim()
        # 跳过注释行
        if ($trimmed -match '^\s*//') { continue }
        # 跳过结构体标签行（如 `json:"password"`）
        if ($trimmed -match '^\s*\w+\s+\w+.*`.*`') { continue }
        # 跳过仅声明字段名的行
        if ($trimmed -match '^\s*Password\s+string') { continue }

        foreach ($patternInfo in $secretsPatterns) {
            if ($line -imatch $patternInfo.Pattern) {
                $hasRealSecret = $true
                break
            }
        }
        if ($hasRealSecret) { break }
    }

    if ($hasRealSecret) {
        $relPath = $file.FullName.Replace($Script:ProjectRoot, "").TrimStart("\")
        $suspiciousFiles += $relPath
    }
}

if ($suspiciousFiles.Count -gt 0) {
    Write-Host "[WARN] Found $($suspiciousFiles.Count) files may contain hardcoded secrets" -ForegroundColor Yellow
    $Script:HasWarnings = $true
    foreach ($f in $suspiciousFiles) {
        Write-Host "  - $f" -ForegroundColor Yellow
    }
}
else {
    Write-Host "[PASS] No hardcoded secrets found" -ForegroundColor Green
}

# 检查 exec.CommandContext 使用（替代旧的 exec.Command 检查）
$execWithoutCtxFiles = @()
foreach ($file in $goFiles) {
    if ($file.Name -match '_test\.go$') { continue }
    $content = Get-Content $file.FullName -Raw -ErrorAction SilentlyContinue
    if (-not $content) { continue }

    # 检查是否有 exec.Command 调用（非 CommandContext）
    $execCommandMatches = [regex]::Matches($content, '\bexec\.Command\s*\(')
    $execCommandCtxMatches = [regex]::Matches($content, '\bexec\.CommandContext\s*\(')

    # 如果有 exec.Command 调用但没有任何 CommandContext，则报告
    if ($execCommandMatches.Count -gt 0 -and $execCommandCtxMatches.Count -eq 0) {
        $relPath = $file.FullName.Replace($Script:ProjectRoot, "").TrimStart("\")
        $execWithoutCtxFiles += $relPath
    }
}

if ($execWithoutCtxFiles.Count -gt 0) {
    Write-Host ""
    Write-Host "[WARN] Found $($execWithoutCtxFiles.Count) files using exec.Command without context (prefer exec.CommandContext)" -ForegroundColor Yellow
    $Script:HasWarnings = $true
    foreach ($f in $execWithoutCtxFiles) {
        Write-Host "  - $f" -ForegroundColor Yellow
    }
}
else {
    Write-Host "[PASS] exec.CommandContext usage OK" -ForegroundColor Green
}

# ============================================================
# Summary
# ============================================================
Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host " Summary" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

if ($Script:HasErrors) {
    Write-Host "========================================" -ForegroundColor Red
    Write-Host " Check Complete - ERRORS Found" -ForegroundColor Red
    Write-Host "========================================" -ForegroundColor Red
    Write-Host ""
    Write-Host "Tips:" -ForegroundColor Yellow
    Write-Host "  -Fix         : Auto-fix formatting and import issues" -ForegroundColor Yellow
    Write-Host "  -Verbose     : Show detailed output" -ForegroundColor Yellow
    Write-Host "  -SkipTests   : Skip test execution" -ForegroundColor Yellow
    Write-Host "  -CoverageThreshold 50 : Require 50% test coverage" -ForegroundColor Yellow
    Pop-Location
    exit 1
}
elseif ($Script:HasWarnings) {
    Write-Host "========================================" -ForegroundColor Yellow
    Write-Host " Check Complete - Warnings (non-blocking)" -ForegroundColor Yellow
    Write-Host "========================================" -ForegroundColor Yellow
    Pop-Location
    exit 0
}
else {
    Write-Host "========================================" -ForegroundColor Green
    Write-Host " Check Complete - All Passed!" -ForegroundColor Green
    Write-Host "========================================" -ForegroundColor Green
    Pop-Location
    exit 0
}
