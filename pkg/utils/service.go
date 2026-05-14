package utils

import (
	"context"
	"fmt"
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
			LogCommand(cmdStr, string(output), false)
			Infof("Windows 服务 [%s] 已启动", serviceName)
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
			LogCommand(cmdStr, string(output), false)
			Infof("Windows 服务 [%s] 已停止", serviceName)
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
