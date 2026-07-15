package svcmgmt

import (
	"context"
	"runtime"
	"testing"
	"time"
)

func TestIsWindowsServiceStopped_NonExistentService(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	// 不存在的服务：sc query 会报错，不应视为已停止
	ctx := context.Background()
	stopped := IsWindowsServiceStopped(ctx, "NonExistentService99999")
	if stopped {
		t.Error("不存在的服务不应被视为已停止")
	}
}

func TestWaitForWindowsServiceStopped_WaitsForStoppedNotJustNotRunning(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	// 验证核心语义：WaitForWindowsServiceStopped 应等待 STOPPED 状态，
	// 而非仅仅等待"不是 RUNNING"状态。
	//
	// 模拟场景：服务从 RUNNING → STOP_PENDING → STOPPED
	// 若仅检查非 RUNNING，则在 STOP_PENDING 时就立即返回（Bug！）
	// 而 IsWindowsServiceStopped 在 STOP_PENDING 时返回 false，继续等待（正确）
	//
	// 此测试验证：当服务处于 STOP_PENDING 状态时，
	// WaitForWindowsServiceStopped 不应立即返回。
	// 我们用一个会在超时后被取消的 context 来验证等待行为。

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 使用一个不存在的服务名：sc query 会失败，既不是 RUNNING 也不是 STOPPED
	// 若用"非 RUNNING 即停止"的判断会立即返回 nil（Bug），
	// 用 IsWindowsServiceStopped 应该等待直到超时
	err := WaitForWindowsServiceStopped(ctx, "NonExistentService99999")

	// 不存在的服务不会被视为 STOPPED，应该超时
	if err == nil {
		t.Error("WaitForWindowsServiceStopped 对不存在的服务应返回超时错误，" +
			"不应在未确认 STOPPED 状态时立即返回 nil")
	}
}
