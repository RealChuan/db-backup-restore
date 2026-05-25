# Go 测试规范

## 测试结构
- 单元测试：`*_test.go` 与源码同包（白盒）或 `_test` 后缀包（黑盒）。
- 集成测试放 `test/integration/`，标记 `//go:build integration`。
- 基准测试：`BenchmarkXxx(b *testing.B)`，必须对比 `b.N` 与 `b.ReportAllocs()`。
- 模糊测试：关键解析器/序列化逻辑必须写 `FuzzXxx(f *testing.F)`。

## 测试命令
```bash
go test -race -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```
- CI 必须开启 `-race`，覆盖率门槛 ≥ 70%，核心包 ≥ 85%。

## 测试原则
1. **表驱动测试**：所有逻辑分支用 `[]struct{ name string ... }` 覆盖。
2. **Mock 边界**：外部依赖（HTTP/DB/消息队列）必须 mock，用 `gomock` 或手写 interface。
3. **并行测试**：无共享状态的测试加 `t.Parallel()`。
4. **测试隔离**：每个测试独立，禁止依赖执行顺序；使用 `t.TempDir()` 与 `httptest`。
5. **断言风格**：推荐 `testify/assert` + `require`，错误信息必须包含上下文（`"expected %v, got %v", want, got`）。

## 辅助工具
- `go.uber.org/goleak`：检测 goroutine 泄漏。
- `github.com/ory/dockertest/v3`：集成测试启动真实依赖容器。
- `github.com/sebdah/goldie`：快照测试用于复杂输出对比。

## 测试命名
- 函数：`TestService_CreateUser_Success`、`TestService_CreateUser_InvalidEmail`
- 子测试：`t.Run("invalid_email", func(t *testing.T){})`
