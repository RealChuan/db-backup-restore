package utils

import (
	"fmt"
)

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
