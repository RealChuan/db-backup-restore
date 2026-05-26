package svcmgmt

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
)

// StartWindowsService 启动 Windows 服务
func StartWindowsService(ctx context.Context, serviceName string) error {
	commands := []string{
		fmt.Sprintf("sc start %s", serviceName),
		fmt.Sprintf("net start %s", serviceName),
	}

	for _, cmdStr := range commands {
		cmd := exec.CommandContext(ctx, "cmd", "/c", cmdStr)
		output, err := cmd.CombinedOutput()
		if err == nil {
			slog.Info("命令执行", "cmd", cmdStr)
			slog.Debug("命令输出", "output", string(output))
			slog.Info("Windows 服务已启动", "service", serviceName)
			return nil
		}
	}

	return fmt.Errorf("无法启动 Windows 服务 [%s]", serviceName)
}

// StopWindowsService 停止 Windows 服务
func StopWindowsService(ctx context.Context, serviceName string) error {
	commands := []string{
		fmt.Sprintf("sc stop %s", serviceName),
		fmt.Sprintf("net stop %s", serviceName),
	}

	for _, cmdStr := range commands {
		cmd := exec.CommandContext(ctx, "cmd", "/c", cmdStr)
		output, err := cmd.CombinedOutput()
		if err == nil {
			slog.Info("命令执行", "cmd", cmdStr)
			slog.Debug("命令输出", "output", string(output))
			slog.Info("Windows 服务已停止", "service", serviceName)
			return nil
		}
	}

	return fmt.Errorf("无法停止 Windows 服务 [%s]", serviceName)
}

// IsWindowsServiceRunning 检查 Windows 服务是否正在运行
func IsWindowsServiceRunning(ctx context.Context, serviceName string) bool {
	cmd := exec.CommandContext(ctx, "cmd", "/c", fmt.Sprintf("sc query %s | findstr RUNNING", serviceName))
	output, err := cmd.CombinedOutput()
	if err == nil && len(output) > 0 {
		return true
	}
	return false
}
