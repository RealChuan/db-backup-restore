package fileutil

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
)

// isWindows 判断是否为 Windows 系统（内部使用）
func isWindows() bool {
	return runtime.GOOS == "windows"
}

// IsWindows 判断是否为 Windows 系统（公开接口）
func IsWindows() bool {
	return isWindows()
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

// IsAdmin 检查当前进程是否以管理员身份运行
func IsAdmin() bool {
	return checkAdmin()
}

// GetDirSize 计算目录大小
func GetDirSize(path string) int64 {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	if err != nil {
		slog.Warn(fmt.Sprintf("计算目录大小失败: %v", err))
	}
	return size
}

// CopyDir 递归复制目录
func CopyDir(srcDir, dstDir string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dstDir, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		dstFile, err := os.Create(dstPath)
		if err != nil {
			return err
		}
		defer dstFile.Close()

		_, err = io.Copy(dstFile, srcFile)
		if err != nil {
			return err
		}

		return os.Chmod(dstPath, info.Mode())
	})
}

// EnsureDir 确保目录存在，不存在则创建
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}
