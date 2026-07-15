package svcmgmt

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const (
	scActionStart = "start"
	scActionStop  = "stop"
	windowsOS     = "windows"
)

// StartWindowsService 启动 Windows 服务
func StartWindowsService(ctx context.Context, serviceName string) error {
	commands := []struct {
		cmd  string
		args []string
	}{
		{"sc", []string{scActionStart, serviceName}},
		{"net", []string{scActionStart, serviceName}},
	}

	var lastErr error
	var lastOutput string
	for _, c := range commands {
		cmd := exec.CommandContext(ctx, c.cmd, c.args...)
		output, err := cmd.CombinedOutput()
		if err == nil {
			slog.Info("命令执行", "cmd", c.cmd, "args", strings.Join(c.args, " "))
			slog.Debug("命令输出", "output", string(output))
			slog.Info("Windows 服务已启动", "service", serviceName)
			return nil
		}
		lastErr = err
		lastOutput = string(output)
	}

	return fmt.Errorf("无法启动 Windows 服务 [%s]: %w, output: %s", serviceName, lastErr, lastOutput)
}

// StopWindowsService 停止 Windows 服务并发送停止信号后等待服务完全停止
func StopWindowsService(ctx context.Context, serviceName string) error {
	commands := []struct {
		cmd  string
		args []string
	}{
		{"sc", []string{scActionStop, serviceName}},
		{"net", []string{scActionStop, serviceName}},
	}

	var lastErr error
	var lastOutput string
	for _, c := range commands {
		cmd := exec.CommandContext(ctx, c.cmd, c.args...)
		output, err := cmd.CombinedOutput()
		if err == nil {
			slog.Info("命令执行", "cmd", c.cmd, "args", strings.Join(c.args, " "))
			slog.Debug("命令输出", "output", string(output))
			// sc stop 是异步的，需要等待服务完全停止
			if err := WaitForWindowsServiceStopped(ctx, serviceName); err != nil {
				slog.Warn("等待服务停止超时，继续执行", "service", serviceName, "error", err)
			}
			slog.Info("Windows 服务已停止", "service", serviceName)
			return nil
		}
		lastErr = err
		lastOutput = string(output)
	}

	return fmt.Errorf("无法停止 Windows 服务 [%s]: %w, output: %s", serviceName, lastErr, lastOutput)
}

// IsWindowsServiceStopped 检查 Windows 服务是否已完全停止
// 检查 STOPPED 状态而非简单取反 RUNNING 状态，避免 STOP_PENDING 状态下的假阳性：
// 服务从 RUNNING → STOP_PENDING → STOPPED 过渡期间，
// 若仅检查非 RUNNING，则在 STOP_PENDING 时就返回 true（错误），
// 而 IsWindowsServiceStopped 在 STOP_PENDING 时返回 false（正确）。
func IsWindowsServiceStopped(ctx context.Context, serviceName string) bool {
	cmd := exec.CommandContext(ctx, "sc", "query", serviceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	s := string(output)
	return !strings.Contains(s, "RUNNING") && strings.Contains(s, "STOPPED")
}

// WaitForWindowsServiceStopped 轮询等待 Windows 服务完全停止
// 超时时间为 30 秒，每 500 毫秒检查一次
// 使用 IsWindowsServiceStopped 检查 STOPPED 状态，确保服务真正停止后再返回
func WaitForWindowsServiceStopped(ctx context.Context, serviceName string) error {
	const (
		interval   = 500 * time.Millisecond
		maxWait    = 30 * time.Second
		maxRetries = int(maxWait / interval)
	)
	for i := 0; i < maxRetries; i++ {
		if IsWindowsServiceStopped(ctx, serviceName) {
			// 服务已报告 STOPPED 状态，额外等待一小段时间确保文件锁释放
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(1 * time.Second):
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
	return fmt.Errorf("等待服务 %s 停止超时（%v）", serviceName, maxWait)
}

// ServiceConfig 定义服务管理的配置，支持自定义命令和系统服务管理
type ServiceConfig struct {
	// ServiceName 服务名称，用于系统服务管理
	ServiceName string
	// StopCommand 停止服务的自定义命令，为空时使用系统服务管理
	StopCommand string
	// StopArgs 停止命令的参数
	StopArgs []string
	// StartCommand 启动服务的自定义命令，为空时使用系统服务管理
	StartCommand string
	// StartArgs 启动命令的参数
	StartArgs []string
	// Env 自定义命令的环境变量，格式为 "KEY=VALUE"
	Env []string
}

// IsWindows 判断当前操作系统是否为 Windows
func IsWindows() bool {
	return runtime.GOOS == windowsOS
}

// StopService 根据配置停止服务：优先使用自定义命令，否则使用系统服务管理
func StopService(ctx context.Context, cfg ServiceConfig) error {
	if cfg.StopCommand != "" {
		return stopWithCustomCommand(ctx, cfg.StopCommand, cfg.StopArgs, cfg.Env)
	}
	return stopWithSystemService(ctx, cfg.ServiceName)
}

// StartService 根据配置启动服务：优先使用自定义命令，否则使用系统服务管理
func StartService(ctx context.Context, cfg ServiceConfig) error {
	if cfg.StartCommand != "" {
		return startWithCustomCommand(ctx, cfg.StartCommand, cfg.StartArgs, cfg.Env)
	}
	return startWithSystemService(ctx, cfg.ServiceName)
}

// stopWithCustomCommand 使用自定义命令停止服务
func stopWithCustomCommand(ctx context.Context, command string, args []string, env []string) error {
	cmd := exec.CommandContext(ctx, command, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("自定义停止命令执行失败 [%s %s]: %w, output: %s",
			command, strings.Join(args, " "), err, string(output))
	}
	slog.Info("自定义停止命令执行成功", "cmd", command, "args", strings.Join(args, " "))
	slog.Debug("命令输出", "output", string(output))
	return nil
}

// startWithCustomCommand 使用自定义命令启动服务
func startWithCustomCommand(ctx context.Context, command string, args []string, env []string) error {
	cmd := exec.CommandContext(ctx, command, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("自定义启动命令执行失败 [%s %s]: %w, output: %s",
			command, strings.Join(args, " "), err, string(output))
	}
	slog.Info("自定义启动命令执行成功", "cmd", command, "args", strings.Join(args, " "))
	slog.Debug("命令输出", "output", string(output))
	return nil
}

// stopWithSystemService 使用系统服务管理停止服务
func stopWithSystemService(ctx context.Context, serviceName string) error {
	if IsWindows() {
		return StopWindowsService(ctx, serviceName)
	}

	// Linux: 优先尝试 systemctl，再尝试 service
	commands := []struct {
		cmd  string
		args []string
	}{
		{"systemctl", []string{scActionStop, serviceName}},
		{"service", []string{serviceName, scActionStop}},
	}

	var lastErr error
	var lastOutput string
	for _, c := range commands {
		cmd := exec.CommandContext(ctx, c.cmd, c.args...)
		output, err := cmd.CombinedOutput()
		if err == nil {
			slog.Info("系统服务停止成功", "cmd", c.cmd, "args", strings.Join(c.args, " "))
			slog.Debug("命令输出", "output", string(output))
			return nil
		}
		lastErr = err
		lastOutput = string(output)
	}

	return fmt.Errorf("无法通过系统服务管理停止服务 [%s]: %w, output: %s", serviceName, lastErr, lastOutput)
}

// startWithSystemService 使用系统服务管理启动服务
func startWithSystemService(ctx context.Context, serviceName string) error {
	if IsWindows() {
		return StartWindowsService(ctx, serviceName)
	}

	// Linux: 优先尝试 systemctl，再尝试 service
	commands := []struct {
		cmd  string
		args []string
	}{
		{"systemctl", []string{scActionStart, serviceName}},
		{"service", []string{serviceName, scActionStart}},
	}

	var lastErr error
	var lastOutput string
	for _, c := range commands {
		cmd := exec.CommandContext(ctx, c.cmd, c.args...)
		output, err := cmd.CombinedOutput()
		if err == nil {
			slog.Info("系统服务启动成功", "cmd", c.cmd, "args", strings.Join(c.args, " "))
			slog.Debug("命令输出", "output", string(output))
			return nil
		}
		lastErr = err
		lastOutput = string(output)
	}

	return fmt.Errorf("无法通过系统服务管理启动服务 [%s]: %w, output: %s", serviceName, lastErr, lastOutput)
}
