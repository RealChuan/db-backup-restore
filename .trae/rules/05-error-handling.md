# Go 错误处理、日志与可观测性

## 错误处理

- 使用 `fmt.Errorf("...: %w", err)` 包装错误，保留调用链。
- sentinel 错误：包级变量 `var ErrNotFound = errors.New("not found")`。
- 禁止吞错：每个返回 `error` 的调用必须处理或显式注释 `//nolint:errcheck`。
- 使用 `errors.Is()` 和 `errors.As()` 做错误判断，禁止字符串匹配。

## 结构化日志

- 使用标准库 `log/slog`（Go 1.21+），JSON 格式输出。
- **强制 key-value 格式**：`slog.InfoContext(ctx, "message", "key", value)`，禁止 `slog.Info(fmt.Sprintf(...))`。
- 自定义日志封装也应遵循 key-value 模式：`logging.InfoCtx(ctx, "message", "key", value)`。
- 关键路径埋点：Trace ID、请求耗时、错误码。

## Context 优先

- 所有 IO 操作、函数调用链必须传递 `context.Context` 作为第一个参数。
- 命名：`ctx context.Context`，禁止用 `context.TODO()` 除非顶层测试桩。
- 超时控制：HTTP handler 用 `http.TimeoutHandler`，数据库用 `ctx.WithTimeout`。
- 长时间运行的命令（如备份/还原）**不强制要求支持 ctx 取消**，避免中途终止导致数据损坏。

## 资源管理

- `defer` 释放资源：文件、数据库连接、HTTP body（`defer resp.Body.Close()`）。
- 数据库事务：显式 `tx.Rollback()` 在错误路径，`tx.Commit()` 在成功路径。
- HTTP Client 复用全局 `http.Client`，禁止每次请求 `&http.Client{}`。
- 切片/缓冲区：长时间运行的收集器必须设上限（环形缓冲区或定期清理），防止内存泄漏。

## 可观测性

- 指标暴露：Prometheus `/metrics`（如适用）。
- 健康检查：`/healthz` 与 `/readyz`（如适用）。
- OpenTelemetry 集成：在关键操作入口创建 Span，记录属性和耗时（如适用）。
