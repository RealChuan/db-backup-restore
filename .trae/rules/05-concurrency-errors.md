# Go 并发、错误与上下文规范

## Context 优先
- 所有 IO 操作、函数调用链必须传递 `context.Context` 作为第一个参数。
- 命名：`ctx context.Context`，禁止用 `context.TODO()` 除非顶层测试桩。
- 超时控制：HTTP handler 用 `http.TimeoutHandler`，数据库用 `ctx.WithTimeout`。

## 错误处理
- 使用 `fmt.Errorf("...: %w", err)` 包装错误，保留调用链。
-  sentinel 错误：包级变量 `var ErrNotFound = errors.New("not found")`。
-  禁止吞错：每个返回 `error` 的调用必须处理或显式注释 `// nolint:errcheck`。
-  日志记录错误用结构化日志（`log/slog`），禁止 `fmt.Println` 或 `log.Printf`。

## 并发安全
- 共享状态用 `sync.RWMutex` 或 `sync.Map`（仅高并发读多写少场景）。
- 优先用 channel 传递数据，而非共享内存。
- Goroutine 必须能优雅退出：监听 `ctx.Done()` 或 `stopCh`。
- 禁止在循环中直接 `go func() { use(i) }()`，必须传参避免闭包陷阱。

## 资源管理
- `defer` 释放资源：文件、数据库连接、HTTP body（`defer resp.Body.Close()`）。
- 数据库事务：显式 `tx.Rollback()` 在错误路径，`tx.Commit()` 在成功路径。
- HTTP Client 复用全局 `http.Client`，禁止每次请求 `&http.Client{}`。

## 日志与可观测
- 使用标准库 `log/slog`（Go 1.21+）或 `zap`，JSON 格式输出。
- 关键路径埋点：Trace ID、请求耗时、错误码。
- 指标暴露：Prometheus `/metrics`，健康检查 `/healthz` 与 `/readyz`。
