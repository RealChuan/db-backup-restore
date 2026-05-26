# Go 项目结构规范

> 参考：[golang-standards/project-layout](https://github.com/golang-standards/project-layout) | [Organizing a Go module](https://go.dev/doc/modules/layout)

## 技术栈

- Go 最新稳定版（当前 1.26+），使用 `toolchain` 指令锁定。
- Go Modules，多模块仓库用 `go.work`。

## 目录布局

```
.
├── cmd/            # 主应用入口。每子目录一个可执行文件（如 /cmd/myapp），
│                   # main.go 仅做依赖组装，不超 50 行。
├── internal/       # 私有应用和库代码。Go 编译器强制禁止外部导入。
│                   # 子包结构按需组织，不强制 DDD 分层。
│   └── app/       # （可选）实际应用代码
│   └── pkg/       # （可选）内部共享库
├── pkg/            # 可被外部项目导入的库代码。小型项目可省略此目录。
├── api/            # OpenAPI/Swagger specs、Protobuf 定义、JSON Schema 文件。（按需）
├── web/            # Web 应用特定组件：静态资源、服务端模板、SPA。（按需）
├── configs/        # 配置文件模板或默认配置（不含密钥）。
├── init/           # 系统 init（systemd、supervisord）配置。（按需）
├── scripts/        # 构建、安装、分析等辅助脚本。
├── build/          # 打包和 CI 配置。
│   └── ci/         # CI 配置文件和脚本。
├── deployments/    # IaaS/PaaS/容器编排部署配置（docker-compose、k8s/helm）。（按需）
├── test/           # 额外的外部测试应用和测试数据。
├── docs/           # 设计和用户文档。
├── tools/          # 此项目的支持工具，可导入 /pkg 和 /internal 的代码。
├── examples/       # 示例代码。（按需）
├── third_party/     # 第三方工具。（按需）
├── githooks/       # Git hooks。（按需）
├── website/        # 项目网站数据。（按需）
├── vendor/         # 应用依赖（通过 go mod vendor 生成）。（按需）
├── go.mod
├── Makefile
└── README.md
```

> **以上目录并非全部必需**——按项目规模选用需要的目录。PoC 或简单项目只需 `main.go` + `go.mod` 即可。

## 铁律

1. **业务代码放 `internal/`**，通用可复用库放 `pkg/`。
2. **`cmd/` 每子目录只输出一个可执行文件**，`main.go` 不超 50 行。
3. **禁止根目录放 `.go` 源码**（除 `tools.go` 外）。
4. **配置与代码分离**：通过环境变量或配置文件注入，禁止硬编码密钥。
5. **API 版本化**（如适用）：`api/v1/`、`api/v2/`，向后兼容至少保留一个主版本。

## internal 子包组织建议

| 项目规模 | 推荐结构 |
|---------|---------|
| 小型/PoC | 单层包，直接在 `internal/` 下组织 |
| 中型 | 按功能域分：`internal/backup/`、`internal/config/`、`internal/logging/` |
| 大型 | 可选 DDD 分层：`internal/domain/` + `internal/app/` + `internal/infra/` |
