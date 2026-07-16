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
	stopped := isWindowsServiceStopped(ctx, "NonExistentService99999")
	if stopped {
		t.Error("不存在的服务不应被视为已停止")
	}
}

func TestWaitForWindowsServiceStopped_WaitsForStoppedNotJustNotRunning(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	// 验证核心语义：waitForWindowsServiceStopped 应等待 STOPPED 状态，
	// 而非仅仅等待"不是 RUNNING"状态。
	//
	// 模拟场景：服务从 RUNNING → STOP_PENDING → STOPPED
	// 若仅检查非 RUNNING，则在 STOP_PENDING 时就立即返回（Bug！）
	// 而 isWindowsServiceStopped 在 STOP_PENDING 时返回 false，继续等待（正确）
	//
	// 此测试验证：当服务处于 STOP_PENDING 状态时，
	// waitForWindowsServiceStopped 不应立即返回。
	// 我们用一个会在超时后被取消的 context 来验证等待行为。

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 使用一个不存在的服务名：sc query 会失败，既不是 RUNNING 也不是 STOPPED
	// 若用"非 RUNNING 即停止"的判断会立即返回 nil（Bug），
	// 用 isWindowsServiceStopped 应该等待直到超时
	err := waitForWindowsServiceStopped(ctx, "NonExistentService99999")

	// 不存在的服务不会被视为 STOPPED，应该超时
	if err == nil {
		t.Error("waitForWindowsServiceStopped 对不存在的服务应返回超时错误，" +
			"不应在未确认 STOPPED 状态时立即返回 nil")
	}
}

// TestStopService_CustomCommand_Success 测试自定义停止命令成功执行（跨平台）
func TestStopService_CustomCommand_Success(t *testing.T) {
	t.Parallel()

	cmd, args := successCommand()
	cfg := ServiceConfig{
		StopCommand: cmd,
		StopArgs:    args,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := StopService(ctx, cfg); err != nil {
		t.Errorf("StopService 自定义命令成功路径不应报错: %v", err)
	}
}

// TestStartService_CustomCommand_Success 测试自定义启动命令成功执行（跨平台）
func TestStartService_CustomCommand_Success(t *testing.T) {
	t.Parallel()

	cmd, args := successCommand()
	cfg := ServiceConfig{
		StartCommand: cmd,
		StartArgs:    args,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := StartService(ctx, cfg); err != nil {
		t.Errorf("StartService 自定义命令成功路径不应报错: %v", err)
	}
}

// TestStopService_CustomCommand_Failure 测试自定义停止命令失败时返回错误（跨平台）
func TestStopService_CustomCommand_Failure(t *testing.T) {
	t.Parallel()

	cfg := ServiceConfig{
		StopCommand: "nonexistent-command-12345",
		StopArgs:    []string{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := StopService(ctx, cfg)
	if err == nil {
		t.Fatal("不存在的命令应返回错误")
	}
}

// TestStartService_CustomCommand_Failure 测试自定义启动命令失败时返回错误（跨平台）
func TestStartService_CustomCommand_Failure(t *testing.T) {
	t.Parallel()

	cfg := ServiceConfig{
		StartCommand: "nonexistent-command-12345",
		StartArgs:    []string{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := StartService(ctx, cfg)
	if err == nil {
		t.Fatal("不存在的命令应返回错误")
	}
}

// TestStopService_CustomCommand_WithEnv 测试自定义命令带环境变量执行（跨平台）
func TestStopService_CustomCommand_WithEnv(t *testing.T) {
	t.Parallel()

	cmd, args := successCommand()
	cfg := ServiceConfig{
		StopCommand: cmd,
		StopArgs:    args,
		Env:         []string{"TEST_ENV_VAR=stop_value"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := StopService(ctx, cfg); err != nil {
		t.Errorf("StopService 带环境变量不应报错: %v", err)
	}
}

// TestIsWindows 测试 IsWindows 函数返回值与 runtime.GOOS 一致
func TestIsWindows(t *testing.T) {
	t.Parallel()

	expected := runtime.GOOS == "windows"
	if got := IsWindows(); got != expected {
		t.Errorf("IsWindows() = %v, 期望 %v", got, expected)
	}
}

// successCommand 返回一个在当前平台上必定成功的命令及其参数
func successCommand() (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/c", "exit", "0"}
	}
	return "true", nil
}
