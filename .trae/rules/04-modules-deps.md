# Go 模块与依赖管理

## go.mod 规范

- 模块路径：与仓库地址一致（如 `github.com/org/project`）。
- `go` 指令设为当前使用的最低版本，`toolchain` 指令锁定具体版本。
- 最小依赖原则：只引入必要的直接依赖，间接依赖由 `go mod tidy` 管理。
- 禁止提交 vendor/（除非离线构建需求，需在 `.gitignore` 中排除）。

## 工具依赖管理（Go 1.24+ tool 指令）

使用 `go.mod` 的 `tool` 指令管理工具依赖：

```
tool github.com/golangci/golangci-lint/cmd/golangci-lint
tool github.com/securego/gosec/v2/cmd/gosec
tool golang.org/x/vuln/cmd/govulncheck
```

或使用 `tools/tools.go` 传统方式：

```go
//go:build tools
package tools

import (
    _ "github.com/golangci/golangci-lint/cmd/golangci-lint"
    _ "github.com/securego/gosec/v2/cmd/gosec"
)
```

## 依赖更新策略

- 定期执行 `go get -u ./... && go mod tidy`。
- 安全漏洞扫描：`govulncheck ./...`（Go 官方漏洞检测工具）。

## 多模块仓库

- 根目录放 `go.work`，子模块各自 `go.mod`。
- 共享代码放 `pkg/`，作为独立子模块或根模块的一部分。
