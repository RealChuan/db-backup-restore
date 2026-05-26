# Go 并发安全

## 基本原则

- 共享状态用 `sync.RWMutex` 或 `sync.Map`（仅高并发读多写少场景）。
- 优先用 channel 传递数据，而非共享内存。
- Goroutine 必须能优雅退出：监听 `ctx.Done()` 或 `stopCh`。
- 禁止在循环中直接 `go func() { use(i) }()`，必须传参避免闭包陷阱。

## 常见模式

- Worker Pool：用带缓冲 channel 控制并发数。
- Fan-out/Fan-in：用 `sync.WaitGroup` + channel 聚合结果。
- Pipeline：用 channel 串联处理阶段。
- Context 取消：通过 `ctx.Done()` 广播退出信号。

## 注意事项

- 避免在持有锁时做 IO 操作或调用可能阻塞的函数。
- `sync.Once` 用于延迟初始化，不要用于条件执行。
- 禁止 `copy` `sync.Mutex`/`sync.WaitGroup` 等同步原语——它们必须通过指针传递。
