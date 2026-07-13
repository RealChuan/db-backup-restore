package fileutil

import (
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
)

// IsWindows 判断是否为 Windows 系统
func IsWindows() bool {
	return runtime.GOOS == "windows"
}

// AddExeExt 如果是 Windows 系统且无扩展名，添加 .exe 扩展名
func AddExeExt(path string) string {
	if !IsWindows() {
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
	err := filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			info, infoErr := d.Info()
			if infoErr != nil {
				return infoErr
			}
			size += info.Size()
		}
		return nil
	})
	if err != nil {
		slog.Warn("计算目录大小失败", "error", err)
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

		dstFile, err := os.Create(dstPath)
		if err != nil {
			closeErr := srcFile.Close()
			if closeErr != nil {
				slog.Warn("CopyDir 关闭源文件失败", "path", path, "error", closeErr)
			}
			return err
		}

		_, err = io.CopyBuffer(dstFile, srcFile, make([]byte, 32*1024))
		if err != nil {
			closeErr := srcFile.Close()
			if closeErr != nil {
				slog.Warn("CopyDir 关闭源文件失败", "path", path, "error", closeErr)
			}
			closeErr = dstFile.Close()
			if closeErr != nil {
				slog.Warn("CopyDir 关闭目标文件失败", "path", dstPath, "error", closeErr)
			}
			return err
		}

		if syncErr := dstFile.Sync(); syncErr != nil {
			closeErr := srcFile.Close()
			if closeErr != nil {
				slog.Warn("CopyDir 关闭源文件失败", "path", path, "error", closeErr)
			}
			closeErr = dstFile.Close()
			if closeErr != nil {
				slog.Warn("CopyDir 关闭目标文件失败", "path", dstPath, "error", closeErr)
			}
			return fmt.Errorf("sync 目标文件失败: %w", syncErr)
		}

		if closeErr := srcFile.Close(); closeErr != nil {
			slog.Warn("CopyDir 关闭源文件失败", "path", path, "error", closeErr)
		}
		if closeErr := dstFile.Close(); closeErr != nil {
			slog.Warn("CopyDir 关闭目标文件失败", "path", dstPath, "error", closeErr)
		}

		return os.Chmod(dstPath, info.Mode())
	})
}

// CopyFile 复制单个文件从 src 到 dst
func CopyFile(src, dst string) (err error) {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		closeErr := dstFile.Close()
		if err == nil {
			err = closeErr
		}
	}()

	if _, err = io.Copy(dstFile, sourceFile); err != nil {
		return err
	}

	if err = dstFile.Sync(); err != nil {
		return err
	}

	sourceInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	return os.Chmod(dst, sourceInfo.Mode())
}

// EnsureDir 确保目录存在，不存在则创建
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}
