//go:build linux || darwin

package utils

import "os"

// checkAdmin 检查当前进程是否以管理员身份运行（Unix-like 系统）
func checkAdmin() bool {
	return os.Getuid() == 0
}
