package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/RealChuan/db-backup-restore/internal/backup"
)

// OperationResult 操作结果（用于结构化输出）
type OperationResult struct {
	Success   bool                   `json:"success"`
	Operation string                 `json:"operation"`
	DBType    string                 `json:"db_type,omitempty"`
	Message   string                 `json:"message,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Error     string                 `json:"error,omitempty"`
}

// OutputWriter 输出写入器
type OutputWriter struct {
	format backup.OutputFormat
	writer io.Writer
}

// NewOutputWriter 创建输出写入器
func NewOutputWriter(format backup.OutputFormat) *OutputWriter {
	return &OutputWriter{
		format: format,
		writer: os.Stdout,
	}
}

// Write 写入操作结果
func (w *OutputWriter) Write(result *OperationResult) error {
	if w.format == backup.OutputFormatJSON {
		encoder := json.NewEncoder(w.writer)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	}
	// text 格式
	if result.Success {
		fmt.Fprintf(w.writer, "%s: %s\n", result.Operation, result.Message)
	} else {
		fmt.Fprintf(w.writer, "%s 失败: %s\n", result.Operation, result.Error)
	}
	return nil
}
