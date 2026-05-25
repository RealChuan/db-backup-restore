# Go 模块与依赖管理

## go.mod 规范
- 模块路径：`github.com/org/project`（与仓库一致）。
- `go 1.24.0`，`toolchain go1.24.0`。
- 最小依赖原则：只引入必要的直接依赖，间接依赖由 `go mod tidy` 管理。
- 禁止提交 vendor/（除非离线构建需求，需在 `.gitignore` 中排除）。

## 工具依赖管理（2026 标准）
使用 `tools/tools.go` + `go.mod` 的 `tool` 指令（Go 1.24+）：
```go
//go:build tools
package tools

import (
    _ "github.com/golangci/golangci-lint/cmd/golangci-lint"
    _ "github.com/securego/gosec/v2/cmd/gosec"
)
```
或直接在 `go.mod` 中声明：
```
tool github.com/golangci/golangci-lint/cmd/golangci-lint
tool github.com/securego/gosec/v2/cmd/gosec
```

## 依赖更新策略
- 定期执行 `go get -u ./... && go mod tidy`。
- 关键安全漏洞用 `govulncheck ./...` 扫描。
- 版本锁定：生产依赖使用 `>=v1.2.3,<v2.0.0`，避免自动升级破坏。

## 私有模块
- 配置 `GOPRIVATE=*.corp.example.com`。
- 使用 `.netrc` 或 `git config url."ssh://git@github.com/".insteadOf` 鉴权。

## 多模块仓库
- 根目录放 `go.work`，子模块各自 `go.mod`。
- 共享代码放 `pkg/`，作为独立子模块或根模块的一部分。
