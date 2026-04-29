package utils

import (
	"fmt"
	"path/filepath"
	"runtime"
)

// isWindows 判断是否为 Windows 系统
func isWindows() bool {
	return runtime.GOOS == "windows"
}

// AddExeExt 如果是 Windows 系统且无扩展名，添加 .exe 扩展名
func AddExeExt(path string) string {
	if !isWindows() {
		return path
	}
	if filepath.Ext(path) == "" {
		return path + ".exe"
	}
	return path
}

// Format 格式化字符串（与 fmt.Sprintf 功能相同）
func Format(format string, args ...interface{}) string {
	return fmt.Sprintf(format, args...)
}

// FormatFileSize 将字节大小转换为更可读的格式
func FormatFileSize(size int64) string {
	const (
		unit      = 1024
		precision = 2
	)
	units := []string{"bytes", "KB", "MB", "GB", "TB", "PB", "EB", "ZB", "YB"}

	index := 0
	currentSize := float64(size)

	for currentSize >= unit && index < len(units)-1 {
		currentSize /= unit
		index++
	}

	if index == 0 {
		return fmt.Sprintf("%d %s", size, units[index])
	}

	return fmt.Sprintf("%.2f %s", currentSize, units[index])
}
