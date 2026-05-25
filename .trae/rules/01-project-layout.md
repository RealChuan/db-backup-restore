# Go 项目结构规范

## 技术栈
- Go 1.24+（`toolchain go1.24.0`）
- Go Modules，多模块用 `go.work`

## 目录布局（project-layout 标准）

```
.
├── api/            # API 定义（Protobuf/OpenAPI）
├── assets/         # 静态资源
├── build/ci/       # CI 脚本
├── cmd/            # 主应用入口，每子目录一个可执行文件
│   └── myapp/
│       └── main.go
├── configs/        # 配置模板（不存密钥）
├── deployments/    # Docker/K8s/Helm
├── docs/           # 设计文档、ADR
├── examples/       # 示例代码
├── internal/       # 私有业务逻辑，禁止外部导入
│   ├── app/        # 用例、服务编排
│   ├── domain/     # 实体、值对象
│   ├── infra/      # 存储、缓存、队列
│   └── ports/      # 接口定义
├── pkg/            # 可复用公共库
├── scripts/        # 构建、分析脚本
├── test/           # 集成测试辅助
├── tools/          # 工具依赖
├── web/            # Web 静态文件
├── go.mod
├── Makefile
└── README.md
```

## 铁律
1. **业务代码放 `internal/`**，通用工具放 `pkg/`。
2. **`cmd/` 每子目录只输出一个可执行文件**，`main.go` 不超 50 行，仅做依赖组装。
3. **禁止根目录放 `.go` 源码**（除 `tools.go` 外）。
4. **配置与代码分离**：环境配置通过环境变量或配置文件注入，禁止硬编码。
5. **API 版本化**：`api/v1/`、`api/v2/`，向后兼容至少保留一个主版本。
