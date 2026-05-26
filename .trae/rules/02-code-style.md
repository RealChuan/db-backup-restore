# Go 代码风格与质量红线

## 格式化

- 使用 `gofumpt`（比 `gofmt` 更严格）+ `goimports` 自动管理导入分组。
- 导入顺序：标准库 → 第三方 → 项目内部，每组空行分隔。
- 行宽限制 120 字符，函数长度不超过 60 行，圈复杂度不超过 15。

## 命名规范

- 包名：全小写、单数、无下划线（`userrepo` 而非 `user_repo`）。
- 接口名：方法集描述者用 `-er`（`Reader`、`Writer`），行为描述用动词（`Validate`）。
- 导出标识符必须有文档注释；未导出首字母小写。
- 常量用驼峰（`maxBufferSize`），枚举加类型前缀（`StatusPending`）。
- 避免 `util`、`common`、`helper` 等模糊包名。

## Lint 与静态分析

- 强制通过 `golangci-lint` v2.x，启用以下 linter：
  - `errcheck`、`gosimple`、`govet`、`ineffassign`、`staticcheck`、`unused`
  - `gofmt`、`goimports`、`revive`（替代 golint）
  - `gocritic`、`bodyclose`、`noctx`、`rowserrcheck`
  - `errorlint`（错误包装最佳实践）
  - `gosec`（安全扫描）
  - `funlen`（函数长度）、`gocyclo`（圈复杂度）
  - `goconst`（重复字符串常量检测）
- 零警告策略：CI 中 `golangci-lint run --timeout 5m` 必须全绿。
- 使用 `go vet ./...` 作为基础检查。

## 文档

- 每个导出类型/函数必须有 `// 名称 ...` 格式注释。
- `README.md` 包含：一句话描述、安装、快速开始、贡献指南。

## 性能红线

- 禁止在热路径使用 `reflect` 或 `fmt.Sprintf` 做序列化。
- 优先用 `strings.Builder` 做字符串拼接，切片预分配容量。
- 数据库查询必须带 `context.Context` 与超时控制。
- 正则表达式必须在包级别或结构体字段预编译（`regexp.MustCompile`），禁止在函数内反复编译。
