package svcmgmt

import (
	"context"
	"testing"
	"time"
)

func TestIsWindowsServiceStopped_NonExistentService(t *testing.T) {
	// 不存在的服务：sc query 会报错，不应视为已停止
	ctx := context.Background()
	stopped := IsWindowsServiceStopped(ctx, "NonExistentService99999")
	if stopped {
		t.Error("不存在的服务不应被视为已停止")
	}
}

func TestWaitForWindowsServiceStopped_WaitsForStoppedNotJustNotRunning(t *testing.T) {
	// 验证核心语义：WaitForWindowsServiceStopped 应等待 STOPPED 状态，
	// 而非仅仅等待"不是 RUNNING"状态。
	//
	// 模拟场景：服务从 RUNNING → STOP_PENDING → STOPPED
	// 旧逻辑：STOP_PENDING 时 !IsWindowsServiceRunning=true，立即返回（Bug！）
	// 新逻辑：STOP_PENDING 时 IsWindowsServiceStopped=false，继续等待（正确）
	//
	// 此测试验证：当服务处于 STOP_PENDING 状态时，
	// WaitForWindowsServiceStopped 不应立即返回。
	// 我们用一个会在超时后被取消的 context 来验证等待行为。

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 使用一个不存在的服务名：sc query 会失败，既不是 RUNNING 也不是 STOPPED
	// 旧逻辑（!IsWindowsServiceRunning）会立即返回 nil（因为不是 RUNNING）
	// 新逻辑（IsWindowsServiceStopped）应该等待直到超时
	err := WaitForWindowsServiceStopped(ctx, "NonExistentService99999")

	// 新逻辑下，不存在的服务不会被视为 STOPPED，应该超时
	if err == nil {
		t.Error("WaitForWindowsServiceStopped 对不存在的服务应返回超时错误，" +
			"因为旧逻辑用 !IsWindowsServiceRunning 会立即返回 nil（Bug），" +
			"新逻辑用 IsWindowsServiceStopped 应正确等待")
	}
}

func TestIsWindowsServiceRunning_NonExistentService(t *testing.T) {
	ctx := context.Background()
	running := IsWindowsServiceRunning(ctx, "NonExistentService99999")
	if running {
		t.Error("不存在的服务不应被视为正在运行")
	}
}
